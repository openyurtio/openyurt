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
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	discovery "k8s.io/api/discovery/v1"
	discoveryV1beta1 "k8s.io/api/discovery/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apiserver/pkg/endpoints/filters"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/client-go/informers"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	filterutil "github.com/openyurtio/openyurt/pkg/yurthub/filter/util"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/serializer"
	"github.com/openyurtio/openyurt/pkg/yurthub/proxy/util"
	hubutil "github.com/openyurtio/openyurt/pkg/yurthub/util"
	nodepoolv1alpha1 "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/apis/apps/v1alpha1"
	yurtfake "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/client/clientset/versioned/fake"
	yurtinformers "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/client/informers/externalversions"
)

func newTestRequestInfoResolver() *request.RequestInfoFactory {
	return &request.RequestInfoFactory{
		APIPrefixes:          sets.NewString("api", "apis"),
		GrouplessAPIPrefixes: sets.NewString("api"),
	}
}

func TestServiceTopologyHandler(t *testing.T) {
	currentNodeName := "node1"
	nodeName2 := "node2"
	nodeName3 := "node3"

	testcases := map[string]struct {
		path         string
		object       runtime.Object
		kubeClient   *k8sfake.Clientset
		yurtClient   *yurtfake.Clientset
		expectResult runtime.Object
	}{
		"v1beta1.EndpointSliceList: topologyKeys is kubernetes.io/hostname": {
			path: "/apis/discovery.k8s.io/v1beta1/endpointslices",
			object: &discoveryV1beta1.EndpointSliceList{
				Items: []discoveryV1beta1.EndpointSlice{
					{
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
			expectResult: &discoveryV1beta1.EndpointSliceList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "EndpointSliceList",
					APIVersion: "discovery.k8s.io/v1beta1",
				},
				Items: []discoveryV1beta1.EndpointSlice{
					{
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
			},
		},
		"v1beta1.EndpointSliceList: topologyKeys is openyurt.io/nodepool": {
			path: "/apis/discovery.k8s.io/v1beta1/endpointslices",
			object: &discoveryV1beta1.EndpointSliceList{
				Items: []discoveryV1beta1.EndpointSlice{
					{
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
			expectResult: &discoveryV1beta1.EndpointSliceList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "EndpointSliceList",
					APIVersion: "discovery.k8s.io/v1beta1",
				},
				Items: []discoveryV1beta1.EndpointSlice{
					{
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
			},
		},
		"v1beta1.EndpointSliceList: topologyKeys is kubernetes.io/zone": {
			path: "/apis/discovery.k8s.io/v1beta1/endpointslices",
			object: &discoveryV1beta1.EndpointSliceList{
				Items: []discoveryV1beta1.EndpointSlice{
					{
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
			expectResult: &discoveryV1beta1.EndpointSliceList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "EndpointSliceList",
					APIVersion: "discovery.k8s.io/v1beta1",
				},
				Items: []discoveryV1beta1.EndpointSlice{
					{
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
			},
		},
		"v1beta1.EndpointSliceList: without openyurt.io/topologyKeys": {
			path: "/apis/discovery.k8s.io/v1beta1/endpointslices",
			object: &discoveryV1beta1.EndpointSliceList{
				Items: []discoveryV1beta1.EndpointSlice{
					{
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
			expectResult: &discoveryV1beta1.EndpointSliceList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "EndpointSliceList",
					APIVersion: "discovery.k8s.io/v1beta1",
				},
				Items: []discoveryV1beta1.EndpointSlice{
					{
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
			},
		},
		"v1beta1.EndpointSliceList: currentNode is not in any nodepool": {
			path: "/apis/discovery.k8s.io/v1beta1/endpointslices",
			object: &discoveryV1beta1.EndpointSliceList{
				Items: []discoveryV1beta1.EndpointSlice{
					{
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
			expectResult: &discoveryV1beta1.EndpointSliceList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "EndpointSliceList",
					APIVersion: "discovery.k8s.io/v1beta1",
				},
				Items: []discoveryV1beta1.EndpointSlice{
					{
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
			},
		},
		"v1beta1.EndpointSliceList: currentNode has no endpoints on node": {
			path: "/apis/discovery.k8s.io/v1beta1/endpointslices",
			object: &discoveryV1beta1.EndpointSliceList{
				Items: []discoveryV1beta1.EndpointSlice{
					{
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
			expectResult: &discoveryV1beta1.EndpointSliceList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "EndpointSliceList",
					APIVersion: "discovery.k8s.io/v1beta1",
				},
				Items: []discoveryV1beta1.EndpointSlice{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1-np7sf",
							Namespace: "default",
							Labels: map[string]string{
								discoveryV1beta1.LabelServiceName: "svc1",
							},
						},
					},
				},
			},
		},
		"v1beta1.EndpointSliceList: currentNode has no endpoints in nodepool": {
			path: "/apis/discovery.k8s.io/v1beta1/endpointslices",
			object: &discoveryV1beta1.EndpointSliceList{
				Items: []discoveryV1beta1.EndpointSlice{
					{
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
			expectResult: &discoveryV1beta1.EndpointSliceList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "EndpointSliceList",
					APIVersion: "discovery.k8s.io/v1beta1",
				},
				Items: []discoveryV1beta1.EndpointSlice{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1-np7sf",
							Namespace: "default",
							Labels: map[string]string{
								discoveryV1beta1.LabelServiceName: "svc1",
							},
						},
					},
				},
			},
		},
		"v1beta1.EndpointSliceList: no service info in endpointslice": {
			path: "/apis/discovery.k8s.io/v1beta1/endpointslices",
			object: &discoveryV1beta1.EndpointSliceList{
				Items: []discoveryV1beta1.EndpointSlice{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1-np7sf",
							Namespace: "default",
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
									corev1.LabelHostname: nodeName2,
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
									corev1.LabelHostname: nodeName3,
								},
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
			expectResult: &discoveryV1beta1.EndpointSliceList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "EndpointSliceList",
					APIVersion: "discovery.k8s.io/v1beta1",
				},
				Items: []discoveryV1beta1.EndpointSlice{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1-np7sf",
							Namespace: "default",
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
									corev1.LabelHostname: nodeName2,
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
									corev1.LabelHostname: nodeName3,
								},
							},
						},
					},
				},
			},
		},
		"v1.EndpointSliceList: topologyKeys is kubernetes.io/hostname": {
			path: "/apis/discovery.k8s.io/v1/endpointslices",
			object: &discovery.EndpointSliceList{
				Items: []discovery.EndpointSlice{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1-np7sf",
							Namespace: "default",
							Labels: map[string]string{
								discovery.LabelServiceName: "svc1",
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
			expectResult: &discovery.EndpointSliceList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "EndpointSliceList",
					APIVersion: "discovery.k8s.io/v1",
				},
				Items: []discovery.EndpointSlice{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1-np7sf",
							Namespace: "default",
							Labels: map[string]string{
								discovery.LabelServiceName: "svc1",
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
			},
		},
		"v1.EndpointSliceList: topologyKeys is openyurt.io/nodepool": {
			path: "/apis/discovery.k8s.io/v1/endpointslices",
			object: &discovery.EndpointSliceList{
				Items: []discovery.EndpointSlice{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1-np7sf",
							Namespace: "default",
							Labels: map[string]string{
								discovery.LabelServiceName: "svc1",
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
			expectResult: &discovery.EndpointSliceList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "EndpointSliceList",
					APIVersion: "discovery.k8s.io/v1",
				},
				Items: []discovery.EndpointSlice{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1-np7sf",
							Namespace: "default",
							Labels: map[string]string{
								discovery.LabelServiceName: "svc1",
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
			},
		},
		"v1.EndpointSliceList: topologyKeys is kubernetes.io/zone": {
			path: "/apis/discovery.k8s.io/v1/endpointslices",
			object: &discovery.EndpointSliceList{
				Items: []discovery.EndpointSlice{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1-np7sf",
							Namespace: "default",
							Labels: map[string]string{
								discovery.LabelServiceName: "svc1",
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
			expectResult: &discovery.EndpointSliceList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "EndpointSliceList",
					APIVersion: "discovery.k8s.io/v1",
				},
				Items: []discovery.EndpointSlice{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1-np7sf",
							Namespace: "default",
							Labels: map[string]string{
								discovery.LabelServiceName: "svc1",
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
			},
		},
		"v1.EndpointSliceList: without openyurt.io/topologyKeys": {
			path: "/apis/discovery.k8s.io/v1/endpointslices",
			object: &discovery.EndpointSliceList{
				Items: []discovery.EndpointSlice{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1-np7sf",
							Namespace: "default",
							Labels: map[string]string{
								discovery.LabelServiceName: "svc1",
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
			expectResult: &discovery.EndpointSliceList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "EndpointSliceList",
					APIVersion: "discovery.k8s.io/v1",
				},
				Items: []discovery.EndpointSlice{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1-np7sf",
							Namespace: "default",
							Labels: map[string]string{
								discovery.LabelServiceName: "svc1",
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
			},
		},
		"v1.EndpointSliceList: currentNode is not in any nodepool": {
			path: "/apis/discovery.k8s.io/v1/endpointslices",
			object: &discovery.EndpointSliceList{
				Items: []discovery.EndpointSlice{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1-np7sf",
							Namespace: "default",
							Labels: map[string]string{
								discovery.LabelServiceName: "svc1",
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
			expectResult: &discovery.EndpointSliceList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "EndpointSliceList",
					APIVersion: "discovery.k8s.io/v1",
				},
				Items: []discovery.EndpointSlice{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1-np7sf",
							Namespace: "default",
							Labels: map[string]string{
								discovery.LabelServiceName: "svc1",
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
			},
		},
		"v1.EndpointSliceList: currentNode has no endpoints on node": {
			path: "/apis/discovery.k8s.io/v1/endpointslices",
			object: &discovery.EndpointSliceList{
				Items: []discovery.EndpointSlice{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1-np7sf",
							Namespace: "default",
							Labels: map[string]string{
								discovery.LabelServiceName: "svc1",
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
			expectResult: &discovery.EndpointSliceList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "EndpointSliceList",
					APIVersion: "discovery.k8s.io/v1",
				},
				Items: []discovery.EndpointSlice{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1-np7sf",
							Namespace: "default",
							Labels: map[string]string{
								discovery.LabelServiceName: "svc1",
							},
						},
					},
				},
			},
		},
		"v1.EndpointSliceList: currentNode has no endpoints in nodePool": {
			path: "/apis/discovery.k8s.io/v1/endpointslices",
			object: &discovery.EndpointSliceList{
				Items: []discovery.EndpointSlice{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1-np7sf",
							Namespace: "default",
							Labels: map[string]string{
								discovery.LabelServiceName: "svc1",
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
			expectResult: &discovery.EndpointSliceList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "EndpointSliceList",
					APIVersion: "discovery.k8s.io/v1",
				},
				Items: []discovery.EndpointSlice{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1-np7sf",
							Namespace: "default",
							Labels: map[string]string{
								discovery.LabelServiceName: "svc1",
							},
						},
					},
				},
			},
		},
		"v1.EndpointsList: topologyKeys is kubernetes.io/hostname": {
			path: "/api/v1/endpoints",
			object: &corev1.EndpointsList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "EndpointsList",
					APIVersion: "v1",
				},
				Items: []corev1.Endpoints{
					{
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
			expectResult: &corev1.EndpointsList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "EndpointsList",
					APIVersion: "v1",
				},
				Items: []corev1.Endpoints{
					{
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
			},
		},
		"v1.EndpointsList: topologyKeys is openyurt.io/nodepool": {
			path: "/api/v1/endpoints",
			object: &corev1.EndpointsList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "EndpointsList",
					APIVersion: "v1",
				},
				Items: []corev1.Endpoints{
					{
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
			expectResult: &corev1.EndpointsList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "EndpointsList",
					APIVersion: "v1",
				},
				Items: []corev1.Endpoints{
					{
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
			},
		},
		"v1.EndpointsList: topologyKeys is kubernetes.io/zone": {
			path: "/api/v1/endpoints",
			object: &corev1.EndpointsList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "EndpointsList",
					APIVersion: "v1",
				},
				Items: []corev1.Endpoints{
					{
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
			expectResult: &corev1.EndpointsList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "EndpointsList",
					APIVersion: "v1",
				},
				Items: []corev1.Endpoints{
					{
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
			},
		},
		"v1.EndpointsList: without openyurt.io/topologyKeys": {
			path: "/api/v1/endpoints",
			object: &corev1.EndpointsList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "EndpointsList",
					APIVersion: "v1",
				},
				Items: []corev1.Endpoints{
					{
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
			expectResult: &corev1.EndpointsList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "EndpointsList",
					APIVersion: "v1",
				},
				Items: []corev1.Endpoints{
					{
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
			},
		},
		"v1.EndpointsList: currentNode is not in any nodepool": {
			path: "/api/v1/endpoints",
			object: &corev1.EndpointsList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "EndpointsList",
					APIVersion: "v1",
				},
				Items: []corev1.Endpoints{
					{
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
			expectResult: &corev1.EndpointsList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "EndpointsList",
					APIVersion: "v1",
				},
				Items: []corev1.Endpoints{
					{
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
			},
		},
		"v1.EndpointsList: currentNode has no endpoints on node": {
			path: "/api/v1/endpoints",
			object: &corev1.EndpointsList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "EndpointsList",
					APIVersion: "v1",
				},
				Items: []corev1.Endpoints{
					{
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
										IP:       "10.244.1.5",
										NodeName: &nodeName3,
									},
								},
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
			expectResult: &corev1.EndpointsList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "EndpointsList",
					APIVersion: "v1",
				},
				Items: []corev1.Endpoints{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1",
							Namespace: "default",
						},
					},
				},
			},
		},
		"v1.EndpointsList: currentNode has no endpoints in nodepool": {
			path: "/api/v1/endpoints",
			object: &corev1.EndpointsList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "EndpointsList",
					APIVersion: "v1",
				},
				Items: []corev1.Endpoints{
					{
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
										IP:       "10.244.1.5",
										NodeName: &nodeName3,
									},
								},
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
			expectResult: &corev1.EndpointsList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "EndpointsList",
					APIVersion: "v1",
				},
				Items: []corev1.Endpoints{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1",
							Namespace: "default",
						},
					},
				},
			},
		},
		"v1.EndpointsList: unknown openyurt.io/topologyKeys": {
			path: "/api/v1/endpoints",
			object: &corev1.EndpointsList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "EndpointsList",
					APIVersion: "v1",
				},
				Items: []corev1.Endpoints{
					{
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
										IP:       "10.244.1.5",
										NodeName: &nodeName3,
									},
								},
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
			expectResult: &corev1.EndpointsList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "EndpointsList",
					APIVersion: "v1",
				},
				Items: []corev1.Endpoints{
					{
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
										IP:       "10.244.1.5",
										NodeName: &nodeName3,
									},
								},
							},
						},
					},
				},
			},
		},
		"v1.Pod: un-recognized object for filter": {
			path: "/api/v1/pods",
			object: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "default",
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
			expectResult: &corev1.Pod{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Pod",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "default",
				},
			},
		},
	}

	resolver := newTestRequestInfoResolver()
	sw := serializer.NewSerializerManager()
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

			req, err := http.NewRequest("GET", tt.path, nil)
			if err != nil {
				t.Errorf("failed to create request, %v", err)
			}
			req.RemoteAddr = "127.0.0.1"
			req.Header.Set("Accept", "application/json")

			var handledObject runtime.Object
			var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				ctx := req.Context()
				reqContentType, _ := hubutil.ReqContentTypeFrom(ctx)
				ctx = hubutil.WithRespContentType(ctx, reqContentType)
				req = req.WithContext(ctx)
				s := filterutil.CreateSerializer(req, sw)
				if s == nil {
					t.Fatalf("failed to create serializer, %v", s)
				}

				fh := NewServiceTopologyFilterHandler(currentNodeName, s, serviceLister, nodePoolLister, nodeGetter)
				inputB, _ := s.Encode(tt.object)
				filteredB, _ := fh.ObjectResponseFilter(inputB)
				handledObject, _ = s.Decode(filteredB)
			})

			handler = util.WithRequestContentType(handler)
			handler = filters.WithRequestInfo(handler, resolver)
			handler.ServeHTTP(httptest.NewRecorder(), req)

			if !reflect.DeepEqual(handledObject, tt.expectResult) {
				t.Errorf("serviceTopologyHandler expect: \n%#+v\nbut got: \n%#+v\n", tt.expectResult, handledObject)
			}
		})
	}
}
