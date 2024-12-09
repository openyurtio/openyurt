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

package v1_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	discovery "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openyurtio/openyurt/pkg/apis"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
	nodeutils "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/node"
	v1 "github.com/openyurtio/openyurt/pkg/yurtmanager/webhook/endpointslice/v1"
)

func TestDefault_AutonomyAnnotations(t *testing.T) {
	// Fix the pod to ready for the test
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod1",
			Namespace: "default",
		},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{
				{
					Type:   corev1.PodReady,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}

	// Test cases for Default
	tests := []struct {
		name        string
		node        *corev1.Node
		inputObj    runtime.Object
		expectedObj *discovery.EndpointSlice
		expectErr   bool
	}{
		{
			name: "Node autonomy duration annotation",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
					Annotations: map[string]string{
						projectinfo.GetNodeAutonomyDurationAnnotation(): "10m",
					},
				},
			},
			inputObj: &discovery.EndpointSlice{
				Endpoints: []discovery.Endpoint{endpoint1(true), endpoint2(false)},
			},
			expectedObj: &discovery.EndpointSlice{
				Endpoints: []discovery.Endpoint{endpoint1(true), endpoint2(true)},
			},
			expectErr: false,
		},
		{
			name: "Node autonomy duration annotation empty",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
					Annotations: map[string]string{
						projectinfo.GetNodeAutonomyDurationAnnotation(): "", // empty
					},
				},
			},
			inputObj: &discovery.EndpointSlice{
				Endpoints: []discovery.Endpoint{endpoint1(false), endpoint2(false)},
			},
			expectedObj: &discovery.EndpointSlice{
				Endpoints: []discovery.Endpoint{endpoint1(false), endpoint2(false)}, // not updated to Ready
			},
			expectErr: false,
		},
		{
			name: "Autonomy annotation true",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
					Annotations: map[string]string{
						projectinfo.GetAutonomyAnnotation(): "true",
					},
				},
			},
			inputObj: &discovery.EndpointSlice{
				Endpoints: []discovery.Endpoint{endpoint1(false), endpoint2(false)},
			},
			expectedObj: &discovery.EndpointSlice{
				Endpoints: []discovery.Endpoint{endpoint1(true), endpoint2(true)},
			},
			expectErr: false,
		},
		{
			name: "Autonomy annotation false",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
					Annotations: map[string]string{
						projectinfo.GetAutonomyAnnotation(): "false",
					},
				},
			},
			inputObj: &discovery.EndpointSlice{
				Endpoints: []discovery.Endpoint{endpoint1(true), endpoint2(false)},
			},
			expectedObj: &discovery.EndpointSlice{
				Endpoints: []discovery.Endpoint{endpoint1(true), endpoint2(false)}, // not updated to Ready
			},
			expectErr: false,
		},
		{
			name: "Pod binding annotation true",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
					Annotations: map[string]string{
						nodeutils.PodBindingAnnotation: "true",
					},
				},
			},
			inputObj: &discovery.EndpointSlice{
				Endpoints: []discovery.Endpoint{endpoint1(false), endpoint2(false)},
			},
			expectedObj: &discovery.EndpointSlice{
				Endpoints: []discovery.Endpoint{endpoint1(true), endpoint2(true)},
			},
			expectErr: false,
		},
		{
			name: "Pod binding annotation false",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
					Annotations: map[string]string{
						nodeutils.PodBindingAnnotation: "false",
					},
				},
			},
			inputObj: &discovery.EndpointSlice{
				Endpoints: []discovery.Endpoint{endpoint1(false), endpoint2(false)},
			},
			expectedObj: &discovery.EndpointSlice{
				Endpoints: []discovery.Endpoint{endpoint1(false), endpoint2(false)}, // not updated to Ready
			},
			expectErr: false,
		},
		{
			name: "Node has no annotations",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "node1",
					Annotations: map[string]string{}, // Nothing
				},
			},
			inputObj: &discovery.EndpointSlice{
				Endpoints: []discovery.Endpoint{endpoint1(false)},
			},
			expectedObj: &discovery.EndpointSlice{
				Endpoints: []discovery.Endpoint{endpoint1(false)},
			},
			expectErr: false,
		},
		{
			name: "Other node",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node3", // Other
					Annotations: map[string]string{
						projectinfo.GetNodeAutonomyDurationAnnotation(): "10m",
					},
				},
			},
			inputObj: &discovery.EndpointSlice{
				Endpoints: []discovery.Endpoint{endpoint1(false)},
			},
			expectedObj: &discovery.EndpointSlice{
				Endpoints: []discovery.Endpoint{endpoint1(false)},
			},
			expectErr: false,
		},
		{
			name: "Node name is empty",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "",
					Annotations: map[string]string{
						projectinfo.GetNodeAutonomyDurationAnnotation(): "", // empty
					},
				},
			},
			inputObj: &discovery.EndpointSlice{
				Endpoints: []discovery.Endpoint{endpoint1(false), endpoint2(false)},
			},
			expectedObj: &discovery.EndpointSlice{
				Endpoints: []discovery.Endpoint{endpoint1(false), endpoint2(false)}, // not updated to Ready
			},
			expectErr: false,
		},
		{
			name: "Node name and target ref nil",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						projectinfo.GetNodeAutonomyDurationAnnotation(): "", // empty
					},
				},
			},
			inputObj: &discovery.EndpointSlice{
				Endpoints: []discovery.Endpoint{{
					Addresses: []string{"172.16.0.17", "172.16.0.18"},
					Conditions: discovery.EndpointConditions{
						Ready: ptr.To(false),
					},
				}},
			},
			expectedObj: &discovery.EndpointSlice{
				Endpoints: []discovery.Endpoint{{
					Addresses: []string{"172.16.0.17", "172.16.0.18"},
					Conditions: discovery.EndpointConditions{
						Ready: ptr.To(false),
					},
				}}, // not updated to Ready
			},
			expectErr: false,
		},
		{
			name: "Endpoint slice is empty",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						projectinfo.GetNodeAutonomyDurationAnnotation(): "", // empty
					},
				},
			},
			inputObj: &discovery.EndpointSlice{
				Endpoints: []discovery.Endpoint{},
			},
			expectedObj: &discovery.EndpointSlice{
				Endpoints: []discovery.Endpoint{},
			},
			expectErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			err := clientgoscheme.AddToScheme(scheme)
			require.NoError(t, err, "Fail to add kubernetes clint-go custom resource")

			apis.AddToScheme(scheme)

			// Build client
			clientBuilder := fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(pod)
			if tc.node != nil {
				clientBuilder = clientBuilder.WithObjects(tc.node)
			}

			// Invoke Default
			w := &v1.EndpointSliceHandler{Client: clientBuilder.Build()}
			err = w.Default(context.TODO(), tc.inputObj)
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			// Check the result
			require.Equal(t, tc.expectedObj, tc.inputObj)
		})
	}
}

func TestDefault_PodCrashLoopBack(t *testing.T) {
	// Fix the node annotation to autonomy duration
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node1",
			Annotations: map[string]string{
				projectinfo.GetNodeAutonomyDurationAnnotation(): "10m",
			},
		},
	}

	// Test cases for Default
	tests := []struct {
		name        string
		inputObj    runtime.Object
		pod         *corev1.Pod
		expectedObj *discovery.EndpointSlice
		expectErr   bool
	}{
		{
			name: "Pod not crashloopback",
			inputObj: &discovery.EndpointSlice{
				Endpoints: []discovery.Endpoint{endpoint1(false), endpoint2Pod2(false)},
			},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "default",
				},
				Status: corev1.PodStatus{
					ContainerStatuses: []corev1.ContainerStatus{
						{
							State: corev1.ContainerState{
								Running: &corev1.ContainerStateRunning{},
							},
						},
					},
				},
			},
			expectedObj: &discovery.EndpointSlice{
				Endpoints: []discovery.Endpoint{endpoint1(true), endpoint2Pod2(true)}, /// updates regardless of matching pod
			},
			expectErr: false,
		},
		{
			name: "Pod is crashloopbackoff",
			inputObj: &discovery.EndpointSlice{
				Endpoints: []discovery.Endpoint{endpoint2Pod2(false)},
			},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod2",
					Namespace: "default",
				},
				Status: corev1.PodStatus{
					ContainerStatuses: []corev1.ContainerStatus{
						{
							State: corev1.ContainerState{
								Waiting: &corev1.ContainerStateWaiting{
									Reason: "CrashLoopBackOff",
								},
							},
						},
					},
				},
			},
			expectedObj: &discovery.EndpointSlice{
				Endpoints: []discovery.Endpoint{endpoint2Pod2(false)},
			},
			expectErr: false,
		},
		{
			name: "Pod no container states",
			inputObj: &discovery.EndpointSlice{
				Endpoints: []discovery.Endpoint{endpoint1(false)},
			},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod2",
					Namespace: "default",
				},
				Status: corev1.PodStatus{},
			},
			expectedObj: &discovery.EndpointSlice{
				Endpoints: []discovery.Endpoint{endpoint1(true)},
			},
			expectErr: false,
		},
		{
			name: "Pod multiple container statuses",
			inputObj: &discovery.EndpointSlice{
				Endpoints: []discovery.Endpoint{endpoint1(false), endpoint2Pod2(false)},
			},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod2",
					Namespace: "default",
				},
				Status: corev1.PodStatus{
					ContainerStatuses: []corev1.ContainerStatus{
						{
							State: corev1.ContainerState{
								Running: &corev1.ContainerStateRunning{},
							},
						},
						{
							State: corev1.ContainerState{
								Waiting: &corev1.ContainerStateWaiting{
									Reason: "CrashLoopBackOff",
								},
							},
						},
					},
				},
			},
			expectedObj: &discovery.EndpointSlice{
				Endpoints: []discovery.Endpoint{endpoint1(true), endpoint2Pod2(false)},
			},
			expectErr: false,
		},
		{
			name: "Pod is empty",
			inputObj: &discovery.EndpointSlice{
				Endpoints: []discovery.Endpoint{endpoint1(false), endpoint2Pod2(false)},
			},
			pod: &corev1.Pod{}, // Empty pod
			expectedObj: &discovery.EndpointSlice{
				Endpoints: []discovery.Endpoint{endpoint1(true), endpoint2Pod2(true)},
			},
			expectErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			err := clientgoscheme.AddToScheme(scheme)
			require.NoError(t, err, "Fail to add kubernetes clint-go custom resource")

			apis.AddToScheme(scheme)

			// Build client
			clientBuilder := fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(node)

			// Pod
			if tc.pod != nil {
				clientBuilder = clientBuilder.WithObjects(tc.pod)
			}

			// Invoke Default
			w := &v1.EndpointSliceHandler{Client: clientBuilder.Build()}
			err = w.Default(context.TODO(), tc.inputObj)
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			// Check the result
			require.Equal(t, tc.expectedObj, tc.inputObj)
		})
	}
}

func endpoint1(ready bool) discovery.Endpoint {
	return discovery.Endpoint{
		Addresses: []string{"172.16.0.17", "172.16.0.18"},
		Conditions: discovery.EndpointConditions{
			Ready: ptr.To(ready),
		},
		NodeName: ptr.To("node1"),
		TargetRef: &corev1.ObjectReference{
			Kind:      "Pod",
			Name:      "pod1",
			Namespace: "default",
		},
	}
}

func endpoint2(ready bool) discovery.Endpoint {
	return discovery.Endpoint{
		Addresses: []string{"10.244.1.2", "10.244.1.3"},
		Conditions: discovery.EndpointConditions{
			Ready: ptr.To(ready),
		},
		NodeName: ptr.To("node1"),
		TargetRef: &corev1.ObjectReference{
			Kind:      "Pod",
			Name:      "pod1",
			Namespace: "default",
		},
	}
}

func endpoint2Pod2(ready bool) discovery.Endpoint {
	return discovery.Endpoint{
		Addresses: []string{"10.244.1.2", "10.244.1.3"},
		Conditions: discovery.EndpointConditions{
			Ready: ptr.To(ready),
		},
		NodeName: ptr.To("node1"),
		TargetRef: &corev1.ObjectReference{
			Kind:      "Pod",
			Name:      "pod2",
			Namespace: "default",
		},
	}
}
