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
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/config"
	"github.com/openyurtio/openyurt/pkg/controller/daemonpodupdater"
	"github.com/openyurtio/openyurt/pkg/yurthub/cachemanager"
	"github.com/openyurtio/openyurt/pkg/yurthub/healthchecker"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/rest"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage/disk"
)

func newPod(podName string) *corev1.Pod {
	pod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Pod",
		},
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: podName,
			Namespace:    metav1.NamespaceDefault,
		},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{},
		},
	}
	pod.Name = podName
	return pod
}

func newPodWithCondition(podName string, ready corev1.ConditionStatus) *corev1.Pod {
	pod := newPod(podName)
	SetPodUpgradeCondition(pod, ready)

	return pod
}

func SetPodUpgradeCondition(pod *corev1.Pod, ready corev1.ConditionStatus) {
	cond := corev1.PodCondition{
		Type:   daemonpodupdater.PodNeedUpgrade,
		Status: ready,
	}
	pod.Status.Conditions = append(pod.Status.Conditions, cond)
}

func TestGetPods(t *testing.T) {
	dir := t.TempDir()
	dStorage, err := disk.NewDiskStorage(dir)
	if err != nil {
		t.Errorf("failed to create disk storage, %v", err)
	}
	sWrapper := cachemanager.NewStorageWrapper(dStorage)

	updatablePod := newPodWithCondition("updatablePod", corev1.ConditionTrue)
	notUpdatablePod := newPodWithCondition("notUpdatablePod", corev1.ConditionFalse)
	normalPod := newPod("normalPod")

	pods := []*corev1.Pod{updatablePod, notUpdatablePod, normalPod}
	for _, pod := range pods {
		err = sWrapper.Create(fmt.Sprintf("kubelet/pods/default/%s", pod.Name), pod)
		if err != nil {
			t.Errorf("failed to create obj, %v", err)
		}
	}

	req, err := http.NewRequest("GET", "/openyurt.io/v1/pods", nil)
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()

	GetPods(sWrapper).ServeHTTP(rr, req)

	expectedCode := http.StatusOK
	assert.Equal(t, expectedCode, rr.Code)
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
			pod:          newPodWithCondition("updatablePod", corev1.ConditionTrue),
			expectedCode: http.StatusOK,
			expectedData: "Start updating pod default/updatablePod",
		},
		{
			reqURL:       "/openyurt.io/v1/namespaces/default/pods/notUpdatablePod/update",
			podName:      "notUpdatablePod",
			pod:          newPodWithCondition("notUpdatablePod", corev1.ConditionFalse),
			expectedCode: http.StatusForbidden,
			expectedData: "Pod is not-updatable",
		},
		{
			reqURL:       "/openyurt.io/v1/namespaces/default/pods/wrongName/update",
			podName:      "wrongName",
			pod:          newPodWithCondition("trueName", corev1.ConditionFalse),
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

		UpdatePod(clientset, "").ServeHTTP(rr, req)

		assert.Equal(t, test.expectedCode, rr.Code)
		assert.Equal(t, test.expectedData, rr.Body.String())
	}

}

func TestHealthyCheck(t *testing.T) {
	u, _ := url.Parse("https://10.10.10.113:6443")
	fakeHealthchecker := healthchecker.NewFakeChecker(false, nil)
	cfg := &config.YurtHubConfiguration{
		RemoteServers: []*url.URL{u},
	}

	rcm, err := rest.NewRestConfigManager(cfg, nil, fakeHealthchecker)
	if err != nil {
		t.Fatal(err)
	}

	req, err := http.NewRequest("POST", "", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()

	HealthyCheck(rcm, "", UpdatePod).ServeHTTP(rr, req)
	assert.Equal(t, http.StatusForbidden, rr.Code)
}
