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
package adapter

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes"
	discorveryV1listers "k8s.io/client-go/listers/discovery/v1"
	"k8s.io/klog/v2"
)

func NewEndpointsV1Adapter(client kubernetes.Interface, epSliceLister discorveryV1listers.EndpointSliceLister) Adapter {
	return &endpointslicev1{
		client:        client,
		epSliceLister: epSliceLister,
	}
}

type endpointslicev1 struct {
	client        kubernetes.Interface
	epSliceLister discorveryV1listers.EndpointSliceLister
}

func (s *endpointslicev1) GetEnqueueKeysBySvc(svc *corev1.Service) []string {
	var keys []string
	selector := getSvcSelector(discoveryv1.LabelServiceName, svc.Name)
	epSliceList, err := s.epSliceLister.EndpointSlices(svc.Namespace).List(selector)
	if err != nil {
		klog.V(4).Infof("Error listing endpointslices sets: %v", err)
		return keys
	}

	for _, epSlice := range epSliceList {
		keys = appendKeys(keys, epSlice)
	}
	return keys
}

func (s *endpointslicev1) GetEnqueueKeysByNodePool(svcTopologyTypes map[string]string, allNpNodes sets.String) []string {
	var keys []string
	epSliceList, err := s.epSliceLister.List(labels.Everything())
	if err != nil {
		klog.V(4).Infof("Error listing endpointslices sets: %v", err)
		return keys
	}

	for _, epSlice := range epSliceList {
		svcNamespace := epSlice.Namespace
		svcName := epSlice.Labels[discoveryv1.LabelServiceName]
		if !isNodePoolTypeSvc(svcNamespace, svcName, svcTopologyTypes) {
			continue
		}
		if s.getNodesInEpSlice(epSlice).Intersection(allNpNodes).Len() == 0 {
			continue
		}
		keys = appendKeys(keys, epSlice)
	}

	return keys
}

func (s *endpointslicev1) getNodesInEpSlice(epSlice *discoveryv1.EndpointSlice) sets.String {
	nodes := sets.NewString()
	for _, ep := range epSlice.Endpoints {
		if ep.NodeName != nil {
			nodes.Insert(*ep.NodeName)
		}
	}
	return nodes
}

func (s *endpointslicev1) UpdateTriggerAnnotations(namespace, name string) error {
	patch := getUpdateTriggerPatch()
	_, err := s.client.DiscoveryV1().EndpointSlices(namespace).Patch(context.Background(), name, types.StrategicMergePatchType, patch, metav1.PatchOptions{})
	return err
}
