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

package podenvupdater

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/base"
)

const (
	// FilterName filter is used to mutate the Kubernetes service host
	// in order for pods on edge nodes to access kube-apiserver via Yurthub proxy
	FilterName = "podenvupdater"

	envVarServiceHost = "KUBERNETES_SERVICE_HOST"
)

// Register registers a filter
func Register(filters *base.Filters) {
	filters.Register(FilterName, func() (filter.ObjectFilter, error) {
		return NewPodEnvFilter()
	})
}

type podEnvFilter struct {
	host string
}

func NewPodEnvFilter() (*podEnvFilter, error) {
	return &podEnvFilter{}, nil
}

func (pef *podEnvFilter) Name() string {
	return FilterName
}

func (pef *podEnvFilter) SupportedResourceAndVerbs() map[string]sets.Set[string] {
	return map[string]sets.Set[string]{
		"pods": sets.New("list", "watch", "get", "patch"),
	}
}

func (pef *podEnvFilter) SetMasterServiceHost(host string) error {
	pef.host = host
	return nil
}

func (pef *podEnvFilter) SetMasterServicePort(portStr string) error {
	// do nothing
	return nil
}

func (pef *podEnvFilter) Filter(obj runtime.Object, _ <-chan struct{}) runtime.Object {
	switch v := obj.(type) {
	case *corev1.Pod:
		return pef.mutatePodEnv(v)
	default:
		return v
	}
}

func (pef *podEnvFilter) mutatePodEnv(req *corev1.Pod) *corev1.Pod {
	for i := range req.Spec.Containers {
		for j, envVar := range req.Spec.Containers[i].Env {
			if envVar.Name == envVarServiceHost {
				req.Spec.Containers[i].Env[j].Value = pef.host
				break
			}
		}
	}
	return req
}
