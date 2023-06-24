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
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewEndpointsAdapter(client client.Client) Adapter {
	return &endpoints{
		client: client,
	}
}

type endpoints struct {
	client client.Client
}

func (s *endpoints) GetEnqueueKeysBySvc(svc *corev1.Service) []string {
	var keys []string
	return appendKeys(keys, svc)
}

func (s *endpoints) UpdateTriggerAnnotations(namespace, name string) error {
	patch := getUpdateTriggerPatch()
	err := s.client.Patch(
		context.Background(),
		&corev1.Endpoints{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      name,
			},
		},
		client.RawPatch(types.StrategicMergePatchType, patch), &client.PatchOptions{},
	)
	return err
}
