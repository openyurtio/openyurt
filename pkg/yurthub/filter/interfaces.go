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
	Filter(req *http.Request, rc io.ReadCloser) (int, io.ReadCloser, error)
}

// ObjectFilter is used for filtering runtime object.
// runtime object is only a standalone object(like Service).
// Every Filter need to implement ObjectFilter interface.
type ObjectFilter interface {
	Name() string
	// Filter is used for filtering runtime object
	// all filter logic should be located in it.
	Filter(obj runtime.Object) runtime.Object
}

type FilterFinder interface {
	FindResponseFilter(req *http.Request) (ResponseFilter, bool)
	FindObjectFilter(req *http.Request) (ObjectFilter, bool)
	ResourceSyncer
}

type NodeGetter func(name string) (*v1.Node, error)

// ResourceSyncer is used for verifying the resources which filter depends on has been synced or not.
// For example: servicetopology filter depends on service and nodebucket metadata, filter can be worked
// before all these metadata has been synced completely.
type ResourceSyncer interface {
	HasSynced() bool
}
