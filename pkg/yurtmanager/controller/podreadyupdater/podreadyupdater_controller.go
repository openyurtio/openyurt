/*
Copyright 2023 The OpenYurt Authors.

Licensed under the Apache License, Version 2.0 (the License);
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an AS IS BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package podreadyupdater

import (
	"context"
	"flag"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	appconfig "github.com/openyurtio/openyurt/cmd/yurt-manager/app/config"
	"github.com/openyurtio/openyurt/pkg/apis/apps"
	appsv1beta1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
	nodeutil "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/node"
	podutil "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/pod"
)

func init() {
	flag.IntVar(&concurrentReconciles, "podreadyupdater-workers", concurrentReconciles, "Max concurrent workers for PodReadyUpdater controller.")
}

var (
	concurrentReconciles = 3
	controllerKind       = corev1.SchemeGroupVersion.WithKind("Pod")
)

const (
	ControllerName = "podReadyUpdater"
)

func Format(format string, args ...interface{}) string {
	s := fmt.Sprintf(format, args...)
	return fmt.Sprintf("%s: %s", ControllerName, s)
}

// Add creates a new PodReadyUpdater Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(c *appconfig.CompletedConfig, mgr manager.Manager) error {
	klog.Infof("podreadyupdater-controller add controller %s", controllerKind.String())
	return add(mgr, newReconciler(c, mgr))
}

var _ reconcile.Reconciler = &ReconcilePodReadyUpdater{}

// ReconcilePodReadyUpdater reconciles a PodReadyUpdater object
type ReconcilePodReadyUpdater struct {
	client.Client
	scheme *runtime.Scheme
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(c *appconfig.CompletedConfig, mgr manager.Manager) reconcile.Reconciler {
	return &ReconcilePodReadyUpdater{
		Client: mgr.GetClient(),
		scheme: mgr.GetScheme(),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(ControllerName, mgr, controller.Options{
		Reconciler: r, MaxConcurrentReconciles: concurrentReconciles,
	})
	if err != nil {
		return err
	}

	// 1. Watch for changes to Pod
	podReadyPredicate := predicate.Funcs{
		CreateFunc: func(evt event.CreateEvent) bool {
			return false
		},
		DeleteFunc: func(evt event.DeleteEvent) bool {
			return false
		},
		UpdateFunc: func(evt event.UpdateEvent) bool {
			return podUpdate(evt)
		},
		GenericFunc: func(evt event.GenericEvent) bool {
			return false
		},
	}

	if err := c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForObject{}, podReadyPredicate); err != nil {
		return err
	}

	// 2. Watch for changes to NodePool
	// only watch pods changes may leave out some pods, because nodepool status may not update as quick as pod
	nodePoolPredicate := predicate.Funcs{
		CreateFunc: func(evt event.CreateEvent) bool {
			return nodePoolCreate(evt)
		},
		DeleteFunc: func(evt event.DeleteEvent) bool {
			return false
		},
		UpdateFunc: func(evt event.UpdateEvent) bool {
			return nodePoolUpdate(evt)
		},
		GenericFunc: func(evt event.GenericEvent) bool {
			return false
		},
	}

	reconcileNotReadyPod := func(obj client.Object, c client.Client) []reconcile.Request {
		var requests []reconcile.Request
		nodePool, ok := obj.(*appsv1beta1.NodePool)
		if !ok {
			return requests
		}

		// list all pods need to update
		for _, nodeName := range nodePool.Status.Nodes {
			podList := &corev1.PodList{}
			selector := fields.OneTermEqualSelector("spec.nodeName", nodeName)
			err := c.List(context.TODO(), podList, &client.ListOptions{FieldSelector: selector})
			if err != nil {
				continue
			}
			for i := range podList.Items {
				// if pod Ready Condition is already True, skip.
				if podutil.IsPodReadyConditionTrue(podList.Items[i].Status) {
					continue
				}
				requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{
					Namespace: podList.Items[i].Namespace,
					Name:      podList.Items[i].Name,
				}})
			}
		}

		klog.V(2).Infof(Format("syncNodePoolHandler need to update ready status of pod, number: %+v", len(requests)))
		return requests
	}

	if err := c.Watch(
		&source.Kind{Type: &appsv1beta1.NodePool{}},
		handler.EnqueueRequestsFromMapFunc(func(obj client.Object) []reconcile.Request {
			return reconcileNotReadyPod(obj, mgr.GetClient())
		}),
		nodePoolPredicate,
	); err != nil {
		return err
	}

	return nil
}

// podUpdate filter events: pod update with Ready Condition from True to False
func podUpdate(evt event.UpdateEvent) bool {
	oldPod, ok := evt.ObjectOld.(*corev1.Pod)
	if !ok {
		return false
	}
	newPod, ok := evt.ObjectNew.(*corev1.Pod)
	if !ok {
		return false
	}

	// pod Ready Condition from True to False
	// if pod have ready condition and condition is False
	oldCondition := podutil.GetPodReadyCondition(oldPod.Status)
	newCondition := podutil.GetPodReadyCondition(newPod.Status)
	if oldCondition == nil || newCondition == nil {
		return false
	}
	if oldCondition.Status == corev1.ConditionTrue && newCondition.Status == corev1.ConditionFalse {
		klog.V(2).Infof(Format("podReadyUpdaterController enqueue pod update event for %v", newPod.Name))
		return true
	}
	return false
}

// nodePoolCreate filter events: nodePool disconnect with cloud
func nodePoolCreate(evt event.CreateEvent) bool {
	nodePool, ok := evt.Object.(*appsv1beta1.NodePool)
	if !ok {
		return false
	}

	if nodePool.Status.UnreadyNodeNum == int32(len(nodePool.Status.Nodes)) {
		klog.V(2).Infof(Format("podReadyUpdaterController enqueue nodepool create event for %v", nodePool.Name))
		return true
	}
	return false
}

// nodePoolUpdate filter events: nodePool disconnect with cloud
func nodePoolUpdate(evt event.UpdateEvent) bool {
	oldNodePool, ok := evt.ObjectOld.(*appsv1beta1.NodePool)
	if !ok {
		return false
	}
	newNodePool, ok := evt.ObjectNew.(*appsv1beta1.NodePool)
	if !ok {
		return false
	}

	if oldNodePool.Status.UnreadyNodeNum == newNodePool.Status.UnreadyNodeNum {
		return false
	}
	if newNodePool.Status.UnreadyNodeNum == int32(len(newNodePool.Status.Nodes)) {
		klog.V(2).Infof(Format("podReadyUpdaterController enqueue nodepool update event for %v", newNodePool.Name))
		return true
	}
	return false
}

// +kubebuilder:rbac:groups=apps.openyurt.io,resources=nodepools,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods/status,verbs=update
// +kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch;update;patch

// Reconcile reads that state of the cluster for a Pod object and makes changes based on the state read
// and what is in the Pod.Spec
func (r *ReconcilePodReadyUpdater) Reconcile(_ context.Context, request reconcile.Request) (reconcile.Result, error) {

	// Note !!!!!!!!!!
	// We strongly recommend use Format() to  encapsulation because Format() can print logs by module
	// @kadisi
	klog.Infof(Format("Reconcile PodReadyUpdater %s/%s", request.Namespace, request.Name))

	// Fetch the PodReadyUpdater instance
	instance := &corev1.Pod{}
	err := r.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		klog.Errorf(Format("Fail to get Pod %v, %v", request.NamespacedName, err))
		return reconcile.Result{}, err
	}

	if instance.DeletionTimestamp != nil {
		return reconcile.Result{}, nil
	}

	if len(instance.Spec.NodeName) == 0 {
		return reconcile.Result{}, nil
	}

	node := &corev1.Node{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: instance.Spec.NodeName}, node)
	if err != nil {
		return reconcile.Result{}, err
	}

	// if nodepool is disconnect with cloud, we need to keep pod Ready Condition for True
	nodePoolName := node.Labels[apps.NodePoolLabel]
	if len(nodePoolName) == 0 {
		return reconcile.Result{}, nil
	}

	nodePool := &appsv1beta1.NodePool{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: nodePoolName}, nodePool)
	if err != nil {
		return reconcile.Result{}, err
	}
	if nodePool.Status.UnreadyNodeNum == int32(len(nodePool.Status.Nodes)) {
		return MarkPodReady(r.Client, instance)
	}

	return reconcile.Result{}, nil
}

// MarkPodReady updates ready status of given pod return true if success
func MarkPodReady(kubeClient client.Client, pod *corev1.Pod) (reconcile.Result, error) {
	// Pod will be modified, so making copy is requiered.
	newPod := pod.DeepCopy()
	for _, cond := range newPod.Status.Conditions {
		if cond.Type == corev1.PodReady && cond.Status != corev1.ConditionTrue {
			cond.Status = corev1.ConditionTrue
			if !nodeutil.UpdatePodCondition(&newPod.Status, &cond) {
				break
			}
			klog.V(2).Infof(Format("Updating ready status of pod %v to true", newPod.Name))
			err := kubeClient.Status().Update(context.TODO(), newPod)
			if err != nil {
				if apierrors.IsNotFound(err) {
					// NotFound error means that pod was already deleted.
					// There is nothing left to do with this pod.
					break
				}
				klog.Warningf(Format("Failed to update status for pod %s: %v", newPod.Name, err))
				return reconcile.Result{Requeue: true}, err
			}
			break
		}
	}
	return reconcile.Result{}, nil
}
