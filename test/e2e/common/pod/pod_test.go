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

package pod

import (
	"testing"

	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes/fake"
)

func TestGetPodByLabelAndNode(t *testing.T) {
	selector := labels.SelectorFromSet(labels.Set(map[string]string{"app": "yurt-e2e-test-nginx"}))

	t.Run("returns pod on requested node", func(t *testing.T) {
		client := fake.NewSimpleClientset(
			&apiv1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nginx-worker",
					Namespace: "default",
					Labels:    map[string]string{"app": "yurt-e2e-test-nginx"},
				},
				Spec: apiv1.PodSpec{NodeName: "openyurt-e2e-test-worker"},
			},
			&apiv1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nginx-worker2",
					Namespace: "default",
					Labels:    map[string]string{"app": "yurt-e2e-test-nginx"},
				},
				Spec: apiv1.PodSpec{NodeName: "openyurt-e2e-test-worker2"},
			},
		)

		pod, err := GetPodByLabelAndNode(client, "default", selector, "openyurt-e2e-test-worker2")
		if err != nil {
			t.Fatalf("GetPodByLabelAndNode() error = %v", err)
		}
		if pod.Name != "nginx-worker2" {
			t.Fatalf("GetPodByLabelAndNode() pod name = %s, want %s", pod.Name, "nginx-worker2")
		}
	})

	t.Run("ignores deleting pods", func(t *testing.T) {
		now := metav1.Now()
		client := fake.NewSimpleClientset(
			&apiv1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "nginx-worker2-deleting",
					Namespace:         "default",
					Labels:            map[string]string{"app": "yurt-e2e-test-nginx"},
					DeletionTimestamp: &now,
				},
				Spec: apiv1.PodSpec{NodeName: "openyurt-e2e-test-worker2"},
			},
			&apiv1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nginx-worker2",
					Namespace: "default",
					Labels:    map[string]string{"app": "yurt-e2e-test-nginx"},
				},
				Spec: apiv1.PodSpec{NodeName: "openyurt-e2e-test-worker2"},
			},
		)

		pod, err := GetPodByLabelAndNode(client, "default", selector, "openyurt-e2e-test-worker2")
		if err != nil {
			t.Fatalf("GetPodByLabelAndNode() error = %v", err)
		}
		if pod.Name != "nginx-worker2" {
			t.Fatalf("GetPodByLabelAndNode() pod name = %s, want %s", pod.Name, "nginx-worker2")
		}
	})

	t.Run("returns error when node has no matching pod", func(t *testing.T) {
		client := fake.NewSimpleClientset(&apiv1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "nginx-worker",
				Namespace: "default",
				Labels:    map[string]string{"app": "yurt-e2e-test-nginx"},
			},
			Spec: apiv1.PodSpec{NodeName: "openyurt-e2e-test-worker"},
		})

		if _, err := GetPodByLabelAndNode(client, "default", selector, "openyurt-e2e-test-worker2"); err == nil {
			t.Fatalf("GetPodByLabelAndNode() error = nil, want non-nil")
		}
	})
}
