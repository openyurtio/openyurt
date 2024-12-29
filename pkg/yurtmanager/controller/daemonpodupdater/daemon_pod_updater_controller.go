/*
Copyright 2023 The OpenYurt Authors.
Copyright 2017 The Kubernetes Authors.

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

package daemonpodupdater

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	intstrutil "k8s.io/apimachinery/pkg/util/intstr"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
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
	k8sutil "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/daemonpodupdater/kubernetes"
	podutil "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/pod"
)

var (
	// controllerKind contains the schema.GroupVersionKind for this controller type.
	controllerKind = appsv1.SchemeGroupVersion.WithKind("DaemonSet")
)

const (
	// UpdateAnnotation is the annotation key used in DaemonSet spec to indicate
	// which update strategy is selected. Currently, "OTA" and "AdvancedRollingUpdate" are supported.
	UpdateAnnotation = "apps.openyurt.io/update-strategy"

	// OTAUpdate set DaemonSet to over-the-air update mode.
	// In daemonPodUpdater controller, we add PodNeedUpgrade condition to pods.
	OTAUpdate = "OTA"
	// AutoUpdate set DaemonSet to Auto update mode.
	// In this mode, DaemonSet will keep updating even if there are not-ready nodes.
	// For more details, see https://github.com/openyurtio/openyurt/pull/921.
	AutoUpdate            = "Auto"
	AdvancedRollingUpdate = "AdvancedRollingUpdate"

	// PodNeedUpgrade indicates whether the pod is able to upgrade.
	PodNeedUpgrade corev1.PodConditionType = "PodNeedUpgrade"

	// MaxUnavailableAnnotation is the annotation key added to DaemonSet to indicate
	// the max unavailable pods number. It's used with "apps.openyurt.io/update-strategy=AdvancedRollingUpdate".
	// If this annotation is not explicitly stated, it will be set to the default value 1.
	MaxUnavailableAnnotation = "apps.openyurt.io/max-unavailable"
	DefaultMaxUnavailable    = "10%"

	// BurstReplicas is a rate limiter for booting pods on a lot of pods.
	// The value of 250 is chosen b/c values that are too high can cause registry DoS issues.
	BurstReplicas = 250
)

func Format(format string, args ...interface{}) string {
	s := fmt.Sprintf(format, args...)
	return fmt.Sprintf("%s: %s", names.DaemonPodUpdaterController, s)
}

// Add creates a new Daemonpodupdater Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(ctx context.Context, c *appconfig.CompletedConfig, mgr manager.Manager) error {
	klog.Infof("daemonupdater-controller add controller %s", controllerKind.String())
	r, err := newReconciler(c, mgr)
	if err != nil {
		return err
	}
	return add(mgr, c, r)
}

var _ reconcile.Reconciler = &ReconcileDaemonpodupdater{}

// ReconcileDaemonpodupdater reconciles a DaemonSet object
type ReconcileDaemonpodupdater struct {
	client.Client
	recorder     record.EventRecorder
	expectations k8sutil.ControllerExpectationsInterface
	podControl   k8sutil.PodControlInterface
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(_ *appconfig.CompletedConfig, mgr manager.Manager) (reconcile.Reconciler, error) {
	r := &ReconcileDaemonpodupdater{
		Client:       yurtClient.GetClientByControllerNameOrDie(mgr, names.DaemonPodUpdaterController),
		expectations: k8sutil.NewControllerExpectations(),
		recorder:     mgr.GetEventRecorderFor(names.DaemonPodUpdaterController),
	}

	c, err := kubernetes.NewForConfig(yurtClient.GetConfigByControllerNameOrDie(mgr, names.DaemonPodUpdaterController))
	if err != nil {
		klog.Errorf("could not create kube client, %v", err)
		return nil, err
	}
	// Use PodControlInterface to delete pods, which is convenient for testing
	r.podControl = k8sutil.RealPodControl{
		KubeClient: c,
		Recorder:   r.recorder,
	}

	return r, nil
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, cfg *appconfig.CompletedConfig, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(names.DaemonPodUpdaterController, mgr, controller.Options{Reconciler: r, MaxConcurrentReconciles: int(cfg.Config.ComponentConfig.DaemonPodUpdaterController.ConcurrentDaemonPodUpdaterWorkers)})
	if err != nil {
		return err
	}

	// 1. Watch for changes to DaemonSet
	daemonsetUpdatePredicate := predicate.Funcs{
		CreateFunc: func(evt event.CreateEvent) bool {
			return false
		},
		DeleteFunc: func(evt event.DeleteEvent) bool {
			return false
		},
		UpdateFunc: func(evt event.UpdateEvent) bool {
			return daemonsetUpdate(evt)
		},
		GenericFunc: func(evt event.GenericEvent) bool {
			return false
		},
	}

	if err := c.Watch(source.Kind[client.Object](mgr.GetCache(), &appsv1.DaemonSet{}, &handler.EnqueueRequestForObject{}, daemonsetUpdatePredicate)); err != nil {
		return err
	}

	// 2. Watch for deletion of pods. The reason we watch is that we don't want a daemon set to delete
	// more pods until all the effects (expectations) of a daemon set's delete have been observed.
	updater := r.(*ReconcileDaemonpodupdater)
	if err := c.Watch(source.Kind[client.Object](mgr.GetCache(), &corev1.Pod{}, &handler.Funcs{
		DeleteFunc: updater.deletePod,
	})); err != nil {
		return err
	}
	return nil
}

// daemonsetUpdate filter events: DaemonSet update with customized annotation
func daemonsetUpdate(evt event.UpdateEvent) bool {
	if _, ok := evt.ObjectOld.(*appsv1.DaemonSet); !ok {
		return false
	}

	oldDS := evt.ObjectOld.(*appsv1.DaemonSet)
	newDS := evt.ObjectNew.(*appsv1.DaemonSet)

	// Only handle DaemonSet meets prerequisites
	if !checkPrerequisites(newDS) {
		return false
	}

	if newDS.ResourceVersion == oldDS.ResourceVersion {
		return false
	}

	klog.V(5).Infof("Got DaemonSet update event: %v", newDS.Name)
	return true
}

// +kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get;update
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=nodes,verbs=get;update;patch

// Reconcile reads that state of the cluster for a DaemonSet object and makes changes based on the state read
// and what is in the DaemonSet.Spec
func (r *ReconcileDaemonpodupdater) Reconcile(_ context.Context, request reconcile.Request) (reconcile.Result, error) {

	// Note !!!!!!!!!!
	// We strongly recommend use Format() to  encapsulation because Format() can print logs by module
	// @kadisi
	klog.V(4).Info(Format("Reconcile DaemonpodUpdater %s", request.Name))

	// Fetch the DaemonSet instance
	instance := &appsv1.DaemonSet{}
	if err := r.Get(context.TODO(), request.NamespacedName, instance); err != nil {
		klog.Errorf("could not get DaemonSet %v, %v", request.NamespacedName, err)
		if apierrors.IsNotFound(err) {
			r.expectations.DeleteExpectations(request.NamespacedName.String())
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if instance.DeletionTimestamp != nil {
		return reconcile.Result{}, nil
	}

	// Only process DaemonSet that meets expectations
	// Otherwise, wait native DaemonSet controller reconciling
	if !r.expectations.SatisfiedExpectations(request.NamespacedName.String()) {
		return reconcile.Result{}, nil
	}

	// Recheck required annotation
	v, ok := instance.Annotations[UpdateAnnotation]
	if !ok {
		klog.V(4).Infof("won't sync DaemonSet %q without annotation 'apps.openyurt.io/update-strategy'",
			request.NamespacedName)
		return reconcile.Result{}, nil
	}

	switch strings.ToLower(v) {
	case strings.ToLower(OTAUpdate):
		if err := r.otaUpdate(instance); err != nil {
			klog.Error(Format("could not OTA update DaemonSet %v pod: %v", request.NamespacedName, err))
			return reconcile.Result{}, err
		}

	case strings.ToLower(AutoUpdate), strings.ToLower(AdvancedRollingUpdate):
		if err := r.advancedRollingUpdate(instance); err != nil {
			klog.Error(Format("could not advanced rolling update DaemonSet %v pod: %v", request.NamespacedName, err))
			return reconcile.Result{}, err
		}
	default:
		klog.Error(Format("Unknown update type for DaemonSet %v pod: %v", request.NamespacedName, v))
		return reconcile.Result{}, fmt.Errorf("unknown update type %v", v)
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileDaemonpodupdater) deletePod(ctx context.Context, evt event.DeleteEvent, _ workqueue.RateLimitingInterface) {
	pod, ok := evt.Object.(*corev1.Pod)
	if !ok {
		utilruntime.HandleError(fmt.Errorf("deletepod could not deal with object that is not a pod %#v", evt.Object))
		return
	}

	klog.V(5).Infof("DaemonSet pod %s deleted.", pod.Name)

	controllerRef := metav1.GetControllerOf(pod)
	if controllerRef == nil {
		// No controller should care about orphans being deleted.
		return
	}
	ds := r.resolveControllerRef(pod.Namespace, controllerRef)
	if ds == nil {
		return
	}

	// Only care DaemonSet meets prerequisites
	if !checkPrerequisites(ds) {
		return
	}
	dsKey, err := cache.MetaNamespaceKeyFunc(ds)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}

	r.expectations.DeletionObserved(dsKey)
}

// otaUpdate compare every pod to its owner DaemonSet to check if pod is updatable
// If pod is in line with the latest DaemonSet spec, set pod condition "PodNeedUpgrade" to "false"
// while not, set pod condition "PodNeedUpgrade" to "true"
func (r *ReconcileDaemonpodupdater) otaUpdate(ds *appsv1.DaemonSet) error {
	pods, err := GetDaemonsetPods(r.Client, ds)
	if err != nil {
		return err
	}

	for _, pod := range pods {
		if err := SetPodUpgradeCondition(r.Client, ds, pod); err != nil {
			return err
		}
	}
	return nil
}

// advancedRollingUpdate identifies the set of old pods to delete within the constraints imposed by the max-unavailable number.
// Just ignore and do not calculate not-ready nodes.
func (r *ReconcileDaemonpodupdater) advancedRollingUpdate(ds *appsv1.DaemonSet) error {
	nodeToDaemonPods, err := r.getNodesToDaemonPods(ds)
	if err != nil {
		return fmt.Errorf("couldn't get node to daemon pod mapping for daemon set %q: %v", ds.Name, err)
	}

	// Calculate maxUnavailable specified by user, default is 1
	maxUnavailable, err := r.maxUnavailableCounts(ds, nodeToDaemonPods)
	if err != nil {
		return fmt.Errorf("couldn't get maxUnavailable number for daemon set %q: %v", ds.Name, err)
	}

	var numUnavailable int
	var allowedReplacementPods []string
	var candidatePodsToDelete []string

	for nodeName, pods := range nodeToDaemonPods {
		// Check if node is ready, ignore not-ready node
		// this is a significant difference from the native DaemonSet controller
		ready, err := NodeReadyByName(r.Client, nodeName)
		if err != nil {
			return fmt.Errorf("couldn't check node %q ready status, %v", nodeName, err)
		}
		if !ready {
			continue
		}

		newPod, oldPod, ok := findUpdatedPodsOnNode(ds, pods)
		if !ok {
			// Let the manage loop clean up this node, and treat it as an unavailable node
			klog.V(3).Infof("DaemonSet %s/%s has excess pods on node %s, skipping to allow the core loop to process", ds.Namespace, ds.Name, nodeName)
			numUnavailable++
			continue
		}
		switch {
		case oldPod == nil && newPod == nil, oldPod != nil && newPod != nil:
			// The manage loop will handle creating or deleting the appropriate pod, consider this unavailable
			numUnavailable++
		case newPod != nil:
			// This pod is up-to-date, check its availability
			if !podutil.IsPodAvailable(newPod, ds.Spec.MinReadySeconds, metav1.Time{Time: time.Now()}) {
				// An unavailable new pod is counted against maxUnavailable
				numUnavailable++
			}
		default:
			// This pod is old, it is an update candidate
			switch {
			case !podutil.IsPodAvailable(oldPod, ds.Spec.MinReadySeconds, metav1.Time{Time: time.Now()}):
				// The old pod isn't available, so it needs to be replaced
				klog.V(5).Infof("DaemonSet %s/%s pod %s on node %s is out of date and not available, allowing replacement", ds.Namespace, ds.Name, oldPod.Name, nodeName)
				// Record the replacement
				if allowedReplacementPods == nil {
					allowedReplacementPods = make([]string, 0, len(nodeToDaemonPods))
				}
				allowedReplacementPods = append(allowedReplacementPods, oldPod.Name)
			case numUnavailable >= maxUnavailable:
				// No point considering any other candidates
				continue
			default:
				klog.V(5).Infof("DaemonSet %s/%s pod %s on node %s is out of date, this is a candidate to replace", ds.Namespace, ds.Name, oldPod.Name, nodeName)
				// Record the candidate
				if candidatePodsToDelete == nil {
					candidatePodsToDelete = make([]string, 0, maxUnavailable)
				}
				candidatePodsToDelete = append(candidatePodsToDelete, oldPod.Name)
			}
		}
	}
	// Use any of the candidates we can, including the allowedReplacemnntPods
	klog.V(5).Infof("DaemonSet %s/%s allowing %d replacements, up to %d unavailable, %d are unavailable, %d candidates", ds.Namespace, ds.Name, len(allowedReplacementPods), maxUnavailable, numUnavailable, len(candidatePodsToDelete))
	remainingUnavailable := maxUnavailable - numUnavailable
	if remainingUnavailable < 0 {
		remainingUnavailable = 0
	}
	if max := len(candidatePodsToDelete); remainingUnavailable > max {
		remainingUnavailable = max
	}
	oldPodsToDelete := append(allowedReplacementPods, candidatePodsToDelete[:remainingUnavailable]...)

	return r.syncPodsOnNodes(ds, oldPodsToDelete)
}

// getNodesToDaemonPods returns a map from nodes to daemon pods (corresponding to ds) created for the nodes.
func (r *ReconcileDaemonpodupdater) getNodesToDaemonPods(ds *appsv1.DaemonSet) (map[string][]*corev1.Pod, error) {
	// Ignore adopt/orphan pod, just deal with pods in podLister
	pods, err := GetDaemonsetPods(r.Client, ds)
	if err != nil {
		return nil, err
	}

	// Group Pods by Node name.
	nodeToDaemonPods := make(map[string][]*corev1.Pod)
	for _, pod := range pods {
		nodeName, err := GetTargetNodeName(pod)
		if err != nil {
			klog.Warningf("could not get target node name of Pod %v/%v in DaemonSet %v/%v",
				pod.Namespace, pod.Name, ds.Namespace, ds.Name)
			continue
		}

		nodeToDaemonPods[nodeName] = append(nodeToDaemonPods[nodeName], pod)
	}

	return nodeToDaemonPods, nil
}

// syncPodsOnNodes deletes pods on the given nodes.
// returns slice with errors if any.
func (r *ReconcileDaemonpodupdater) syncPodsOnNodes(ds *appsv1.DaemonSet, podsToDelete []string) error {
	// We need to set expectations before deleting pods to avoid race conditions.
	dsKey, err := cache.MetaNamespaceKeyFunc(ds)
	if err != nil {
		return fmt.Errorf("couldn't get key for object %#v: %v", ds, err)
	}

	deleteDiff := len(podsToDelete)

	if deleteDiff > BurstReplicas {
		deleteDiff = BurstReplicas
	}

	r.expectations.SetExpectations(dsKey, 0, deleteDiff)

	// Error channel to communicate back failures, make the buffer big enough to avoid any blocking
	errCh := make(chan error, deleteDiff)

	// Delete pods process
	klog.V(4).Infof("Pods to delete for daemon set %s: %+v, deleting %d", ds.Name, podsToDelete, deleteDiff)
	deleteWait := sync.WaitGroup{}
	deleteWait.Add(deleteDiff)
	for i := 0; i < deleteDiff; i++ {
		go func(ix int) {
			defer deleteWait.Done()
			if err := r.podControl.DeletePod(context.TODO(), ds.Namespace, podsToDelete[ix], ds); err != nil {
				r.expectations.DeletionObserved(dsKey)
				if !apierrors.IsNotFound(err) {
					klog.V(2).Infof("Failed deletion, decremented expectations for set %q/%q", ds.Namespace, ds.Name)
					errCh <- err
					utilruntime.HandleError(err)
				}
			}
			klog.Infof("AdvancedRollingUpdate pod %v/%v", ds.Name, podsToDelete[ix])
		}(i)
	}
	deleteWait.Wait()

	// Collect errors if any for proper reporting/retry logic in the controller
	errors := []error{}
	close(errCh)
	for err := range errCh {
		errors = append(errors, err)
	}
	return utilerrors.NewAggregate(errors)
}

// maxUnavailableCounts calculates the true number of allowed unavailable
func (r *ReconcileDaemonpodupdater) maxUnavailableCounts(ds *appsv1.DaemonSet, nodeToDaemonPods map[string][]*corev1.Pod) (int, error) {
	// If annotation is not set, use default value one
	v, ok := ds.Annotations[MaxUnavailableAnnotation]
	if !ok || v == "0" || v == "0%" {
		v = DefaultMaxUnavailable
	}

	intstrv := intstrutil.Parse(v)
	maxUnavailable, err := intstrutil.GetScaledValueFromIntOrPercent(&intstrv, len(nodeToDaemonPods), true)
	if err != nil {
		return -1, fmt.Errorf("invalid value for MaxUnavailable: %v", err)
	}

	klog.V(5).Infof("DaemonSet %s/%s, maxUnavailable: %d", ds.Namespace, ds.Name, maxUnavailable)
	return maxUnavailable, nil
}

// resolveControllerRef returns the controller referenced by a ControllerRef,
// or nil if the ControllerRef could not be resolved to a matching controller
// of the correct Kind.
func (r *ReconcileDaemonpodupdater) resolveControllerRef(namespace string, controllerRef *metav1.OwnerReference) *appsv1.DaemonSet {
	// We can't look up by UID, so look up by Name and then verify UID.
	// Don't even try to look up by Name if it's the wrong Kind.
	if controllerRef.Kind != controllerKind.Kind {
		return nil
	}
	ds := &appsv1.DaemonSet{}
	if err := r.Get(context.TODO(), types.NamespacedName{Name: controllerRef.Name, Namespace: namespace}, ds); err != nil {
		return nil
	}
	if ds.UID != controllerRef.UID {
		// The controller we found with this Name is not the same one that the
		// ControllerRef points to.
		return nil
	}
	return ds
}
