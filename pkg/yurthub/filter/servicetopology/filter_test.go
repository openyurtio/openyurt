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
	"reflect"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	discovery "k8s.io/api/discovery/v1"
	discoveryV1beta1 "k8s.io/api/discovery/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/informers"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	"github.com/openyurtio/openyurt/pkg/util"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
	nodepoolv1alpha1 "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/apis/apps/v1alpha1"
	yurtfake "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/client/clientset/versioned/fake"
	yurtinformers "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/client/informers/externalversions"
)

func TestName(t *testing.T) {
	stf := &serviceTopologyFilter{}
	if stf.Name() != filter.ServiceTopologyFilterName {
		t.Errorf("expect %s, but got %s", filter.ServiceTopologyFilterName, stf.Name())
	}
}

func TestSupportedResourceAndVerbs(t *testing.T) {
	stf := &serviceTopologyFilter{}
	rvs := stf.SupportedResourceAndVerbs()
	if len(rvs) != 2 {
		t.Errorf("supported not two resources, %v", rvs)
	}

	for resource, verbs := range rvs {
		if resource != "endpoints" && resource != "endpointslices" {
			t.Errorf("expect resource is endpoints/endpointslices, but got %s", resource)
		}

		if !verbs.Equal(sets.NewString("list", "watch")) {
			t.Errorf("expect verbs are list/watch, but got %v", verbs.UnsortedList())
		}
	}
}

func TestFilter(t *testing.T) {
	currentNodeName := "node1"
	nodeName2 := "node2"
	nodeName3 := "node3"

	testcases := map[string]struct {
		poolName       string
		nodeName       string
		responseObject runtime.Object
		kubeClient     *k8sfake.Clientset
		yurtClient     *yurtfake.Clientset
		expectObject   runtime.Object
	}{
		"v1beta1.EndpointSliceList: topologyKeys is kubernetes.io/hostname": {
			poolName: "hangzhou",
			responseObject: &discoveryV1beta1.EndpointSliceList{
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
							nodepoolv1alpha1.LabelDesiredNodePool: "hangzhou",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelDesiredNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelDesiredNodePool: "hangzhou",
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
			expectObject: &discoveryV1beta1.EndpointSliceList{
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
			poolName: "hangzhou",
			responseObject: &discoveryV1beta1.EndpointSliceList{
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
							nodepoolv1alpha1.LabelDesiredNodePool: "hangzhou",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelDesiredNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelDesiredNodePool: "hangzhou",
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
			expectObject: &discoveryV1beta1.EndpointSliceList{
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
			responseObject: &discoveryV1beta1.EndpointSliceList{
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
							nodepoolv1alpha1.LabelDesiredNodePool: "hangzhou",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelDesiredNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelDesiredNodePool: "hangzhou",
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
			expectObject: &discoveryV1beta1.EndpointSliceList{
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
			responseObject: &discoveryV1beta1.EndpointSliceList{
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
							nodepoolv1alpha1.LabelDesiredNodePool: "hangzhou",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelDesiredNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelDesiredNodePool: "hangzhou",
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
			expectObject: &discoveryV1beta1.EndpointSliceList{
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
			responseObject: &discoveryV1beta1.EndpointSliceList{
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
							nodepoolv1alpha1.LabelDesiredNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelDesiredNodePool: "hangzhou",
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
			expectObject: &discoveryV1beta1.EndpointSliceList{
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
			responseObject: &discoveryV1beta1.EndpointSliceList{
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
							nodepoolv1alpha1.LabelDesiredNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelDesiredNodePool: "hangzhou",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelDesiredNodePool: "hangzhou",
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
			expectObject: &discoveryV1beta1.EndpointSliceList{
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
			responseObject: &discoveryV1beta1.EndpointSliceList{
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
							nodepoolv1alpha1.LabelDesiredNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelDesiredNodePool: "hangzhou",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelDesiredNodePool: "hangzhou",
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
			expectObject: &discoveryV1beta1.EndpointSliceList{
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
			responseObject: &discoveryV1beta1.EndpointSliceList{
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
							nodepoolv1alpha1.LabelDesiredNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelDesiredNodePool: "hangzhou",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelDesiredNodePool: "hangzhou",
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
			expectObject: &discoveryV1beta1.EndpointSliceList{
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
			responseObject: &discovery.EndpointSliceList{
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
							nodepoolv1alpha1.LabelDesiredNodePool: "hangzhou",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelDesiredNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelDesiredNodePool: "hangzhou",
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
			expectObject: &discovery.EndpointSliceList{
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
			responseObject: &discovery.EndpointSliceList{
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
							nodepoolv1alpha1.LabelDesiredNodePool: "hangzhou",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelDesiredNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelDesiredNodePool: "hangzhou",
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
			expectObject: &discovery.EndpointSliceList{
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
			responseObject: &discovery.EndpointSliceList{
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
							nodepoolv1alpha1.LabelDesiredNodePool: "hangzhou",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelDesiredNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelDesiredNodePool: "hangzhou",
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
			expectObject: &discovery.EndpointSliceList{
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
			responseObject: &discovery.EndpointSliceList{
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
							nodepoolv1alpha1.LabelDesiredNodePool: "hangzhou",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelDesiredNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelDesiredNodePool: "hangzhou",
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
			expectObject: &discovery.EndpointSliceList{
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
			responseObject: &discovery.EndpointSliceList{
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
							nodepoolv1alpha1.LabelDesiredNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelDesiredNodePool: "hangzhou",
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
			expectObject: &discovery.EndpointSliceList{
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
			responseObject: &discovery.EndpointSliceList{
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
							nodepoolv1alpha1.LabelDesiredNodePool: "hangzhou",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelDesiredNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelDesiredNodePool: "shanghai",
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
			expectObject: &discovery.EndpointSliceList{
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
			responseObject: &discovery.EndpointSliceList{
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
							nodepoolv1alpha1.LabelDesiredNodePool: "hangzhou",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelDesiredNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelDesiredNodePool: "shanghai",
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
			expectObject: &discovery.EndpointSliceList{
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
			responseObject: &corev1.EndpointsList{
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
							nodepoolv1alpha1.LabelDesiredNodePool: "hangzhou",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelDesiredNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelDesiredNodePool: "hangzhou",
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
			expectObject: &corev1.EndpointsList{
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
			responseObject: &corev1.EndpointsList{
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
							nodepoolv1alpha1.LabelDesiredNodePool: "hangzhou",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelDesiredNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelDesiredNodePool: "hangzhou",
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
			expectObject: &corev1.EndpointsList{
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
			responseObject: &corev1.EndpointsList{
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
							nodepoolv1alpha1.LabelDesiredNodePool: "hangzhou",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelDesiredNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelDesiredNodePool: "hangzhou",
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
			expectObject: &corev1.EndpointsList{
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
			responseObject: &corev1.EndpointsList{
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
							nodepoolv1alpha1.LabelDesiredNodePool: "hangzhou",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelDesiredNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelDesiredNodePool: "hangzhou",
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
			expectObject: &corev1.EndpointsList{
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
			responseObject: &corev1.EndpointsList{
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
							nodepoolv1alpha1.LabelDesiredNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelDesiredNodePool: "hangzhou",
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
			expectObject: &corev1.EndpointsList{
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
			responseObject: &corev1.EndpointsList{
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
							nodepoolv1alpha1.LabelDesiredNodePool: "hangzhou",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelDesiredNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelDesiredNodePool: "shanghai",
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
			expectObject: &corev1.EndpointsList{
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
			responseObject: &corev1.EndpointsList{
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
							nodepoolv1alpha1.LabelDesiredNodePool: "hangzhou",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelDesiredNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelDesiredNodePool: "shanghai",
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
			expectObject: &corev1.EndpointsList{
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
			responseObject: &corev1.EndpointsList{
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
							nodepoolv1alpha1.LabelDesiredNodePool: "hangzhou",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelDesiredNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelDesiredNodePool: "shanghai",
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
			expectObject: &corev1.EndpointsList{
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
			responseObject: &corev1.Pod{
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
							nodepoolv1alpha1.LabelDesiredNodePool: "hangzhou",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelDesiredNodePool: "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							nodepoolv1alpha1.LabelDesiredNodePool: "shanghai",
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
			expectObject: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "default",
				},
			},
		},
	}

	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			factory := informers.NewSharedInformerFactory(tt.kubeClient, 24*time.Hour)
			serviceInformer := factory.Core().V1().Services()
			serviceInformer.Informer()
			serviceLister := serviceInformer.Lister()
			serviceSynced := serviceInformer.Informer().HasSynced

			stopper := make(chan struct{})
			defer close(stopper)
			factory.Start(stopper)
			factory.WaitForCacheSync(stopper)

			yurtFactory := yurtinformers.NewSharedInformerFactory(tt.yurtClient, 24*time.Hour)
			nodePoolInformer := yurtFactory.Apps().V1alpha1().NodePools()
			nodePoolLister := nodePoolInformer.Lister()
			nodePoolSynced := nodePoolInformer.Informer().HasSynced

			stopper2 := make(chan struct{})
			defer close(stopper2)
			yurtFactory.Start(stopper2)
			yurtFactory.WaitForCacheSync(stopper2)

			stopCh := make(<-chan struct{})
			stf := &serviceTopologyFilter{
				nodeName:       currentNodeName,
				serviceLister:  serviceLister,
				nodePoolLister: nodePoolLister,
				serviceSynced:  serviceSynced,
				nodePoolSynced: nodePoolSynced,
				client:         tt.kubeClient,
			}

			if len(tt.poolName) != 0 {
				stf.nodePoolName = tt.poolName
			} else {
				stf.nodeName = currentNodeName
			}

			newObj := stf.Filter(tt.responseObject, stopCh)
			if util.IsNil(newObj) {
				t.Errorf("empty object is returned")
			}
			if !reflect.DeepEqual(newObj, tt.expectObject) {
				t.Errorf("serviceTopologyHandler expect: \n%#+v\nbut got: \n%#+v\n", tt.expectObject, newObj)
			}
		})
	}
}
