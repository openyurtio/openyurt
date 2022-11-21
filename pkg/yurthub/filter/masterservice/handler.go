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
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
)

const (
	MasterServiceNamespace = "default"
	MasterServiceName      = "kubernetes"
	MasterServicePortName  = "https"
)

type masterServiceFilterHandler struct {
	host string
	port int32
}

func NewMasterServiceFilterHandler(host string, port int32) filter.ObjectHandler {
	return &masterServiceFilterHandler{
		host: host,
		port: port,
	}
}

// RuntimeObjectFilter mutate master service(default/kubernetes) in the response object
func (fh *masterServiceFilterHandler) RuntimeObjectFilter(obj runtime.Object) (runtime.Object, bool) {
	switch v := obj.(type) {
	case *v1.ServiceList:
		for i := range v.Items {
			newSvc, mutated := fh.mutateMasterService(&v.Items[i])
			if mutated {
				v.Items[i] = *newSvc
				break
			}
		}
		return v, false
	case *v1.Service:
		svc, _ := fh.mutateMasterService(v)
		return svc, false
	default:
		return v, false
	}
}

func (fh *masterServiceFilterHandler) mutateMasterService(svc *v1.Service) (*v1.Service, bool) {
	mutated := false
	if svc.Namespace == MasterServiceNamespace && svc.Name == MasterServiceName {
		svc.Spec.ClusterIP = fh.host
		for j := range svc.Spec.Ports {
			if svc.Spec.Ports[j].Name == MasterServicePortName {
				svc.Spec.Ports[j].Port = fh.port
				break
			}
		}
		mutated = true
		klog.V(2).Infof("mutate master service with ClusterIP:Port=%s:%d", fh.host, fh.port)
	}
	return svc, mutated
}
