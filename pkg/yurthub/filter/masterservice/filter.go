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
	"strconv"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
)

const (
	MasterServiceNamespace = "default"
	MasterServiceName      = "kubernetes"
	MasterServicePortName  = "https"
)

// Register registers a filter
func Register(filters *filter.Filters) {
	filters.Register(filter.MasterServiceFilterName, func() (filter.ObjectFilter, error) {
		return &masterServiceFilter{}, nil
	})
}

type masterServiceFilter struct {
	host string
	port int32
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

func (msf *masterServiceFilter) Filter(obj runtime.Object, _ <-chan struct{}) runtime.Object {
	switch v := obj.(type) {
	case *v1.ServiceList:
		for i := range v.Items {
			newSvc, mutated := msf.mutateMasterService(&v.Items[i])
			if mutated {
				v.Items[i] = *newSvc
				break
			}
		}
		return v
	case *v1.Service:
		svc, _ := msf.mutateMasterService(v)
		return svc
	default:
		return v
	}
}

func (msf *masterServiceFilter) mutateMasterService(svc *v1.Service) (*v1.Service, bool) {
	mutated := false
	if svc.Namespace == MasterServiceNamespace && svc.Name == MasterServiceName {
		svc.Spec.ClusterIP = msf.host
		for j := range svc.Spec.Ports {
			if svc.Spec.Ports[j].Name == MasterServicePortName {
				svc.Spec.Ports[j].Port = msf.port
				break
			}
		}
		mutated = true
		klog.V(2).Infof("mutate master service with ClusterIP:Port=%s:%d", msf.host, msf.port)
	}
	return svc, mutated
}
