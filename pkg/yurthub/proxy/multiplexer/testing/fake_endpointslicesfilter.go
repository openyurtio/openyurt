/*
Copyright 2024 The OpenYurt Authors.

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

package testing

import (
	discovery "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
)

type IgnoreEndpointslicesWithNodeName struct {
	IgnoreNodeName string
}

func (ie *IgnoreEndpointslicesWithNodeName) Name() string {
	return "ignoreendpointsliceswithname"
}

func (ie *IgnoreEndpointslicesWithNodeName) SupportedResourceAndVerbs() map[string]sets.Set[string] {
	return nil
}

// Filter is used for filtering runtime object
// all filter logic should be located in it.
func (ie *IgnoreEndpointslicesWithNodeName) Filter(obj runtime.Object, stopCh <-chan struct{}) runtime.Object {
	endpointslice, ok := obj.(*discovery.EndpointSlice)
	if !ok {
		return obj
	}

	var newEps []discovery.Endpoint

	for _, ep := range endpointslice.Endpoints {
		if *ep.NodeName != ie.IgnoreNodeName {
			newEps = append(newEps, ep)
		}
	}
	endpointslice.Endpoints = newEps

	return endpointslice
}
