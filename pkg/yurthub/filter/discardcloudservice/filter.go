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

	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/serializer"
)

// Register registers a filter
func Register(filters *filter.Filters, sm *serializer.SerializerManager) {
	filters.Register(filter.DiscardCloudServiceFilterName, func() (filter.Runner, error) {
		return NewFilter(sm), nil
	})
}

func NewFilter(sm *serializer.SerializerManager) *discardCloudServiceFilter {
	return &discardCloudServiceFilter{
		serializerManager: sm,
	}
}

type discardCloudServiceFilter struct {
	serializerManager *serializer.SerializerManager
}

func (sf *discardCloudServiceFilter) Name() string {
	return filter.DiscardCloudServiceFilterName
}

func (sf *discardCloudServiceFilter) SupportedResourceAndVerbs() map[string]sets.String {
	return map[string]sets.String{
		"services": sets.NewString("list", "watch"),
	}
}

func (sf *discardCloudServiceFilter) Filter(req *http.Request, rc io.ReadCloser, stopCh <-chan struct{}) (int, io.ReadCloser, error) {
	return filter.NewFilterReadCloser(req, sf.serializerManager, rc, NewDiscardCloudServiceFilterHandler(), sf.Name(), stopCh)
}
