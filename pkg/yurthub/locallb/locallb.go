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
	"strconv"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
)

type locallbManager struct {
	ips             []string
	iptablesManager *IptablesManager
}

func NewLocalLBManager(informerFactory informers.SharedInformerFactory) (*locallbManager, error) {
	m := &locallbManager{
		ips:             make([]string, 0),
		iptablesManager: NewIptablesManager(options.HubAgentDummyIfIP, strconv.Itoa(options.YurtHubProxyPort)),
	}
	podInformer := informerFactory.Core().V1().Pods()
	informer := podInformer.Informer()
	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    m.addPod,
		UpdateFunc: m.updatePod,
		DeleteFunc: m.deletePod,
	})

	m.watchIP(informerFactory)
	return m, nil
}

func (m *locallbManager) Run() {
	m.loadbalancing()
}

func (m *locallbManager) loadbalancing() error {
	m.configureNetwork()
	return nil
}

func (m *locallbManager) configureNetwork() error {
	// m.fetchIP()
	return nil
}

func (m *locallbManager) watchIP(informerFactory informers.SharedInformerFactory) {
	stopCh := make(chan struct{})
	defer close(stopCh)
	informerFactory.Start(stopCh)
	informerFactory.WaitForCacheSync(stopCh)
	<-stopCh
}

func (m *locallbManager) addPod(obj interface{}) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		klog.Errorf("could not convert to *corev1.Pod")
		return
	}

	klog.Infof("pod added %s", pod.GetName())
	m.ips = append(m.ips, pod.Status.PodIP)
}

func (m *locallbManager) updatePod(oldObj, newObj interface{}) {
	oldPod, ok := oldObj.(*corev1.Pod)
	if !ok {
		klog.Errorf("could not convert to *corev1.Pod")
		return
	}
	newPod, ok := newObj.(*corev1.Pod)
	if !ok {
		klog.Errorf("could not convert to *corev1.Pod")
		return
	}

	klog.Infof("pod updated from %s to %s", oldPod.GetName(), newPod.GetName())
	for i, ip := range m.ips {
		if ip == oldPod.Status.PodIP {
			m.ips[i] = newPod.Status.PodIP
			return
		}
	}
}

func (m *locallbManager) deletePod(obj interface{}) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		klog.Errorf("could not convert to *corev1.Pod")
		return
	}

	klog.Infof("pod deleted %s", pod.GetName())
	var updatedIPs []string
	for _, existingIP := range m.ips {
		if existingIP != pod.Status.PodIP {
			updatedIPs = append(updatedIPs, existingIP)
		}
	}
	m.ips = updatedIPs
}
