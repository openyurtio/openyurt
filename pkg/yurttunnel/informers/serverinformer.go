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

package informers

import (
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	coreinformers "k8s.io/client-go/informers/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/yurttunnel/constants"
	"github.com/openyurtio/openyurt/pkg/yurttunnel/util"
)

// RegisterInformersForTunnelServer registers shared informers that tunnel server use.
func RegisterInformersForTunnelServer(informerFactory informers.SharedInformerFactory) {
	// add node informers
	informerFactory.Core().V1().Nodes()

	// add service informers
	informerFactory.InformerFor(&corev1.Service{}, newServiceInformer)

	// add configMap informers
	informerFactory.InformerFor(&corev1.ConfigMap{}, newConfigMapInformer)

	// add endpoints informers
	informerFactory.InformerFor(&corev1.Endpoints{}, newEndPointsInformer)
}

// newServiceInformer creates a shared index informers that returns services related to yurttunnel
func newServiceInformer(cs clientset.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
	// this informers will be used by coreDNSRecordController and certificate manager,
	// so it should return x-tunnel-server-svc and x-tunnel-server-internal-svc
	selector := fmt.Sprintf("name=%v", projectinfo.YurtTunnelServerLabel())
	tweakListOptions := func(options *metav1.ListOptions) {
		options.LabelSelector = selector
	}
	return coreinformers.NewFilteredServiceInformer(cs, constants.YurttunnelServerServiceNs, resyncPeriod, nil, tweakListOptions)
}

// newConfigMapInformer creates a shared index informers that returns only interested configmaps
func newConfigMapInformer(cs clientset.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
	selector := fmt.Sprintf("metadata.name=%v", util.YurttunnelServerDnatConfigMapName)
	tweakListOptions := func(options *metav1.ListOptions) {
		options.FieldSelector = selector
	}
	return coreinformers.NewFilteredConfigMapInformer(cs, util.YurttunnelServerDnatConfigMapNs, resyncPeriod, nil, tweakListOptions)
}

// newEndPointsInformer creates a shared index informers that returns only interested endpoints
func newEndPointsInformer(cs clientset.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
	selector := fmt.Sprintf("metadata.name=%v", constants.YurttunnelEndpointsName)
	tweakListOptions := func(options *metav1.ListOptions) {
		options.FieldSelector = selector
	}
	return coreinformers.NewFilteredEndpointsInformer(cs, constants.YurttunnelEndpointsNs, resyncPeriod, nil, tweakListOptions)
}
