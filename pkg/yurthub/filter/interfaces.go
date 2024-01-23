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

package filter

import (
	"io"
	"net/http"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
)

type NodesInPoolGetter func(poolName string) ([]string, error)

type Initializer interface {
	Initialize(filter ObjectFilter) error
}

// Approver check the response of specified request need to go through filter or not.
// and get all filters' names for the specified request.
type Approver interface {
	Approve(req *http.Request) (bool, []string)
}

// ResponseFilter is used for filtering response for get/list/watch requests.
// it only prepares the common framework for ObjectFilter and don't cover
// the filter logic.
type ResponseFilter interface {
	Name() string
	// Filter is used to filter data returned from the cloud.
	Filter(req *http.Request, rc io.ReadCloser, stopCh <-chan struct{}) (int, io.ReadCloser, error)
}

// ObjectFilter is used for filtering runtime object.
// runtime object includes List object(like ServiceList) that has multiple items and
// Standalone object(like Service).
// Every Filter need to implement ObjectFilter interface.
type ObjectFilter interface {
	Name() string
	// SupportedResourceAndVerbs is used to specify which resource and request verb is supported by the filter.
	// Because each filter can make sure what requests with resource and verb can be handled.
	SupportedResourceAndVerbs() map[string]sets.String
	// Filter is used for filtering runtime object
	// all filter logic should be located in it.
	Filter(obj runtime.Object, stopCh <-chan struct{}) runtime.Object
}

type NodeGetter func(name string) (*v1.Node, error)
