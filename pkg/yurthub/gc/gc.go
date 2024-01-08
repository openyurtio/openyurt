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
	"context"
	"time"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/config"
	"github.com/openyurtio/openyurt/pkg/yurthub/cachemanager"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/rest"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

var (
	defaultEventGcInterval = 60
)

// GCManager is responsible for cleanup garbage of yurthub
type GCManager struct {
	store             cachemanager.StorageWrapper
	restConfigManager *rest.RestConfigManager
	nodeName          string
	eventsGCFrequency time.Duration
	lastTime          time.Time
	stopCh            <-chan struct{}
}

// NewGCManager creates a *GCManager object
func NewGCManager(cfg *config.YurtHubConfiguration, restConfigManager *rest.RestConfigManager, stopCh <-chan struct{}) (*GCManager, error) {
	gcFrequency := cfg.GCFrequency
	if gcFrequency == 0 {
		gcFrequency = defaultEventGcInterval
	}
	mgr := &GCManager{
		// TODO: use disk storage directly
		store:             cfg.StorageWrapper,
		nodeName:          cfg.NodeName,
		restConfigManager: restConfigManager,
		eventsGCFrequency: time.Duration(gcFrequency) * time.Minute,
		stopCh:            stopCh,
	}
	mgr.gcPodsWhenRestart()
	return mgr, nil
}

// Run starts GCManager
func (m *GCManager) Run() {
	// run gc events after a time duration between eventsGCFrequency and 3 * eventsGCFrequency
	m.lastTime = time.Now()
	go wait.JitterUntil(func() {
		klog.V(2).Infof("start gc events after waiting %v from previous gc", time.Since(m.lastTime))
		m.lastTime = time.Now()
		cfg := m.restConfigManager.GetRestConfig(true)
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

func (m *GCManager) gcPodsWhenRestart() {
	localPodKeys, err := m.store.ListResourceKeysOfComponent("kubelet", schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "pods",
	})
	if err != nil {
		klog.Errorf("could not list keys for kubelet pods, %v", err)
		return
	} else if len(localPodKeys) == 0 {
		klog.Infof("local storage for kubelet pods is empty, not need to gc pods")
		return
	}
	klog.Infof("list pod keys from storage, total: %d", len(localPodKeys))

	if len(localPodKeys) == 0 {
		return
	}
	cfg := m.restConfigManager.GetRestConfig(true)
	if cfg == nil {
		klog.Errorf("could not get rest config, so skip gc pods when restart")
		return
	}
	kubeClient, err := clientset.NewForConfig(cfg)
	if err != nil {
		klog.Errorf("could not new kube client, %v", err)
		return
	}

	listOpts := metav1.ListOptions{FieldSelector: fields.OneTermEqualSelector("spec.nodeName", m.nodeName).String()}
	podList, err := kubeClient.CoreV1().Pods(v1.NamespaceAll).List(context.Background(), listOpts)
	if err != nil {
		klog.Errorf("could not list pods for node(%s), %v", m.nodeName, err)
		return
	}

	currentPodKeys := make(map[storage.Key]struct{}, len(podList.Items))
	for i := range podList.Items {
		name := podList.Items[i].Name
		ns := podList.Items[i].Namespace
		key, err := m.store.KeyFunc(storage.KeyBuildInfo{
			Component: "kubelet",
			Namespace: ns,
			Name:      name,
			Resources: "pods",
		})
		if err != nil {
			klog.Errorf("could not get pod key for %s/%s, %v", ns, name, err)
			continue
		}
		currentPodKeys[key] = struct{}{}
	}
	klog.V(2).Infof("list all of pod that on the node: total: %d", len(currentPodKeys))

	deletedPods := make([]storage.Key, 0)
	for i := range localPodKeys {
		if _, ok := currentPodKeys[localPodKeys[i]]; !ok {
			deletedPods = append(deletedPods, localPodKeys[i])
		}
	}

	if len(deletedPods) == len(localPodKeys) {
		klog.Infof("it's dangerous to gc all cache pods, so skip gc")
		return
	}

	for _, key := range deletedPods {
		if err := m.store.Delete(key); err != nil {
			klog.Errorf("could not gc pod %s, %v", key.Key(), err)
		} else {
			klog.Infof("gc pod %s successfully", key.Key())
		}
	}

}

func (m *GCManager) gcEvents(kubeClient clientset.Interface, component string) {
	if kubeClient == nil {
		return
	}

	localEventKeys, err := m.store.ListResourceKeysOfComponent(component, schema.GroupVersionResource{
		Group:    "events.k8s.io",
		Version:  "v1",
		Resource: "events",
	})
	if err != nil {
		klog.Errorf("could not list keys for %s events, %v", component, err)
		return
	} else if len(localEventKeys) == 0 {
		klog.Infof("no %s events in local storage, skip %s events gc", component, component)
		return
	}
	klog.Infof("list %s event keys from storage, total: %d", component, len(localEventKeys))

	deletedEvents := make([]storage.Key, 0)
	for _, key := range localEventKeys {
		_, _, ns, name := util.SplitKey(key.Key())
		if len(ns) == 0 || len(name) == 0 {
			klog.Infof("could not get namespace or name for event %s", key.Key())
			continue
		}

		_, err := kubeClient.CoreV1().Events(ns).Get(context.Background(), name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			deletedEvents = append(deletedEvents, key)
		} else if err != nil {
			klog.Errorf("could not get %s %s event for node(%s), %v", component, key.Key(), m.nodeName, err)
			break
		}
	}

	for _, key := range deletedEvents {
		if err := m.store.Delete(key); err != nil {
			klog.Errorf("could not gc events %s, %v", key.Key(), err)
		} else {
			klog.Infof("gc events %s successfully", key.Key())
		}
	}
}
