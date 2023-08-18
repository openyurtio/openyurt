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
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/cache"
)

type Adapter interface {
	GetEnqueueKeysBySvc(svc *corev1.Service) []string
	UpdateTriggerAnnotations(namespace, name string) error
}

func getSvcSelector(key, value string) labels.Selector {
	return labels.SelectorFromSet(
		map[string]string{
			key: value,
		},
	)
}

func appendKeys(keys []string, obj interface{}) []string {
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		runtime.HandleError(err)
		return nil
	}
	keys = append(keys, key)
	return keys
}

func getUpdateTriggerPatch() []byte {
	patch := fmt.Sprintf(`{"metadata":{"annotations": {"openyurt.io/update-trigger": "%d"}}}`, time.Now().Unix())
	return []byte(patch)
}
