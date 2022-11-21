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
package servicetopology

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/controller/servicetopology/adapter"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/servicetopology"
	nodepoolv1alpha1 "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/apis/apps/v1alpha1"
	yurtinformers "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/client/informers/externalversions"
)

const (
	endpointsWorkerSize     = 2
	endpointsliceWorkerSize = 4
)

type ServiceTopologyController struct {
	endpointsQueue         workqueue.RateLimitingInterface
	endpointsliceQueue     workqueue.RateLimitingInterface
	serviceInformerSynced  cache.InformerSynced
	nodepoolInformerSynced cache.InformerSynced
	endpointsAdapter       adapter.Adapter
	endpointsliceAdapter   adapter.Adapter
	svcLister              corelisters.ServiceLister
}

func NewServiceTopologyController(client kubernetes.Interface,
	sharedInformers informers.SharedInformerFactory,
	yurtInformers yurtinformers.SharedInformerFactory) (*ServiceTopologyController, error) {
	epSliceAdapter, err := getEndpointSliceAdapter(client, sharedInformers)
	if err != nil {
		return nil, fmt.Errorf("get endpointslice adapter with error: %v", err)
	}
	endpointsLister := sharedInformers.Core().V1().Endpoints().Lister()
	epAdapter := adapter.NewEndpointsAdapter(client, endpointsLister)

	epQueue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	epSliceQueue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

	serviceInformer := sharedInformers.Core().V1().Services()
	nodepoolInformer := yurtInformers.Apps().V1alpha1().NodePools()
	sc := &ServiceTopologyController{
		endpointsQueue:         epQueue,
		endpointsliceQueue:     epSliceQueue,
		serviceInformerSynced:  serviceInformer.Informer().HasSynced,
		nodepoolInformerSynced: nodepoolInformer.Informer().HasSynced,
		svcLister:              serviceInformer.Lister(),
		endpointsAdapter:       epAdapter,
		endpointsliceAdapter:   epSliceAdapter,
	}
	serviceInformer.Informer().AddEventHandler(sc.getServiceHandler())
	nodepoolInformer.Informer().AddEventHandler(sc.getNodepoolHandler())
	return sc, nil
}

func (s *ServiceTopologyController) Run(stopCh <-chan struct{}) {
	defer runtime.HandleCrash()
	defer s.endpointsQueue.ShutDown()
	defer s.endpointsliceQueue.ShutDown()

	klog.Info("starting the service topology controller")
	defer klog.Info("stopping the service topology controller")
	if !cache.WaitForCacheSync(stopCh, s.serviceInformerSynced, s.nodepoolInformerSynced) {
		klog.Error("sync service topology controller timeout")
		return
	}
	klog.Info("sync service topology controller succeed")

	for i := 0; i < endpointsWorkerSize; i++ {
		go wait.Until(s.runEndpointsWorker, time.Second, stopCh)
	}
	for i := 0; i < endpointsliceWorkerSize; i++ {
		go wait.Until(s.runEndpointsliceWorker, time.Second, stopCh)
	}
	<-stopCh
}

func (s *ServiceTopologyController) runEndpointsWorker() {
	for s.processNextEndpoints() {
	}
}

func (s *ServiceTopologyController) runEndpointsliceWorker() {
	for s.processNextEndpointslice() {
	}
}

func (s *ServiceTopologyController) getServiceHandler() cache.ResourceEventHandlerFuncs {
	return cache.ResourceEventHandlerFuncs{
		UpdateFunc: func(old, new interface{}) {
			oldSvc, ok := old.(*corev1.Service)
			if !ok {
				return
			}
			newSvc, ok := new.(*corev1.Service)
			if !ok {
				return
			}
			if s.isServiceTopologyTypeChanged(oldSvc, newSvc) {
				s.enqueueEndpointsForSvc(newSvc)
				s.enqueueEndpointsilceForSvc(newSvc)
			}
		},
	}
}

func (s *ServiceTopologyController) getNodepoolHandler() cache.ResourceEventHandlerFuncs {
	return cache.ResourceEventHandlerFuncs{
		DeleteFunc: func(obj interface{}) {
			nodePool, ok := obj.(*nodepoolv1alpha1.NodePool)
			if !ok {
				return
			}
			klog.Infof("nodepool %s is deleted", nodePool.Name)
			allNpNodes := sets.NewString(nodePool.Status.Nodes...)
			svcTopologyTypes := s.getSvcTopologyTypes()
			s.enqueueEndpointsForNodePool(svcTopologyTypes, allNpNodes, nodePool)
			s.enqueueEndpointsliceForNodePool(svcTopologyTypes, allNpNodes, nodePool)
		},
		UpdateFunc: func(old, new interface{}) {
			oldNp, ok := old.(*nodepoolv1alpha1.NodePool)
			if !ok {
				return
			}
			newNp, ok := new.(*nodepoolv1alpha1.NodePool)
			if !ok {
				return
			}
			newNpNodes := sets.NewString(newNp.Status.Nodes...)
			oldNpNodes := sets.NewString(oldNp.Status.Nodes...)
			if newNpNodes.Equal(oldNpNodes) {
				return
			}
			klog.Infof("the nodes record of nodepool %s is changed from %v to %v.", newNp.Name, oldNp.Status.Nodes, newNp.Status.Nodes)
			allNpNodes := newNpNodes.Union(oldNpNodes)
			svcTopologyTypes := s.getSvcTopologyTypes()
			s.enqueueEndpointsForNodePool(svcTopologyTypes, allNpNodes, newNp)
			s.enqueueEndpointsliceForNodePool(svcTopologyTypes, allNpNodes, newNp)
		},
	}
}

func (s *ServiceTopologyController) enqueueEndpointsForSvc(newSvc *corev1.Service) {
	keys := s.endpointsAdapter.GetEnqueueKeysBySvc(newSvc)
	klog.Infof("the topology configuration of svc %s/%s is changed, enqueue endpoints: %v", newSvc.Namespace, newSvc.Name, keys)
	for _, key := range keys {
		s.endpointsQueue.AddRateLimited(key)
	}
}

func (s *ServiceTopologyController) enqueueEndpointsilceForSvc(newSvc *corev1.Service) {
	keys := s.endpointsliceAdapter.GetEnqueueKeysBySvc(newSvc)
	klog.Infof("the topology configuration of svc %s/%s is changed, enqueue endpointslices: %v", newSvc.Namespace, newSvc.Name, keys)
	for _, key := range keys {
		s.endpointsliceQueue.AddRateLimited(key)
	}
}

func (s *ServiceTopologyController) enqueueEndpointsForNodePool(svcTopologyTypes map[string]string, allNpNodes sets.String, np *nodepoolv1alpha1.NodePool) {
	keys := s.endpointsAdapter.GetEnqueueKeysByNodePool(svcTopologyTypes, allNpNodes)
	klog.Infof("according to the change of the nodepool %s, enqueue endpoints: %v", np.Name, keys)
	for _, key := range keys {
		s.endpointsQueue.AddRateLimited(key)
	}
}

func (s *ServiceTopologyController) enqueueEndpointsliceForNodePool(svcTopologyTypes map[string]string, allNpNodes sets.String, np *nodepoolv1alpha1.NodePool) {
	keys := s.endpointsliceAdapter.GetEnqueueKeysByNodePool(svcTopologyTypes, allNpNodes)
	klog.Infof("according to the change of the nodepool %s, enqueue endpointslice: %v", np.Name, keys)
	for _, key := range keys {
		s.endpointsliceQueue.AddRateLimited(key)
	}
}

func (s *ServiceTopologyController) isServiceTopologyTypeChanged(oldSvc, newSvc *corev1.Service) bool {
	oldType := oldSvc.Annotations[servicetopology.AnnotationServiceTopologyKey]
	newType := newSvc.Annotations[servicetopology.AnnotationServiceTopologyKey]
	if oldType == newType {
		return false
	}
	return true
}

func (s *ServiceTopologyController) getSvcTopologyTypes() map[string]string {
	svcTopologyTypes := make(map[string]string)
	svcList, err := s.svcLister.List(labels.Everything())
	if err != nil {
		klog.V(4).Infof("Error listing service sets: %v", err)
		return svcTopologyTypes
	}

	for _, svc := range svcList {
		topologyType, ok := svc.Annotations[servicetopology.AnnotationServiceTopologyKey]
		if !ok {
			continue
		}

		key, err := cache.MetaNamespaceKeyFunc(svc)
		if err != nil {
			runtime.HandleError(err)
			continue
		}
		svcTopologyTypes[key] = topologyType
	}
	return svcTopologyTypes
}

func (s *ServiceTopologyController) processNextEndpoints() bool {
	key, quit := s.endpointsQueue.Get()
	if quit {
		return false
	}
	defer s.endpointsQueue.Done(key)

	klog.V(4).Infof("sync endpoints %s", key)
	if err := s.syncEndpoints(key.(string)); err != nil {
		s.endpointsQueue.AddRateLimited(key)
		runtime.HandleError(fmt.Errorf("sync endpoints %v failed with : %v", key, err))
		return true
	}
	s.endpointsQueue.Forget(key)
	return true
}

func (s *ServiceTopologyController) syncEndpoints(key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return fmt.Errorf("split meta namespace key with error: %v", err)
	}

	return s.endpointsAdapter.UpdateTriggerAnnotations(namespace, name)
}

func (s *ServiceTopologyController) processNextEndpointslice() bool {
	key, quit := s.endpointsliceQueue.Get()
	if quit {
		return false
	}
	defer s.endpointsliceQueue.Done(key)

	klog.V(4).Infof("sync endpointslice %s", key)
	if err := s.syncEndpointslice(key.(string)); err != nil {
		s.endpointsliceQueue.AddRateLimited(key)
		runtime.HandleError(fmt.Errorf("sync endpointslice %v failed with : %v", key, err))
		return true
	}
	s.endpointsliceQueue.Forget(key)
	return true
}

func (s *ServiceTopologyController) syncEndpointslice(key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return fmt.Errorf("split meta namespace key with error: %v", err)
	}

	return s.endpointsliceAdapter.UpdateTriggerAnnotations(namespace, name)
}

func getEndpointSliceAdapter(client kubernetes.Interface,
	sharedInformers informers.SharedInformerFactory) (adapter.Adapter, error) {
	_, err := client.DiscoveryV1().EndpointSlices(corev1.NamespaceAll).List(context.TODO(), metav1.ListOptions{})

	if err == nil {
		klog.Infof("v1.EndpointSlice is supported.")
		epSliceLister := sharedInformers.Discovery().V1().EndpointSlices().Lister()
		return adapter.NewEndpointsV1Adapter(client, epSliceLister), nil
	}
	if errors.IsNotFound(err) {
		klog.Infof("fall back to v1beta1.EndpointSlice.")
		epSliceLister := sharedInformers.Discovery().V1beta1().EndpointSlices().Lister()
		return adapter.NewEndpointsV1Beta1Adapter(client, epSliceLister), nil
	}
	return nil, err
}
