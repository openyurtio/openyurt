/*
Copyright 2024 The OpenYurt Authors.

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

package serviceenvupdater

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/base"
)

const (
	// FilterName filter is used to mutate the Kubernetes service host and port
	// in order for pods on edge nodes to access kube-apiserver via Yurthub proxy
	FilterName = "serviceenvupdater"

	envVarServiceHost = "KUBERNETES_SERVICE_HOST"
	envVarServicePort = "KUBERNETES_SERVICE_PORT"
)

// Register registers a filter
func Register(filters *base.Filters) {
	filters.Register(FilterName, func() (filter.ObjectFilter, error) {
		return NewServiceEnvUpdaterFilter()
	})
}

type serviceEnvUpdaterFilter struct {
	host string
	port string
}

func NewServiceEnvUpdaterFilter() (*serviceEnvUpdaterFilter, error) {
	return &serviceEnvUpdaterFilter{}, nil
}

func (sef *serviceEnvUpdaterFilter) Name() string {
	return FilterName
}

func (sef *serviceEnvUpdaterFilter) SetMasterServiceHost(host string) error {
	sef.host = host
	return nil
}

func (sef *serviceEnvUpdaterFilter) SetMasterServicePort(port string) error {
	sef.port = port
	return nil
}

func (sef *serviceEnvUpdaterFilter) Filter(obj runtime.Object, _ <-chan struct{}) runtime.Object {
	switch v := obj.(type) {
	case *corev1.Pod:
		return sef.mutatePodEnv(v)
	default:
		return v
	}
}

func (sef *serviceEnvUpdaterFilter) mutatePodEnv(req *corev1.Pod) *corev1.Pod {
	for i := range req.Spec.Containers {
		foundHost := false
		foundPort := false

		for j, envVar := range req.Spec.Containers[i].Env {
			switch envVar.Name {
			case envVarServiceHost:
				req.Spec.Containers[i].Env[j].Value = sef.host
				foundHost = true
			case envVarServicePort:
				req.Spec.Containers[i].Env[j].Value = sef.port
				foundPort = true
			}

			if foundHost && foundPort {
				break
			}
		}
	}
	return req
}
