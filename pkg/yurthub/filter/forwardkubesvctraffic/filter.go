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

package forwardkubesvctraffic

import (
	"strconv"

	discovery "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
	utilnet "k8s.io/utils/net"

	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/base"
)

const (
	// FilterName filter is used to mutate the default/kubernetes endpointslices
	// in order to make pods on edge nodes can access kube-apiserver directly by default/kubernetes service.
	FilterName = "forwardkubesvctraffic"

	KubeSVCNamespace = "default"
	KubeSVCName      = "kubernetes"
	KubeSVCPortName  = "https"
)

// Register registers a filter
func Register(filters *base.Filters) {
	filters.Register(FilterName, func() (filter.ObjectFilter, error) {
		return NewForwardKubeSVCTrafficFilter()
	})
}

func NewForwardKubeSVCTrafficFilter() (*forwardKubeSVCTrafficFilter, error) {
	return &forwardKubeSVCTrafficFilter{addressType: discovery.AddressTypeIPv4}, nil
}

type forwardKubeSVCTrafficFilter struct {
	addressType discovery.AddressType
	host        string
	port        int32
}

func (fkst *forwardKubeSVCTrafficFilter) Name() string {
	return FilterName
}

func (fkst *forwardKubeSVCTrafficFilter) SupportedResourceAndVerbs() map[string]sets.String {
	return map[string]sets.String{
		"endpointslices": sets.NewString("list", "watch"),
	}
}

func (fkst *forwardKubeSVCTrafficFilter) SetMasterServiceHost(host string) error {
	fkst.host = host
	if utilnet.IsIPv6String(host) {
		fkst.addressType = discovery.AddressTypeIPv6
	}
	return nil

}

func (fkst *forwardKubeSVCTrafficFilter) SetMasterServicePort(portStr string) error {
	port, err := strconv.ParseInt(portStr, 10, 32)
	if err != nil {
		return err
	}
	fkst.port = int32(port)
	return nil
}

func (fkst *forwardKubeSVCTrafficFilter) Filter(obj runtime.Object, stopCh <-chan struct{}) runtime.Object {
	switch v := obj.(type) {
	case *discovery.EndpointSlice:
		fkst.mutateDefaultKubernetesEps(v)
		return v
	default:
		return obj
	}
}

func (fkst *forwardKubeSVCTrafficFilter) mutateDefaultKubernetesEps(eps *discovery.EndpointSlice) {
	trueCondition := true
	if eps.Namespace == KubeSVCNamespace && eps.Name == KubeSVCName {
		if eps.AddressType != fkst.addressType {
			klog.Warningf("address type of default/kubernetes endpoinstlice(%s) and hub server is different(%s), hub server address type need to be configured", eps.AddressType, fkst.addressType)
			return
		}
		for j := range eps.Ports {
			if eps.Ports[j].Name != nil && *eps.Ports[j].Name == KubeSVCPortName {
				eps.Ports[j].Port = &fkst.port
				break
			}
		}
		eps.Endpoints = []discovery.Endpoint{
			{
				Addresses: []string{fkst.host},
				Conditions: discovery.EndpointConditions{
					Ready: &trueCondition,
				},
			},
		}
		klog.V(2).Infof("mutate default/kubernetes endpointslice to %v in forwardkubesvctraffic filter", *eps)
	}
	return
}
