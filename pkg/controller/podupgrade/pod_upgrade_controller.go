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
	"reflect"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
)

type Controller struct {
	kubeclientset client.Interface

	daemonsetLister appslisters.DaemonSetLister
	daemonsetSynced cache.InformerSynced
	nodeLister      corelisters.NodeLister
	nodeSynced      cache.InformerSynced
	podLister       corelisters.PodLister
	podSynced       cache.InformerSynced

	daemonsetWorkqueue workqueue.RateLimitingInterface
	nodeWorkqueue      workqueue.Interface
}

func NewController(kc client.Interface, daemonsetInformer appsinformers.DaemonSetInformer,
	nodeInformer coreinformers.NodeInformer, podInformer coreinformers.PodInformer) *Controller {
	// TODO: Is eventBroadCaster needed?
	ctrl := Controller{
		kubeclientset:   kc,
		daemonsetLister: daemonsetInformer.Lister(),
		daemonsetSynced: daemonsetInformer.Informer().HasSynced,

		nodeLister: nodeInformer.Lister(),
		nodeSynced: nodeInformer.Informer().HasSynced,

		podLister: podInformer.Lister(),
		podSynced: podInformer.Informer().HasSynced,

		daemonsetWorkqueue: workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter()),
		nodeWorkqueue:      workqueue.New(),
	}

	daemonsetInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		UpdateFunc: func(old, new interface{}) {
			newDS := new.(*appsv1.DaemonSet)
			oldDS := old.(*appsv1.DaemonSet)

			// Only control daemonset with annotation "apps.openyurt.io/upgrade-strategy"
			if !metav1.HasAnnotation(newDS.ObjectMeta, UpgradeAnnotation) || !metav1.HasAnnotation(oldDS.ObjectMeta, UpgradeAnnotation) {
				return
			}

			// TODO: change to compare generation and revision hash
			if newDS.ResourceVersion == oldDS.ResourceVersion || reflect.DeepEqual(newDS.Spec.Template.Spec, oldDS.Spec.Template.Spec) {
				return
			}

			ctrl.enqueueDaemonset(new)
		},
	})

	nodeInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		UpdateFunc: func(old, new interface{}) {
			oldNode := old.(*corev1.Node)
			newNode := new.(*corev1.Node)
			if !NodeReady(&oldNode.Status) && NodeReady(&newNode.Status) {
				klog.Infof("node %q turn to ready", newNode.Name)
				ctrl.nodeWorkqueue.Add(newNode.Name)
			}
		},
	})
	return &ctrl
}

func (c *Controller) enqueueDaemonset(obj interface{}) {
	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		utilruntime.HandleError(err)
		return
	}

	klog.Infof("Got udpate event: %v", key)
	c.daemonsetWorkqueue.AddRateLimited(key)
}

func (c *Controller) Run(threadiness int, stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()

	klog.Info("Starting pod upgrade controller")
	defer klog.Info("Shutting down pod upgrade controller")
	defer c.daemonsetWorkqueue.ShutDown()
	defer c.nodeWorkqueue.ShutDown()

	//synchronize the cache before starting to process events
	if !cache.WaitForCacheSync(stopCh, c.daemonsetSynced, c.nodeSynced,
		c.podSynced) {
		klog.Error("sync podupgrade controller timeout")
	}

	for i := 0; i < threadiness; i++ {
		go wait.Until(c.runDaemonsetWorker, time.Second, stopCh)
	}

	for i := 0; i < threadiness; i++ {
		go wait.Until(c.runNodeWorker, time.Second, stopCh)
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
		c.daemonsetWorkqueue.Forget(obj)
		c.daemonsetWorkqueue.Done(obj)
	}
}

func (c *Controller) runNodeWorker() {
	for {
		obj, shutdown := c.nodeWorkqueue.Get()
		if shutdown {
			return
		}

		nodeName := obj.(string)
		if err := c.syncNodeHandler(nodeName); err != nil {
			utilruntime.HandleError(err)
		}

		c.nodeWorkqueue.Done(nodeName)
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
			return nil
		}
		return err
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
		klog.Infof("OTA upgrade %v", ds.Name)
		if err := c.checkOTAUpgrade(ds, pods); err != nil {
			return err
		}

	case AutoUpgrade:
		klog.Infof("Auto upgrade %v", ds.Name)
		if err := c.autoUpgrade(ds, pods); err != nil {
			return err
		}
	default:
		// error
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

// autoUpgrade perform pod upgrade operation when
// 1. pod is upgradable (using IsDaemonsetPodLatest to check)
// 2. pod node is ready
func (c *Controller) autoUpgrade(ds *appsv1.DaemonSet, pods []*corev1.Pod) error {
	for _, pod := range pods {
		latestOK, err := IsDaemonsetPodLatest(ds, pod)
		if err != nil {
			return err
		}

		nodeOK, err := NodeReadyByName(c.nodeLister, pod.Spec.NodeName)
		if err != nil {
			return err
		}

		if !latestOK && nodeOK {
			if err := c.kubeclientset.CoreV1().Pods(pod.Namespace).
				Delete(context.TODO(), pod.Name, metav1.DeleteOptions{}); err != nil {
				return err
			}
			klog.Infof("Auto upgrade pod %v/%v", ds.Name, pod.Name)
		}
	}

	return nil
}

// syncNodeHandler delete the pod of daemonset that needs to be upgrade when node turns ready
func (c *Controller) syncNodeHandler(key string) error {
	defer func() {
		klog.V(4).Infof("Finish syncing pod upgrade request %q", key)
	}()

	klog.V(4).Infof("Start handler pod upgrade request %q", key)

	node, err := c.nodeLister.Get(key)
	if err != nil {
		// If node not found, just ignore it.
		if apierrors.IsNotFound(err) {
			klog.V(5).Infof("Sync node %v not found", key)
			return nil
		}
		return err
	}

	// get all pod in this node
	pods, err := GetNodePods(c.podLister, node)
	if err != nil {
		return err
	}

	if err := c.upgradePodsWhenNodeReady(c.kubeclientset, pods); err != nil {
		return err
	}

	return nil
}

// upgradePodsWhenNodeReady check pods in current node need to be upgraded
// Only workloads with annotation "apps.openyurt.io/upgrade-strategy" and updateStrategy "OnDelete" will be processed
func (c *Controller) upgradePodsWhenNodeReady(clientset client.Interface, pods []*corev1.Pod) error {
	for _, pod := range pods {
		owner := metav1.GetControllerOf(pod)
		switch owner.Kind {
		case DaemonSet:
			ds, err := clientset.AppsV1().DaemonSets(pod.Namespace).Get(context.TODO(), owner.Name, metav1.GetOptions{})
			if err != nil {
				return err
			}

			// consider only the case with annotation "apps.openyurt.io/upgrade-strategy"
			v, ok := ds.Annotations[UpgradeAnnotation]
			// consider only the case with updateStrategy "OnDelete"
			updateStrategyOK := (ds.Spec.UpdateStrategy.Type == appsv1.OnDeleteDaemonSetStrategyType)
			if ok && updateStrategyOK {
				switch v {
				// set pod annotation when "apps.openyurt.io/upgrade-strategy=ota"
				case OTAUpgrade:
					if err := SetPodUpgradeAnnotation(clientset, ds, pod); err != nil {
						return err
					}

				// auto upgrade pod when "apps.openyurt.io/upgrade-strategy=auto"
				case AutoUpgrade:
					latestOK, err := IsDaemonsetPodLatest(ds, pod)
					if err != nil {
						return err
					}
					if !latestOK {
						if err := clientset.CoreV1().Pods(pod.Namespace).
							Delete(context.TODO(), pod.Name, metav1.DeleteOptions{}); err != nil {
							return err
						}
						klog.Infof("Auto upgrade pod %v/%v", ds.Name, pod.Name)
					}
				}
			}
		default:
			continue
		}
	}
	return nil
}
