/*
Copyright 2023 The OpenYurt Authors.

Licensed under the Apache License, Version 2.0 (the License);
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an AS IS BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package util

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openyurtio/openyurt/pkg/yurthub/filter/servicetopology"
)

func ServiceTopologyTypeChanged(oldSvc, newSvc *corev1.Service) bool {
	oldType := oldSvc.Annotations[servicetopology.AnnotationServiceTopologyKey]
	newType := newSvc.Annotations[servicetopology.AnnotationServiceTopologyKey]
	if oldType == newType {
		return false
	}
	return true
}

func GetSvcTopologyTypes(c client.Client) map[string]string {
	svcTopologyTypes := make(map[string]string)
	svcList := &corev1.ServiceList{}
	if err := c.List(context.TODO(), svcList, &client.ListOptions{LabelSelector: labels.Everything()}); err != nil {
		klog.V(4).Infof("failed to list service sets: %v", err)
		return svcTopologyTypes
	}

	for _, svc := range svcList.Items {
		topologyType, ok := svc.Annotations[servicetopology.AnnotationServiceTopologyKey]
		if !ok {
			continue
		}

		key, err := cache.MetaNamespaceKeyFunc(svc)
		if err != nil {
			runtime.HandleError(err)
			continue
		}
		svcTopologyTypes[key] = topologyType
	}
	return svcTopologyTypes
}
