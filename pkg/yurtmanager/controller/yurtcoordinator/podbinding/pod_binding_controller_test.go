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

package podbinding

import (
	"context"
	"reflect"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
)

func podIndexer(rawObj client.Object) []string {
	pod, ok := rawObj.(*corev1.Pod)
	if !ok {
		return []string{}
	}
	if len(pod.Spec.NodeName) == 0 {
		return []string{}
	}
	return []string{pod.Spec.NodeName}
}

type FakeCountingClient struct {
	client.Client
	UpdateCount int
}

func (c *FakeCountingClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	c.UpdateCount++
	return c.Client.Update(ctx, obj, opts...)
}

func TestReconcile(t *testing.T) {
	second1 := int64(300)
	second2 := int64(100)
	testcases := map[string]struct {
		pod         *corev1.Pod
		node        *corev1.Node
		resultPod   *corev1.Pod
		resultErr   error
		resultCount int
	}{
		"update pod toleration seconds as node autonomy setting": {
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: corev1.PodSpec{
					NodeName: "node1",
					Tolerations: []corev1.Toleration{
						{
							Key:               corev1.TaintNodeNotReady,
							Operator:          corev1.TolerationOpExists,
							Effect:            corev1.TaintEffectNoExecute,
							TolerationSeconds: &second1,
						},
						{
							Key:               corev1.TaintNodeUnreachable,
							Operator:          corev1.TolerationOpExists,
							Effect:            corev1.TaintEffectNoExecute,
							TolerationSeconds: &second1,
						},
					},
				},
			},
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
					Labels: map[string]string{
						projectinfo.GetEdgeWorkerLabelKey(): "true",
					},
					Annotations: map[string]string{
						"node.openyurt.io/autonomy-duration": "100s",
					},
				},
			},
			resultPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: metav1.NamespaceDefault,
					Annotations: map[string]string{
						originalNotReadyTolerationDurationAnnotation:    "300",
						originalUnreachableTolerationDurationAnnotation: "300",
					},
				},
				Spec: corev1.PodSpec{
					NodeName: "node1",
					Tolerations: []corev1.Toleration{
						{
							Key:               corev1.TaintNodeNotReady,
							Operator:          corev1.TolerationOpExists,
							Effect:            corev1.TaintEffectNoExecute,
							TolerationSeconds: &second2,
						},
						{
							Key:               corev1.TaintNodeUnreachable,
							Operator:          corev1.TolerationOpExists,
							Effect:            corev1.TaintEffectNoExecute,
							TolerationSeconds: &second2,
						},
					},
				},
			},
			resultCount: 1,
		},
		"update pod toleration seconds with node autonomy duration is 0": {
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: corev1.PodSpec{
					NodeName: "node1",
					Tolerations: []corev1.Toleration{
						{
							Key:               corev1.TaintNodeNotReady,
							Operator:          corev1.TolerationOpExists,
							Effect:            corev1.TaintEffectNoExecute,
							TolerationSeconds: &second1,
						},
						{
							Key:               corev1.TaintNodeUnreachable,
							Operator:          corev1.TolerationOpExists,
							Effect:            corev1.TaintEffectNoExecute,
							TolerationSeconds: &second1,
						},
					},
				},
			},
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
					Labels: map[string]string{
						projectinfo.GetEdgeWorkerLabelKey(): "true",
					},
					Annotations: map[string]string{
						"node.openyurt.io/autonomy-duration": "0s",
					},
				},
			},
			resultPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: metav1.NamespaceDefault,
					Annotations: map[string]string{
						originalNotReadyTolerationDurationAnnotation:    "300",
						originalUnreachableTolerationDurationAnnotation: "300",
					},
				},
				Spec: corev1.PodSpec{
					NodeName: "node1",
					Tolerations: []corev1.Toleration{
						{
							Key:      corev1.TaintNodeNotReady,
							Operator: corev1.TolerationOpExists,
							Effect:   corev1.TaintEffectNoExecute,
						},
						{
							Key:      corev1.TaintNodeUnreachable,
							Operator: corev1.TolerationOpExists,
							Effect:   corev1.TaintEffectNoExecute,
						},
					},
				},
			},
			resultCount: 1,
		},
		"restore pod toleration seconds as node autonomy setting": {
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: metav1.NamespaceDefault,
					Annotations: map[string]string{
						originalNotReadyTolerationDurationAnnotation:    "100",
						originalUnreachableTolerationDurationAnnotation: "100",
					},
				},
				Spec: corev1.PodSpec{
					NodeName: "node1",
					Tolerations: []corev1.Toleration{
						{
							Key:               corev1.TaintNodeNotReady,
							Operator:          corev1.TolerationOpExists,
							Effect:            corev1.TaintEffectNoExecute,
							TolerationSeconds: &second1,
						},
						{
							Key:               corev1.TaintNodeUnreachable,
							Operator:          corev1.TolerationOpExists,
							Effect:            corev1.TaintEffectNoExecute,
							TolerationSeconds: &second1,
						},
					},
				},
			},
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
					Labels: map[string]string{
						projectinfo.GetEdgeWorkerLabelKey(): "true",
					},
				},
			},
			resultPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: metav1.NamespaceDefault,
					Annotations: map[string]string{
						originalNotReadyTolerationDurationAnnotation:    "100",
						originalUnreachableTolerationDurationAnnotation: "100",
					},
				},
				Spec: corev1.PodSpec{
					NodeName: "node1",
					Tolerations: []corev1.Toleration{
						{
							Key:               corev1.TaintNodeNotReady,
							Operator:          corev1.TolerationOpExists,
							Effect:            corev1.TaintEffectNoExecute,
							TolerationSeconds: &second2,
						},
						{
							Key:               corev1.TaintNodeUnreachable,
							Operator:          corev1.TolerationOpExists,
							Effect:            corev1.TaintEffectNoExecute,
							TolerationSeconds: &second2,
						},
					},
				},
			},
			resultCount: 1,
		},
		"pod toleration seconds is not changed with invalid duration": {
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: metav1.NamespaceDefault,
					Annotations: map[string]string{
						originalNotReadyTolerationDurationAnnotation:    "300",
						originalUnreachableTolerationDurationAnnotation: "300",
					},
				},
				Spec: corev1.PodSpec{
					NodeName: "node1",
					Tolerations: []corev1.Toleration{
						{
							Key:               corev1.TaintNodeNotReady,
							Operator:          corev1.TolerationOpExists,
							Effect:            corev1.TaintEffectNoExecute,
							TolerationSeconds: &second1,
						},
						{
							Key:               corev1.TaintNodeUnreachable,
							Operator:          corev1.TolerationOpExists,
							Effect:            corev1.TaintEffectNoExecute,
							TolerationSeconds: &second1,
						},
					},
				},
			},
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
					Annotations: map[string]string{
						"node.openyurt.io/autonomy-duration": "invalid duration",
					},
				},
			},
			resultPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: metav1.NamespaceDefault,
					Annotations: map[string]string{
						originalNotReadyTolerationDurationAnnotation:    "300",
						originalUnreachableTolerationDurationAnnotation: "300",
					},
				},
				Spec: corev1.PodSpec{
					NodeName: "node1",
					Tolerations: []corev1.Toleration{
						{
							Key:               corev1.TaintNodeNotReady,
							Operator:          corev1.TolerationOpExists,
							Effect:            corev1.TaintEffectNoExecute,
							TolerationSeconds: &second1,
						},
						{
							Key:               corev1.TaintNodeUnreachable,
							Operator:          corev1.TolerationOpExists,
							Effect:            corev1.TaintEffectNoExecute,
							TolerationSeconds: &second1,
						},
					},
				},
			},
			resultCount: 0,
		},
		"pod related node is not found": {
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: metav1.NamespaceDefault,
					Annotations: map[string]string{
						originalNotReadyTolerationDurationAnnotation:    "300",
						originalUnreachableTolerationDurationAnnotation: "300",
					},
				},
				Spec: corev1.PodSpec{
					NodeName: "node1",
					Tolerations: []corev1.Toleration{
						{
							Key:               corev1.TaintNodeNotReady,
							Operator:          corev1.TolerationOpExists,
							Effect:            corev1.TaintEffectNoExecute,
							TolerationSeconds: &second1,
						},
						{
							Key:               corev1.TaintNodeUnreachable,
							Operator:          corev1.TolerationOpExists,
							Effect:            corev1.TaintEffectNoExecute,
							TolerationSeconds: &second1,
						},
					},
				},
			},
			resultPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: metav1.NamespaceDefault,
					Annotations: map[string]string{
						originalNotReadyTolerationDurationAnnotation:    "300",
						originalUnreachableTolerationDurationAnnotation: "300",
					},
				},
				Spec: corev1.PodSpec{
					NodeName: "node1",
					Tolerations: []corev1.Toleration{
						{
							Key:               corev1.TaintNodeNotReady,
							Operator:          corev1.TolerationOpExists,
							Effect:            corev1.TaintEffectNoExecute,
							TolerationSeconds: &second1,
						},
						{
							Key:               corev1.TaintNodeUnreachable,
							Operator:          corev1.TolerationOpExists,
							Effect:            corev1.TaintEffectNoExecute,
							TolerationSeconds: &second1,
						},
					},
				},
			},
			resultCount: 0,
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			builder := fakeclient.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(tc.pod).WithIndex(&corev1.Pod{}, "spec.nodeName", podIndexer)
			if tc.node != nil {
				builder.WithObjects(tc.node)
			}

			fClient := &FakeCountingClient{
				Client: builder.Build(),
			}

			reconciler := ReconcilePodBinding{
				Client: fClient,
			}

			var req = reconcile.Request{NamespacedName: types.NamespacedName{Namespace: tc.pod.Namespace, Name: tc.pod.Name}}

			_, err := reconciler.Reconcile(context.TODO(), req)
			if tc.resultErr != nil {
				if err == nil || !strings.Contains(err.Error(), tc.resultErr.Error()) {
					t.Errorf("expect error %s, but got %s", tc.resultErr.Error(), err.Error())
				}
			}

			if fClient.UpdateCount != tc.resultCount {
				t.Errorf("expect update count %d, but got %d", tc.resultCount, fClient.UpdateCount)
			}

			if tc.resultPod != nil {
				currentPod := &corev1.Pod{}
				err = reconciler.Get(context.TODO(), types.NamespacedName{Namespace: tc.pod.Namespace, Name: tc.pod.Name}, currentPod)
				if err != nil {
					t.Errorf("couldn't get current pod, %v", err)
					return
				}

				if !reflect.DeepEqual(tc.resultPod.Annotations, currentPod.Annotations) {
					t.Errorf("expect pod annotations %v, but got %v", tc.resultPod.Annotations, currentPod.Annotations)
				}

				if !reflect.DeepEqual(tc.resultPod.Spec.Tolerations, tc.resultPod.Spec.Tolerations) {
					t.Errorf("expect pod annotations %v, but got %v", tc.resultPod.Spec.Tolerations, currentPod.Spec.Tolerations)
				}
			}
		})
	}
}

func TestGetPodsAssignedToNode(t *testing.T) {
	testcases := map[string]struct {
		nodeName   string
		pods       []client.Object
		resultPods sets.Set[string]
		resultErr  error
	}{
		"all pods are related to node": {
			nodeName: "node1",
			pods: []client.Object{
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod1",
						Namespace: metav1.NamespaceDefault,
					},
					Spec: corev1.PodSpec{
						NodeName: "node1",
					},
				},
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod2",
						Namespace: metav1.NamespaceDefault,
					},
					Spec: corev1.PodSpec{
						NodeName: "node1",
					},
				},
			},
			resultPods: sets.New("pod1", "pod2"),
		},
		"not all pods are related to node": {
			nodeName: "node1",
			pods: []client.Object{
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod1",
						Namespace: metav1.NamespaceDefault,
					},
					Spec: corev1.PodSpec{
						NodeName: "node1",
					},
				},
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod2",
						Namespace: metav1.NamespaceDefault,
					},
					Spec: corev1.PodSpec{
						NodeName: "node2",
					},
				},
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod3",
						Namespace: metav1.NamespaceDefault,
					},
					Spec: corev1.PodSpec{
						NodeName: "node1",
					},
				},
			},
			resultPods: sets.New("pod1", "pod3"),
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			builder := fakeclient.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(tc.pods...).WithIndex(&corev1.Pod{}, "spec.nodeName", podIndexer)
			fClient := &FakeCountingClient{
				Client: builder.Build(),
			}

			reconciler := ReconcilePodBinding{
				Client: fClient,
			}
			pods, err := reconciler.getPodsAssignedToNode(tc.nodeName)
			if tc.resultErr != nil {
				if err == nil || !strings.Contains(err.Error(), tc.resultErr.Error()) {
					t.Errorf("expect error %s, but got %s", tc.resultErr.Error(), err.Error())
				}
			}

			if len(tc.resultPods) != 0 {
				if len(pods) != len(tc.resultPods) {
					t.Errorf("expect pods count %d, but got %d", len(tc.resultPods), len(pods))
				}

				currentPods := sets.New[string]()
				for i := range pods {
					currentPods.Insert(pods[i].Name)
				}
				if !currentPods.Equal(tc.resultPods) {
					t.Errorf("expect pods %v, but got %v", tc.resultPods.UnsortedList(), currentPods.UnsortedList())
				}
			}
		})
	}
}

func TestIsDaemonSetPodOrStaticPod(t *testing.T) {
	testcases := map[string]struct {
		pod    *corev1.Pod
		result bool
	}{
		"normal pod": {
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: metav1.NamespaceDefault,
				},
			},
			result: false,
		},
		"daemon pod": {
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: metav1.NamespaceDefault,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "apps/v1",
							Kind:       "DaemonSet",
							Name:       "kube-proxy",
						},
					},
				},
			},
			result: true,
		},
		"static pod": {
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: metav1.NamespaceDefault,
					Annotations: map[string]string{
						"kubernetes.io/config.mirror": "abcdef123456789",
						"kubernetes.io/config.seen":   "2025-01-02",
						"kubernetes.io/config.source": "file",
					},
				},
			},
			result: true,
		},
	}
	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			if got := isDaemonSetPodOrStaticPod(tc.pod); got != tc.result {
				t.Errorf("isDaemonSetPodOrStaticPod() got = %v, expect %v", got, tc.result)
			}
		})
	}
}
