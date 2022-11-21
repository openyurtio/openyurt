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
	"fmt"
	"reflect"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	discoveryv1beta1 "k8s.io/api/discovery/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	discorveryV1beta1listers "k8s.io/client-go/listers/discovery/v1beta1"
)

func TestEndpointSliceV1Beta1AdapterGetEnqueueKeysByNodePool(t *testing.T) {
	svcName := "svc1"
	svcNamespace := "default"
	svcKey := fmt.Sprintf("%s/%s", svcNamespace, svcName)
	nodeName1 := "node1"
	nodeName2 := "node2"
	tcases := map[string]struct {
		kubeClient       kubernetes.Interface
		nodepoolNodes    sets.String
		svcTopologyTypes map[string]string
		expectResult     []string
	}{
		"service topology type: kubernetes.io/hostname": {
			kubeClient: fake.NewSimpleClientset(
				getV1Beta1EndpointSlice(svcNamespace, svcName, nodeName1),
			),
			nodepoolNodes: sets.NewString(nodeName1),
			svcTopologyTypes: map[string]string{
				svcKey: "kubernetes.io/hostname",
			},
			expectResult: nil,
		},
		"service topology type: openyurt.io/nodepool, don't contain nodepool nodes": {
			kubeClient: fake.NewSimpleClientset(
				getV1Beta1EndpointSlice(svcNamespace, svcName, nodeName1),
			),
			nodepoolNodes: sets.NewString(nodeName2),
			svcTopologyTypes: map[string]string{
				svcKey: "openyurt.io/nodepool",
			},
			expectResult: nil,
		},
		"service topology type: kubernetes.io/zone, contain nodepool nodes": {
			kubeClient: fake.NewSimpleClientset(
				getV1Beta1EndpointSlice(svcNamespace, svcName, nodeName1),
			),
			nodepoolNodes: sets.NewString(nodeName1),
			svcTopologyTypes: map[string]string{
				svcKey: "kubernetes.io/zone",
			},
			expectResult: []string{
				getCacheKey(getV1Beta1EndpointSlice(svcNamespace, svcName, nodeName1)),
			},
		},
	}

	for k, tt := range tcases {
		t.Logf("current test case is %s", k)
		factory := informers.NewSharedInformerFactory(tt.kubeClient, 24*time.Hour)
		endpointSliceLister := factory.Discovery().V1beta1().EndpointSlices().Lister()

		stopper := make(chan struct{})
		defer close(stopper)
		factory.Start(stopper)
		factory.WaitForCacheSync(stopper)

		adapter := NewEndpointsV1Beta1Adapter(tt.kubeClient, endpointSliceLister)
		keys := adapter.GetEnqueueKeysByNodePool(tt.svcTopologyTypes, tt.nodepoolNodes)
		if !reflect.DeepEqual(keys, tt.expectResult) {
			t.Errorf("expect enqueue keys %v, but got %v", tt.expectResult, keys)
		}

	}
}

func TestEndpointSliceV1Beta1AdapterUpdateTriggerAnnotations(t *testing.T) {
	svcName := "svc1"
	svcNamespace := "default"
	epSlice := getV1Beta1EndpointSlice(svcNamespace, svcName, "node1")

	kubeClient := fake.NewSimpleClientset(epSlice)
	stopper := make(chan struct{})
	defer close(stopper)
	epSliceLister := getV1Beta1EndpointSliceLister(kubeClient, stopper)
	adapter := NewEndpointsV1Beta1Adapter(kubeClient, epSliceLister)
	err := adapter.UpdateTriggerAnnotations(epSlice.Namespace, epSlice.Name)
	if err != nil {
		t.Errorf("update endpointsSlice trigger annotations failed")
	}

	newEpSlice, err := kubeClient.DiscoveryV1beta1().EndpointSlices(epSlice.Namespace).Get(context.TODO(), epSlice.Name, metav1.GetOptions{})
	if err != nil || epSlice.Annotations["openyurt.io/update-trigger"] == newEpSlice.Annotations["openyurt.io/update-trigger"] {
		t.Errorf("update endpoints trigger annotations failed")
	}
}

func TestEndpointSliceV1Beta1AdapterGetEnqueueKeysBySvc(t *testing.T) {
	svcName := "svc1"
	svcNamespace := "default"
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      svcName,
			Namespace: svcNamespace,
		},
	}
	epSlice := getV1Beta1EndpointSlice(svcNamespace, svcName, "node1")
	expectResult := []string{getCacheKey(epSlice)}

	stopper := make(chan struct{})
	defer close(stopper)
	kubeClient := fake.NewSimpleClientset(epSlice)
	epSliceLister := getV1Beta1EndpointSliceLister(kubeClient, stopper)
	adapter := NewEndpointsV1Beta1Adapter(kubeClient, epSliceLister)

	keys := adapter.GetEnqueueKeysBySvc(svc)
	if !reflect.DeepEqual(keys, expectResult) {
		t.Errorf("expect enqueue keys %v, but got %v", expectResult, keys)
	}
}

func getV1Beta1EndpointSlice(svcNamespace, svcName string, nodes ...string) *discoveryv1beta1.EndpointSlice {
	var endpoints []discoveryv1beta1.Endpoint
	for i := range nodes {
		endpoints = append(endpoints,
			discoveryv1beta1.Endpoint{
				Topology: map[string]string{
					corev1.LabelHostname: nodes[i],
				},
			},
		)
	}
	return &discoveryv1beta1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-xad21", svcName),
			Namespace: svcNamespace,
			Labels: map[string]string{
				discoveryv1beta1.LabelServiceName: svcName,
			},
		},
		Endpoints: endpoints,
	}
}

func getV1Beta1EndpointSliceLister(client kubernetes.Interface, stopCh <-chan struct{}) discorveryV1beta1listers.EndpointSliceLister {
	factory := informers.NewSharedInformerFactory(client, 24*time.Hour)
	endpointSliceLister := factory.Discovery().V1beta1().EndpointSlices().Lister()

	factory.Start(stopCh)
	factory.WaitForCacheSync(stopCh)
	return endpointSliceLister
}
