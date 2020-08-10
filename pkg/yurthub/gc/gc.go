/*
Copyright 2020 The OpenYurt Authors.

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

package gc

import (
	"fmt"
	"time"

	"github.com/alibaba/openyurt/cmd/yurthub/app/config"
	"github.com/alibaba/openyurt/pkg/yurthub/storage"
	"github.com/alibaba/openyurt/pkg/yurthub/transport"
	"github.com/alibaba/openyurt/pkg/yurthub/util"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog"
)

var (
	defaultEventGcInterval = 60
)

// GCManager is responsible for cleanup garbage of yurthub
type GCManager struct {
	store             storage.Store
	transportManager  transport.Interface
	nodeName          string
	eventsGCFrequency time.Duration
	lastTime          time.Time
	stopCh            <-chan struct{}
}

// NewGCManager creates a *GCManager object
func NewGCManager(cfg *config.YurtHubConfiguration, store storage.Store, transportManager transport.Interface, stopCh <-chan struct{}) (*GCManager, error) {
	gcFrequency := cfg.GCFrequency
	if gcFrequency == 0 {
		gcFrequency = defaultEventGcInterval
	}
	mgr := &GCManager{
		store:             store,
		transportManager:  transportManager,
		nodeName:          cfg.NodeName,
		eventsGCFrequency: time.Duration(gcFrequency) * time.Minute,
		stopCh:            stopCh,
	}
	_ = mgr.gcPodsWhenRestart()
	return mgr, nil
}

// Run starts GCManager
func (m *GCManager) Run() {
	// run gc events after a time duration between eventsGCFrequency and 3 * eventsGCFrequency
	m.lastTime = time.Now()
	go wait.JitterUntil(func() {
		klog.V(2).Infof("start gc events after waiting %v from previous gc", time.Since(m.lastTime))
		m.lastTime = time.Now()
		cfg := m.transportManager.GetRestClientConfig()
		if cfg == nil {
			klog.Errorf("could not get rest config, so skip gc")
			return
		}
		kubeClient, err := clientset.NewForConfig(cfg)
		if err != nil {
			klog.Errorf("could not new kube client, %v", err)
			return
		}

		m.gcEvents(kubeClient, "kubelet")
		m.gcEvents(kubeClient, "kube-proxy")
	}, m.eventsGCFrequency, 2, true, m.stopCh)
}

func (m *GCManager) gcPodsWhenRestart() error {
	localPodKeys, err := m.store.ListKeys("kubelet/pods")
	if err != nil || len(localPodKeys) == 0 {
		return nil
	}
	klog.Infof("list pod keys from storage, total: %d", len(localPodKeys))

	cfg := m.transportManager.GetRestClientConfig()
	if cfg == nil {
		klog.Errorf("could not get rest config, so skip gc pods when restart")
		return err
	}
	kubeClient, err := clientset.NewForConfig(cfg)
	if err != nil {
		klog.Errorf("could not new kube client, %v", err)
		return err
	}

	listOpts := metav1.ListOptions{FieldSelector: fields.OneTermEqualSelector("spec.nodeName", m.nodeName).String()}
	podList, err := kubeClient.CoreV1().Pods(v1.NamespaceAll).List(listOpts)
	if err != nil {
		klog.Errorf("could not list pods for node(%s), %v", m.nodeName, err)
		return err
	}

	currentPodKeys := make(map[string]struct{}, len(podList.Items))
	for i := range podList.Items {
		name := podList.Items[i].Name
		ns := podList.Items[i].Namespace

		key, _ := util.KeyFunc("kubelet", "pods", ns, name)
		currentPodKeys[key] = struct{}{}
	}
	klog.V(2).Infof("list all of pod that on the node: total: %d", len(currentPodKeys))

	deletedPods := make([]string, 0)
	for i := range localPodKeys {
		if _, ok := currentPodKeys[localPodKeys[i]]; !ok {
			deletedPods = append(deletedPods, localPodKeys[i])
		}
	}

	if len(deletedPods) == len(localPodKeys) {
		klog.Infof("it's dangerous to gc all cache pods, so skip gc")
		return nil
	}

	for _, key := range deletedPods {
		if err := m.store.Delete(key); err != nil {
			klog.Errorf("failed to gc pod %s, %v", key, err)
		} else {
			klog.Infof("gc pod %s successfully", key)
		}
	}

	return nil
}

func (m *GCManager) gcEvents(kubeClient clientset.Interface, component string) {
	if kubeClient == nil {
		return
	}

	localEventKeys, err := m.store.ListKeys(fmt.Sprintf("%s/events", component))
	if err != nil {
		klog.Errorf("could not list keys for %s events, %v", component, err)
		return
	} else if len(localEventKeys) == 0 {
		klog.Infof("no %s events in local storage, skip %s events gc", component, component)
		return
	}
	klog.Infof("list %s event keys from storage, total: %d", component, len(localEventKeys))

	deletedEvents := make([]string, 0)
	for _, key := range localEventKeys {
		_, _, ns, name := util.SplitKey(key)
		if len(ns) == 0 || len(name) == 0 {
			klog.Infof("could not get namespace or name for event %s", key)
			continue
		}

		_, err := kubeClient.CoreV1().Events(ns).Get(name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			deletedEvents = append(deletedEvents, key)
		} else if err != nil {
			klog.Errorf("could not get %s %s event for node(%s), %v", component, key, m.nodeName, err)
			break
		}
	}

	for _, key := range deletedEvents {
		if err := m.store.Delete(key); err != nil {
			klog.Errorf("failed to gc events %s, %v", key, err)
		} else {
			klog.Infof("gc events %s successfully", key)
		}
	}
}
