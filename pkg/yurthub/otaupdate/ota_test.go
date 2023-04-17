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
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/openyurtio/openyurt/pkg/yurthub/cachemanager"
	"github.com/openyurtio/openyurt/pkg/yurthub/healthchecker"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/rest"
	"github.com/openyurtio/openyurt/pkg/yurthub/otaupdate/util"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage/disk"
)

func TestGetPods(t *testing.T) {
	dir := t.TempDir()
	dStorage, err := disk.NewDiskStorage(dir)
	if err != nil {
		t.Errorf("failed to create disk storage, %v", err)
	}
	sWrapper := cachemanager.NewStorageWrapper(dStorage)

	updatablePod := util.NewPodWithCondition("updatablePod", "", corev1.ConditionTrue)
	notUpdatablePod := util.NewPodWithCondition("notUpdatablePod", "", corev1.ConditionFalse)
	normalPod := util.NewPod("normalPod", "")

	pods := []*corev1.Pod{updatablePod, notUpdatablePod, normalPod}
	for _, pod := range pods {
		key, err := sWrapper.KeyFunc(storage.KeyBuildInfo{
			Component: "kubelet",
			Resources: "pods",
			Namespace: "default",
			Group:     "",
			Version:   "v1",
			Name:      pod.Name,
		})
		if err != nil {
			t.Errorf("failed to get key, %v", err)
		}
		err = sWrapper.Create(key, pod)
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
	pod := util.NewPodWithCondition("nginx", DaemonPod, corev1.ConditionTrue)
	clientset := fake.NewSimpleClientset(pod)

	req, err := http.NewRequest("POST", "/openyurt.io/v1/namespaces/default/pods/nginx/update", nil)
	if err != nil {
		t.Fatal(err)
	}
	vars := map[string]string{
		"ns":      "default",
		"podname": "nginx",
	}
	req = mux.SetURLVars(req, vars)
	rr := httptest.NewRecorder()

	UpdatePod(clientset, "").ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestHealthyCheck(t *testing.T) {
	fakeHealthchecker := healthchecker.NewFakeChecker(false, nil)

	rcm, err := rest.NewRestConfigManager(nil, fakeHealthchecker)
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

func Test_preCheck(t *testing.T) {
	pod := util.NewPodWithCondition("nginx", "", corev1.ConditionTrue)
	pod.Spec.NodeName = "node"
	clientset := fake.NewSimpleClientset(pod)

	t.Run("Test_preCheck", func(t *testing.T) {
		_, ok := preCheck(clientset, metav1.NamespaceDefault, "nginx", "node")
		assert.Equal(t, true, ok)
	})

	t.Run("Test_preCheckOKGetPodFailed", func(t *testing.T) {
		_, ok := preCheck(clientset, metav1.NamespaceDefault, "nginx1", "node")
		assert.Equal(t, false, ok)
	})

	t.Run("Test_preCheckNodeNotMatch", func(t *testing.T) {
		_, ok := preCheck(clientset, metav1.NamespaceDefault, "nginx", "node1")
		assert.Equal(t, false, ok)
	})

	t.Run("Test_preCheckNotUpdatable", func(t *testing.T) {
		_, ok := preCheck(fake.NewSimpleClientset(util.NewPodWithCondition("nginx1", "", corev1.ConditionFalse)), metav1.NamespaceDefault, "nginx1", "node")
		assert.Equal(t, false, ok)
	})
}
