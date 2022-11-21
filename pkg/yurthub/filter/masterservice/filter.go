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

package masterservice

import (
	"io"
	"net/http"
	"strconv"

	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/serializer"
)

// Register registers a filter
func Register(filters *filter.Filters, sm *serializer.SerializerManager) {
	filters.Register(filter.MasterServiceFilterName, func() (filter.Runner, error) {
		return NewFilter(sm), nil
	})
}

func NewFilter(sm *serializer.SerializerManager) *masterServiceFilter {
	return &masterServiceFilter{
		serializerManager: sm,
	}
}

type masterServiceFilter struct {
	serializerManager *serializer.SerializerManager
	host              string
	port              int32
}

func (msf *masterServiceFilter) Name() string {
	return filter.MasterServiceFilterName
}

func (msf *masterServiceFilter) SupportedResourceAndVerbs() map[string]sets.String {
	return map[string]sets.String{
		"services": sets.NewString("list", "watch"),
	}
}

func (msf *masterServiceFilter) SetMasterServiceHost(host string) error {
	msf.host = host
	return nil

}

func (msf *masterServiceFilter) SetMasterServicePort(portStr string) error {
	port, err := strconv.ParseInt(portStr, 10, 32)
	if err != nil {
		return err
	}
	msf.port = int32(port)
	return nil
}

func (msf *masterServiceFilter) Filter(req *http.Request, rc io.ReadCloser, stopCh <-chan struct{}) (int, io.ReadCloser, error) {
	handler := NewMasterServiceFilterHandler(msf.host, msf.port)
	return filter.NewFilterReadCloser(req, msf.serializerManager, rc, handler, msf.Name(), stopCh)
}
