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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewEndpointsAdapter(kubeClient kubernetes.Interface, client client.Client) Adapter {
	return &endpoints{
		kubeClient: kubeClient,
		client:     client,
	}
}

type endpoints struct {
	kubeClient kubernetes.Interface
	client     client.Client
}

func (s *endpoints) GetEnqueueKeysBySvc(svc *corev1.Service) []string {
	var keys []string
	return appendKeys(keys, svc)
}

func (s *endpoints) GetEnqueueKeysByNodePool(svcTopologyTypes map[string]string, allNpNodes sets.String) []string {
	var keys []string
	endpointsList := &corev1.EndpointsList{}
	if err := s.client.List(context.TODO(), endpointsList, &client.ListOptions{LabelSelector: labels.Everything()}); err != nil {
		klog.V(4).Infof("Error listing endpoints sets: %v", err)
		return keys
	}

	for _, ep := range endpointsList.Items {
		if !isNodePoolTypeSvc(ep.Namespace, ep.Name, svcTopologyTypes) {
			continue
		}

		if s.getNodesInEp(&ep).Intersection(allNpNodes).Len() == 0 {
			continue
		}
		keys = appendKeys(keys, &ep)
	}
	return keys
}

func (s *endpoints) getNodesInEp(ep *corev1.Endpoints) sets.String {
	nodes := sets.NewString()
	for _, subset := range ep.Subsets {
		for _, addr := range subset.Addresses {
			if addr.NodeName != nil {
				nodes.Insert(*addr.NodeName)
			}
		}
	}
	return nodes
}

func (s *endpoints) UpdateTriggerAnnotations(namespace, name string) error {
	patch := getUpdateTriggerPatch()
	_, err := s.kubeClient.CoreV1().Endpoints(namespace).Patch(context.Background(), name, types.StrategicMergePatchType, patch, metav1.PatchOptions{})
	return err
}
