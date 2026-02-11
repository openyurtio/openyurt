/*
Copyright 2023 The OpenYurt Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package podbinding

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	yurtClient "github.com/openyurtio/openyurt/cmd/yurt-manager/app/client"
	appconfig "github.com/openyurtio/openyurt/cmd/yurt-manager/app/config"
	"github.com/openyurtio/openyurt/cmd/yurt-manager/names"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
	nodeutil "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/node"
)

const (
	originalNotReadyTolerationDurationAnnotation    = "apps.openyurt.io/original-not-ready-toleration-duration"
	originalUnreachableTolerationDurationAnnotation = "apps.openyurt.io/original-unreachable-toleration-duration"

	// infiniteTolerationValue is the annotation value used to indicate that the original
	// TolerationSeconds was nil (meaning infinite/no timeout). This allows proper
	// round-trip restoration when autonomy is disabled.
	infiniteTolerationValue = "infinite"
)

var (
	controllerKind            = appsv1.SchemeGroupVersion.WithKind("Node")
	TolerationKeyToAnnotation = map[string]string{
		corev1.TaintNodeNotReady:    originalNotReadyTolerationDurationAnnotation,
		corev1.TaintNodeUnreachable: originalUnreachableTolerationDurationAnnotation,
	}
)

type ReconcilePodBinding struct {
	client.Client
}

// Add creates a PodBingding controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(ctx context.Context, cfg *appconfig.CompletedConfig, mgr manager.Manager) error {
	klog.Infof("podbinding-controller add controller %s", controllerKind.String())

	reconciler := &ReconcilePodBinding{
		Client: yurtClient.GetClientByControllerNameOrDie(mgr, names.PodBindingController),
	}

	c, err := controller.New(names.PodBindingController, mgr, controller.Options{
		Reconciler: reconciler, MaxConcurrentReconciles: int(cfg.ComponentConfig.PodBindingController.ConcurrentPodBindingWorkers),
	})
	if err != nil {
		return err
	}

	nodeHandler := handler.Funcs{
		UpdateFunc: func(ctx context.Context, updateEvent event.TypedUpdateEvent[client.Object], wq workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			newNode := updateEvent.ObjectNew.(*corev1.Node)
			pods, err := reconciler.getPodsAssignedToNode(newNode.Name)
			if err != nil {
				return
			}

			for i := range pods {
				// skip DaemonSet pods and static pod
				if isDaemonSetPodOrStaticPod(&pods[i]) {
					continue
				}
				if len(pods[i].Spec.NodeName) != 0 {
					wq.Add(reconcile.Request{NamespacedName: types.NamespacedName{Namespace: pods[i].Namespace, Name: pods[i].Name}})
				}
			}
		},
	}

	nodePredicate := predicate.Funcs{
		CreateFunc: func(evt event.CreateEvent) bool {
			return false
		},
		DeleteFunc: func(evt event.DeleteEvent) bool {
			return false
		},
		UpdateFunc: func(evt event.UpdateEvent) bool {
			oldNode, ok := evt.ObjectOld.(*corev1.Node)
			if !ok {
				return false
			}
			newNode, ok := evt.ObjectNew.(*corev1.Node)
			if !ok {
				return false
			}

			// only process edge nodes, and skip nodes with other type.
			if newNode.Labels[projectinfo.GetEdgeWorkerLabelKey()] != "true" {
				klog.Infof("node %s is not a edge node, skip node autonomy settings reconcile.", newNode.Name)
				return false
			}

			// only enqueue if autonomy annotations changed
			if (oldNode.Annotations[projectinfo.GetAutonomyAnnotation()] != newNode.Annotations[projectinfo.GetAutonomyAnnotation()]) ||
				(oldNode.Annotations[projectinfo.GetNodeAutonomyDurationAnnotation()] != newNode.Annotations[projectinfo.GetNodeAutonomyDurationAnnotation()]) {
				return true
			}
			return false
		},
		GenericFunc: func(evt event.GenericEvent) bool {
			return false
		},
	}
	if err := c.Watch(source.Kind[client.Object](mgr.GetCache(), &corev1.Node{}, &nodeHandler, nodePredicate)); err != nil {
		return err
	}

	podPredicate := predicate.Funcs{
		CreateFunc: func(evt event.CreateEvent) bool {
			pod, ok := evt.Object.(*corev1.Pod)
			if !ok {
				return false
			}
			// skip daemonset pod and static pod
			if isDaemonSetPodOrStaticPod(pod) {
				return false
			}

			// check all pods with node name when yurt-manager restarts
			if len(pod.Spec.NodeName) != 0 {
				return true
			}
			return false
		},
		UpdateFunc: func(evt event.UpdateEvent) bool {
			oldPod, ok := evt.ObjectOld.(*corev1.Pod)
			if !ok {
				return false
			}
			newPod, ok := evt.ObjectNew.(*corev1.Pod)
			if !ok {
				return false
			}
			// skip daemonset pod and static pod
			if isDaemonSetPodOrStaticPod(newPod) {
				return false
			}

			// reconcile pod in the following cases:
			// 1. pod is assigned to a node
			// 2. pod tolerations is changed
			// 3. original not ready toleration of pod is changed
			// 4. original unreachable toleration of pod is changed
			if (oldPod.Spec.NodeName != newPod.Spec.NodeName) ||
				!reflect.DeepEqual(oldPod.Spec.Tolerations, newPod.Spec.Tolerations) ||
				(oldPod.Annotations[originalNotReadyTolerationDurationAnnotation] != newPod.Annotations[originalNotReadyTolerationDurationAnnotation]) ||
				(oldPod.Annotations[originalUnreachableTolerationDurationAnnotation] != newPod.Annotations[originalUnreachableTolerationDurationAnnotation]) {
				return true
			}

			return false
		},
		DeleteFunc: func(evt event.DeleteEvent) bool {
			return false
		},
		GenericFunc: func(evt event.GenericEvent) bool {
			return false
		},
	}

	if err := c.Watch(source.Kind[client.Object](mgr.GetCache(), &corev1.Pod{}, &handler.EnqueueRequestForObject{}, podPredicate)); err != nil {
		return err
	}

	return nil
}

// +kubebuilder:rbac:groups="",resources=nodes,verbs=get
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;update

// Reconcile reads that state of Node in cluster and makes changes if node autonomy state has been changed
func (r *ReconcilePodBinding) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	klog.Infof("reconcile pod request: %s/%s", req.Namespace, req.Name)
	pod := &corev1.Pod{}
	if err := r.Get(ctx, req.NamespacedName, pod); err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	if err := r.reconcilePod(pod); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *ReconcilePodBinding) reconcilePod(pod *corev1.Pod) error {
	// skip pod which is not assigned to node
	if len(pod.Spec.NodeName) == 0 {
		return nil
	}

	node := &corev1.Node{}
	if err := r.Get(context.Background(), client.ObjectKey{Name: pod.Spec.NodeName}, node); err != nil {
		return client.IgnoreNotFound(err)
	}

	// skip pods which don't run on edge nodes
	if node.Labels[projectinfo.GetEdgeWorkerLabelKey()] != "true" {
		return nil
	}

	storedPod := pod.DeepCopy()
	if isAutonomous, duration := resolveNodeAutonomySetting(node); isAutonomous {
		// update pod tolerationSeconds according to node autonomy annotation,
		// store the original toleration seconds into pod annotations.
		for i := range pod.Spec.Tolerations {
			if (pod.Spec.Tolerations[i].Key == corev1.TaintNodeNotReady || pod.Spec.Tolerations[i].Key == corev1.TaintNodeUnreachable) &&
				(pod.Spec.Tolerations[i].Effect == corev1.TaintEffectNoExecute) {
				if pod.Annotations == nil {
					pod.Annotations = make(map[string]string)
				}
				if _, ok := pod.Annotations[TolerationKeyToAnnotation[pod.Spec.Tolerations[i].Key]]; !ok {
					// Store original TolerationSeconds value. If nil (infinite tolerance),
					// store a sentinel value to enable proper restoration later.
					if pod.Spec.Tolerations[i].TolerationSeconds != nil {
						pod.Annotations[TolerationKeyToAnnotation[pod.Spec.Tolerations[i].Key]] = fmt.Sprintf("%d", *pod.Spec.Tolerations[i].TolerationSeconds)
					} else {
						pod.Annotations[TolerationKeyToAnnotation[pod.Spec.Tolerations[i].Key]] = infiniteTolerationValue
					}
				}
				pod.Spec.Tolerations[i].TolerationSeconds = duration
			}
		}
	} else {
		// restore toleration seconds from original toleration seconds annotations
		for i := range pod.Spec.Tolerations {
			if (pod.Spec.Tolerations[i].Key == corev1.TaintNodeNotReady || pod.Spec.Tolerations[i].Key == corev1.TaintNodeUnreachable) &&
				(pod.Spec.Tolerations[i].Effect == corev1.TaintEffectNoExecute) {
				if durationStr, ok := pod.Annotations[TolerationKeyToAnnotation[pod.Spec.Tolerations[i].Key]]; ok {
					// Handle restoration: if original was infinite, restore to nil
					if durationStr == infiniteTolerationValue {
						pod.Spec.Tolerations[i].TolerationSeconds = nil
					} else {
						duration, err := strconv.ParseInt(durationStr, 10, 64)
						if err != nil {
							continue
						}
						pod.Spec.Tolerations[i].TolerationSeconds = &duration
					}
				}
			}
		}
	}

	if !reflect.DeepEqual(storedPod, pod) {
		if err := r.Update(context.TODO(), pod, &client.UpdateOptions{}); err != nil {
			klog.Errorf("could not update pod(%s/%s), %v", pod.Namespace, pod.Name, err)
			return err
		}
	}
	return nil
}

func (r *ReconcilePodBinding) getPodsAssignedToNode(name string) ([]corev1.Pod, error) {
	listOptions := &client.ListOptions{
		FieldSelector: fields.SelectorFromSet(fields.Set{
			"spec.nodeName": name,
		}),
	}

	podList := &corev1.PodList{}
	err := r.List(context.TODO(), podList, listOptions)
	if err != nil {
		klog.Errorf("could not get podList for node(%s), %v", name, err)
		return nil, err
	}
	return podList.Items, nil
}

func isDaemonSetPodOrStaticPod(pod *corev1.Pod) bool {
	if pod != nil {
		for i := range pod.OwnerReferences {
			if pod.OwnerReferences[i].Kind == "DaemonSet" {
				return true
			}
		}

		if pod.Annotations != nil && len(pod.Annotations[corev1.MirrorPodAnnotationKey]) != 0 {
			return true
		}
	}

	return false
}

// resolveNodeAutonomySetting is used for resolving node autonomy information.
// The node is configured as autonomous if the node has the following annotations:
// -[deprecated] apps.openyurt.io/binding: "true"
// -[deprecated] node.beta.openyurt.io/autonomy: "true"
// -[recommended] node.openyurt.io/autonomy-duration: "duration"
//
// The first return value indicates whether the node has autonomous mode enabled:
// true means autonomy is enabled, while false means it is not.
// The second return value is only relevant when the first return value is true and
// can be ignored otherwise. This value represents the duration of the node's autonomy.
// If the duration of heartbeat loss is leass then this period, pods on the node will not be evicted.
// However, if the duration of heartbeat loss exceeds this period, then the pods on the node will be evicted.
func resolveNodeAutonomySetting(node *corev1.Node) (bool, *int64) {
	if len(node.Annotations) == 0 {
		return false, nil
	}

	// Pod binding takes precedence against node autonomy
	if node.Annotations[nodeutil.PodBindingAnnotation] == "true" ||
		node.Annotations[projectinfo.GetAutonomyAnnotation()] == "true" {
		return true, nil
	}

	// Node autonomy duration has the least precedence
	duration, ok := node.Annotations[projectinfo.GetNodeAutonomyDurationAnnotation()]
	if !ok {
		return false, nil
	}

	durationTime, err := time.ParseDuration(duration)
	if err != nil {
		klog.Errorf("could not parse autonomy duration %s, %v", duration, err)
		return false, nil
	}

	if durationTime <= 0 {
		return true, nil
	}

	tolerationSeconds := int64(durationTime.Seconds())
	return true, &tolerationSeconds
}
