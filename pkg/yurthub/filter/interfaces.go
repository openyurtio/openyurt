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
	"k8s.io/apimachinery/pkg/watch"
)

type FilterInitializer interface {
	Initialize(filter Interface) error
}

// Interface of data filtering framework.
type Interface interface {
	// Approve is used to determine whether the data returned
	// from the cloud needs to enter the filtering framework for processing.
	Approve(comp, resource, verb string) bool

	// Filter is used to filter data returned from the cloud.
	Filter(req *http.Request, rc io.ReadCloser, stopCh <-chan struct{}) (int, io.ReadCloser, error)
}

// Handler customizes data filtering processing interface for each handler.
// In the data filtering framework, data is mainly divided into two types:
// 	Object data: data returned by list/get request.
// 	Streaming data: The data returned by the watch request will be continuously pushed to the edge by the cloud.
type Handler interface {
	// StreamResponseFilter is used to filter processing of streaming data.
	StreamResponseFilter(rc io.ReadCloser, ch chan watch.Event) error

	// ObjectResponseFilter is used to filter processing of object data.
	ObjectResponseFilter(b []byte) ([]byte, error)
}

type NodeGetter func(name string) (*v1.Node, error)
