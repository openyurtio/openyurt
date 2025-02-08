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

package locallb

import (
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

type locallbManager struct {
	tenantKasService string
	apiserverAddrs   []string // ip1:port1,ip2:port2,...
	iptablesManager  *IptablesManager
}

func NewLocalLBManager(tenantKasAddress string, informerFactory informers.SharedInformerFactory) (*locallbManager, error) {
	m := &locallbManager{
		tenantKasService: tenantKasAddress,
		apiserverAddrs:   []string{},
		iptablesManager:  NewIptablesManager(),
	}

	endpointsInformer := informerFactory.Core().V1().Endpoints()
	informer := endpointsInformer.Informer()
	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    m.addEndpoints,
		UpdateFunc: m.updateEndpoints,
	})

	return m, nil
}

func (m *locallbManager) addEndpoints(obj interface{}) {
	endpoints, ok := obj.(*corev1.Endpoints)
	if !ok {
		klog.Errorf("could not convert to *corev1.Endpoints")
		return
	}

	klog.Infof("endpoints added: %s", endpoints.GetName())
	for _, subset := range endpoints.Subsets {
		var apiserverAddrs []string
		for _, address := range subset.Addresses {
			for _, port := range subset.Ports {
				apiserverAddrs = append(apiserverAddrs, address.IP+":"+fmt.Sprint(port.Port))
			}
		}
		m.apiserverAddrs = append(m.apiserverAddrs, apiserverAddrs...)
	}
	m.iptablesManager.updateIptablesRules(m.tenantKasService, m.apiserverAddrs)
}

func (m *locallbManager) updateEndpoints(oldObj, newObj interface{}) {
	oldEndpoints, ok := oldObj.(*corev1.Endpoints)
	if !ok {
		klog.Errorf("could not convert to *corev1.Endpoints")
		return
	}
	newEndpoints, ok := newObj.(*corev1.Endpoints)
	if !ok {
		klog.Errorf("could not convert to *corev1.Endpoints")
		return
	}

	klog.Infof("endpoints updated from %s to %s", oldEndpoints.GetName(), newEndpoints.GetName())

	// we only update iptables if the set of apiserverAddrs are different between newEndpoints and oldEndpoints.
	var oldApiserverAddrs []string
	var newApiserverAddrs []string
	for _, oldSubset := range oldEndpoints.Subsets {
		var oldAddrs []string
		for _, oldAddress := range oldSubset.Addresses {
			for _, oldPort := range oldSubset.Ports {
				oldAddrs = append(oldAddrs, oldAddress.IP+":"+fmt.Sprint(oldPort.Port))
			}
		}
		oldApiserverAddrs = append(oldApiserverAddrs, oldAddrs...)
	}
	for _, newSubset := range newEndpoints.Subsets {
		var newAddrs []string
		for _, newAddress := range newSubset.Addresses {
			for _, newPort := range newSubset.Ports {
				newAddrs = append(newAddrs, newAddress.IP+":"+fmt.Sprint(newPort.Port))
			}
		}
		newApiserverAddrs = append(newApiserverAddrs, newAddrs...)
	}
	// if newApiserverAddrs are the same as oldApiserverAddrs, that means endpoints are updated except addresses, do nothing.
	// if not the same, we delete oldApiserverAddrs from m.apiserverAddrs, then append newApiserverAddrs to m.apiserverAddrs.
	if !reflect.DeepEqual(oldApiserverAddrs, newApiserverAddrs) {
		m.deleteOldApiserverAddrs(&m.apiserverAddrs, oldApiserverAddrs)
		m.apiserverAddrs = append(m.apiserverAddrs, newApiserverAddrs...)
		m.iptablesManager.updateIptablesRules(m.tenantKasService, m.apiserverAddrs)
	}
}

func (m *locallbManager) deleteOldApiserverAddrs(addrs *[]string, itemsToRemove []string) {
	removeMap := make(map[string]bool)
	for _, item := range itemsToRemove {
		removeMap[item] = true
	}
	res := []string{}
	for _, item := range *addrs {
		if !removeMap[item] {
			res = append(res, item)
		}
	}
	*addrs = res
}

func (m *locallbManager) CleanIptables() error {
	err := m.iptablesManager.cleanIptablesRules()
	if err != nil {
		klog.Errorf("error cleaning Iptables: %v", err)
		return err
	}
	return nil
}
