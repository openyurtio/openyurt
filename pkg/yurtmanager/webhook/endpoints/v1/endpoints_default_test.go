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

//nolint:staticcheck // SA1019: corev1.Endpoints is deprecated but still supported for backward compatibility
package v1_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openyurtio/openyurt/pkg/apis"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
	nodeutils "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/node"
	v1 "github.com/openyurtio/openyurt/pkg/yurtmanager/webhook/endpoints/v1"
)

func TestDefault_AutonomyAnnotations(t *testing.T) {
	endpoint1 := corev1.EndpointAddress{
		IP:       "10.0.0.1",
		NodeName: ptr.To("node1"),
		TargetRef: &corev1.ObjectReference{
			Kind:      "Pod",
			Name:      "pod1",
			Namespace: "default",
		},
	}

	// Endpoint2 is mapped to pod2 which is always ready
	endpoint2 := corev1.EndpointAddress{
		IP:       "10.0.0.2",
		NodeName: ptr.To("node1"),
		TargetRef: &corev1.ObjectReference{
			Kind:      "Pod",
			Name:      "pod2",
			Namespace: "default",
		},
	}

	// Fix the pod to ready for the test
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod2",
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
	// endpoint2 should either be remapped or not
	tests := []struct {
		name              string
		endpoints         *corev1.Endpoints
		node              *corev1.Node
		expectedEndpoints *corev1.Endpoints
		expectErr         bool
	}{
		{
			name: "Node autonomy duration annotation",
			endpoints: &corev1.Endpoints{
				Subsets: []corev1.EndpointSubset{
					{
						Addresses:         []corev1.EndpointAddress{endpoint1},
						NotReadyAddresses: []corev1.EndpointAddress{endpoint2},
					},
				},
			},
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
					Annotations: map[string]string{
						projectinfo.GetNodeAutonomyDurationAnnotation(): "10m",
					},
				},
			},
			expectedEndpoints: &corev1.Endpoints{
				Subsets: []corev1.EndpointSubset{
					{
						Addresses: []corev1.EndpointAddress{
							endpoint1,
							endpoint2, // endpoint2 moved to readyAddresses
						},
						NotReadyAddresses: []corev1.EndpointAddress{},
					},
				},
			},
			expectErr: false,
		},
		{
			name: "Node autonomy duration annotation empty",
			endpoints: &corev1.Endpoints{
				Subsets: []corev1.EndpointSubset{
					{
						Addresses:         []corev1.EndpointAddress{endpoint1},
						NotReadyAddresses: []corev1.EndpointAddress{endpoint2},
					},
				},
			},
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
					Annotations: map[string]string{
						projectinfo.GetNodeAutonomyDurationAnnotation(): "", // empty
					},
				},
			},
			expectedEndpoints: &corev1.Endpoints{
				Subsets: []corev1.EndpointSubset{
					{
						Addresses:         []corev1.EndpointAddress{endpoint1},
						NotReadyAddresses: []corev1.EndpointAddress{endpoint2}, // not moved to ready
					},
				},
			},
			expectErr: false,
		},
		{
			name: "Autonomy annotation true",
			endpoints: &corev1.Endpoints{
				Subsets: []corev1.EndpointSubset{
					{
						Addresses:         []corev1.EndpointAddress{endpoint1},
						NotReadyAddresses: []corev1.EndpointAddress{endpoint2},
					},
				},
			},
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
					Annotations: map[string]string{
						projectinfo.GetAutonomyAnnotation(): "true",
					},
				},
			},
			expectedEndpoints: &corev1.Endpoints{
				Subsets: []corev1.EndpointSubset{
					{
						Addresses: []corev1.EndpointAddress{
							endpoint1,
							endpoint2,
						}, // endpoint2 moved to readyAddresses
						NotReadyAddresses: []corev1.EndpointAddress{},
					},
				},
			},
			expectErr: false,
		},
		{
			name: "Autonomy annotation false",
			endpoints: &corev1.Endpoints{
				Subsets: []corev1.EndpointSubset{
					{
						Addresses:         []corev1.EndpointAddress{endpoint1},
						NotReadyAddresses: []corev1.EndpointAddress{endpoint2},
					},
				},
			},
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
					Annotations: map[string]string{
						projectinfo.GetAutonomyAnnotation(): "false",
					},
				},
			},
			expectedEndpoints: &corev1.Endpoints{
				Subsets: []corev1.EndpointSubset{
					{
						Addresses:         []corev1.EndpointAddress{endpoint1},
						NotReadyAddresses: []corev1.EndpointAddress{endpoint2}, // not moved to ready
					},
				},
			},
			expectErr: false,
		},
		{
			name: "Pod binding annotation true",
			endpoints: &corev1.Endpoints{
				Subsets: []corev1.EndpointSubset{
					{
						Addresses:         []corev1.EndpointAddress{endpoint1},
						NotReadyAddresses: []corev1.EndpointAddress{endpoint2},
					},
				},
			},
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
					Annotations: map[string]string{
						nodeutils.PodBindingAnnotation: "true",
					},
				},
			},
			expectedEndpoints: &corev1.Endpoints{
				Subsets: []corev1.EndpointSubset{
					{
						Addresses: []corev1.EndpointAddress{
							endpoint1,
							endpoint2,
						}, // endpoint2 moved to readyAddresses
						NotReadyAddresses: []corev1.EndpointAddress{},
					},
				},
			},
			expectErr: false,
		},
		{
			name: "Pod binding annotation false",
			endpoints: &corev1.Endpoints{
				Subsets: []corev1.EndpointSubset{
					{
						Addresses:         []corev1.EndpointAddress{endpoint1},
						NotReadyAddresses: []corev1.EndpointAddress{endpoint2},
					},
				},
			},
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
					Annotations: map[string]string{
						nodeutils.PodBindingAnnotation: "false",
					},
				},
			},
			expectedEndpoints: &corev1.Endpoints{
				Subsets: []corev1.EndpointSubset{
					{
						Addresses:         []corev1.EndpointAddress{endpoint1},
						NotReadyAddresses: []corev1.EndpointAddress{endpoint2}, // not moved to ready
					},
				},
			},
			expectErr: false,
		},
		{
			name: "Node has no annotations",
			endpoints: &corev1.Endpoints{
				Subsets: []corev1.EndpointSubset{
					{
						Addresses:         []corev1.EndpointAddress{endpoint1},
						NotReadyAddresses: []corev1.EndpointAddress{endpoint2},
					},
				},
			},
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "node1",
					Annotations: map[string]string{}, // Nothing
				},
			},
			expectedEndpoints: &corev1.Endpoints{
				Subsets: []corev1.EndpointSubset{
					{
						Addresses:         []corev1.EndpointAddress{endpoint1},
						NotReadyAddresses: []corev1.EndpointAddress{endpoint2},
					},
				},
			},
			expectErr: false,
		},
		{
			name: "Other node",
			endpoints: &corev1.Endpoints{
				Subsets: []corev1.EndpointSubset{
					{
						Addresses:         []corev1.EndpointAddress{endpoint1},
						NotReadyAddresses: []corev1.EndpointAddress{endpoint2},
					},
				},
			},
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node3", // Other
					Annotations: map[string]string{
						projectinfo.GetNodeAutonomyDurationAnnotation(): "10m",
					},
				},
			},
			expectedEndpoints: &corev1.Endpoints{
				Subsets: []corev1.EndpointSubset{
					{
						Addresses:         []corev1.EndpointAddress{endpoint1},
						NotReadyAddresses: []corev1.EndpointAddress{endpoint2},
					},
				},
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
			w := &v1.EndpointsHandler{Client: clientBuilder.Build()}
			err = w.Default(context.TODO(), tc.endpoints)
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			// Check the result
			require.Equal(t, tc.expectedEndpoints, tc.endpoints)
		})
	}
}

func TestDefault_PodCrashLoopBack(t *testing.T) {
	endpoint1 := corev1.EndpointAddress{
		IP:       "10.0.0.1",
		NodeName: ptr.To("node1"),
		TargetRef: &corev1.ObjectReference{
			Kind:      "Pod",
			Name:      "pod1",
			Namespace: "default",
		},
	}

	endpoint2 := corev1.EndpointAddress{
		IP:       "10.0.0.2",
		NodeName: ptr.To("node1"),
		TargetRef: &corev1.ObjectReference{
			Kind:      "Pod",
			Name:      "pod2",
			Namespace: "default",
		},
	}

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
	// endpoint2 should either be remapped or not
	tests := []struct {
		name              string
		endpoints         *corev1.Endpoints
		pod               *corev1.Pod
		expectedEndpoints *corev1.Endpoints
		expectErr         bool
	}{
		{
			name: "Pod not crashloopback",
			endpoints: &corev1.Endpoints{
				Subsets: []corev1.EndpointSubset{
					{
						Addresses:         []corev1.EndpointAddress{endpoint1},
						NotReadyAddresses: []corev1.EndpointAddress{endpoint2},
					},
				},
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
					},
				},
			},
			expectedEndpoints: &corev1.Endpoints{
				Subsets: []corev1.EndpointSubset{
					{
						Addresses: []corev1.EndpointAddress{
							endpoint1,
							endpoint2, // endpoint2 moved to readyAddresses
						},
						NotReadyAddresses: []corev1.EndpointAddress{},
					},
				},
			},
			expectErr: false,
		},
		{
			name: "Pod is crashloopbackoff",
			endpoints: &corev1.Endpoints{
				Subsets: []corev1.EndpointSubset{
					{
						Addresses:         []corev1.EndpointAddress{endpoint1},
						NotReadyAddresses: []corev1.EndpointAddress{endpoint2},
					},
				},
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
			expectedEndpoints: &corev1.Endpoints{
				Subsets: []corev1.EndpointSubset{
					{
						Addresses:         []corev1.EndpointAddress{endpoint1},
						NotReadyAddresses: []corev1.EndpointAddress{endpoint2}, // not moved to ready
					},
				},
			},
			expectErr: false,
		},
		{
			name: "Pod no container states",
			endpoints: &corev1.Endpoints{
				Subsets: []corev1.EndpointSubset{
					{
						Addresses:         []corev1.EndpointAddress{endpoint1},
						NotReadyAddresses: []corev1.EndpointAddress{endpoint2},
					},
				},
			},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod2",
					Namespace: "default",
				},
				Status: corev1.PodStatus{},
			},
			expectedEndpoints: &corev1.Endpoints{
				Subsets: []corev1.EndpointSubset{
					{
						Addresses:         []corev1.EndpointAddress{endpoint1, endpoint2},
						NotReadyAddresses: []corev1.EndpointAddress{},
					},
				},
			},
			expectErr: false,
		},
		{
			name: "Pod multiple container statuses",
			endpoints: &corev1.Endpoints{
				Subsets: []corev1.EndpointSubset{
					{
						Addresses:         []corev1.EndpointAddress{endpoint1},
						NotReadyAddresses: []corev1.EndpointAddress{endpoint2},
					},
				},
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
			expectedEndpoints: &corev1.Endpoints{
				Subsets: []corev1.EndpointSubset{
					{
						Addresses:         []corev1.EndpointAddress{endpoint1},
						NotReadyAddresses: []corev1.EndpointAddress{endpoint2}, // not moved to ready
					},
				},
			},
			expectErr: false,
		},
		{
			name: "Pod is empty",
			endpoints: &corev1.Endpoints{
				Subsets: []corev1.EndpointSubset{
					{
						Addresses:         []corev1.EndpointAddress{endpoint1},
						NotReadyAddresses: []corev1.EndpointAddress{endpoint2},
					},
				},
			},
			pod: &corev1.Pod{}, // Empty pod
			expectedEndpoints: &corev1.Endpoints{
				Subsets: []corev1.EndpointSubset{
					{
						Addresses: []corev1.EndpointAddress{
							endpoint1,
							endpoint2,
						}, // endpoint2 moved to readyAddresses
						NotReadyAddresses: []corev1.EndpointAddress{},
					},
				},
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
			w := &v1.EndpointsHandler{Client: clientBuilder.Build()}
			err = w.Default(context.TODO(), tc.endpoints)
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			// Check the result
			require.Equal(t, tc.expectedEndpoints, tc.endpoints)
		})
	}
}
