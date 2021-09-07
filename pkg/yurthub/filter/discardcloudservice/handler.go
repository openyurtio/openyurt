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
	"fmt"
	"io"

	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/serializer"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/klog"
)

var (
	cloudClusterIPService = map[string]struct{}{
		"kube-system/x-tunnel-server-internal-svc": {},
	}
)

type discardCloudServiceFilterHandler struct {
	serializer *serializer.Serializer
}

func NewDiscardCloudServiceFilterHandler(serializer *serializer.Serializer) filter.Handler {
	return &discardCloudServiceFilterHandler{
		serializer: serializer,
	}
}

// ObjectResponseFilter remove the cloud service(like LoadBalancer service) from response object
func (fh *discardCloudServiceFilterHandler) ObjectResponseFilter(b []byte) ([]byte, error) {
	list, err := fh.serializer.Decode(b)
	if err != nil || list == nil {
		klog.Errorf("skip filter, failed to decode response in ObjectResponseFilter of discardCloudServiceFilterHandler %v", err)
		return b, nil
	}

	serviceList, ok := list.(*v1.ServiceList)
	if ok {
		var svcNew []v1.Service
		for i := range serviceList.Items {
			nsName := fmt.Sprintf("%s/%s", serviceList.Items[i].Namespace, serviceList.Items[i].Name)
			// remove lb service
			if serviceList.Items[i].Spec.Type == v1.ServiceTypeLoadBalancer {
				klog.V(2).Infof("load balancer service(%s) is discarded in ObjectResponseFilter of discardCloudServiceFilterHandler", nsName)
				continue
			}

			// remove cloud clusterIP service
			if _, ok := cloudClusterIPService[nsName]; ok {
				klog.V(2).Infof("clusterIP service(%s) is discarded in ObjectResponseFilter of discardCloudServiceFilterHandler", nsName)
				continue
			}

			svcNew = append(svcNew, serviceList.Items[i])
		}
		serviceList.Items = svcNew
		return fh.serializer.Encode(serviceList)
	}

	return b, nil
}

// StreamResponseFilter filter the cloud service(like LoadBalancer service) from watch stream response
func (fh *discardCloudServiceFilterHandler) StreamResponseFilter(rc io.ReadCloser, ch chan watch.Event) error {
	defer func() {
		close(ch)
	}()

	d, err := fh.serializer.WatchDecoder(rc)
	if err != nil {
		klog.Errorf("StreamResponseFilter for discardCloudServiceFilterHandler ended with error, %v", err)
		return err
	}

	for {
		watchType, obj, err := d.Decode()
		if err != nil {
			return err
		}

		service, ok := obj.(*v1.Service)
		if ok {
			nsName := fmt.Sprintf("%s/%s", service.Namespace, service.Name)
			// remove cloud LoadBalancer service
			if service.Spec.Type == v1.ServiceTypeLoadBalancer {
				klog.V(2).Infof("load balancer service(%s) is discarded in StreamResponseFilter of discardCloudServiceFilterHandler", nsName)
				continue
			}

			// remove cloud clusterIP service
			if _, ok := cloudClusterIPService[nsName]; ok {
				klog.V(2).Infof("clusterIP service(%s) is discarded in StreamResponseFilter of discardCloudServiceFilterHandler", nsName)
				continue
			}
		}

		var wEvent watch.Event
		wEvent.Type = watchType
		wEvent.Object = obj
		ch <- wEvent
	}
}
