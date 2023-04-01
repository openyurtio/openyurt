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

package delegatelease

import (
	"context"
	"fmt"
	"time"

	coordv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	informercoordv1 "k8s.io/client-go/informers/coordination/v1"
	v1 "k8s.io/client-go/informers/core/v1"
	client "k8s.io/client-go/kubernetes"
	leaselisterv1 "k8s.io/client-go/listers/coordination/v1"
	listerv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/controller/poolcoordinator/constant"
	"github.com/openyurtio/openyurt/pkg/controller/poolcoordinator/utils"
	nodeutil "github.com/openyurtio/openyurt/pkg/controller/util/node"
)

const (
	numWorkers       = 5
	nodeNameKeyIndex = "spec.nodeName"
)

type Controller struct {
	client          client.Interface
	nodeInformer    v1.NodeInformer
	nodeSynced      cache.InformerSynced
	nodeLister      listerv1.NodeLister
	leaseInformer   informercoordv1.LeaseInformer
	leaseSynced     cache.InformerSynced
	leaseLister     leaselisterv1.LeaseNamespaceLister
	nodeUpdateQueue workqueue.Interface

	podInformer           v1.PodInformer
	podUpdateQueue        workqueue.RateLimitingInterface
	getPodsAssignedToNode func(nodeName string) ([]*corev1.Pod, error)
	podLister             listerv1.PodLister
	podInformerSynced     cache.InformerSynced

	ldc    *utils.LeaseDelegatedCounter
	delLdc *utils.LeaseDelegatedCounter
}

func (c *Controller) onLeaseCreate(n interface{}) {
	nl := n.(*coordv1.Lease)
	if nl.Namespace != corev1.NamespaceNodeLease {
		return
	}

	key, err := cache.MetaNamespaceKeyFunc(n)
	if err == nil {
		c.nodeUpdateQueue.Add(key)
	}
}

func (c *Controller) onLeaseUpdate(o interface{}, n interface{}) {
	nl := n.(*coordv1.Lease)
	if nl.Namespace != corev1.NamespaceNodeLease {
		return
	}

	key, err := cache.MetaNamespaceKeyFunc(n)
	if err == nil {
		c.nodeUpdateQueue.Add(key)
	}
}

func NewController(kc client.Interface, informerFactory informers.SharedInformerFactory) *Controller {
	ctl := &Controller{
		client:          kc,
		nodeUpdateQueue: workqueue.NewNamed("poolcoordinator_node"),
		podUpdateQueue:  workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "poolcoordinator_pods"),
		ldc:             utils.NewLeaseDelegatedCounter(),
		delLdc:          utils.NewLeaseDelegatedCounter(),
	}

	if informerFactory != nil {
		ctl.nodeInformer = informerFactory.Core().V1().Nodes()
		ctl.nodeSynced = ctl.nodeInformer.Informer().HasSynced
		ctl.nodeLister = ctl.nodeInformer.Lister()
		ctl.leaseInformer = informerFactory.Coordination().V1().Leases()
		ctl.leaseSynced = ctl.leaseInformer.Informer().HasSynced
		ctl.leaseLister = ctl.leaseInformer.Lister().Leases(corev1.NamespaceNodeLease)
		ctl.podInformer = informerFactory.Core().V1().Pods()
		ctl.podInformerSynced = ctl.podInformer.Informer().HasSynced
		ctl.podLister = ctl.podInformer.Lister()

		ctl.leaseInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc:    ctl.onLeaseCreate,
			UpdateFunc: ctl.onLeaseUpdate,
			DeleteFunc: nil,
		})

		ctl.podInformer.Informer().AddIndexers(cache.Indexers{
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
		podIndexer := ctl.podInformer.Informer().GetIndexer()
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
	}

	return ctl
}

func (c *Controller) taintNodeNotSchedulable(name string) {
	node, err := c.nodeLister.Get(name)
	if err != nil {
		klog.Error(err)
		return
	}
	c.doTaintNodeNotSchedulable(node)
}

func (c *Controller) doTaintNodeNotSchedulable(node *corev1.Node) *corev1.Node {
	taints := node.Spec.Taints
	if utils.TaintKeyExists(taints, constant.NodeNotSchedulableTaint) {
		klog.Infof("taint %s: key %s already exists, nothing to do\n", node.Name, constant.NodeNotSchedulableTaint)
		return node
	}
	nn := node.DeepCopy()
	t := corev1.Taint{
		Key:    constant.NodeNotSchedulableTaint,
		Value:  "true",
		Effect: corev1.TaintEffectNoSchedule,
	}
	nn.Spec.Taints = append(nn.Spec.Taints, t)
	var err error
	if c.client != nil {
		nn, err = c.client.CoreV1().Nodes().Update(context.TODO(), nn, metav1.UpdateOptions{})
		if err != nil {
			klog.Error(err)
		}
	}
	return nn
}

func (c *Controller) deTaintNodeNotSchedulable(name string) {
	node, err := c.nodeLister.Get(name)
	if err != nil {
		klog.Error(err)
		return
	}
	c.doDeTaintNodeNotSchedulable(node)
}

func (c *Controller) doDeTaintNodeNotSchedulable(node *corev1.Node) *corev1.Node {
	taints := node.Spec.Taints
	taints, deleted := utils.DeleteTaintsByKey(taints, constant.NodeNotSchedulableTaint)
	if !deleted {
		c.delLdc.Inc(node.Name)
		klog.Infof("detaint %s: no key %s exists, nothing to do\n", node.Name, constant.NodeNotSchedulableTaint)
		return node
	}
	nn := node.DeepCopy()
	nn.Spec.Taints = taints
	var err error
	if c.client != nil {
		nn, err = c.client.CoreV1().Nodes().Update(context.TODO(), nn, metav1.UpdateOptions{})
		if err != nil {
			klog.Error(err)
		} else {
			c.delLdc.Inc(node.Name)
		}
	}
	return nn
}

func (c *Controller) syncHandler(key string) error {
	_, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return fmt.Errorf("invalid resource key: %s", key)
	}

	nl, err := c.leaseLister.Get(name)
	if err != nil {
		klog.Errorf("couldn't get lease for %s, maybe it has been deleted\n", name)
		c.ldc.Del(name)
		return nil
	}

	nval, nok := nl.Annotations[constant.DelegateHeartBeat]

	if nok && nval == "true" {
		c.ldc.Inc(nl.Name)
		if c.ldc.Counter(nl.Name) >= constant.LeaseDelegationThreshold {
			c.taintNodeNotSchedulable(nl.Name)
			c.checkNodeReadyConditionAndSetIt(nl.Name)
			c.delLdc.Reset(nl.Name)
		}
	} else {
		if c.delLdc.Counter(nl.Name) == 0 {
			c.deTaintNodeNotSchedulable(nl.Name)
		}
		c.ldc.Reset(nl.Name)
	}

	return nil
}

func (c *Controller) syncHandlerForPod(key string) error {
	ns, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return fmt.Errorf("invalid resource key: %s", key)
	}

	po, err := c.podLister.Pods(ns).Get(name)
	if err != nil {
		klog.Errorf("couldn't get pod for %s, maybe it has been deleted\n", name)
		return nil
	}

	// Pod will be modified, so making copy is required.
	pod := po.DeepCopy()
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady {
			cond.Status = corev1.ConditionTrue
			if !nodeutil.UpdatePodCondition(&pod.Status, &cond) {
				break
			}
			klog.V(2).Infof("Updating ready status of pod %v to true", pod.Name)
			_, err := c.client.CoreV1().Pods(pod.Namespace).UpdateStatus(context.TODO(), pod, metav1.UpdateOptions{})
			if err != nil {
				if apierrors.IsNotFound(err) {
					// NotFound error means that pod was already deleted.
					// There is nothing left to do with this pod.
					continue
				}
				klog.Warningf("Failed to update status for pod %s/%s: %v", ns, name, err)
				return err
			}
			break
		}
	}
	klog.V(2).Infof("Update ready status of pod %v to true successful", pod.Name)
	return nil
}

func (c *Controller) handleErr(err error, event interface{}) {
	if err == nil {
		c.podUpdateQueue.Forget(event)
		return
	}

	if c.podUpdateQueue.NumRequeues(event) < 6 {
		klog.Infof("error syncing event %v: %v", event, err)
		c.podUpdateQueue.AddRateLimited(event)
		return
	}

	runtime.HandleError(err)
	klog.Infof("dropping event %q out of the queue: %v", event, err)
	c.podUpdateQueue.Forget(event)
}

func (c *Controller) nodeWorker() {
	for {
		key, shutdown := c.nodeUpdateQueue.Get()
		if shutdown {
			klog.Info("node work queue shutdown")
			return
		}

		if err := c.syncHandler(key.(string)); err != nil {
			runtime.HandleError(err)
		}

		c.nodeUpdateQueue.Done(key)
	}
}

func (c *Controller) doPodProcessingWorker() {
	for {
		key, shutdown := c.podUpdateQueue.Get()
		if shutdown {
			klog.Info("pod work queue shutdown")
			return
		}

		err := c.syncHandlerForPod(key.(string))
		c.handleErr(err, key)
	}
}

func (c *Controller) Run(stopCh <-chan struct{}) {
	if !cache.WaitForCacheSync(stopCh, c.nodeSynced, c.leaseSynced, c.podInformerSynced) {
		klog.Error("sync poolcoordinator controller timeout")
	}

	defer c.nodeUpdateQueue.ShutDown()
	defer c.podUpdateQueue.ShutDown()

	klog.Info("start node taint workers")
	for i := 0; i < numWorkers; i++ {
		go wait.Until(c.nodeWorker, time.Second, stopCh)
	}

	klog.Info("start pod ready condition workers")
	for i := 0; i < numWorkers; i++ {
		go wait.Until(c.doPodProcessingWorker, time.Second, stopCh)
	}

	<-stopCh
}

// If node lease was delegate, check node ready condition.
// If ready condition is unknown, update to true.
// Because when node ready condition is unknown, the native kubernetes will set node.kubernetes.io/unreachable taints in node,
// and the pod will be evict after 300s, that's not what we're trying to do in delegate lease.
// Up to now, it's only happen when leader in nodePool is disconnected with cloud, and this node will be not-ready,
// because in an election cycle, the node lease will not delegate to cloud, after 40s, the kubernetes will set unknown.
func (c *Controller) checkNodeReadyConditionAndSetIt(name string) {
	node, err := c.nodeLister.Get(name)
	if err != nil {
		klog.Error(err)
		return
	}

	// check node ready condition
	newNode := node.DeepCopy()
	_, currentCondition := nodeutil.GetNodeCondition(&newNode.Status, corev1.NodeReady)
	if currentCondition.Status != corev1.ConditionUnknown {
		// don't need to reset node ready condition
		return
	}

	// reset node ready condition as true
	currentCondition.Status = corev1.ConditionTrue
	currentCondition.Reason = "NodeDelegateLease"
	currentCondition.Message = "Node disconnect with ApiServer and lease delegate."
	currentCondition.LastTransitionTime = metav1.NewTime(time.Now())

	// update
	if _, err := c.client.CoreV1().Nodes().UpdateStatus(context.TODO(), newNode, metav1.UpdateOptions{}); err != nil {
		klog.Errorf("Error updating node %s: %v", newNode.Name, err)
		return
	}
	klog.Infof("successful set node %s ready condition with true", newNode.Name)

	// mark pods in node if pods ready condition is false
	if err = c.markPodsReady(newNode.Name); err != nil {
		klog.Errorf("Error mark pods in node %s ready condition: %v", newNode.Name, err)
		return
	}
}

// markPodsReady updates ready status of given pods running on given node from master
func (c *Controller) markPodsReady(nodeName string) error {
	pods, err := c.getPodsAssignedToNode(nodeName)
	if err != nil {
		return err
	}
	klog.V(2).Infof("Update ready status of pods on node [%v]", nodeName)

	for i := range pods {
		if pods[i].Spec.NodeName != nodeName {
			continue
		}

		// Only modify pod which Ready Condition is False.
		for _, cond := range pods[i].Status.Conditions {
			if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionFalse {
				key, err := cache.MetaNamespaceKeyFunc(pods[i])
				if err == nil {
					c.podUpdateQueue.Add(key)
				}
			}
		}
	}
	return nil
}
