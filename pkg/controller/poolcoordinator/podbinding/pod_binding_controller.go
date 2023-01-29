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
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	client "k8s.io/client-go/kubernetes"
	listerv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/controller/poolcoordinator/constant"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
)

const (
	numWorkers       = 5
	nodeNameKeyIndex = "spec.nodeName"
)

var (
	notReadyToleration = corev1.Toleration{
		Key:      corev1.TaintNodeNotReady,
		Operator: corev1.TolerationOpExists,
		Effect:   corev1.TaintEffectNoExecute,
	}

	unreachableToleration = corev1.Toleration{
		Key:      corev1.TaintNodeUnreachable,
		Operator: corev1.TolerationOpExists,
		Effect:   corev1.TaintEffectNoExecute,
	}
	defaultTolerationSeconds = 300
)

type Controller struct {
	client                client.Interface
	nodeSynced            cache.InformerSynced
	nodeLister            listerv1.NodeLister
	podSynced             cache.InformerSynced
	podLister             listerv1.PodLister
	getPodsAssignedToNode func(nodeName string) ([]*corev1.Pod, error)
	podUpdateQueue        workqueue.RateLimitingInterface
}

func (c *Controller) onNodeUpdate(o interface{}, n interface{}) {
	oldNode := o.(*corev1.Node)
	newNode := n.(*corev1.Node)

	// if node autonomy annotation changed, we need to handle all pods on this node except DaemonSet pod.
	if oldNode.Annotations[projectinfo.GetAutonomyAnnotation()] != newNode.Annotations[projectinfo.GetAutonomyAnnotation()] {
		pods, err := c.getPodsAssignedToNode(newNode.Name)
		if err != nil {
			klog.Errorf("failed to get pods for node(%s)", newNode.Name)
			return
		}

		for i := range pods {
			// skip DaemonSet pods and static pod
			if isDaemonSetPodOrStaticPod(pods[i]) {
				continue
			}

			// skip not running pods
			if pods[i].Status.Phase != corev1.PodRunning {
				continue
			}

			key, err := cache.MetaNamespaceKeyFunc(pods[i])
			if err == nil {
				c.podUpdateQueue.Add(key)
				klog.Infof("pod(%s) tolerations should be handled for node annotation changed", key)
			}
		}
	}
}

func (c *Controller) onPodUpdate(_ interface{}, n interface{}) {
	newPod := n.(*corev1.Pod)

	// skip DaemonSet pod and static pod
	if isDaemonSetPodOrStaticPod(newPod) {
		return
	}

	// skip not running pods
	if newPod.Status.Phase != corev1.PodRunning {
		return
	}

	needHandled := false
	// only handle pods with binding annotation or work on node with autonomy annotation
	if newPod.Annotations != nil && newPod.Annotations[constant.PodBindingAnnotation] == "true" {
		needHandled = true
	} else if len(newPod.Spec.NodeName) != 0 {
		node, err := c.nodeLister.Get(newPod.Spec.NodeName)
		if err != nil {
			klog.Errorf("failed to get node(%s) for pod(%s/%s)", newPod.Spec.NodeName, newPod.Namespace, newPod.Name)
			return
		}

		if node.Annotations != nil && node.Annotations[projectinfo.GetAutonomyAnnotation()] == "true" {
			needHandled = true
		}
	}

	if needHandled {
		key, err := cache.MetaNamespaceKeyFunc(newPod)
		if err == nil {
			klog.Infof("pod(%s) tolerations should be handled for pod update", key)
			c.podUpdateQueue.Add(key)
		}
	}
}

func NewController(kc client.Interface, informerFactory informers.SharedInformerFactory) *Controller {
	ctl := &Controller{
		client:         kc,
		podUpdateQueue: workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter()),
	}

	nodeInformer := informerFactory.Core().V1().Nodes()
	nodeInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		UpdateFunc: ctl.onNodeUpdate,
	})
	ctl.nodeSynced = nodeInformer.Informer().HasSynced
	ctl.nodeLister = nodeInformer.Lister()

	podInformer := informerFactory.Core().V1().Pods()
	podInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		UpdateFunc: ctl.onPodUpdate,
	})
	podInformer.Informer().AddIndexers(cache.Indexers{
		nodeNameKeyIndex: func(obj interface{}) ([]string, error) {
			pod, ok := obj.(*corev1.Pod)
			if !ok {
				return []string{}, nil
			}
			if len(pod.Spec.NodeName) == 0 {
				return []string{}, nil
			}
			return []string{pod.Spec.NodeName}, nil
		},
	})

	podIndexer := podInformer.Informer().GetIndexer()
	ctl.getPodsAssignedToNode = func(nodeName string) ([]*corev1.Pod, error) {
		objs, err := podIndexer.ByIndex(nodeNameKeyIndex, nodeName)
		if err != nil {
			return nil, err
		}
		pods := make([]*corev1.Pod, 0, len(objs))
		for _, obj := range objs {
			pod, ok := obj.(*corev1.Pod)
			if !ok {
				continue
			}
			pods = append(pods, pod)
		}
		return pods, nil
	}
	ctl.podSynced = podInformer.Informer().HasSynced
	ctl.podLister = podInformer.Lister()

	return ctl
}

func (c *Controller) configureTolerationForPod(pod *corev1.Pod, tolerationSeconds *int64) error {
	// reset toleration seconds
	notReadyToleration.TolerationSeconds = tolerationSeconds
	unreachableToleration.TolerationSeconds = tolerationSeconds
	toleratesNodeNotReady := addOrUpdateTolerationInPodSpec(&pod.Spec, &notReadyToleration)
	toleratesNodeUnreachable := addOrUpdateTolerationInPodSpec(&pod.Spec, &unreachableToleration)

	klog.Infof("pod(%s/%s) => toleratesNodeNotReady=%v, toleratesNodeUnreachable=%v, tolerationSeconds=%v", pod.Namespace, pod.Name, toleratesNodeNotReady, toleratesNodeUnreachable, tolerationSeconds)
	if toleratesNodeNotReady || toleratesNodeUnreachable {
		_, err := c.client.CoreV1().Pods(pod.Namespace).Update(context.TODO(), pod, metav1.UpdateOptions{})
		if err != nil {
			klog.Errorf("failed to update toleration of pod(%s/%s), %v", pod.Namespace, pod.Name, err)
			return err
		}
	}

	return nil
}

func (c *Controller) syncHandler(key string) error {
	ns, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		klog.Errorf("invalid resource key: %s", key)
		return nil
	}

	pod, err := c.podLister.Pods(ns).Get(name)
	if err != nil {
		klog.Errorf("couldn't get pod(%s/%s), maybe it has been deleted\n", ns, name)
		return nil
	}

	// only pods with binding annotation and pods on node with autonomy annotation
	// should be handled
	shouldBeAdded := false
	if pod.Annotations != nil && pod.Annotations[constant.PodBindingAnnotation] == "true" {
		shouldBeAdded = true
	}

	if len(pod.Spec.NodeName) != 0 {
		node, _ := c.nodeLister.Get(pod.Spec.NodeName)
		if node != nil && node.Annotations != nil && node.Annotations[projectinfo.GetAutonomyAnnotation()] == "true" {
			shouldBeAdded = true
		}
	}

	if shouldBeAdded {
		return c.configureTolerationForPod(pod, nil)
	} else {
		tolerationSeconds := int64(defaultTolerationSeconds)
		return c.configureTolerationForPod(pod, &tolerationSeconds)
	}
}

func (c *Controller) podWorker() {
	for c.processNextItem() {
	}
}

func (c *Controller) processNextItem() bool {
	key, shutdown := c.podUpdateQueue.Get()
	if shutdown {
		klog.Info("pod work queue shutdown")
		return false
	}
	defer c.podUpdateQueue.Done(key)

	if err := c.syncHandler(key.(string)); err != nil {
		c.podUpdateQueue.AddRateLimited(key)
		runtime.HandleError(err)
		return true
	}

	c.podUpdateQueue.Forget(key)
	return true
}

func (c *Controller) Run(stopCH <-chan struct{}) {
	if !cache.WaitForCacheSync(stopCH, c.nodeSynced, c.podSynced) {
		klog.Error("sync pod binding controller timeout")
	}

	defer c.podUpdateQueue.ShutDown()

	klog.Info("start pod binding workers")
	for i := 0; i < numWorkers; i++ {
		go wait.Until(c.podWorker, time.Second, stopCH)
	}

	<-stopCH
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

// addOrUpdateTolerationInPodSpec tries to add a toleration to the toleration list in PodSpec.
// Returns true if something was updated, false otherwise.
func addOrUpdateTolerationInPodSpec(spec *corev1.PodSpec, toleration *corev1.Toleration) bool {
	podTolerations := spec.Tolerations

	var newTolerations []corev1.Toleration
	updated := false
	for i := range podTolerations {
		if toleration.MatchToleration(&podTolerations[i]) {
			if (toleration.TolerationSeconds == nil && podTolerations[i].TolerationSeconds == nil) ||
				(toleration.TolerationSeconds != nil && podTolerations[i].TolerationSeconds != nil &&
					(*toleration.TolerationSeconds == *podTolerations[i].TolerationSeconds)) {
				return false
			}

			newTolerations = append(newTolerations, *toleration)
			updated = true
			continue
		}

		newTolerations = append(newTolerations, podTolerations[i])
	}

	if !updated {
		newTolerations = append(newTolerations, *toleration)
	}

	spec.Tolerations = newTolerations
	return true
}
