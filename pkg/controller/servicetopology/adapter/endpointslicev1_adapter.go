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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewEndpointsV1Adapter(client client.Client) Adapter {
	return &endpointslicev1{
		client: client,
	}
}

type endpointslicev1 struct {
	client client.Client
}

func (s *endpointslicev1) GetEnqueueKeysBySvc(svc *corev1.Service) []string {
	var keys []string
	selector := getSvcSelector(discoveryv1.LabelServiceName, svc.Name)
	epSliceList := &discoveryv1.EndpointSliceList{}
	if err := s.client.List(context.TODO(), epSliceList, &client.ListOptions{Namespace: svc.Namespace, LabelSelector: selector}); err != nil {
		klog.V(4).Infof("Error listing endpointslices sets: %v", err)
		return keys
	}

	for _, epSlice := range epSliceList.Items {
		keys = appendKeys(keys, &epSlice)
	}
	return keys
}

func (s *endpointslicev1) UpdateTriggerAnnotations(namespace, name string) error {
	patch := getUpdateTriggerPatch()
	err := s.client.Patch(
		context.Background(),
		&discoveryv1.EndpointSlice{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      name,
			},
		},
		client.RawPatch(types.StrategicMergePatchType, patch), &client.PatchOptions{},
	)
	return err
}
