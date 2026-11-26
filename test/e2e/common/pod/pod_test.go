/*
Copyright 2026 The OpenYurt Authors.

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
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	clientsetfake "k8s.io/client-go/kubernetes/fake"
)

func TestGetPodBySelectorOnNode(t *testing.T) {
	labelSelector := labels.SelectorFromSet(labels.Set(map[string]string{"app": "yurt-e2e-test-nginx"}))
	client := clientsetfake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "worker1-pod",
				Namespace: "default",
				Labels:    map[string]string{"app": "yurt-e2e-test-nginx"},
			},
			Spec: corev1.PodSpec{
				NodeName: "openyurt-e2e-test-worker",
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "worker2-pod",
				Namespace: "default",
				Labels:    map[string]string{"app": "yurt-e2e-test-nginx"},
			},
			Spec: corev1.PodSpec{
				NodeName: "openyurt-e2e-test-worker2",
			},
		},
	)

	pod, err := GetPodBySelectorOnNode(client, "default", labelSelector, "openyurt-e2e-test-worker2")
	if err != nil {
		t.Fatalf("GetPodBySelectorOnNode returned error: %v", err)
	}
	if pod.Name != "worker2-pod" {
		t.Fatalf("expected worker2-pod, got %s", pod.Name)
	}
}

func TestGetPodBySelectorOnNodeNotFound(t *testing.T) {
	labelSelector := labels.SelectorFromSet(labels.Set(map[string]string{"app": "yurt-e2e-test-nginx"}))
	client := clientsetfake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "worker1-pod",
				Namespace: "default",
				Labels:    map[string]string{"app": "yurt-e2e-test-nginx"},
			},
			Spec: corev1.PodSpec{
				NodeName: "openyurt-e2e-test-worker",
			},
		},
	)

	if _, err := GetPodBySelectorOnNode(client, "default", labelSelector, "openyurt-e2e-test-worker2"); err == nil {
		t.Fatal("expected GetPodBySelectorOnNode to fail when no pod matches the node")
	}
}

func TestGetPodBySelectorOnNodeSkipsTerminatingPods(t *testing.T) {
	now := metav1.NewTime(time.Now())
	labelSelector := labels.SelectorFromSet(labels.Set(map[string]string{"app": "yurt-e2e-test-nginx"}))
	client := clientsetfake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "terminating-worker2-pod",
				Namespace:         "default",
				Labels:            map[string]string{"app": "yurt-e2e-test-nginx"},
				DeletionTimestamp: &now,
			},
			Spec: corev1.PodSpec{
				NodeName: "openyurt-e2e-test-worker2",
			},
		},
	)

	if _, err := GetPodBySelectorOnNode(client, "default", labelSelector, "openyurt-e2e-test-worker2"); err == nil {
		t.Fatal("expected GetPodBySelectorOnNode to ignore terminating pods on the target node")
	}
}
