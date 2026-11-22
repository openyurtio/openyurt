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
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/base"
)

const (
	// FilterName filter is used to discard cloud service(like loadBalancer service)
	// on kube-proxy list/watch service request from edge nodes.
	FilterName = "discardcloudservice"

	// DiscardServiceAnnotation is annotation used by LB service.
	// If end users want to discard specified LB service at the edge side,
	// End users should add annotation["svc.openyurt.io/discard"]="true" for LB service.
	DiscardServiceAnnotation = "svc.openyurt.io/discard"

	// DiscardClusterIPAnnotation is annotation used to discard clusterIP service.
	// If end users want to discard specified clusterIP service at the edge side,
	// End users should add annotation["svc.openyurt.io/discard-clusterip"]="true" for the service.
	DiscardClusterIPAnnotation = "svc.openyurt.io/discard-clusterip"
)

var (
	// cloudClusterIPService contains hardcoded services to discard (backward compatibility).
	// Deprecated: use DiscardClusterIPAnnotation instead.
	cloudClusterIPService = map[string]struct{}{
		"kube-system/x-tunnel-server-internal-svc": {},
	}
)

// Register registers a filter
func Register(filters *base.Filters) {
	filters.Register(FilterName, func() (filter.ObjectFilter, error) {
		return NewDiscardCloudServiceFilter()
	})
}

type discardCloudServiceFilter struct{}

func NewDiscardCloudServiceFilter() (filter.ObjectFilter, error) {
	return &discardCloudServiceFilter{}, nil
}

func (sf *discardCloudServiceFilter) Name() string {
	return FilterName
}

func (sf *discardCloudServiceFilter) Filter(obj runtime.Object, _ <-chan struct{}) runtime.Object {
	switch v := obj.(type) {
	case *v1.Service:
		return discardCloudService(v)
	default:
		return v
	}
}

func discardCloudService(svc *v1.Service) runtime.Object {
	nsName := fmt.Sprintf("%s/%s", svc.Namespace, svc.Name)

	// 1. remove cloud LoadBalancer service
	if svc.Spec.Type == v1.ServiceTypeLoadBalancer {
		if svc.Annotations[DiscardServiceAnnotation] == "true" {
			klog.V(2).Infof("load balancer service(%s) is discarded in StreamResponseFilter of discardCloudServiceFilterHandler", nsName)
			return nil
		}
	}

	// 2. remove cloud clusterIP service - check annotation first (preferred)
	if svc.Annotations[DiscardClusterIPAnnotation] == "true" {
		klog.V(2).Infof("clusterIP service(%s) is discarded via annotation in discardCloudServiceFilterHandler", nsName)
		return nil
	}

	// 3. fallback to hardcoded list (backward compatibility)
	if _, ok := cloudClusterIPService[nsName]; ok {
		klog.V(2).Infof("clusterIP service(%s) is discarded via hardcoded list in discardCloudServiceFilterHandler", nsName)
		return nil
	}

	return svc
}
