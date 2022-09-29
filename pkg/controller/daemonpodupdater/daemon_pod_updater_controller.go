/*
Copyright 2022 The OpenYurt Authors.
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
	"sync"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	intstrutil "k8s.io/apimachinery/pkg/util/intstr"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	appsinformers "k8s.io/client-go/informers/apps/v1"
	coreinformers "k8s.io/client-go/informers/core/v1"
	client "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	appslisters "k8s.io/client-go/listers/apps/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	k8sutil "github.com/openyurtio/openyurt/pkg/controller/daemonpodupdater/kubernetes"
)

const (
	// UpdateAnnotation is the annotation key used in daemonset spec to indicate
	// which update strategy is selected. Currently, "ota" and "auto" are supported.
	UpdateAnnotation = "apps.openyurt.io/update-strategy"

	// OTAUpdate set daemonset to over-the-air update mode.
	// In daemonPodUpdater controller, we add PodNeedUpgrade condition to pods.
	OTAUpdate = "ota"
	// AutoUpdate set daemonset to auto update mode.
	// In this mode, daemonset will keep updating even if there are not-ready nodes.
	// For more details, see https://github.com/openyurtio/openyurt/pull/921.
	AutoUpdate = "auto"

	// PodNeedUpgrade indicates whether the pod is able to upgrade.
	PodNeedUpgrade corev1.PodConditionType = "PodNeedUpgrade"

	// MaxUnavailableAnnotation is the annotation key added to daemonset to indicate
	// the max unavailable pods number. It's used with "apps.openyurt.io/update-strategy=auto".
	// If this annotation is not explicitly stated, it will be set to the default value 1.
	MaxUnavailableAnnotation = "apps.openyurt.io/max-unavailable"
	DefaultMaxUnavailable    = "10%"

	// BurstReplicas is a rate limiter for booting pods on a lot of pods.
	// The value of 250 is chosen b/c values that are too high can cause registry DoS issues.
	BurstReplicas = 250

	maxRetries = 30
)

// controllerKind contains the schema.GroupVersionKind for this controller type.
var controllerKind = appsv1.SchemeGroupVersion.WithKind("DaemonSet")

type Controller struct {
	kubeclientset client.Interface
	podControl    k8sutil.PodControlInterface
	// daemonPodUpdater watches daemonset, node and pod resource
	daemonsetLister    appslisters.DaemonSetLister
	daemonsetSynced    cache.InformerSynced
	nodeLister         corelisters.NodeLister
	nodeSynced         cache.InformerSynced
	podLister          corelisters.PodLister
	podSynced          cache.InformerSynced
	daemonsetWorkqueue workqueue.RateLimitingInterface
	expectations       k8sutil.ControllerExpectationsInterface
}

func NewController(kc client.Interface, daemonsetInformer appsinformers.DaemonSetInformer,
	nodeInformer coreinformers.NodeInformer, podInformer coreinformers.PodInformer) *Controller {

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartStructuredLogging(0)
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: kc.CoreV1().Events("")})

	ctrl := Controller{
		kubeclientset: kc,
		// Use PodControlInterface to delete pods, which is convenient for testing
		podControl: k8sutil.RealPodControl{
			KubeClient: kc,
			Recorder:   eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "daemonPodUpdater"}),
		},

		daemonsetLister: daemonsetInformer.Lister(),
		daemonsetSynced: daemonsetInformer.Informer().HasSynced,

		nodeLister: nodeInformer.Lister(),
		nodeSynced: nodeInformer.Informer().HasSynced,

		podLister: podInformer.Lister(),
		podSynced: podInformer.Informer().HasSynced,

		daemonsetWorkqueue: workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter()),
		expectations:       k8sutil.NewControllerExpectations(),
	}

	// In this controller, we focus three cases
	// 1. daemonset specification changes
	// 2. node turns from not-ready to ready
	// 3. pods were deleted successfully
	// In case 2, daemonset.Status.DesiredNumberScheduled will change and, in case 3, daemonset.Status.NumberReady
	// will change. Therefore, we focus only on the daemonset Update event, which can cover the above situations.
	daemonsetInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		UpdateFunc: func(old, new interface{}) {
			newDS := new.(*appsv1.DaemonSet)
			oldDS := old.(*appsv1.DaemonSet)

			// Only handle daemonset meets prerequisites
			if !checkPrerequisites(newDS) {
				return
			}

			if newDS.ResourceVersion == oldDS.ResourceVersion {
				return
			}

			klog.V(5).Infof("Got daemonset udpate event: %v", newDS.Name)
			ctrl.enqueueDaemonSet(newDS)
		},
	})

	// Watch for deletion of pods. The reason we watch is that we don't want a daemon set to delete
	// more pods until all the effects (expectations) of a daemon set's delete have been observed.
	podInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		DeleteFunc: ctrl.deletePod,
	})
	return &ctrl
}

func (c *Controller) enqueueDaemonSet(ds *appsv1.DaemonSet) {
	key, err := cache.MetaNamespaceKeyFunc(ds)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("couldn't get key for object %#v: %v", ds, err))
		return
	}

	klog.V(5).Infof("Daemonset %v queued", key)
	c.daemonsetWorkqueue.Add(key)
}

func (c *Controller) deletePod(obj interface{}) {
	pod, ok := obj.(*corev1.Pod)
	// When a deletion is dropped, the relist will notice a pod in the store not
	// in the list, leading to the insertion of a tombstone object which contains
	// the deleted key/value. Note that this value might be stale. If the pod
	// changed labels the new daemonset will not be woken up till the periodic
	// resync.
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("couldn't get object from tombstone %#v", obj))
			return
		}
		pod, ok = tombstone.Obj.(*corev1.Pod)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("tombstone contained object that is not a pod %#v", obj))
			return
		}
	}

	klog.V(5).Infof("Daemonset pod %s deleted.", pod.Name)

	controllerRef := metav1.GetControllerOf(pod)
	if controllerRef == nil {
		// No controller should care about orphans being deleted.
		return
	}
	ds := c.resolveControllerRef(pod.Namespace, controllerRef)
	if ds == nil {
		return
	}

	// Only care daemonset meets prerequisites
	if !checkPrerequisites(ds) {
		return
	}
	dsKey, err := cache.MetaNamespaceKeyFunc(ds)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}

	c.expectations.DeletionObserved(dsKey)
}

func (c *Controller) Run(threadiness int, stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()

	klog.Info("Starting daemonPodUpdater controller")
	defer klog.Info("Shutting down daemonPodUpdater controller")
	defer c.daemonsetWorkqueue.ShutDown()

	// Synchronize the cache before starting to process events
	if !cache.WaitForCacheSync(stopCh, c.daemonsetSynced, c.nodeSynced, c.podSynced) {
		klog.Error("sync daemonPodUpdater controller timeout")
	}

	for i := 0; i < threadiness; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	<-stopCh
}

func (c *Controller) runWorker() {
	for {
		obj, shutdown := c.daemonsetWorkqueue.Get()
		if shutdown {
			return
		}

		if err := c.syncHandler(obj.(string)); err != nil {
			if c.daemonsetWorkqueue.NumRequeues(obj) < maxRetries {
				klog.Infof("error syncing event %v: %v", obj, err)
				c.daemonsetWorkqueue.AddRateLimited(obj)
				c.daemonsetWorkqueue.Done(obj)
				continue
			}
			utilruntime.HandleError(err)
		}

		c.daemonsetWorkqueue.Forget(obj)
		c.daemonsetWorkqueue.Done(obj)
	}
}

func (c *Controller) syncHandler(key string) error {
	defer func() {
		klog.V(4).Infof("Finish syncing daemonPodUpdater request %q", key)
	}()

	klog.V(4).Infof("Start handling daemonPodUpdater request %q", key)

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return fmt.Errorf("invalid resource key: %s", key)
	}

	// Daemonset that need to be synced
	ds, err := c.daemonsetLister.DaemonSets(namespace).Get(name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			c.expectations.DeleteExpectations(key)
			return nil
		}
		return err
	}

	if ds.DeletionTimestamp != nil {
		return nil
	}

	// Only process daemonset that meets expectations
	// Otherwise, wait native daemonset controller reconciling
	if !c.expectations.SatisfiedExpectations(key) {
		return nil
	}

	// Recheck required annotation
	v, ok := ds.Annotations[UpdateAnnotation]
	if !ok {
		return fmt.Errorf("won't sync daemonset %q without annotation 'apps.openyurt.io/update-strategy'", ds.Name)
	}

	switch v {
	case OTAUpdate:
		if err := c.otaUpdate(ds); err != nil {
			return err
		}

	case AutoUpdate:
		if err := c.autoUpdate(ds); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown annotation type %v", v)
	}

	return nil
}

// otaUpdate compare every pod to its owner daemonset to check if pod is updatable
// If pod is in line with the latest daemonset spec, set pod condition "PodNeedUpgrade" to "false"
// while not, set pod condition "PodNeedUpgrade" to "true"
func (c *Controller) otaUpdate(ds *appsv1.DaemonSet) error {
	pods, err := GetDaemonsetPods(c.podLister, ds)
	if err != nil {
		return err
	}

	for _, pod := range pods {
		if err := SetPodUpgradeCondition(c.kubeclientset, ds, pod); err != nil {
			return err
		}
	}
	return nil
}

// autoUpdate identifies the set of old pods to delete within the constraints imposed by the max-unavailable number.
// Just ignore and do not calculate not-ready nodes.
func (c *Controller) autoUpdate(ds *appsv1.DaemonSet) error {
	nodeToDaemonPods, err := c.getNodesToDaemonPods(ds)
	if err != nil {
		return fmt.Errorf("couldn't get node to daemon pod mapping for daemon set %q: %v", ds.Name, err)
	}

	// Calculate maxUnavailable specified by user, default is 1
	maxUnavailable, err := c.maxUnavailableCounts(ds, nodeToDaemonPods)
	if err != nil {
		return fmt.Errorf("couldn't get maxUnavailable number for daemon set %q: %v", ds.Name, err)
	}

	var numUnavailable int
	var allowedReplacementPods []string
	var candidatePodsToDelete []string

	for nodeName, pods := range nodeToDaemonPods {
		// Check if node is ready, ignore not-ready node
		// this is a significant difference from the native daemonset controller
		ready, err := NodeReadyByName(c.nodeLister, nodeName)
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
			if !k8sutil.IsPodAvailable(newPod, ds.Spec.MinReadySeconds, metav1.Time{Time: time.Now()}) {
				// An unavailable new pod is counted against maxUnavailable
				numUnavailable++
			}
		default:
			// This pod is old, it is an update candidate
			switch {
			case !k8sutil.IsPodAvailable(oldPod, ds.Spec.MinReadySeconds, metav1.Time{Time: time.Now()}):
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

	return c.syncPodsOnNodes(ds, oldPodsToDelete)
}

// getNodesToDaemonPods returns a map from nodes to daemon pods (corresponding to ds) created for the nodes.
func (c *Controller) getNodesToDaemonPods(ds *appsv1.DaemonSet) (map[string][]*corev1.Pod, error) {
	// Ignore adopt/orphan pod, just deal with pods in podLister
	pods, err := GetDaemonsetPods(c.podLister, ds)
	if err != nil {
		return nil, err
	}

	// Group Pods by Node name.
	nodeToDaemonPods := make(map[string][]*corev1.Pod)
	for _, pod := range pods {
		nodeName, err := GetTargetNodeName(pod)
		if err != nil {
			klog.Warningf("Failed to get target node name of Pod %v/%v in DaemonSet %v/%v",
				pod.Namespace, pod.Name, ds.Namespace, ds.Name)
			continue
		}

		nodeToDaemonPods[nodeName] = append(nodeToDaemonPods[nodeName], pod)
	}

	return nodeToDaemonPods, nil
}

// syncPodsOnNodes deletes pods on the given nodes.
// returns slice with errors if any.
func (c *Controller) syncPodsOnNodes(ds *appsv1.DaemonSet, podsToDelete []string) error {
	// We need to set expectations before deleting pods to avoid race conditions.
	dsKey, err := cache.MetaNamespaceKeyFunc(ds)
	if err != nil {
		return fmt.Errorf("couldn't get key for object %#v: %v", ds, err)
	}

	deleteDiff := len(podsToDelete)

	if deleteDiff > BurstReplicas {
		deleteDiff = BurstReplicas
	}

	c.expectations.SetExpectations(dsKey, 0, deleteDiff)

	// Error channel to communicate back failures, make the buffer big enough to avoid any blocking
	errCh := make(chan error, deleteDiff)

	// Delete pods process
	klog.V(4).Infof("Pods to delete for daemon set %s: %+v, deleting %d", ds.Name, podsToDelete, deleteDiff)
	deleteWait := sync.WaitGroup{}
	deleteWait.Add(deleteDiff)
	for i := 0; i < deleteDiff; i++ {
		go func(ix int) {
			defer deleteWait.Done()
			if err := c.podControl.DeletePod(context.TODO(), ds.Namespace, podsToDelete[ix], ds); err != nil {
				c.expectations.DeletionObserved(dsKey)
				if !apierrors.IsNotFound(err) {
					klog.V(2).Infof("Failed deletion, decremented expectations for set %q/%q", ds.Namespace, ds.Name)
					errCh <- err
					utilruntime.HandleError(err)
				}
			}
			klog.Infof("Auto update pod %v/%v", ds.Name, podsToDelete[ix])
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
func (c *Controller) maxUnavailableCounts(ds *appsv1.DaemonSet, nodeToDaemonPods map[string][]*corev1.Pod) (int, error) {
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
func (c *Controller) resolveControllerRef(namespace string, controllerRef *metav1.OwnerReference) *appsv1.DaemonSet {
	// We can't look up by UID, so look up by Name and then verify UID.
	// Don't even try to look up by Name if it's the wrong Kind.
	if controllerRef.Kind != controllerKind.Kind {
		return nil
	}
	ds, err := c.daemonsetLister.DaemonSets(namespace).Get(controllerRef.Name)
	if err != nil {
		return nil
	}
	if ds.UID != controllerRef.UID {
		// The controller we found with this Name is not the same one that the
		// ControllerRef points to.
		return nil
	}
	return ds
}
