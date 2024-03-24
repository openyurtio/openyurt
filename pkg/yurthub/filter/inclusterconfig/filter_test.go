/*
Copyright 2023 The OpenYurt Authors.

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

package inclusterconfig

import (
	"reflect"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/openyurtio/openyurt/pkg/util"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/base"
)

func TestRegister(t *testing.T) {
	filters := base.NewFilters([]string{})
	Register(filters)
	if !filters.Enabled(FilterName) {
		t.Errorf("couldn't register %s filter", FilterName)
	}
}

func TestName(t *testing.T) {
	iccf, _ := NewInClusterConfigFilter()
	if iccf.Name() != FilterName {
		t.Errorf("expect %s, but got %s", FilterName, iccf.Name())
	}
}

func TestSupportedResourceAndVerbs(t *testing.T) {
	iccf, _ := NewInClusterConfigFilter()
	rvs := iccf.SupportedResourceAndVerbs()
	if len(rvs) != 1 {
		t.Errorf("supported more than one resources, %v", rvs)
	}

	for resource, verbs := range rvs {
		if resource != "configmaps" {
			t.Errorf("expect resource is services, but got %s", resource)
		}

		if !verbs.Equal(sets.NewString("get", "list", "watch")) {
			t.Errorf("expect verbs are get/list/watch, but got %v", verbs.UnsortedList())
		}
	}
}

func TestRuntimeObjectFilter(t *testing.T) {
	iccf, _ := NewInClusterConfigFilter()

	testcases := map[string]struct {
		responseObject runtime.Object
		expectObject   runtime.Object
	}{
		"kube-proxy configmap": {
			responseObject: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      KubeProxyConfigMapName,
					Namespace: KubeProxyConfigMapNamespace,
				},
				Data: map[string]string{
					"config.conf": "    apiVersion: kubeproxy.config.k8s.io/v1alpha1\n    bindAddress: 0.0.0.0\n    bindAddressHardFail: false\n    clientConnection:\n      acceptContentTypes: \"\"\n      burst: 0\n      contentType: \"\"\n      kubeconfig: /var/lib/kube-proxy/kubeconfig.conf\n      qps: 0\n    clusterCIDR: 10.244.0.0/16\n    configSyncPeriod: 0s",
				},
			},
			expectObject: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      KubeProxyConfigMapName,
					Namespace: KubeProxyConfigMapNamespace,
				},
				Data: map[string]string{
					"config.conf": "    apiVersion: kubeproxy.config.k8s.io/v1alpha1\n    bindAddress: 0.0.0.0\n    bindAddressHardFail: false\n    clientConnection:\n      acceptContentTypes: \"\"\n      burst: 0\n      contentType: \"\"\n      #kubeconfig: /var/lib/kube-proxy/kubeconfig.conf\n      qps: 0\n    clusterCIDR: 10.244.0.0/16\n    configSyncPeriod: 0s",
				},
			},
		},
		"kube-proxy configmap without kubeconfig": {
			responseObject: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      KubeProxyConfigMapName,
					Namespace: KubeProxyConfigMapNamespace,
				},
				Data: map[string]string{
					"config.conf": "    apiVersion: kubeproxy.config.k8s.io/v1alpha1\n    bindAddress: 0.0.0.0\n    bindAddressHardFail: false\n    clientConnection:\n      acceptContentTypes: \"\"\n      burst: 0\n      contentType: \"\"\n      qps: 0\n    clusterCIDR: 10.244.0.0/16\n    configSyncPeriod: 0s",
				},
			},
			expectObject: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      KubeProxyConfigMapName,
					Namespace: KubeProxyConfigMapNamespace,
				},
				Data: map[string]string{
					"config.conf": "    apiVersion: kubeproxy.config.k8s.io/v1alpha1\n    bindAddress: 0.0.0.0\n    bindAddressHardFail: false\n    clientConnection:\n      acceptContentTypes: \"\"\n      burst: 0\n      contentType: \"\"\n      qps: 0\n    clusterCIDR: 10.244.0.0/16\n    configSyncPeriod: 0s",
				},
			},
		},
		"not configmapList": {
			responseObject: &v1.PodList{
				Items: []v1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "pod1",
							Namespace: KubeProxyConfigMapNamespace,
						},
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Name:  "nginx",
									Image: "nginx",
								},
							},
						},
					},
				},
			},
			expectObject: &v1.PodList{
				Items: []v1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "pod1",
							Namespace: KubeProxyConfigMapNamespace,
						},
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Name:  "nginx",
									Image: "nginx",
								},
							},
						},
					},
				},
			},
		},
	}

	stopCh := make(<-chan struct{})
	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			newObj := iccf.Filter(tc.responseObject, stopCh)
			if tc.expectObject == nil {
				if !util.IsNil(newObj) {
					t.Errorf("RuntimeObjectFilter expect nil obj, but got %v", newObj)
				}
			} else if !reflect.DeepEqual(newObj, tc.expectObject) {
				t.Errorf("RuntimeObjectFilter got error, expected: \n%v\nbut got: \n%v\n", tc.expectObject, newObj)
			}
		})
	}
}
