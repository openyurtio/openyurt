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
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestEndpointAdapterUpdateTriggerAnnotations(t *testing.T) {
	ep := getEndpoints("default", "svc1", "node1")

	kubeClient := fake.NewSimpleClientset(ep)
	stopper := make(chan struct{})
	defer close(stopper)
	c := fakeclient.NewClientBuilder().WithObjects(ep).Build()

	adapter := NewEndpointsAdapter(kubeClient, c)
	err := adapter.UpdateTriggerAnnotations(ep.Namespace, ep.Name)
	if err != nil {
		t.Errorf("update endpoints trigger annotations failed")
	}

	newEp, err := kubeClient.CoreV1().Endpoints(ep.Namespace).Get(context.TODO(), ep.Name, metav1.GetOptions{})
	if err != nil || ep.Annotations["openyurt.io/update-trigger"] == newEp.Annotations["openyurt.io/update-trigger"] {
		t.Errorf("update endpoints trigger annotations failed")
	}
}

func TestEndpointAdapterGetEnqueueKeysBySvc(t *testing.T) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "svc1",
			Namespace: "default",
		},
	}
	expectResult := []string{getCacheKey(svc)}

	ep := getEndpoints("default", "svc1", "node1")

	kubeClient := fake.NewSimpleClientset(ep)
	stopper := make(chan struct{})
	defer close(stopper)
	c := fakeclient.NewClientBuilder().WithObjects(ep).Build()
	adapter := NewEndpointsAdapter(kubeClient, c)

	keys := adapter.GetEnqueueKeysBySvc(svc)
	if !reflect.DeepEqual(keys, expectResult) {
		t.Errorf("expect enqueue keys %v, but got %v", expectResult, keys)
	}
}

func getEndpoints(ns, name string, nodes ...string) *corev1.Endpoints {
	var addresses []corev1.EndpointAddress
	for i := range nodes {
		addresses = append(addresses, corev1.EndpointAddress{NodeName: &nodes[i]})
	}
	return &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Subsets: []corev1.EndpointSubset{
			{
				Addresses: addresses,
			},
		},
	}
}

func getCacheKey(obj interface{}) string {
	key, _ := cache.MetaNamespaceKeyFunc(obj)
	return key
}
