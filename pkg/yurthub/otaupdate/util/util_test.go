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

package util

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"

	yurtutil "github.com/openyurtio/openyurt/pkg/util"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/daemonsetupgradestrategy"
)

func TestEncodePods(t *testing.T) {
	// Initialize scheme for runtime codec
	scheme.AddToScheme(scheme.Scheme)

	// Create a pod list
	podList := &corev1.PodList{
		// Add necessary initialization for the pod list if needed
	}

	// Encode the pod list
	data, err := EncodePods(podList)
	if err != nil {
		t.Fatalf("EncodePods returned an error: %v", err)
	}

	// Verify the encoded data is not nil
	if data == nil {
		t.Error("EncodePods returned nil, expected non-nil data")
	}
}

func TestWriteErr(t *testing.T) {
	// Create a request and response recorder
	http.NewRequest("GET", "/somepath", nil)
	rr := httptest.NewRecorder()

	// Call WriteErr with a status and error message
	WriteErr(rr, "error message", http.StatusInternalServerError)

	// Check the status code and body
	if status := rr.Code; status != http.StatusInternalServerError {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusInternalServerError)
	}
	expected := "error message"
	if rr.Body.String() != expected {
		t.Errorf("handler returned unexpected body: got %v want %v", rr.Body.String(), expected)
	}
}

func TestWriteJSONResponse(t *testing.T) {
	// Create a request and response recorder
	http.NewRequest("GET", "/somepath", nil)
	rr := httptest.NewRecorder()

	// Test with non-nil data
	data := []byte(`{"key": "value"}`)
	WriteJSONResponse(rr, data)

	// Check the status code, headers and body
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}
	if content := rr.Header().Get(yurtutil.HttpHeaderContentType); content != yurtutil.HttpContentTypeJson {
		t.Errorf("handler returned wrong content type: got %v want %v", content, yurtutil.HttpContentTypeJson)
	}
	if rr.Body.String() != string(data) {
		t.Errorf("handler returned unexpected body: got %v want %v", rr.Body.String(), string(data))
	}

	// Test with nil data
	WriteJSONResponse(rr, nil)

	// Check the status code again to ensure it's still http.StatusOK
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code after nil data: got %v want %v", status, http.StatusOK)
	}
}

func TestNewPod(t *testing.T) {
	podName := "test-pod"
	kind := "DaemonSet"

	pod := NewPod(podName, kind)

	assert.NotNil(t, pod, "NewPod should return a non-nil pod")
	assert.Equal(t, podName, pod.Name, "Pod name should be set correctly")
	assert.Equal(t, metav1.NamespaceDefault, pod.Namespace, "Pod namespace should be set to default")
	assert.NotNil(t, pod.OwnerReferences, "Pod owner references should not be nil")
	assert.Equal(t, kind, pod.OwnerReferences[0].Kind, "Pod owner reference kind should be set correctly")
	assert.NotNil(t, pod.Status.Conditions, "Pod status conditions should not be nil")
}

func TestNewPodWithCondition(t *testing.T) {
	podName := "test-pod-with-condition"
	kind := "DaemonSet"
	ready := corev1.ConditionStatus(corev1.ConditionTrue)

	pod := NewPodWithCondition(podName, kind, ready)

	assert.NotNil(t, pod, "NewPodWithCondition should return a non-nil pod")
	assert.Equal(t, podName, pod.Name, "Pod name should be set correctly")
	assert.Equal(t, kind, pod.OwnerReferences[0].Kind, "Pod owner reference kind should be set correctly")
	assert.NotNil(t, pod.Status.Conditions, "Pod status conditions should not be nil")
	assert.Equal(t, 1, len(pod.Status.Conditions), "Pod should have exactly one condition")
	assert.Equal(t, daemonsetupgradestrategy.PodNeedUpgrade, pod.Status.Conditions[0].Type, "Pod condition type should be set correctly")
	assert.Equal(t, ready, pod.Status.Conditions[0].Status, "Pod condition status should be set correctly")
}

func TestSetPodUpgradeCondition(t *testing.T) {
	pod := &corev1.Pod{
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{},
		},
	}
	ready := corev1.ConditionStatus(corev1.ConditionTrue)

	SetPodUpgradeCondition(pod, ready)

	assert.NotNil(t, pod.Status.Conditions, "Pod status conditions should not be nil after setting condition")
	assert.Equal(t, 1, len(pod.Status.Conditions), "Pod should have exactly one condition after setting condition")
	assert.Equal(t, daemonsetupgradestrategy.PodNeedUpgrade, pod.Status.Conditions[0].Type, "Pod condition type should be set correctly")
	assert.Equal(t, ready, pod.Status.Conditions[0].Status, "Pod condition status should be set correctly")
}

func TestRemoveNodeNameFromStaticPod(t *testing.T) {
	tests := []struct {
		podName     string
		nodeName    string
		expected    bool
		expectedPod string
	}{
		{"test-pod", "node1", false, ""},
		{"test-pod-node1", "node1", true, "test-pod"},
		{"test-pod-node1-node2", "node1", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.podName+"-"+tt.nodeName, func(t *testing.T) {
			result, podName := RemoveNodeNameFromStaticPod(tt.podName, tt.nodeName)
			assert.Equal(t, tt.expected, result, "Expected and actual result should match")
			assert.Equal(t, tt.expectedPod, podName, "Expected and actual pod name should match")
		})
	}
}
