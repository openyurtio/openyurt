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

	corev1 "k8s.io/api/core/v1"
	discoveryv1beta1 "k8s.io/api/discovery/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestEndpointSliceV1Beta1AdapterUpdateTriggerAnnotations(t *testing.T) {
	svcName := "svc1"
	svcNamespace := "default"
	epSlice := getV1Beta1EndpointSlice(svcNamespace, svcName, "node1")

	c := fakeclient.NewClientBuilder().WithObjects(epSlice).Build()
	stopper := make(chan struct{})
	defer close(stopper)
	adapter := NewEndpointsV1Beta1Adapter(c)
	err := adapter.UpdateTriggerAnnotations(epSlice.Namespace, epSlice.Name)
	if err != nil {
		t.Errorf("update endpointsSlice trigger annotations failed")
	}
	newEpSlice := &discoveryv1beta1.EndpointSlice{}
	err = c.Get(context.TODO(), types.NamespacedName{Namespace: epSlice.Namespace, Name: epSlice.Name}, newEpSlice)
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
	c := fakeclient.NewClientBuilder().WithObjects(epSlice).Build()
	adapter := NewEndpointsV1Beta1Adapter(c)

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
