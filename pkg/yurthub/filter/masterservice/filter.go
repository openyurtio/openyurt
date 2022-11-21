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
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
	filterutil "github.com/openyurtio/openyurt/pkg/yurthub/filter/util"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/serializer"
)

// Register registers a filter
func Register(filters *filter.Filters) {
	filters.Register(filter.MasterServiceFilterName, func() (filter.Runner, error) {
		return NewFilter(), nil
	})
}

func NewFilter() *masterServiceFilter {
	return &masterServiceFilter{}
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

func (msf *masterServiceFilter) SetSerializerManager(s *serializer.SerializerManager) error {
	msf.serializerManager = s
	return nil
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
	s := filterutil.CreateSerializer(req, msf.serializerManager)
	if s == nil {
		klog.Errorf("skip filter, failed to create serializer in masterServiceFilter")
		return 0, rc, nil
	}

	handler := NewMasterServiceFilterHandler(req, s, msf.host, msf.port)
	return filter.NewFilterReadCloser(req, rc, handler, s, filter.MasterServiceFilterName, stopCh)
}
