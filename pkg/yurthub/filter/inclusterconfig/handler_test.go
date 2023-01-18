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
)

func TestRuntimeObjectFilter(t *testing.T) {
	fh := NewInClusterConfigFilterHandler()

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
		"configmapList with kube-proxy configmap": {
			responseObject: &v1.ConfigMapList{
				Items: []v1.ConfigMap{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "foo",
							Namespace: "default",
						},
						Data: map[string]string{
							"foo": "bar",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      KubeProxyConfigMapName,
							Namespace: KubeProxyConfigMapNamespace,
						},
						Data: map[string]string{
							"config.conf": "    apiVersion: kubeproxy.config.k8s.io/v1alpha1\n    bindAddress: 0.0.0.0\n    bindAddressHardFail: false\n    clientConnection:\n      acceptContentTypes: \"\"\n      burst: 0\n      contentType: \"\"\n      kubeconfig: /var/lib/kube-proxy/kubeconfig.conf\n      qps: 0\n    clusterCIDR: 10.244.0.0/16\n    configSyncPeriod: 0s",
						},
					},
				},
			},
			expectObject: &v1.ConfigMapList{
				Items: []v1.ConfigMap{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "foo",
							Namespace: "default",
						},
						Data: map[string]string{
							"foo": "bar",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      KubeProxyConfigMapName,
							Namespace: KubeProxyConfigMapNamespace,
						},
						Data: map[string]string{
							"config.conf": "    apiVersion: kubeproxy.config.k8s.io/v1alpha1\n    bindAddress: 0.0.0.0\n    bindAddressHardFail: false\n    clientConnection:\n      acceptContentTypes: \"\"\n      burst: 0\n      contentType: \"\"\n      #kubeconfig: /var/lib/kube-proxy/kubeconfig.conf\n      qps: 0\n    clusterCIDR: 10.244.0.0/16\n    configSyncPeriod: 0s",
						},
					},
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

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			newObj, isNil := fh.RuntimeObjectFilter(tc.responseObject)
			if tc.expectObject == nil {
				if !isNil {
					t.Errorf("RuntimeObjectFilter expect nil obj, but got %v", newObj)
				}
			} else if !reflect.DeepEqual(newObj, tc.expectObject) {
				t.Errorf("RuntimeObjectFilter got error, expected: \n%v\nbut got: \n%v\n, isNil=%v", tc.expectObject, newObj, isNil)
			}
		})
	}
}
