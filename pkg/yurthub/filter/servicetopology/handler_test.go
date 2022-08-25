/*
Copyright 2020 The OpenYurt Authors.

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
	"reflect"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	discovery "k8s.io/api/discovery/v1"
	discoveryV1beta1 "k8s.io/api/discovery/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	nodepoolv1alpha1 "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/apis/apps/v1alpha1"
	yurtfake "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/client/clientset/versioned/fake"
	yurtinformers "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/client/informers/externalversions"
)

func TestServiceTopologyHandler(t *testing.T) {
	currentNodeName := "node1"
	nodeName2 := "node2"
	nodeName3 := "node3"

	testcases := map[string]struct {
		object       runtime.Object
		kubeClient   *k8sfake.Clientset
		yurtClient   *yurtfake.Clientset
		expectResult runtime.Object
	}{
		"v1beta1.EndpointSlice: topologyKeys is kubernetes.io/hostname": {
			object: &discoveryV1beta1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1-np7sf",
					Namespace: "default",
					Labels: map[string]string{
						discoveryV1beta1.LabelServiceName: "svc1",
					},
				},
				Endpoints: []discoveryV1beta1.Endpoint{
					{
						Addresses: []string{
							"10.244.1.2",
						},
						Topology: map[string]string{
							corev1.LabelHostname: currentNodeName,
						},
					},
					{
						Addresses: []string{
							"10.244.1.3",
						},
						Topology: map[string]string{
							corev1.LabelHostname: "node2",
						},
					},
					{
						Addresses: []string{
							"10.244.1.4",
						},
						Topology: map[string]string{
							corev1.LabelHostname: currentNodeName,
						},
					},
					{
						Addresses: []string{
							"10.244.1.5",
						},
						Topology: map[string]string{
							corev1.LabelHostname: "node3",
						},
					},
				},
			},
			kubeClient: k8sfake.NewSimpleClientset(
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: currentNodeName,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "hangzhou",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "hangzhou",
						},
					},
				},
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc1",
						Namespace: "default",
						Annotations: map[string]string{
							AnnotationServiceTopologyKey: AnnotationServiceTopologyValueNode,
						},
					},
				},
			),
			yurtClient: yurtfake.NewSimpleClientset(
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hangzhou",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							currentNodeName,
							"node3",
						},
					},
				},
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "shanghai",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							"node2",
						},
					},
				},
			),
			expectResult: &discoveryV1beta1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1-np7sf",
					Namespace: "default",
					Labels: map[string]string{
						discoveryV1beta1.LabelServiceName: "svc1",
					},
				},
				Endpoints: []discoveryV1beta1.Endpoint{
					{
						Addresses: []string{
							"10.244.1.2",
						},
						Topology: map[string]string{
							corev1.LabelHostname: currentNodeName,
						},
					},
					{
						Addresses: []string{
							"10.244.1.4",
						},
						Topology: map[string]string{
							corev1.LabelHostname: currentNodeName,
						},
					},
				},
			},
		},
		"v1beta1.EndpointSlice: topologyKeys is openyurt.io/nodepool": {
			object: &discoveryV1beta1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1-np7sf",
					Namespace: "default",
					Labels: map[string]string{
						discoveryV1beta1.LabelServiceName: "svc1",
					},
				},
				Endpoints: []discoveryV1beta1.Endpoint{
					{
						Addresses: []string{
							"10.244.1.2",
						},
						Topology: map[string]string{
							corev1.LabelHostname: currentNodeName,
						},
					},
					{
						Addresses: []string{
							"10.244.1.3",
						},
						Topology: map[string]string{
							corev1.LabelHostname: "node2",
						},
					},
					{
						Addresses: []string{
							"10.244.1.4",
						},
						Topology: map[string]string{
							corev1.LabelHostname: currentNodeName,
						},
					},
					{
						Addresses: []string{
							"10.244.1.5",
						},
						Topology: map[string]string{
							corev1.LabelHostname: "node3",
						},
					},
				},
			},
			kubeClient: k8sfake.NewSimpleClientset(
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: currentNodeName,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "hangzhou",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "hangzhou",
						},
					},
				},
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc1",
						Namespace: "default",
						Annotations: map[string]string{
							AnnotationServiceTopologyKey: AnnotationServiceTopologyValueNodePool,
						},
					},
				},
			),
			yurtClient: yurtfake.NewSimpleClientset(
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hangzhou",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							currentNodeName,
							"node3",
						},
					},
				},
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "shanghai",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							"node2",
						},
					},
				},
			),
			expectResult: &discoveryV1beta1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1-np7sf",
					Namespace: "default",
					Labels: map[string]string{
						discoveryV1beta1.LabelServiceName: "svc1",
					},
				},
				Endpoints: []discoveryV1beta1.Endpoint{
					{
						Addresses: []string{
							"10.244.1.2",
						},
						Topology: map[string]string{
							corev1.LabelHostname: currentNodeName,
						},
					},
					{
						Addresses: []string{
							"10.244.1.4",
						},
						Topology: map[string]string{
							corev1.LabelHostname: currentNodeName,
						},
					},
					{
						Addresses: []string{
							"10.244.1.5",
						},
						Topology: map[string]string{
							corev1.LabelHostname: "node3",
						},
					},
				},
			},
		},
		"v1beta1.EndpointSlice: topologyKeys is kubernetes.io/zone": {
			object: &discoveryV1beta1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1-np7sf",
					Namespace: "default",
					Labels: map[string]string{
						discoveryV1beta1.LabelServiceName: "svc1",
					},
				},
				Endpoints: []discoveryV1beta1.Endpoint{
					{
						Addresses: []string{
							"10.244.1.2",
						},
						Topology: map[string]string{
							corev1.LabelHostname: currentNodeName,
						},
					},
					{
						Addresses: []string{
							"10.244.1.3",
						},
						Topology: map[string]string{
							corev1.LabelHostname: "node2",
						},
					},
					{
						Addresses: []string{
							"10.244.1.4",
						},
						Topology: map[string]string{
							corev1.LabelHostname: currentNodeName,
						},
					},
					{
						Addresses: []string{
							"10.244.1.5",
						},
						Topology: map[string]string{
							corev1.LabelHostname: "node3",
						},
					},
				},
			},
			kubeClient: k8sfake.NewSimpleClientset(
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: currentNodeName,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "hangzhou",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "hangzhou",
						},
					},
				},
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc1",
						Namespace: "default",
						Annotations: map[string]string{
							AnnotationServiceTopologyKey: AnnotationServiceTopologyValueZone,
						},
					},
				},
			),
			yurtClient: yurtfake.NewSimpleClientset(
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hangzhou",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							currentNodeName,
							"node3",
						},
					},
				},
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "shanghai",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							"node2",
						},
					},
				},
			),
			expectResult: &discoveryV1beta1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1-np7sf",
					Namespace: "default",
					Labels: map[string]string{
						discoveryV1beta1.LabelServiceName: "svc1",
					},
				},
				Endpoints: []discoveryV1beta1.Endpoint{
					{
						Addresses: []string{
							"10.244.1.2",
						},
						Topology: map[string]string{
							corev1.LabelHostname: currentNodeName,
						},
					},
					{
						Addresses: []string{
							"10.244.1.4",
						},
						Topology: map[string]string{
							corev1.LabelHostname: currentNodeName,
						},
					},
					{
						Addresses: []string{
							"10.244.1.5",
						},
						Topology: map[string]string{
							corev1.LabelHostname: "node3",
						},
					},
				},
			},
		},
		"v1beta1.EndpointSlice: without openyurt.io/topologyKeys": {
			object: &discoveryV1beta1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1-np7sf",
					Namespace: "default",
					Labels: map[string]string{
						discoveryV1beta1.LabelServiceName: "svc1",
					},
				},
				Endpoints: []discoveryV1beta1.Endpoint{
					{
						Addresses: []string{
							"10.244.1.2",
						},
						Topology: map[string]string{
							corev1.LabelHostname: currentNodeName,
						},
					},
					{
						Addresses: []string{
							"10.244.1.3",
						},
						Topology: map[string]string{
							corev1.LabelHostname: "node2",
						},
					},
					{
						Addresses: []string{
							"10.244.1.4",
						},
						Topology: map[string]string{
							corev1.LabelHostname: currentNodeName,
						},
					},
					{
						Addresses: []string{
							"10.244.1.5",
						},
						Topology: map[string]string{
							corev1.LabelHostname: "node3",
						},
					},
				},
			},
			kubeClient: k8sfake.NewSimpleClientset(
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: currentNodeName,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "hangzhou",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "hangzhou",
						},
					},
				},
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "svc1",
						Namespace:   "default",
						Annotations: map[string]string{},
					},
				},
			),
			yurtClient: yurtfake.NewSimpleClientset(
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hangzhou",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							currentNodeName,
							"node3",
						},
					},
				},
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "shanghai",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							"node2",
						},
					},
				},
			),
			expectResult: &discoveryV1beta1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1-np7sf",
					Namespace: "default",
					Labels: map[string]string{
						discoveryV1beta1.LabelServiceName: "svc1",
					},
				},
				Endpoints: []discoveryV1beta1.Endpoint{
					{
						Addresses: []string{
							"10.244.1.2",
						},
						Topology: map[string]string{
							corev1.LabelHostname: currentNodeName,
						},
					},
					{
						Addresses: []string{
							"10.244.1.3",
						},
						Topology: map[string]string{
							corev1.LabelHostname: "node2",
						},
					},
					{
						Addresses: []string{
							"10.244.1.4",
						},
						Topology: map[string]string{
							corev1.LabelHostname: currentNodeName,
						},
					},
					{
						Addresses: []string{
							"10.244.1.5",
						},
						Topology: map[string]string{
							corev1.LabelHostname: "node3",
						},
					},
				},
			},
		},
		"v1beta1.EndpointSlice: currentNode is not in any nodepool": {
			object: &discoveryV1beta1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1-np7sf",
					Namespace: "default",
					Labels: map[string]string{
						discoveryV1beta1.LabelServiceName: "svc1",
					},
				},
				Endpoints: []discoveryV1beta1.Endpoint{
					{
						Addresses: []string{
							"10.244.1.2",
						},
						Topology: map[string]string{
							corev1.LabelHostname: currentNodeName,
						},
					},
					{
						Addresses: []string{
							"10.244.1.3",
						},
						Topology: map[string]string{
							corev1.LabelHostname: "node2",
						},
					},
					{
						Addresses: []string{
							"10.244.1.4",
						},
						Topology: map[string]string{
							corev1.LabelHostname: currentNodeName,
						},
					},
					{
						Addresses: []string{
							"10.244.1.5",
						},
						Topology: map[string]string{
							corev1.LabelHostname: "node3",
						},
					},
				},
			},
			kubeClient: k8sfake.NewSimpleClientset(
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name:   currentNodeName,
						Labels: map[string]string{},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "hangzhou",
						},
					},
				},
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc1",
						Namespace: "default",
						Annotations: map[string]string{
							AnnotationServiceTopologyKey: AnnotationServiceTopologyValueNodePool,
						},
					},
				},
			),
			yurtClient: yurtfake.NewSimpleClientset(
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hangzhou",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							"node3",
						},
					},
				},
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "shanghai",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							"node2",
						},
					},
				},
			),
			expectResult: &discoveryV1beta1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1-np7sf",
					Namespace: "default",
					Labels: map[string]string{
						discoveryV1beta1.LabelServiceName: "svc1",
					},
				},
				Endpoints: []discoveryV1beta1.Endpoint{
					{
						Addresses: []string{
							"10.244.1.2",
						},
						Topology: map[string]string{
							corev1.LabelHostname: currentNodeName,
						},
					},
					{
						Addresses: []string{
							"10.244.1.4",
						},
						Topology: map[string]string{
							corev1.LabelHostname: currentNodeName,
						},
					},
				},
			},
		},
		"v1beta1.EndpointSlice: currentNode has no endpoints on node": {
			object: &discoveryV1beta1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1-np7sf",
					Namespace: "default",
					Labels: map[string]string{
						discoveryV1beta1.LabelServiceName: "svc1",
					},
				},
				Endpoints: []discoveryV1beta1.Endpoint{
					{
						Addresses: []string{
							"10.244.1.2",
						},
						Topology: map[string]string{
							corev1.LabelHostname: nodeName2,
						},
					},
					{
						Addresses: []string{
							"10.244.1.3",
						},
						Topology: map[string]string{
							corev1.LabelHostname: "node2",
						},
					},
					{
						Addresses: []string{
							"10.244.1.4",
						},
						Topology: map[string]string{
							corev1.LabelHostname: nodeName3,
						},
					},
					{
						Addresses: []string{
							"10.244.1.5",
						},
						Topology: map[string]string{
							corev1.LabelHostname: "node3",
						},
					},
				},
			},
			kubeClient: k8sfake.NewSimpleClientset(
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: currentNodeName,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "hangzhou",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "hangzhou",
						},
					},
				},
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc1",
						Namespace: "default",
						Annotations: map[string]string{
							AnnotationServiceTopologyKey: AnnotationServiceTopologyValueNode,
						},
					},
				},
			),
			yurtClient: yurtfake.NewSimpleClientset(
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hangzhou",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							"node2",
							"node3",
						},
					},
				},
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "shanghai",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							currentNodeName,
						},
					},
				},
			),
			expectResult: nil,
		},
		"v1beta1.EndpointSlice: currentNode has no endpoints in nodepool": {
			object: &discoveryV1beta1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1-np7sf",
					Namespace: "default",
					Labels: map[string]string{
						discoveryV1beta1.LabelServiceName: "svc1",
					},
				},
				Endpoints: []discoveryV1beta1.Endpoint{
					{
						Addresses: []string{
							"10.244.1.2",
						},
						Topology: map[string]string{
							corev1.LabelHostname: nodeName2,
						},
					},
					{
						Addresses: []string{
							"10.244.1.3",
						},
						Topology: map[string]string{
							corev1.LabelHostname: "node2",
						},
					},
					{
						Addresses: []string{
							"10.244.1.4",
						},
						Topology: map[string]string{
							corev1.LabelHostname: nodeName3,
						},
					},
					{
						Addresses: []string{
							"10.244.1.5",
						},
						Topology: map[string]string{
							corev1.LabelHostname: "node3",
						},
					},
				},
			},
			kubeClient: k8sfake.NewSimpleClientset(
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: currentNodeName,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "hangzhou",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "hangzhou",
						},
					},
				},
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc1",
						Namespace: "default",
						Annotations: map[string]string{
							AnnotationServiceTopologyKey: AnnotationServiceTopologyValueNodePool,
						},
					},
				},
			),
			yurtClient: yurtfake.NewSimpleClientset(
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hangzhou",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							"node2",
							"node3",
						},
					},
				},
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "shanghai",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							currentNodeName,
						},
					},
				},
			),
			expectResult: nil,
		},
		"v1.EndpointSlice: topologyKeys is kubernetes.io/hostname": {
			object: &discovery.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1-np7sf",
					Namespace: "default",
					Labels: map[string]string{
						discoveryV1beta1.LabelServiceName: "svc1",
					},
				},
				Endpoints: []discovery.Endpoint{
					{
						Addresses: []string{
							"10.244.1.2",
						},
						NodeName: &currentNodeName,
					},
					{
						Addresses: []string{
							"10.244.1.3",
						},
						NodeName: &nodeName2,
					},
					{
						Addresses: []string{
							"10.244.1.4",
						},
						NodeName: &currentNodeName,
					},
					{
						Addresses: []string{
							"10.244.1.5",
						},
						NodeName: &nodeName3,
					},
				},
			},
			kubeClient: k8sfake.NewSimpleClientset(
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: currentNodeName,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "hangzhou",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "hangzhou",
						},
					},
				},
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc1",
						Namespace: "default",
						Annotations: map[string]string{
							AnnotationServiceTopologyKey: AnnotationServiceTopologyValueNode,
						},
					},
				},
			),
			yurtClient: yurtfake.NewSimpleClientset(
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hangzhou",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							currentNodeName,
							"node3",
						},
					},
				},
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "shanghai",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							"node2",
						},
					},
				},
			),
			expectResult: &discovery.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1-np7sf",
					Namespace: "default",
					Labels: map[string]string{
						discoveryV1beta1.LabelServiceName: "svc1",
					},
				},
				Endpoints: []discovery.Endpoint{
					{
						Addresses: []string{
							"10.244.1.2",
						},
						NodeName: &currentNodeName,
					},
					{
						Addresses: []string{
							"10.244.1.4",
						},
						NodeName: &currentNodeName,
					},
				},
			},
		},
		"v1.EndpointSlice: topologyKeys is openyurt.io/nodepool": {
			object: &discovery.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1-np7sf",
					Namespace: "default",
					Labels: map[string]string{
						discoveryV1beta1.LabelServiceName: "svc1",
					},
				},
				Endpoints: []discovery.Endpoint{
					{
						Addresses: []string{
							"10.244.1.2",
						},
						NodeName: &currentNodeName,
					},
					{
						Addresses: []string{
							"10.244.1.3",
						},
						NodeName: &nodeName2,
					},
					{
						Addresses: []string{
							"10.244.1.4",
						},
						NodeName: &currentNodeName,
					},
					{
						Addresses: []string{
							"10.244.1.5",
						},
						NodeName: &nodeName3,
					},
				},
			},
			kubeClient: k8sfake.NewSimpleClientset(
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: currentNodeName,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "hangzhou",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "hangzhou",
						},
					},
				},
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc1",
						Namespace: "default",
						Annotations: map[string]string{
							AnnotationServiceTopologyKey: AnnotationServiceTopologyValueNodePool,
						},
					},
				},
			),
			yurtClient: yurtfake.NewSimpleClientset(
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hangzhou",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							currentNodeName,
							"node3",
						},
					},
				},
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "shanghai",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							"node2",
						},
					},
				},
			),
			expectResult: &discovery.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1-np7sf",
					Namespace: "default",
					Labels: map[string]string{
						discoveryV1beta1.LabelServiceName: "svc1",
					},
				},
				Endpoints: []discovery.Endpoint{
					{
						Addresses: []string{
							"10.244.1.2",
						},
						NodeName: &currentNodeName,
					},
					{
						Addresses: []string{
							"10.244.1.4",
						},
						NodeName: &currentNodeName,
					},
					{
						Addresses: []string{
							"10.244.1.5",
						},
						NodeName: &nodeName3,
					},
				},
			},
		},
		"v1.EndpointSlice: topologyKeys is kubernetes.io/zone": {
			object: &discovery.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1-np7sf",
					Namespace: "default",
					Labels: map[string]string{
						discoveryV1beta1.LabelServiceName: "svc1",
					},
				},
				Endpoints: []discovery.Endpoint{
					{
						Addresses: []string{
							"10.244.1.2",
						},
						NodeName: &currentNodeName,
					},
					{
						Addresses: []string{
							"10.244.1.3",
						},
						NodeName: &nodeName2,
					},
					{
						Addresses: []string{
							"10.244.1.4",
						},
						NodeName: &currentNodeName,
					},
					{
						Addresses: []string{
							"10.244.1.5",
						},
						NodeName: &nodeName3,
					},
				},
			},
			kubeClient: k8sfake.NewSimpleClientset(
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: currentNodeName,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "hangzhou",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "hangzhou",
						},
					},
				},
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc1",
						Namespace: "default",
						Annotations: map[string]string{
							AnnotationServiceTopologyKey: AnnotationServiceTopologyValueZone,
						},
					},
				},
			),
			yurtClient: yurtfake.NewSimpleClientset(
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hangzhou",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							currentNodeName,
							"node3",
						},
					},
				},
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "shanghai",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							"node2",
						},
					},
				},
			),
			expectResult: &discovery.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1-np7sf",
					Namespace: "default",
					Labels: map[string]string{
						discoveryV1beta1.LabelServiceName: "svc1",
					},
				},
				Endpoints: []discovery.Endpoint{
					{
						Addresses: []string{
							"10.244.1.2",
						},
						NodeName: &currentNodeName,
					},
					{
						Addresses: []string{
							"10.244.1.4",
						},
						NodeName: &currentNodeName,
					},
					{
						Addresses: []string{
							"10.244.1.5",
						},
						NodeName: &nodeName3,
					},
				},
			},
		},
		"v1.EndpointSlice: without openyurt.io/topologyKeys": {
			object: &discovery.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1-np7sf",
					Namespace: "default",
					Labels: map[string]string{
						discoveryV1beta1.LabelServiceName: "svc1",
					},
				},
				Endpoints: []discovery.Endpoint{
					{
						Addresses: []string{
							"10.244.1.2",
						},
						NodeName: &currentNodeName,
					},
					{
						Addresses: []string{
							"10.244.1.3",
						},
						NodeName: &nodeName2,
					},
					{
						Addresses: []string{
							"10.244.1.4",
						},
						NodeName: &currentNodeName,
					},
					{
						Addresses: []string{
							"10.244.1.5",
						},
						NodeName: &nodeName3,
					},
				},
			},
			kubeClient: k8sfake.NewSimpleClientset(
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: currentNodeName,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "hangzhou",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "hangzhou",
						},
					},
				},
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "svc1",
						Namespace:   "default",
						Annotations: map[string]string{},
					},
				},
			),
			yurtClient: yurtfake.NewSimpleClientset(
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hangzhou",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							currentNodeName,
							"node3",
						},
					},
				},
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "shanghai",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							"node2",
						},
					},
				},
			),
			expectResult: &discovery.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1-np7sf",
					Namespace: "default",
					Labels: map[string]string{
						discoveryV1beta1.LabelServiceName: "svc1",
					},
				},
				Endpoints: []discovery.Endpoint{
					{
						Addresses: []string{
							"10.244.1.2",
						},
						NodeName: &currentNodeName,
					},
					{
						Addresses: []string{
							"10.244.1.3",
						},
						NodeName: &nodeName2,
					},
					{
						Addresses: []string{
							"10.244.1.4",
						},
						NodeName: &currentNodeName,
					},
					{
						Addresses: []string{
							"10.244.1.5",
						},
						NodeName: &nodeName3,
					},
				},
			},
		},
		"v1.EndpointSlice: currentNode is not in any nodepool": {
			object: &discovery.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1-np7sf",
					Namespace: "default",
					Labels: map[string]string{
						discoveryV1beta1.LabelServiceName: "svc1",
					},
				},
				Endpoints: []discovery.Endpoint{
					{
						Addresses: []string{
							"10.244.1.2",
						},
						NodeName: &currentNodeName,
					},
					{
						Addresses: []string{
							"10.244.1.3",
						},
						NodeName: &nodeName2,
					},
					{
						Addresses: []string{
							"10.244.1.4",
						},
						NodeName: &currentNodeName,
					},
					{
						Addresses: []string{
							"10.244.1.5",
						},
						NodeName: &nodeName3,
					},
				},
			},
			kubeClient: k8sfake.NewSimpleClientset(
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name:   currentNodeName,
						Labels: map[string]string{},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "hangzhou",
						},
					},
				},
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc1",
						Namespace: "default",
						Annotations: map[string]string{
							AnnotationServiceTopologyKey: AnnotationServiceTopologyValueNodePool,
						},
					},
				},
			),
			yurtClient: yurtfake.NewSimpleClientset(
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hangzhou",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							"node3",
						},
					},
				},
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "shanghai",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							"node2",
						},
					},
				},
			),
			expectResult: &discovery.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1-np7sf",
					Namespace: "default",
					Labels: map[string]string{
						discoveryV1beta1.LabelServiceName: "svc1",
					},
				},
				Endpoints: []discovery.Endpoint{
					{
						Addresses: []string{
							"10.244.1.2",
						},
						NodeName: &currentNodeName,
					},
					{
						Addresses: []string{
							"10.244.1.4",
						},
						NodeName: &currentNodeName,
					},
				},
			},
		},
		"v1.EndpointSlice: currentNode has no endpoints on node": {
			object: &discovery.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1-np7sf",
					Namespace: "default",
					Labels: map[string]string{
						discoveryV1beta1.LabelServiceName: "svc1",
					},
				},
				Endpoints: []discovery.Endpoint{
					{
						Addresses: []string{
							"10.244.1.3",
						},
						NodeName: &nodeName2,
					},
					{
						Addresses: []string{
							"10.244.1.5",
						},
						NodeName: &nodeName3,
					},
				},
			},
			kubeClient: k8sfake.NewSimpleClientset(
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: currentNodeName,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "hangzhou",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "shanghai",
						},
					},
				},
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc1",
						Namespace: "default",
						Annotations: map[string]string{
							AnnotationServiceTopologyKey: AnnotationServiceTopologyValueNode,
						},
					},
				},
			),
			yurtClient: yurtfake.NewSimpleClientset(
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hangzhou",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							currentNodeName,
						},
					},
				},
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "shanghai",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							"node2",
							"node3",
						},
					},
				},
			),
			expectResult: nil,
		},
		"v1.EndpointSlice: currentNode has no endpoints in nodePool": {
			object: &discovery.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1-np7sf",
					Namespace: "default",
					Labels: map[string]string{
						discoveryV1beta1.LabelServiceName: "svc1",
					},
				},
				Endpoints: []discovery.Endpoint{
					{
						Addresses: []string{
							"10.244.1.3",
						},
						NodeName: &nodeName2,
					},
					{
						Addresses: []string{
							"10.244.1.5",
						},
						NodeName: &nodeName3,
					},
				},
			},
			kubeClient: k8sfake.NewSimpleClientset(
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: currentNodeName,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "hangzhou",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "shanghai",
						},
					},
				},
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc1",
						Namespace: "default",
						Annotations: map[string]string{
							AnnotationServiceTopologyKey: AnnotationServiceTopologyValueNodePool,
						},
					},
				},
			),
			yurtClient: yurtfake.NewSimpleClientset(
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hangzhou",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							currentNodeName,
						},
					},
				},
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "shanghai",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							"node2",
							"node3",
						},
					},
				},
			),
			expectResult: nil,
		},
		"v1.Endpoints: topologyKeys is kubernetes.io/hostname": {
			object: &corev1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1",
					Namespace: "default",
				},
				Subsets: []corev1.EndpointSubset{
					{
						Addresses: []corev1.EndpointAddress{
							{
								IP:       "10.244.1.2",
								NodeName: &currentNodeName,
							},
							{
								IP:       "10.244.1.3",
								NodeName: &nodeName2,
							},
							{
								IP:       "10.244.1.4",
								NodeName: &currentNodeName,
							},
							{
								IP:       "10.244.1.5",
								NodeName: &nodeName3,
							},
						},
					},
				},
			},
			kubeClient: k8sfake.NewSimpleClientset(
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: currentNodeName,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "hangzhou",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "hangzhou",
						},
					},
				},
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc1",
						Namespace: "default",
						Annotations: map[string]string{
							AnnotationServiceTopologyKey: AnnotationServiceTopologyValueNode,
						},
					},
				},
			),
			yurtClient: yurtfake.NewSimpleClientset(
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hangzhou",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							currentNodeName,
							"node3",
						},
					},
				},
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "shanghai",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							"node2",
						},
					},
				},
			),
			expectResult: &corev1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1",
					Namespace: "default",
				},
				Subsets: []corev1.EndpointSubset{
					{
						Addresses: []corev1.EndpointAddress{
							{
								IP:       "10.244.1.2",
								NodeName: &currentNodeName,
							},
							{
								IP:       "10.244.1.4",
								NodeName: &currentNodeName,
							},
						},
					},
				},
			},
		},
		"v1.Endpoints: topologyKeys is openyurt.io/nodepool": {
			object: &corev1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1",
					Namespace: "default",
				},
				Subsets: []corev1.EndpointSubset{
					{
						Addresses: []corev1.EndpointAddress{
							{
								IP:       "10.244.1.2",
								NodeName: &currentNodeName,
							},
							{
								IP:       "10.244.1.3",
								NodeName: &nodeName2,
							},
							{
								IP:       "10.244.1.4",
								NodeName: &currentNodeName,
							},
							{
								IP:       "10.244.1.5",
								NodeName: &nodeName3,
							},
							{
								IP: "10.244.1.6",
							},
						},
					},
				},
			},
			kubeClient: k8sfake.NewSimpleClientset(
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: currentNodeName,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "hangzhou",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "hangzhou",
						},
					},
				},
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc1",
						Namespace: "default",
						Annotations: map[string]string{
							AnnotationServiceTopologyKey: AnnotationServiceTopologyValueNodePool,
						},
					},
				},
			),
			yurtClient: yurtfake.NewSimpleClientset(
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hangzhou",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							currentNodeName,
							"node3",
						},
					},
				},
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "shanghai",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							"node2",
						},
					},
				},
			),
			expectResult: &corev1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1",
					Namespace: "default",
				},
				Subsets: []corev1.EndpointSubset{
					{
						Addresses: []corev1.EndpointAddress{
							{
								IP:       "10.244.1.2",
								NodeName: &currentNodeName,
							},
							{
								IP:       "10.244.1.4",
								NodeName: &currentNodeName,
							},
							{
								IP:       "10.244.1.5",
								NodeName: &nodeName3,
							},
						},
					},
				},
			},
		},
		"v1.Endpoints: topologyKeys is kubernetes.io/zone": {
			object: &corev1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1",
					Namespace: "default",
				},
				Subsets: []corev1.EndpointSubset{
					{
						Addresses: []corev1.EndpointAddress{
							{
								IP:       "10.244.1.2",
								NodeName: &currentNodeName,
							},
							{
								IP:       "10.244.1.3",
								NodeName: &nodeName2,
							},
							{
								IP:       "10.244.1.4",
								NodeName: &currentNodeName,
							},
							{
								IP:       "10.244.1.5",
								NodeName: &nodeName3,
							},
						},
					},
				},
			},
			kubeClient: k8sfake.NewSimpleClientset(
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: currentNodeName,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "hangzhou",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "hangzhou",
						},
					},
				},
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc1",
						Namespace: "default",
						Annotations: map[string]string{
							AnnotationServiceTopologyKey: AnnotationServiceTopologyValueZone,
						},
					},
				},
			),
			yurtClient: yurtfake.NewSimpleClientset(
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hangzhou",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							currentNodeName,
							"node3",
						},
					},
				},
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "shanghai",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							"node2",
						},
					},
				},
			),
			expectResult: &corev1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1",
					Namespace: "default",
				},
				Subsets: []corev1.EndpointSubset{
					{
						Addresses: []corev1.EndpointAddress{
							{
								IP:       "10.244.1.2",
								NodeName: &currentNodeName,
							},
							{
								IP:       "10.244.1.4",
								NodeName: &currentNodeName,
							},
							{
								IP:       "10.244.1.5",
								NodeName: &nodeName3,
							},
						},
					},
				},
			},
		},
		"v1.Endpoints: without openyurt.io/topologyKeys": {
			object: &corev1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1",
					Namespace: "default",
				},
				Subsets: []corev1.EndpointSubset{
					{
						Addresses: []corev1.EndpointAddress{
							{
								IP:       "10.244.1.2",
								NodeName: &currentNodeName,
							},
							{
								IP:       "10.244.1.3",
								NodeName: &nodeName2,
							},
							{
								IP:       "10.244.1.4",
								NodeName: &currentNodeName,
							},
							{
								IP:       "10.244.1.5",
								NodeName: &nodeName3,
							},
						},
					},
				},
			},
			kubeClient: k8sfake.NewSimpleClientset(
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: currentNodeName,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "hangzhou",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "hangzhou",
						},
					},
				},
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "svc1",
						Namespace:   "default",
						Annotations: map[string]string{},
					},
				},
			),
			yurtClient: yurtfake.NewSimpleClientset(
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hangzhou",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							currentNodeName,
							"node3",
						},
					},
				},
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "shanghai",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							"node2",
						},
					},
				},
			),
			expectResult: &corev1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1",
					Namespace: "default",
				},
				Subsets: []corev1.EndpointSubset{
					{
						Addresses: []corev1.EndpointAddress{
							{
								IP:       "10.244.1.2",
								NodeName: &currentNodeName,
							},
							{
								IP:       "10.244.1.3",
								NodeName: &nodeName2,
							},
							{
								IP:       "10.244.1.4",
								NodeName: &currentNodeName,
							},
							{
								IP:       "10.244.1.5",
								NodeName: &nodeName3,
							},
						},
					},
				},
			},
		},
		"v1.Endpoints: currentNode is not in any nodepool": {
			object: &corev1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1",
					Namespace: "default",
				},
				Subsets: []corev1.EndpointSubset{
					{
						Addresses: []corev1.EndpointAddress{
							{
								IP:       "10.244.1.2",
								NodeName: &currentNodeName,
							},
							{
								IP:       "10.244.1.3",
								NodeName: &nodeName2,
							},
							{
								IP:       "10.244.1.4",
								NodeName: &currentNodeName,
							},
							{
								IP:       "10.244.1.5",
								NodeName: &nodeName3,
							},
						},
					},
				},
			},
			kubeClient: k8sfake.NewSimpleClientset(
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name:   currentNodeName,
						Labels: map[string]string{},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "hangzhou",
						},
					},
				},
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc1",
						Namespace: "default",
						Annotations: map[string]string{
							AnnotationServiceTopologyKey: AnnotationServiceTopologyValueNodePool,
						},
					},
				},
			),
			yurtClient: yurtfake.NewSimpleClientset(
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hangzhou",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							"node3",
						},
					},
				},
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "shanghai",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							"node2",
						},
					},
				},
			),
			expectResult: &corev1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1",
					Namespace: "default",
				},
				Subsets: []corev1.EndpointSubset{
					{
						Addresses: []corev1.EndpointAddress{
							{
								IP:       "10.244.1.2",
								NodeName: &currentNodeName,
							},
							{
								IP:       "10.244.1.4",
								NodeName: &currentNodeName,
							},
						},
					},
				},
			},
		},
		"v1.Endpoints: currentNode has no endpoints on node": {
			object: &corev1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1",
					Namespace: "default",
				},
				Subsets: []corev1.EndpointSubset{
					{
						Addresses: []corev1.EndpointAddress{
							{
								IP:       "10.244.1.3",
								NodeName: &nodeName2,
							},
							{
								IP:       "10.244.1.4",
								NodeName: &nodeName3,
							},
						},
					},
				},
			},
			kubeClient: k8sfake.NewSimpleClientset(
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: currentNodeName,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "hangzhou",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "shanghai",
						},
					},
				},
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc1",
						Namespace: "default",
						Annotations: map[string]string{
							AnnotationServiceTopologyKey: AnnotationServiceTopologyValueNode,
						},
					},
				},
			),
			yurtClient: yurtfake.NewSimpleClientset(
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hangzhou",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							currentNodeName,
						},
					},
				},
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "shanghai",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							"node2",
							"node3",
						},
					},
				},
			),
			expectResult: nil,
		},
		"v1.Endpoints: currentNode has no endpoints in nodepool": {
			object: &corev1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1",
					Namespace: "default",
				},
				Subsets: []corev1.EndpointSubset{
					{
						Addresses: []corev1.EndpointAddress{
							{
								IP:       "10.244.1.3",
								NodeName: &nodeName2,
							},
							{
								IP:       "10.244.1.4",
								NodeName: &nodeName3,
							},
						},
					},
				},
			},
			kubeClient: k8sfake.NewSimpleClientset(
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: currentNodeName,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "hangzhou",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "shanghai",
						},
					},
				},
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc1",
						Namespace: "default",
						Annotations: map[string]string{
							AnnotationServiceTopologyKey: AnnotationServiceTopologyValueNodePool,
						},
					},
				},
			),
			yurtClient: yurtfake.NewSimpleClientset(
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hangzhou",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							currentNodeName,
						},
					},
				},
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "shanghai",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							"node2",
							"node3",
						},
					},
				},
			),
			expectResult: nil,
		},
		"v1.Endpoints: unknown openyurt.io/topologyKeys": {
			object: &corev1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1",
					Namespace: "default",
				},
				Subsets: []corev1.EndpointSubset{
					{
						Addresses: []corev1.EndpointAddress{
							{
								IP:       "10.244.1.3",
								NodeName: &nodeName2,
							},
							{
								IP:       "10.244.1.4",
								NodeName: &nodeName3,
							},
						},
					},
				},
			},
			kubeClient: k8sfake.NewSimpleClientset(
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: currentNodeName,
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "hangzhou",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelCurrentNodePool: "shanghai",
						},
					},
				},
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc1",
						Namespace: "default",
						Annotations: map[string]string{
							AnnotationServiceTopologyKey: "unknown topology",
						},
					},
				},
			),
			yurtClient: yurtfake.NewSimpleClientset(
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hangzhou",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							currentNodeName,
						},
					},
				},
				&nodepoolv1alpha1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "shanghai",
					},
					Spec: nodepoolv1alpha1.NodePoolSpec{
						Type: nodepoolv1alpha1.Edge,
					},
					Status: nodepoolv1alpha1.NodePoolStatus{
						Nodes: []string{
							"node2",
							"node3",
						},
					},
				},
			),
			expectResult: &corev1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1",
					Namespace: "default",
				},
				Subsets: []corev1.EndpointSubset{
					{
						Addresses: []corev1.EndpointAddress{
							{
								IP:       "10.244.1.3",
								NodeName: &nodeName2,
							},
							{
								IP:       "10.244.1.4",
								NodeName: &nodeName3,
							},
						},
					},
				},
			},
		},
	}

	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			//tt.kubeClient.DiscoveryV1beta1().EndpointSlices("default").Create(context.TODO(), tt.endpointSlice, metav1.CreateOptions{})

			factory := informers.NewSharedInformerFactory(tt.kubeClient, 24*time.Hour)
			serviceInformer := factory.Core().V1().Services()
			serviceInformer.Informer()
			serviceLister := serviceInformer.Lister()

			stopper := make(chan struct{})
			defer close(stopper)
			factory.Start(stopper)
			factory.WaitForCacheSync(stopper)

			yurtFactory := yurtinformers.NewSharedInformerFactory(tt.yurtClient, 24*time.Hour)
			nodePoolInformer := yurtFactory.Apps().V1alpha1().NodePools()
			nodePoolLister := nodePoolInformer.Lister()

			stopper2 := make(chan struct{})
			defer close(stopper2)
			yurtFactory.Start(stopper2)
			yurtFactory.WaitForCacheSync(stopper2)

			nodeGetter := func(name string) (*corev1.Node, error) {
				return tt.kubeClient.CoreV1().Nodes().Get(context.TODO(), name, metav1.GetOptions{})
			}

			fh := &serviceTopologyFilterHandler{
				nodeName:       currentNodeName,
				serviceLister:  serviceLister,
				nodePoolLister: nodePoolLister,
				nodeGetter:     nodeGetter,
			}

			isNil, handledObject := fh.serviceTopologyHandler(tt.object)
			if tt.expectResult != nil {
				if isNil {
					t.Errorf("serviceTopologyHandler expect %v, but got nil", tt.expectResult)
				}

				if !reflect.DeepEqual(handledObject, tt.expectResult) {
					t.Errorf("serviceTopologyHandler expect: \n%#+v\nbut got: \n%#+v\n", tt.expectResult, handledObject)
				}
			} else {
				if !isNil {
					t.Errorf("serviceTopologyHandler expect nil, but got %v", handledObject)
				}
			}
		})
	}
}
