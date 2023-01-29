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
)

const (
	numWorkers = 5
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

	ldc *utils.LeaseDelegatedCounter
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
		ldc:             utils.NewLeaseDelegatedCounter(),
	}

	if informerFactory != nil {
		ctl.nodeInformer = informerFactory.Core().V1().Nodes()
		ctl.nodeSynced = ctl.nodeInformer.Informer().HasSynced
		ctl.nodeLister = ctl.nodeInformer.Lister()
		ctl.leaseInformer = informerFactory.Coordination().V1().Leases()
		ctl.leaseSynced = ctl.leaseInformer.Informer().HasSynced
		ctl.leaseLister = ctl.leaseInformer.Lister().Leases(corev1.NamespaceNodeLease)

		ctl.leaseInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc:    ctl.onLeaseCreate,
			UpdateFunc: ctl.onLeaseUpdate,
			DeleteFunc: nil,
		})
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
		}
	} else {
		if c.ldc.Counter(nl.Name) >= constant.LeaseDelegationThreshold {
			c.deTaintNodeNotSchedulable(nl.Name)
		}
		c.ldc.Reset(nl.Name)
	}

	return nil
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

func (c *Controller) Run(stopCH <-chan struct{}) {
	if !cache.WaitForCacheSync(stopCH, c.nodeSynced, c.leaseSynced) {
		klog.Error("sync poolcoordinator controller timeout")
	}

	defer c.nodeUpdateQueue.ShutDown()

	klog.Info("start node taint workers")
	for i := 0; i < numWorkers; i++ {
		go wait.Until(c.nodeWorker, time.Second, stopCH)
	}

	<-stopCH
}
