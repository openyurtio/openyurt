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

func setPodUpdatableAnnotation(pod *corev1.Pod, ok bool) {
	metav1.SetMetaDataAnnotation(&pod.ObjectMeta, PodUpdatableAnnotation, strconv.FormatBool(ok))
}

func TestGetPods(t *testing.T) {
	updatablePod := newPod("updatable")
	setPodUpdatableAnnotation(updatablePod, true)
	notUpdatablePod := newPod("not-updatable")
	setPodUpdatableAnnotation(notUpdatablePod, false)

	clientset := fake.NewSimpleClientset(updatablePod, notUpdatablePod)

	req, err := http.NewRequest("POST", "/", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()

	GetPods(clientset).ServeHTTP(rr, req)

	t.Logf("rr code is %+v, rr body is %+v", rr.Code, rr.Body)
	// if status := rr.Code; status != http.StatusOK {
	// 	t.Errorf("handler returned wrong status code: got %v want %v",
	// 		status, http.StatusOK)
	// }

	expected := `[{"Namespace":"default","PodName":"not-updatable","Updatable":false},{"Namespace":"default","PodName":"updatable","Updatable":true}]`

	// data := json.Unmarshal(rr.Body.Bytes(),)
	assert.Equal(t, expected, rr.Body.String())
	// if rr.Body.String() != expected {
	// 	t.Errorf("handler returned unexpected body: got %v want %v",
	// 		rr.Body.String(), expected)
	// }

}

func TestUpdatePod(t *testing.T) {

	updatablePod := newPod("updatable-pod")
	setPodUpdatableAnnotation(updatablePod, true)
	notUpdatablePod := newPod("not-updatable-pod")
	setPodUpdatableAnnotation(notUpdatablePod, false)

	clientset := fake.NewSimpleClientset(updatablePod, notUpdatablePod)

	req, err := http.NewRequest("POST", "/openyurt.io/v1/namespaces/default/pods/updatable-pod/update", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()

	vars := map[string]string{
		"ns":      "default",
		"podname": "updatable-pod",
	}
	req = mux.SetURLVars(req, vars)

	UpdatePod(clientset).ServeHTTP(rr, req)

	t.Logf("rr code is %+v, rr body is %+v", rr.Code, rr.Body)

}
