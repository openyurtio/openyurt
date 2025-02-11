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
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/options"
	"github.com/openyurtio/openyurt/pkg/yurthub/cachemanager"
	"github.com/openyurtio/openyurt/pkg/yurthub/certificate/manager"
	"github.com/openyurtio/openyurt/pkg/yurthub/certificate/testdata"
	"github.com/openyurtio/openyurt/pkg/yurthub/healthchecker/cloudapiserver"
	"github.com/openyurtio/openyurt/pkg/yurthub/otaupdate/util"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage/disk"
	"github.com/openyurtio/openyurt/pkg/yurthub/transport"
)

func TestGetPods(t *testing.T) {
	dir := t.TempDir()
	dStorage, err := disk.NewDiskStorage(dir)
	if err != nil {
		t.Errorf("couldn't to create disk storage, %v", err)
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
			t.Errorf("couldn't to get key, %v", err)
		}
		err = sWrapper.Create(key, pod)
		if err != nil {
			t.Errorf("cloudn't to create obj, %v", err)
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
	testDir, err := os.MkdirTemp("", "test-client")
	if err != nil {
		t.Fatalf("failed to make temp dir, %v", err)
	}
	nodeName := "foo"
	servers := map[*url.URL]bool{
		{Host: "10.10.10.113:6443"}: false,
	}
	u, _ := url.Parse("https://10.10.10.113:6443")
	remoteServers := []*url.URL{u}
	fakeHealthchecker := cloudapiserver.NewFakeChecker(servers)

	client, err := testdata.CreateCertFakeClient("../certificate/testdata")
	if err != nil {
		t.Errorf("failed to create cert fake client, %v", err)
		return
	}
	certManager, err := manager.NewYurtHubCertManager(&options.YurtHubOptions{
		NodeName:      nodeName,
		RootDir:       testDir,
		YurtHubHost:   "127.0.0.1",
		JoinToken:     "123456.abcdef1234567890",
		ClientForTest: client,
	}, remoteServers)
	if err != nil {
		t.Errorf("failed to create certManager, %v", err)
		return
	}
	certManager.Start()
	defer certManager.Stop()
	defer os.RemoveAll(testDir)

	err = wait.PollUntilContextTimeout(context.Background(), 2*time.Second, 1*time.Minute, true, func(ctx context.Context) (done bool, err error) {
		if certManager.Ready() {
			return true, nil
		}
		return false, nil
	})

	if err != nil {
		t.Errorf("certificates are not ready, %v", err)
	}

	clientManager, err := transport.NewTransportAndClientManager(remoteServers, 10, certManager, context.Background().Done())
	if err != nil {
		t.Fatalf("could not new transport manager, %v", err)
	}

	req, err := http.NewRequest("POST", "", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()

	HealthyCheck(fakeHealthchecker, clientManager, "", UpdatePod).ServeHTTP(rr, req)
	assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
}

func Test_preCheck(t *testing.T) {
	pod := util.NewPodWithCondition("nginx", "", corev1.ConditionTrue)
	pod.Spec.NodeName = "node"
	clientset := fake.NewSimpleClientset(pod)

	t.Run("Test_preCheck", func(t *testing.T) {
		_, ok := preCheck(clientset, metav1.NamespaceDefault, "nginx", "node")
		assert.Equal(t, true, ok)
	})

	t.Run("Test_preCheckCanNotGetPod", func(t *testing.T) {
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
