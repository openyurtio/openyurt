/*
Copyright 2022 The OpenYurt Authors.

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

package podupgrade

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
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	appsinformers "k8s.io/client-go/informers/apps/v1"
	coreinformers "k8s.io/client-go/informers/core/v1"
	client "k8s.io/client-go/kubernetes"
	appslisters "k8s.io/client-go/listers/apps/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	k8sutil "github.com/openyurtio/openyurt/pkg/controller/podupgrade/kubernetes/util"
)

const (
	// BurstReplicas is a rate limiter for booting pods on a lot of pods.
	// The value of 250 is chosen b/c values that are too high can cause registry DoS issues.
	BurstReplicas = 250
)

// controllerKind contains the schema.GroupVersionKind for this controller type.
var controllerKind = appsv1.SchemeGroupVersion.WithKind("DaemonSet")

type Controller struct {
	kubeclientset client.Interface

	daemonsetLister appslisters.DaemonSetLister
	daemonsetSynced cache.InformerSynced
	nodeLister      corelisters.NodeLister
	nodeSynced      cache.InformerSynced
	podLister       corelisters.PodLister
	podSynced       cache.InformerSynced

	daemonsetWorkqueue workqueue.Interface

	expectations k8sutil.ControllerExpectationsInterface
}

func NewController(kc client.Interface, daemonsetInformer appsinformers.DaemonSetInformer,
	nodeInformer coreinformers.NodeInformer, podInformer coreinformers.PodInformer) *Controller {

	ctrl := Controller{
		kubeclientset:   kc,
		daemonsetLister: daemonsetInformer.Lister(),
		daemonsetSynced: daemonsetInformer.Informer().HasSynced,

		nodeLister: nodeInformer.Lister(),
		nodeSynced: nodeInformer.Informer().HasSynced,

		podLister: podInformer.Lister(),
		podSynced: podInformer.Informer().HasSynced,

		daemonsetWorkqueue: workqueue.New(),
		expectations:       k8sutil.NewControllerExpectations(),
	}

	daemonsetInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		UpdateFunc: func(old, new interface{}) {
			newDS := new.(*appsv1.DaemonSet)
			oldDS := old.(*appsv1.DaemonSet)

			// Only control daemonset meets prerequisites
			if !checkPrerequisites(newDS) {
				return
			}

			if newDS.ResourceVersion == oldDS.ResourceVersion || oldDS.Status.CurrentNumberScheduled != newDS.Status.DesiredNumberScheduled {
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
		utilruntime.HandleError(fmt.Errorf("Couldn't get key for object %#v: %v", ds, err))
		return
	}

	klog.V(5).Infof("Daemonset %v queued", key)
	c.daemonsetWorkqueue.Add(key)
}

func (c *Controller) deletePod(obj interface{}) {
	pod, ok := obj.(*corev1.Pod)
	// When a delete is dropped, the relist will notice a pod in the store not
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
		return
	}

	c.expectations.DeletionObserved(dsKey)
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

// enqueueDaemonsetWhenNodeReady add daemonset key to daemonsetWorkqueue for further handling
func (c *Controller) enqueueDaemonsetWhenNodeReady(node *corev1.Node) error {
	// TODO: Question-> is source lister up-to-date?
	pods, err := GetNodePods(c.podLister, node)
	if err != nil {
		return err
	}

	for _, pod := range pods {
		owner := metav1.GetControllerOf(pod)
		switch owner.Kind {
		case DaemonSet:
			ds, err := c.daemonsetLister.DaemonSets(pod.Namespace).Get(owner.Name)
			if err != nil {
				if apierrors.IsNotFound(err) {
					continue
				}
				return err
			}

			if checkPrerequisites(ds) {
				var key string
				if key, err = cache.MetaNamespaceKeyFunc(ds); err != nil {
					return err
				}

				c.daemonsetWorkqueue.Add(key)
			}
		default:
			continue
		}
	}
	return nil
}

func (c *Controller) Run(threadiness int, stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()

	klog.Info("Starting pod upgrade controller")
	defer klog.Info("Shutting down pod upgrade controller")
	defer c.daemonsetWorkqueue.ShutDown()

	//synchronize the cache before starting to process events
	if !cache.WaitForCacheSync(stopCh, c.daemonsetSynced, c.nodeSynced,
		c.podSynced) {
		klog.Error("sync podupgrade controller timeout")
	}

	for i := 0; i < threadiness; i++ {
		go wait.Until(c.runDaemonsetWorker, time.Second, stopCh)
	}

	<-stopCh
}

func (c *Controller) runDaemonsetWorker() {
	for {
		obj, shutdown := c.daemonsetWorkqueue.Get()
		if shutdown {
			return
		}

		if err := c.syncDaemonsetHandler(obj.(string)); err != nil {
			utilruntime.HandleError(err)
		}
		c.daemonsetWorkqueue.Done(obj)
	}
}

func (c *Controller) syncDaemonsetHandler(key string) error {
	defer func() {
		klog.V(4).Infof("Finish syncing pod upgrade request %q", key)
	}()

	klog.V(4).Infof("Start handler pod upgrade request %q", key)

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}

	// daemonset that need to be synced
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

	pods, err := GetDaemonsetPods(c.podLister, ds)
	if err != nil {
		return err
	}

	// recheck required annotation
	v, ok := ds.Annotations[UpgradeAnnotation]
	if !ok {
		return fmt.Errorf("won't sync daemonset %q without annotation 'apps.openyurt.io/upgrade-strategy'", ds.Name)
	}

	switch v {
	case OTAUpgrade:
		if err := c.checkOTAUpgrade(ds, pods); err != nil {
			return err
		}

	case AutoUpgrade:
		if err := c.autoUpgrade(ds); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown annotation type %v", v)
	}

	return nil
}

// checkOTAUpgrade compare every pod to its owner daemonset to check if pod is upgradable
// If pod is in line with the latest daemonset version, set annotation "apps.openyurt.io/pod-upgradable" to "true"
// while not, set annotation "apps.openyurt.io/pod-upgradable" to "false"
func (c *Controller) checkOTAUpgrade(ds *appsv1.DaemonSet, pods []*corev1.Pod) error {
	for _, pod := range pods {
		if err := SetPodUpgradeAnnotation(c.kubeclientset, ds, pod); err != nil {
			return err
		}
	}
	return nil
}

// autoUpgrade identifies the set of old pods to delete
func (c *Controller) autoUpgrade(ds *appsv1.DaemonSet) error {
	nodeToDaemonPods, err := c.getNodesToDaemonPods(ds)
	if err != nil {
		return fmt.Errorf("couldn't get node to daemon pod mapping for daemon set %q: %v", ds.Name, err)
	}

	// TODO(hxc)
	// calculate maxUnavailable specified by user, default is xxx??
	// maxUnavailable := xxx
	maxUnavailable := 1

	var numUnavailable int
	var allowedReplacementPods []string
	var candidatePodsToDelete []string

	for nodeName, pods := range nodeToDaemonPods {
		// check if node is ready, ignore not-ready node
		ready, err := NodeReadyByName(c.nodeLister, nodeName)
		if err != nil {
			return fmt.Errorf("couldn't check node %q ready status, %v", nodeName, err)
		}
		if !ready {
			continue
		}

		newPod, oldPod, ok := findUpdatedPodsOnNode(ds, pods)
		if !ok {
			// let the manage loop clean up this node, and treat it as an unavailable node
			klog.V(3).Infof("DaemonSet %s/%s has excess pods on node %s, skipping to allow the core loop to process", ds.Namespace, ds.Name, nodeName)
			numUnavailable++
			continue
		}
		switch {
		case oldPod == nil && newPod == nil, oldPod != nil && newPod != nil:
			// the manage loop will handle creating or deleting the appropriate pod, consider this unavailable
			numUnavailable++
		case newPod != nil:
			// this pod is up to date, check its availability
			if !k8sutil.IsPodAvailable(newPod, ds.Spec.MinReadySeconds, metav1.Time{Time: time.Now()}) {
				// an unavailable new pod is counted against maxUnavailable
				numUnavailable++
			}
		default:
			// this pod is old, it is an update candidate
			switch {
			case !k8sutil.IsPodAvailable(oldPod, ds.Spec.MinReadySeconds, metav1.Time{Time: time.Now()}):
				// the old pod isn't available, so it needs to be replaced
				klog.V(5).Infof("DaemonSet %s/%s pod %s on node %s is out of date and not available, allowing replacement", ds.Namespace, ds.Name, oldPod.Name, nodeName)
				// record the replacement
				if allowedReplacementPods == nil {
					allowedReplacementPods = make([]string, 0, len(nodeToDaemonPods))
				}
				allowedReplacementPods = append(allowedReplacementPods, oldPod.Name)
			case numUnavailable >= maxUnavailable:
				// no point considering any other candidates
				continue
			default:
				klog.V(5).Infof("DaemonSet %s/%s pod %s on node %s is out of date, this is a candidate to replace", ds.Namespace, ds.Name, oldPod.Name, nodeName)
				// record the candidate
				if candidatePodsToDelete == nil {
					candidatePodsToDelete = make([]string, 0, maxUnavailable)
				}
				candidatePodsToDelete = append(candidatePodsToDelete, oldPod.Name)
			}
		}
	}
	// use any of the candidates we can, including the allowedReplacemnntPods
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

func (c *Controller) getNodesToDaemonPods(ds *appsv1.DaemonSet) (map[string][]*corev1.Pod, error) {
	// TODO(hxc): ignore adopt/orphan pod, just deal with pods in podLister
	pods, err := GetDaemonsetPods(c.podLister, ds)
	if err != nil {
		return nil, err
	}

	// Group Pods by Node name.
	nodeToDaemonPods := make(map[string][]*corev1.Pod)
	for _, pod := range pods {
		nodeName, err := k8sutil.GetTargetNodeName(pod)
		if err != nil {
			klog.Warningf("Failed to get target node name of Pod %v/%v in DaemonSet %v/%v",
				pod.Namespace, pod.Name, ds.Namespace, ds.Name)
			continue
		}

		nodeToDaemonPods[nodeName] = append(nodeToDaemonPods[nodeName], pod)
	}

	return nodeToDaemonPods, nil
}

// syncPodsOnNodes deletes pods on the given nodes
// returns slice with errors if any
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

	// error channel to communicate back failures, make the buffer big enough to avoid any blocking
	errCh := make(chan error, deleteDiff)

	// delete pods process
	klog.V(4).Infof("Pods to delete for daemon set %s: %+v, deleting %d", ds.Name, podsToDelete, deleteDiff)
	deleteWait := sync.WaitGroup{}
	deleteWait.Add(deleteDiff)
	for i := 0; i < deleteDiff; i++ {
		go func(ix int) {
			defer deleteWait.Done()
			if err := c.kubeclientset.CoreV1().Pods(ds.Namespace).
				Delete(context.TODO(), podsToDelete[ix], metav1.DeleteOptions{}); err != nil {
				c.expectations.DeletionObserved(dsKey)
				if !apierrors.IsNotFound(err) {
					klog.V(2).Infof("Failed deletion, decremented expectations for set %q/%q", ds.Namespace, ds.Name)
					errCh <- err
					utilruntime.HandleError(err)
				}
			}
			klog.Infof("Auto upgrade pod %v/%v", ds.Name, podsToDelete[ix])
		}(i)
	}
	deleteWait.Wait()

	// collect errors if any for proper reporting/retry logic in the controller
	errors := []error{}
	close(errCh)
	for err := range errCh {
		errors = append(errors, err)
	}
	return utilerrors.NewAggregate(errors)
}
