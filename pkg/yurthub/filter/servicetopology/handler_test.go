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
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	discovery "k8s.io/api/discovery/v1"
	discoveryV1beta1 "k8s.io/api/discovery/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	nodepoolv1alpha1 "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/apis/apps/v1alpha1"
	yurtfake "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/client/clientset/versioned/fake"
	yurtinformers "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/client/informers/externalversions"
)

func TestReassembleEndpointSliceV1beta1(t *testing.T) {
	currentNodeName := "node1"

	testcases := map[string]struct {
		endpointSlice *discoveryV1beta1.EndpointSlice
		kubeClient    *k8sfake.Clientset
		yurtClient    *yurtfake.Clientset
		expectResult  *discoveryV1beta1.EndpointSlice
	}{
		"service with annotation openyurt.io/topologyKeys: kubernetes.io/hostname": {
			endpointSlice: &discoveryV1beta1.EndpointSlice{
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
		"service with annotation openyurt.io/topologyKeys: openyurt.io/nodepool": {
			endpointSlice: &discoveryV1beta1.EndpointSlice{
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
		"service with annotation openyurt.io/topologyKeys: kubernetes.io/zone": {
			endpointSlice: &discoveryV1beta1.EndpointSlice{
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
		"service without annotation openyurt.io/topologyKeys": {
			endpointSlice: &discoveryV1beta1.EndpointSlice{
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
		"currentNode is not in any nodepool": {
			endpointSlice: &discoveryV1beta1.EndpointSlice{
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
	}

	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			tt.kubeClient.DiscoveryV1beta1().EndpointSlices("default").Create(context.TODO(), tt.endpointSlice, metav1.CreateOptions{})

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

			reassembledEndpointSlice := fh.reassembleEndpointSliceV1beta1(tt.endpointSlice)

			if !isEqualEndpointSliceV1beta1(reassembledEndpointSlice, tt.expectResult) {
				t.Errorf("reassembleEndpointSlice got error, expected: \n%v\nbut got: \n%v\n", tt.expectResult, reassembledEndpointSlice)
			}
		})
	}
}

// isEqualEndpointSlice is used to determine whether two endpointSlice are equal.
// Note that this function can only be used in this test.
func isEqualEndpointSliceV1beta1(endpointSlice1, endpointSlice2 *discoveryV1beta1.EndpointSlice) bool {
	if endpointSlice1.Name != endpointSlice2.Name ||
		endpointSlice1.Namespace != endpointSlice2.Namespace ||
		endpointSlice1.Labels[discoveryV1beta1.LabelServiceName] != endpointSlice2.Labels[discoveryV1beta1.LabelServiceName] {
		return false
	}

	endpoints1 := endpointSlice1.Endpoints
	endpoints2 := endpointSlice2.Endpoints
	if len(endpoints1) != len(endpoints2) {
		return false
	}

	for i := 0; i < len(endpoints1); i++ {
		if !isEqualStrings(endpoints1[i].Addresses, endpoints2[i].Addresses) {
			return false
		}

		if endpoints1[i].Topology[corev1.LabelHostname] != endpoints2[i].Topology[corev1.LabelHostname] {
			return false
		}
	}

	return true
}

func isEqualStrings(s1, s2 []string) bool {
	if len(s1) != len(s2) {
		return false
	}

	for i := 0; i < len(s1); i++ {
		if s1[i] != s2[i] {
			return false
		}
	}

	return true
}

func TestReassembleEndpointSlice(t *testing.T) {
	currentNodeName := "node1"
	nodeName2 := "node2"
	nodeName3 := "node3"

	testcases := map[string]struct {
		endpointSlice *discovery.EndpointSlice
		kubeClient    *k8sfake.Clientset
		yurtClient    *yurtfake.Clientset
		expectResult  *discovery.EndpointSlice
	}{
		"service with annotation openyurt.io/topologyKeys: kubernetes.io/hostname": {
			endpointSlice: &discovery.EndpointSlice{
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
		"service with annotation openyurt.io/topologyKeys: openyurt.io/nodepool": {
			endpointSlice: &discovery.EndpointSlice{
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
		"service with annotation openyurt.io/topologyKeys: kubernetes.io/zone": {
			endpointSlice: &discovery.EndpointSlice{
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
		"service without annotation openyurt.io/topologyKeys": {
			endpointSlice: &discovery.EndpointSlice{
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
		"currentNode is not in any nodepool": {
			endpointSlice: &discovery.EndpointSlice{
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
	}

	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			tt.kubeClient.DiscoveryV1().EndpointSlices("default").Create(context.TODO(), tt.endpointSlice, metav1.CreateOptions{})

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

			reassembledEndpointSlice := fh.reassembleEndpointSlice(tt.endpointSlice)

			if !isEqualEndpointSlice(reassembledEndpointSlice, tt.expectResult) {
				t.Errorf("reassembleEndpointSlice got error, expected: \n%v\nbut got: \n%v\n", tt.expectResult, reassembledEndpointSlice)
			}
		})
	}
}

func isEqualEndpointSlice(endpointSlice1, endpointSlice2 *discovery.EndpointSlice) bool {
	if endpointSlice1.Name != endpointSlice2.Name ||
		endpointSlice1.Namespace != endpointSlice2.Namespace ||
		endpointSlice1.Labels[discovery.LabelServiceName] != endpointSlice2.Labels[discovery.LabelServiceName] {
		return false
	}

	endpoints1 := endpointSlice1.Endpoints
	endpoints2 := endpointSlice2.Endpoints
	if len(endpoints1) != len(endpoints2) {
		return false
	}

	for i := 0; i < len(endpoints1); i++ {
		if !isEqualStrings(endpoints1[i].Addresses, endpoints2[i].Addresses) {
			return false
		}

		if endpoints1[i].NodeName != endpoints2[i].NodeName {
			return false
		}
	}

	return true
}
