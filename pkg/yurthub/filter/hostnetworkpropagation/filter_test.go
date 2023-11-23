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

package hostnetworkpropagation

import (
	"reflect"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	"github.com/openyurtio/openyurt/pkg/apis"
	"github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/util"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
)

func TestName(t *testing.T) {
	hpf, _ := NewHostNetworkPropagationFilter()
	if hpf.Name() != filter.HostNetworkPropagationFilterName {
		t.Errorf("expect %s, but got %s", filter.HostNetworkPropagationFilterName, hpf.Name())
	}
}

func TestSupportedResourceAndVerbs(t *testing.T) {
	hpf, _ := NewHostNetworkPropagationFilter()
	rvs := hpf.SupportedResourceAndVerbs()
	if len(rvs) != 1 {
		t.Errorf("supported not one resource, %v", rvs)
	}

	for resource, verbs := range rvs {
		if resource != "pods" {
			t.Errorf("expect resource is pods, but got %s", resource)
		}

		if !verbs.Equal(sets.NewString("list", "watch")) {
			t.Errorf("expect verbs are list/watch, but got %v", verbs.UnsortedList())
		}
	}
}

func TestFilter(t *testing.T) {
	scheme := runtime.NewScheme()
	apis.AddToScheme(scheme)
	gvrToListKind := map[schema.GroupVersionResource]string{
		{Group: "apps.openyurt.io", Version: "v1beta1", Resource: "nodepools"}: "NodePoolList",
	}

	testcases := map[string]struct {
		poolName       string
		responseObject []runtime.Object
		kubeClient     *k8sfake.Clientset
		dynamicClient  *fake.FakeDynamicClient
		expectObject   []runtime.Object
	}{
		"pod hostnetwork is true": {
			responseObject: []runtime.Object{
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod-foo",
					},
					Spec: corev1.PodSpec{
						NodeName:    "node-foo",
						HostNetwork: true,
					},
				},
			},
			expectObject: []runtime.Object{
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod-foo",
					},
					Spec: corev1.PodSpec{
						NodeName:    "node-foo",
						HostNetwork: true,
					},
				},
			},
		},
		"it is not a pod": {
			responseObject: []runtime.Object{
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-foo",
					},
				},
			},
			expectObject: []runtime.Object{
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-foo",
					},
				},
			},
		},
		"pool hostNetwork is false with specified nodepool": {
			poolName: "pool-foo",
			responseObject: []runtime.Object{
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod-foo",
					},
					Spec: corev1.PodSpec{
						NodeName: "node-foo",
					},
				},
			},
			dynamicClient: fake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrToListKind,
				&v1beta1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pool-foo",
					},
					Spec: v1beta1.NodePoolSpec{
						Type: v1beta1.Edge,
					},
				},
			),
			expectObject: []runtime.Object{
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod-foo",
					},
					Spec: corev1.PodSpec{
						NodeName: "node-foo",
					},
				},
			},
		},
		"pool hostNetwork true false with specified nodepool": {
			poolName: "pool-foo",
			responseObject: []runtime.Object{
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod-foo",
					},
					Spec: corev1.PodSpec{
						NodeName: "node-foo",
					},
				},
			},
			dynamicClient: fake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrToListKind,
				&v1beta1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pool-foo",
					},
					Spec: v1beta1.NodePoolSpec{
						Type:        v1beta1.Edge,
						HostNetwork: true,
					},
				},
			),
			expectObject: []runtime.Object{
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod-foo",
					},
					Spec: corev1.PodSpec{
						NodeName:    "node-foo",
						HostNetwork: true,
					},
				},
			},
		},
		"pool hostNetwork is false without specified node pool": {
			responseObject: []runtime.Object{
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod-foo",
					},
					Spec: corev1.PodSpec{
						NodeName: "node-foo",
					},
				},
			},
			kubeClient: k8sfake.NewSimpleClientset(
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-foo",
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "pool-foo",
						},
					},
				},
			),
			dynamicClient: fake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrToListKind,
				&v1beta1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pool-foo",
					},
					Spec: v1beta1.NodePoolSpec{
						Type: v1beta1.Edge,
					},
				},
			),
			expectObject: []runtime.Object{
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod-foo",
					},
					Spec: corev1.PodSpec{
						NodeName: "node-foo",
					},
				},
			},
		},
		"pool hostNetwork is true without specified node pool": {
			responseObject: []runtime.Object{
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod-foo",
					},
					Spec: corev1.PodSpec{
						NodeName: "node-foo",
					},
				},
			},
			kubeClient: k8sfake.NewSimpleClientset(
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-foo",
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "pool-foo",
						},
					},
				},
			),
			dynamicClient: fake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrToListKind,
				&v1beta1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pool-foo",
					},
					Spec: v1beta1.NodePoolSpec{
						Type:        v1beta1.Edge,
						HostNetwork: true,
					},
				},
			),
			expectObject: []runtime.Object{
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod-foo",
					},
					Spec: corev1.PodSpec{
						NodeName:    "node-foo",
						HostNetwork: true,
					},
				},
			},
		},
		"pool hostNetwork is false with specified unknown node": {
			responseObject: []runtime.Object{
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod-foo",
					},
					Spec: corev1.PodSpec{
						NodeName: "unknown-node",
					},
				},
			},
			kubeClient: k8sfake.NewSimpleClientset(
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-foo",
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "pool-foo",
						},
					},
				},
			),
			dynamicClient: fake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrToListKind,
				&v1beta1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pool-foo",
					},
					Spec: v1beta1.NodePoolSpec{
						Type:        v1beta1.Edge,
						HostNetwork: true,
					},
				},
			),
			expectObject: []runtime.Object{
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod-foo",
					},
					Spec: corev1.PodSpec{
						NodeName: "unknown-node",
					},
				},
			},
		},
		"pool hostNetwork is false with unknown pool": {
			responseObject: []runtime.Object{
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod-foo",
					},
					Spec: corev1.PodSpec{
						NodeName: "node-foo",
					},
				},
			},
			kubeClient: k8sfake.NewSimpleClientset(
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-foo",
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "unknown-pool",
						},
					},
				},
			),
			dynamicClient: fake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrToListKind,
				&v1beta1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pool-foo",
					},
					Spec: v1beta1.NodePoolSpec{
						Type:        v1beta1.Edge,
						HostNetwork: true,
					},
				},
			),
			expectObject: []runtime.Object{
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod-foo",
					},
					Spec: corev1.PodSpec{
						NodeName: "node-foo",
					},
				},
			},
		},
		"two pods with hostnetwork nodepool": {
			responseObject: []runtime.Object{
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod-foo",
					},
					Spec: corev1.PodSpec{
						NodeName: "node-foo",
					},
				},
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod-bar",
					},
					Spec: corev1.PodSpec{
						NodeName: "node-foo",
					},
				},
			},
			kubeClient: k8sfake.NewSimpleClientset(
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-foo",
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "pool-foo",
						},
					},
				},
			),
			dynamicClient: fake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrToListKind,
				&v1beta1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pool-foo",
					},
					Spec: v1beta1.NodePoolSpec{
						Type:        v1beta1.Edge,
						HostNetwork: true,
					},
				},
			),
			expectObject: []runtime.Object{
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod-foo",
					},
					Spec: corev1.PodSpec{
						NodeName:    "node-foo",
						HostNetwork: true,
					},
				},
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod-bar",
					},
					Spec: corev1.PodSpec{
						NodeName:    "node-foo",
						HostNetwork: true,
					},
				},
			},
		},
		"two pods with not hostnetwork nodepool": {
			responseObject: []runtime.Object{
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod-foo",
					},
					Spec: corev1.PodSpec{
						NodeName: "node-foo",
					},
				},
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod-bar",
					},
					Spec: corev1.PodSpec{
						NodeName: "node-foo",
					},
				},
			},
			kubeClient: k8sfake.NewSimpleClientset(
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-foo",
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "pool-foo",
						},
					},
				},
			),
			dynamicClient: fake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrToListKind,
				&v1beta1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pool-foo",
					},
					Spec: v1beta1.NodePoolSpec{
						Type: v1beta1.Edge,
					},
				},
			),
			expectObject: []runtime.Object{
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod-foo",
					},
					Spec: corev1.PodSpec{
						NodeName: "node-foo",
					},
				},
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod-bar",
					},
					Spec: corev1.PodSpec{
						NodeName: "node-foo",
					},
				},
			},
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			hpf := &hostNetworkPropagationFilter{
				nodePoolName: tc.poolName,
				client:       tc.kubeClient,
			}

			if tc.dynamicClient != nil {
				gvr := v1beta1.GroupVersion.WithResource("nodepools")
				dynamicInformerFactory := dynamicinformer.NewDynamicSharedInformerFactory(tc.dynamicClient, 24*time.Hour)
				nodePoolInformer := dynamicInformerFactory.ForResource(gvr)
				nodePoolLister := nodePoolInformer.Lister()
				nodePoolSynced := nodePoolInformer.Informer().HasSynced

				stopper := make(chan struct{})
				defer close(stopper)
				dynamicInformerFactory.Start(stopper)
				dynamicInformerFactory.WaitForCacheSync(stopper)
				hpf.nodePoolLister = nodePoolLister
				hpf.nodePoolSynced = nodePoolSynced
			}

			stopCh := make(<-chan struct{})
			for i := range tc.responseObject {
				newObj := hpf.Filter(tc.responseObject[i], stopCh)
				if util.IsNil(newObj) {
					t.Errorf("empty object is returned")
				}
				if !reflect.DeepEqual(newObj, tc.expectObject[i]) {
					t.Errorf("hostNetworkPropagationFilter expect: \n%#+v\nbut got: \n%#+v\n", tc.expectObject, newObj)
				}
			}
		})
	}
}
