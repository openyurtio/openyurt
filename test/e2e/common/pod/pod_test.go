package pod

import (
	"testing"
	"time"

	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	clientsetfake "k8s.io/client-go/kubernetes/fake"
)

func TestGetPodByLabelAndNodeName(t *testing.T) {
	now := metav1.NewTime(time.Now())
	client := clientsetfake.NewSimpleClientset(
		&apiv1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "nginx-worker1",
				Namespace: "default",
				Labels: map[string]string{
					"app": "yurt-e2e-test-nginx",
				},
			},
			Spec: apiv1.PodSpec{
				NodeName: "openyurt-e2e-test-worker",
			},
		},
		&apiv1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "nginx-worker2-deleting",
				Namespace:         "default",
				DeletionTimestamp: &now,
				Labels: map[string]string{
					"app": "yurt-e2e-test-nginx",
				},
			},
			Spec: apiv1.PodSpec{
				NodeName: "openyurt-e2e-test-worker2",
			},
		},
		&apiv1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "nginx-worker2",
				Namespace: "default",
				Labels: map[string]string{
					"app": "yurt-e2e-test-nginx",
				},
			},
			Spec: apiv1.PodSpec{
				NodeName: "openyurt-e2e-test-worker2",
			},
		},
		&apiv1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "other-worker2",
				Namespace: "default",
				Labels: map[string]string{
					"app": "other",
				},
			},
			Spec: apiv1.PodSpec{
				NodeName: "openyurt-e2e-test-worker2",
			},
		},
	)

	pod, err := GetPodByLabelAndNodeName(
		client,
		"default",
		"openyurt-e2e-test-worker2",
		labels.SelectorFromSet(labels.Set(map[string]string{"app": "yurt-e2e-test-nginx"})),
	)
	if err != nil {
		t.Fatalf("GetPodByLabelAndNodeName() error = %v", err)
	}
	if pod.Name != "nginx-worker2" {
		t.Fatalf("GetPodByLabelAndNodeName() pod name = %s, want %s", pod.Name, "nginx-worker2")
	}
}

func TestGetPodByLabelAndNodeNameNotFound(t *testing.T) {
	client := clientsetfake.NewSimpleClientset(
		&apiv1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "nginx-worker1",
				Namespace: "default",
				Labels: map[string]string{
					"app": "yurt-e2e-test-nginx",
				},
			},
			Spec: apiv1.PodSpec{
				NodeName: "openyurt-e2e-test-worker",
			},
		},
	)

	_, err := GetPodByLabelAndNodeName(
		client,
		"default",
		"openyurt-e2e-test-worker2",
		labels.SelectorFromSet(labels.Set(map[string]string{"app": "yurt-e2e-test-nginx"})),
	)
	if err == nil {
		t.Fatal("GetPodByLabelAndNodeName() error = nil, want error")
	}
}
