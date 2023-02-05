/*
Copyright 2021 The OpenYurt Authors.

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

package discardcloudservice

import (
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
)

var (
	cloudClusterIPService = map[string]struct{}{
		"kube-system/x-tunnel-server-internal-svc": {},
	}
)

// Register registers a filter
func Register(filters *filter.Filters) {
	filters.Register(filter.DiscardCloudServiceFilterName, func() (filter.ObjectFilter, error) {
		return &discardCloudServiceFilter{}, nil
	})
}

type discardCloudServiceFilter struct{}

func (sf *discardCloudServiceFilter) Name() string {
	return filter.DiscardCloudServiceFilterName
}

func (sf *discardCloudServiceFilter) SupportedResourceAndVerbs() map[string]sets.String {
	return map[string]sets.String{
		"services": sets.NewString("list", "watch"),
	}
}

func (sf *discardCloudServiceFilter) Filter(obj runtime.Object, _ <-chan struct{}) runtime.Object {
	switch v := obj.(type) {
	case *v1.ServiceList:
		var svcNew []v1.Service
		for i := range v.Items {
			svc := discardCloudService(&v.Items[i])
			if svc != nil {
				svcNew = append(svcNew, *svc)
			}
		}
		v.Items = svcNew
		return v
	case *v1.Service:
		return discardCloudService(v)
	default:
		return v
	}
}

func discardCloudService(svc *v1.Service) *v1.Service {
	nsName := fmt.Sprintf("%s/%s", svc.Namespace, svc.Name)
	// remove cloud LoadBalancer service
	if svc.Spec.Type == v1.ServiceTypeLoadBalancer {
		if svc.Annotations[filter.SkipDiscardServiceAnnotation] != "true" {
			klog.V(2).Infof("load balancer service(%s) is discarded in StreamResponseFilter of discardCloudServiceFilterHandler", nsName)
			return nil
		}
	}

	// remove cloud clusterIP service
	if _, ok := cloudClusterIPService[nsName]; ok {
		klog.V(2).Infof("clusterIP service(%s) is discarded in StreamResponseFilter of discardCloudServiceFilterHandler", nsName)
		return nil
	}

	return svc
}
