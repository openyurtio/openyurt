/*
Copyright 2021 The OpenYurt Authors.

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

package discardcloudservice

import (
	"reflect"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestDiscardCloudServiceFilter(t *testing.T) {
	filter, err := NewDiscardCloudServiceFilter()
	if err != nil {
		t.Fatalf("failed to create filter: %v", err)
	}

	tests := []struct {
		name          string
		obj           runtime.Object
		wantDiscarded bool
		description   string
	}{
		{
			name: "LoadBalancer service with discard annotation",
			obj: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "lb-svc",
					Namespace: "default",
					Annotations: map[string]string{
						DiscardServiceAnnotation: "true",
					},
				},
				Spec: v1.ServiceSpec{
					Type: v1.ServiceTypeLoadBalancer,
				},
			},
			wantDiscarded: true,
			description:   "LoadBalancer with discard annotation should be discarded",
		},
		{
			name: "LoadBalancer service without annotation",
			obj: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "lb-svc",
					Namespace: "default",
				},
				Spec: v1.ServiceSpec{
					Type: v1.ServiceTypeLoadBalancer,
				},
			},
			wantDiscarded: false,
			description:   "LoadBalancer without annotation should be kept",
		},
		{
			name: "ClusterIP service with discard-clusterip annotation",
			obj: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "clusterip-svc",
					Namespace: "default",
					Annotations: map[string]string{
						DiscardClusterIPAnnotation: "true",
					},
				},
				Spec: v1.ServiceSpec{
					Type: v1.ServiceTypeClusterIP,
				},
			},
			wantDiscarded: true,
			description:   "ClusterIP with discard-clusterip annotation should be discarded",
		},
		{
			name: "ClusterIP service without annotation",
			obj: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "clusterip-svc",
					Namespace: "default",
				},
				Spec: v1.ServiceSpec{
					Type: v1.ServiceTypeClusterIP,
				},
			},
			wantDiscarded: false,
			description:   "ClusterIP without annotation should be kept",
		},
		{
			name: "Hardcoded service (backward compatibility)",
			obj: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "x-tunnel-server-internal-svc",
					Namespace: "kube-system",
				},
				Spec: v1.ServiceSpec{
					Type: v1.ServiceTypeClusterIP,
				},
			},
			wantDiscarded: true,
			description:   "Hardcoded service should still be discarded",
		},
		{
			name: "ClusterIP service with false annotation",
			obj: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "clusterip-svc",
					Namespace: "default",
					Annotations: map[string]string{
						DiscardClusterIPAnnotation: "false",
					},
				},
				Spec: v1.ServiceSpec{
					Type: v1.ServiceTypeClusterIP,
				},
			},
			wantDiscarded: false,
			description:   "ClusterIP with false annotation should be kept",
		},
		{
			name: "NodePort service should be kept",
			obj: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nodeport-svc",
					Namespace: "default",
				},
				Spec: v1.ServiceSpec{
					Type: v1.ServiceTypeNodePort,
				},
			},
			wantDiscarded: false,
			description:   "NodePort service should be kept",
		},
		{
			name: "Non-service object should be kept",
			obj: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cm",
					Namespace: "default",
				},
			},
			wantDiscarded: false,
			description:   "Non-service objects should pass through",
		},
		{
			name: "ClusterIP with empty annotation value",
			obj: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "clusterip-svc",
					Namespace: "default",
					Annotations: map[string]string{
						DiscardClusterIPAnnotation: "",
					},
				},
				Spec: v1.ServiceSpec{
					Type: v1.ServiceTypeClusterIP,
				},
			},
			wantDiscarded: false,
			description:   "ClusterIP with empty annotation should be kept",
		},
		{
			name: "Multiple annotations including discard-clusterip",
			obj: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "multi-anno-svc",
					Namespace: "default",
					Annotations: map[string]string{
						"other-annotation":         "value",
						DiscardClusterIPAnnotation: "true",
					},
				},
				Spec: v1.ServiceSpec{
					Type: v1.ServiceTypeClusterIP,
				},
			},
			wantDiscarded: true,
			description:   "ClusterIP with multiple annotations including discard should be discarded",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filter.Filter(tt.obj, nil)

			if tt.wantDiscarded {
				if result != nil {
					t.Errorf("%s: expected object to be discarded (nil), got %v", tt.description, result)
				}
			} else {
				if result == nil {
					t.Errorf("%s: expected object to be kept, got nil", tt.description)
				}
				// Verify it's the same object
				if !reflect.DeepEqual(result, tt.obj) {
					t.Errorf("%s: returned object differs from input", tt.description)
				}
			}
		})
	}
}

func TestDiscardCloudServiceFilter_Name(t *testing.T) {
	filter, _ := NewDiscardCloudServiceFilter()
	if filter.Name() != FilterName {
		t.Errorf("expected name %s, got %s", FilterName, filter.Name())
	}
}
