/*
Copyright 2022 The OpenYurt Authors.

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

package otaupdate

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func newPod(podName string) *corev1.Pod {
	pod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: podName,
			Namespace:    metav1.NamespaceDefault,
		},
	}
	pod.Name = podName
	return pod
}

func newPodWithAnnotation(podName string, ready bool) *corev1.Pod {
	pod := newPod(podName)
	setPodUpdatableAnnotation(pod, ready)

	return pod
}

func setPodUpdatableAnnotation(pod *corev1.Pod, ok bool) {
	metav1.SetMetaDataAnnotation(&pod.ObjectMeta, PodUpdatableAnnotation, strconv.FormatBool(ok))
}

func TestGetPods(t *testing.T) {
	updatablePod := newPodWithAnnotation("updatablePod", true)
	notUpdatablePod := newPodWithAnnotation("notUpdatablePod", false)
	normalPod := newPod("normalPod")

	clientset := fake.NewSimpleClientset(updatablePod, notUpdatablePod, normalPod)

	req, err := http.NewRequest("GET", "/openyurt.io/v1/pods", nil)
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()

	GetPods(clientset).ServeHTTP(rr, req)

	expectedCode := http.StatusOK
	expectedData := `[{"Namespace":"default","PodName":"normalPod","Updatable":false},{"Namespace":"default","PodName":"notUpdatablePod","Updatable":false},{"Namespace":"default","PodName":"updatablePod","Updatable":true}]`

	assert.Equal(t, expectedCode, rr.Code)
	assert.Equal(t, expectedData, rr.Body.String())
}

func TestUpdatePod(t *testing.T) {
	tests := []struct {
		reqURL       string
		pod          *corev1.Pod
		podName      string
		expectedCode int
		expectedData string
	}{
		{
			reqURL:       "/openyurt.io/v1/namespaces/default/pods/updatablePod/update",
			podName:      "updatablePod",
			pod:          newPodWithAnnotation("updatablePod", true),
			expectedCode: http.StatusOK,
			expectedData: "",
		},
		{
			reqURL:       "/openyurt.io/v1/namespaces/default/pods/notUpdatablePod/update",
			podName:      "notUpdatablePod",
			pod:          newPodWithAnnotation("notUpdatablePod", false),
			expectedCode: http.StatusForbidden,
			expectedData: "Pod is not-updatable",
		},
		{
			reqURL:       "/openyurt.io/v1/namespaces/default/pods/wrongName/update",
			podName:      "wrongName",
			pod:          newPodWithAnnotation("trueName", true),
			expectedCode: http.StatusInternalServerError,
			expectedData: "Apply update failed",
		},
	}
	for _, test := range tests {
		clientset := fake.NewSimpleClientset(test.pod)

		req, err := http.NewRequest("POST", test.reqURL, nil)
		if err != nil {
			t.Fatal(err)
		}
		vars := map[string]string{
			"ns":      "default",
			"podname": test.podName,
		}
		req = mux.SetURLVars(req, vars)
		rr := httptest.NewRecorder()

		UpdatePod(clientset).ServeHTTP(rr, req)

		assert.Equal(t, test.expectedCode, rr.Code)
		assert.Equal(t, test.expectedData, rr.Body.String())
	}

}
