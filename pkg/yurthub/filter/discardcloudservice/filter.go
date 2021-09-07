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
	"io"
	"net/http"

	"k8s.io/klog"

	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
	filterutil "github.com/openyurtio/openyurt/pkg/yurthub/filter/util"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/serializer"
)

// discardcloudservice filter is used to discard cloud service(like loadBalancer service)
// on kube-proxy list/watch service request from edge nodes.
const FilterName = "discardcloudservice"

// Register registers a filter
func Register(filters *filter.Filters) {
	filters.Register(FilterName, func() (filter.Interface, error) {
		return NewFilter(), nil
	})
}

func NewFilter() *discardCloudServiceFilter {
	return &discardCloudServiceFilter{
		Approver: filter.NewApprover("kube-proxy", "services", []string{"list", "watch"}...),
	}
}

type discardCloudServiceFilter struct {
	*filter.Approver
	serializerManager *serializer.SerializerManager
}

func (sf *discardCloudServiceFilter) SetSerializerManager(s *serializer.SerializerManager) error {
	sf.serializerManager = s
	return nil
}

func (sf *discardCloudServiceFilter) Filter(req *http.Request, rc io.ReadCloser, stopCh <-chan struct{}) (int, io.ReadCloser, error) {
	s := filterutil.CreateSerializer(req, sf.serializerManager)
	if s == nil {
		klog.Errorf("skip filter, failed to create serializer in discardCloudServiceFilter")
		return 0, rc, nil
	}

	handler := NewDiscardCloudServiceFilterHandler(s)
	return filter.NewFilterReadCloser(req, rc, handler, s, stopCh)
}
