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
	fakeHealthChecker "github.com/openyurtio/openyurt/pkg/yurthub/healthchecker/fake"
	"github.com/openyurtio/openyurt/pkg/yurthub/otaupdate/util"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage/disk"
	"github.com/openyurtio/openyurt/pkg/yurthub/transport"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/daemonsetupgradestrategy"
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

// Test GetPods error scenarios
func TestGetPodsErrors(t *testing.T) {
	tests := []struct {
		name           string
		setupMock      func(cachemanager.StorageWrapper)
		expectedStatus int
		expectedBody   string
	}{
		{
			name: "key function error",
			setupMock: func(sWrapper cachemanager.StorageWrapper) {
				// Don't create any pods, so key function will fail
			},
			expectedStatus: http.StatusInternalServerError, // This will return 500 when list fails
			expectedBody:   "Get pod list failed",
		},
		{
			name: "list error",
			setupMock: func(sWrapper cachemanager.StorageWrapper) {
				// Create a pod with invalid data to cause list error
				key, _ := sWrapper.KeyFunc(storage.KeyBuildInfo{
					Component: "kubelet",
					Resources: "pods",
					Namespace: "default",
					Group:     "",
					Version:   "v1",
					Name:      "invalidPod",
				})
				// Create with invalid pod data
				invalidPod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "invalidPod",
					},
				}
				sWrapper.Create(key, invalidPod)
			},
			expectedStatus: http.StatusOK, // This will still work with valid pod
			expectedBody:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			dStorage, err := disk.NewDiskStorage(dir)
			if err != nil {
				t.Errorf("couldn't to create disk storage, %v", err)
			}
			sWrapper := cachemanager.NewStorageWrapper(dStorage)

			tt.setupMock(sWrapper)

			req, err := http.NewRequest("GET", "/openyurt.io/v1/pods", nil)
			if err != nil {
				t.Fatal(err)
			}
			rr := httptest.NewRecorder()

			GetPods(sWrapper).ServeHTTP(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)
			if tt.expectedBody != "" {
				assert.Contains(t, rr.Body.String(), tt.expectedBody)
			}
		})
	}
}

func TestUpdatePod(t *testing.T) {
	tests := []struct {
		name           string
		pod            *corev1.Pod
		nodeName       string
		expectedStatus int
		expectedBody   string
		setupMock      func(*fake.Clientset)
		podName        string
	}{
		{
			name:           "successful daemon pod update",
			pod:            createDaemonPod("nginx", "default", "node1"),
			nodeName:       "node1",
			expectedStatus: http.StatusOK,
			expectedBody:   "Start updating pod default/nginx",
			podName:        "nginx",
		},
		{
			name:           "static pod configmap not found",
			pod:            createStaticPod("nginx-node1", "default", "node1"),
			nodeName:       "node1",
			expectedStatus: http.StatusForbidden,
			expectedBody:   "Configmap for static pod does not exist",
			podName:        "nginx-node1",
			setupMock: func(cs *fake.Clientset) {
				// Don't create ConfigMap, so it will be not found
			},
		},
		{
			name:           "pod not found",
			pod:            nil,
			nodeName:       "node1",
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   "Get pod failed",
			podName:        "nginx",
		},
		{
			name:           "pre-check failed - wrong node",
			pod:            createDaemonPod("nginx", "default", "node1"),
			nodeName:       "node2",
			expectedStatus: http.StatusForbidden,
			expectedBody:   "Pre check update pod failed",
			podName:        "nginx",
		},
		{
			name:           "pre-check failed - pod deleting",
			pod:            createDeletingPod("nginx", "default", "node1"),
			nodeName:       "node1",
			expectedStatus: http.StatusForbidden,
			expectedBody:   "Pre check update pod failed",
			podName:        "nginx",
		},
		{
			name:           "pre-check failed - pod not updatable",
			pod:            createNonUpdatablePod("nginx", "default", "node1"),
			nodeName:       "node1",
			expectedStatus: http.StatusForbidden,
			expectedBody:   "Pre check update pod failed",
			podName:        "nginx",
		},
		{
			name:           "unsupported pod type",
			pod:            createReplicaSetPod("nginx", "default"),
			nodeName:       "node1",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Not support ota upgrade pod type ReplicaSet",
			podName:        "nginx",
		},
		{
			name:           "static pod with configmap found but apply fails",
			pod:            createStaticPod("nginx-node1", "default", "node1"),
			nodeName:       "node1",
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   "Apply update failed",
			podName:        "nginx-node1",
			setupMock: func(cs *fake.Clientset) {
				// Add ConfigMap for static pod
				cm := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "yurt-static-set-nginx",
						Namespace: "default",
					},
					Data: map[string]string{
						"nginx.yaml": "apiVersion: v1\nkind: Pod\nmetadata:\n  name: nginx\nspec:\n  containers:\n  - name: nginx\n    image: nginx:latest",
					},
				}
				cs.CoreV1().ConfigMaps("default").Create(context.TODO(), cm, metav1.CreateOptions{})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var clientset *fake.Clientset
			if tt.pod != nil {
				clientset = fake.NewSimpleClientset(tt.pod)
			} else {
				clientset = fake.NewSimpleClientset()
			}

			if tt.setupMock != nil {
				tt.setupMock(clientset)
			}

			req, err := http.NewRequest("POST", "/openyurt.io/v1/namespaces/default/pods/nginx/update", nil)
			if err != nil {
				t.Fatal(err)
			}
			vars := map[string]string{
				"ns":      "default",
				"podname": tt.podName,
			}
			req = mux.SetURLVars(req, vars)
			rr := httptest.NewRecorder()

			UpdatePod(clientset, tt.nodeName).ServeHTTP(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)
			if tt.expectedBody != "" {
				assert.Contains(t, rr.Body.String(), tt.expectedBody)
			}
		})
	}
}

// Helper functions to create different types of pods for testing
func createDaemonPod(name, namespace, nodeName string) *corev1.Pod {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			OwnerReferences: []metav1.OwnerReference{
				{Kind: DaemonPod},
			},
		},
		Spec: corev1.PodSpec{
			NodeName: nodeName,
		},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{
				{
					Type:   daemonsetupgradestrategy.PodNeedUpgrade,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}
	return pod
}

func createStaticPod(name, namespace, nodeName string) *corev1.Pod {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			OwnerReferences: []metav1.OwnerReference{
				{Kind: StaticPod},
			},
		},
		Spec: corev1.PodSpec{
			NodeName: nodeName,
		},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{
				{
					Type:   daemonsetupgradestrategy.PodNeedUpgrade,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}
	return pod
}

func createDeletingPod(name, namespace, nodeName string) *corev1.Pod {
	now := metav1.Now()
	pod := createDaemonPod(name, namespace, nodeName)
	pod.DeletionTimestamp = &now
	return pod
}

func createNonUpdatablePod(name, namespace, nodeName string) *corev1.Pod {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			OwnerReferences: []metav1.OwnerReference{
				{Kind: DaemonPod},
			},
		},
		Spec: corev1.PodSpec{
			NodeName: nodeName,
		},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{
				{
					Type:   daemonsetupgradestrategy.PodNeedUpgrade,
					Status: corev1.ConditionFalse,
				},
			},
		},
	}
	return pod
}

func createReplicaSetPod(name, namespace string) *corev1.Pod {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "ReplicaSet"},
			},
		},
		Spec: corev1.PodSpec{
			NodeName: "node1",
		},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{
				{
					Type:   daemonsetupgradestrategy.PodNeedUpgrade,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}
	return pod
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
	fakeHealthchecker := fakeHealthChecker.NewFakeChecker(servers)

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

	t.Run("Test_preCheck", func(t *testing.T) {
		err := preCheckUpdatePod(pod, "node")
		assert.NoError(t, err)
	})

	t.Run("Test_preCheckNodeNotMatch", func(t *testing.T) {
		err := preCheckUpdatePod(pod, "node1")
		assert.Error(t, err)
	})

	t.Run("Test_preCheckNotUpdatable", func(t *testing.T) {
		err := preCheckUpdatePod(util.NewPodWithCondition("nginx1", "", corev1.ConditionFalse), "node")
		assert.Error(t, err)
	})
}

// Test additional edge cases and error scenarios
func TestUpdatePodEdgeCases(t *testing.T) {
	tests := []struct {
		name           string
		pod            *corev1.Pod
		nodeName       string
		expectedStatus int
		expectedBody   string
		setupMock      func(*fake.Clientset)
		podName        string
	}{
		{
			name:           "pod with image ready condition false",
			pod:            createPodWithImageReadyFalse("nginx", "default", "node1"),
			nodeName:       "node1",
			expectedStatus: http.StatusForbidden,
			expectedBody:   "Pre check update pod failed",
			podName:        "nginx",
		},
		{
			name:           "pod with image ready condition but wrong version",
			pod:            createPodWithWrongImageVersion("nginx", "default", "node1"),
			nodeName:       "node1",
			expectedStatus: http.StatusForbidden,
			expectedBody:   "Pre check update pod failed",
			podName:        "nginx",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var clientset *fake.Clientset
			if tt.pod != nil {
				clientset = fake.NewSimpleClientset(tt.pod)
			} else {
				clientset = fake.NewSimpleClientset()
			}

			if tt.setupMock != nil {
				tt.setupMock(clientset)
			}

			req, err := http.NewRequest("POST", "/openyurt.io/v1/namespaces/default/pods/nginx/update", nil)
			if err != nil {
				t.Fatal(err)
			}
			vars := map[string]string{
				"ns":      "default",
				"podname": tt.podName,
			}
			req = mux.SetURLVars(req, vars)
			rr := httptest.NewRecorder()

			UpdatePod(clientset, tt.nodeName).ServeHTTP(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)
			if tt.expectedBody != "" {
				assert.Contains(t, rr.Body.String(), tt.expectedBody)
			}
		})
	}
}

// Test helper functions for edge cases
func createPodWithImageReadyFalse(name, namespace, nodeName string) *corev1.Pod {
	pod := createDaemonPod(name, namespace, nodeName)
	pod.Status.Conditions = append(pod.Status.Conditions, corev1.PodCondition{
		Type:    daemonsetupgradestrategy.PodImageReady,
		Status:  corev1.ConditionFalse,
		Reason:  "ImagePullBackOff",
		Message: "Failed to pull image",
	})
	return pod
}

func createPodWithWrongImageVersion(name, namespace, nodeName string) *corev1.Pod {
	pod := createDaemonPod(name, namespace, nodeName)
	pod.Status.Conditions = append(pod.Status.Conditions, corev1.PodCondition{
		Type:    daemonsetupgradestrategy.PodImageReady,
		Status:  corev1.ConditionTrue,
		Reason:  "Ready",
		Message: daemonsetupgradestrategy.VersionPrefix + "wrong-version",
	})
	return pod
}

// Test helper functions directly
func TestCheckPodStatus(t *testing.T) {
	tests := []struct {
		name      string
		pod       *corev1.Pod
		nodeName  string
		expectErr bool
	}{
		{
			name:      "valid pod",
			pod:       createDaemonPod("nginx", "default", "node1"),
			nodeName:  "node1",
			expectErr: false,
		},
		{
			name:      "wrong node",
			pod:       createDaemonPod("nginx", "default", "node1"),
			nodeName:  "node2",
			expectErr: true,
		},
		{
			name:      "deleting pod",
			pod:       createDeletingPod("nginx", "default", "node1"),
			nodeName:  "node1",
			expectErr: true,
		},
		{
			name:      "not updatable pod",
			pod:       createNonUpdatablePod("nginx", "default", "node1"),
			nodeName:  "node1",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkPodStatus(tt.pod, tt.nodeName)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCheckPodImageReady(t *testing.T) {
	tests := []struct {
		name      string
		pod       *corev1.Pod
		expectErr bool
	}{
		{
			name:      "no image ready condition",
			pod:       createDaemonPod("nginx", "default", "node1"),
			expectErr: false,
		},
		{
			name:      "image ready condition false",
			pod:       createPodWithImageReadyFalse("nginx", "default", "node1"),
			expectErr: true,
		},
		{
			name:      "image ready condition true with wrong version",
			pod:       createPodWithWrongImageVersion("nginx", "default", "node1"),
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkPodImageReady(tt.pod)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetPodImageReadyCondition(t *testing.T) {
	pod := createDaemonPod("nginx", "default", "node1")

	// Test with no image ready condition
	cond := getPodImageReadyCondition(pod)
	assert.Nil(t, cond)

	// Test with image ready condition
	pod.Status.Conditions = append(pod.Status.Conditions, corev1.PodCondition{
		Type:   daemonsetupgradestrategy.PodImageReady,
		Status: corev1.ConditionTrue,
	})
	cond = getPodImageReadyCondition(pod)
	assert.NotNil(t, cond)
	assert.Equal(t, daemonsetupgradestrategy.PodImageReady, cond.Type)
}

// Test PullPodImage function
func TestImagePullPod(t *testing.T) {
	tests := []struct {
		name           string
		pod            *corev1.Pod
		nodeName       string
		expectedStatus int
		expectedBody   string
		podName        string
	}{
		{
			name:           "successful image pull request",
			pod:            createDaemonPod("nginx", "default", "node1"),
			nodeName:       "node1",
			expectedStatus: http.StatusOK,
			expectedBody:   "Image pre-pull requested for pod default/nginx",
			podName:        "nginx",
		},
		{
			name:           "pod not found",
			pod:            nil,
			nodeName:       "node1",
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   "Get pod failed",
			podName:        "nginx",
		},
		{
			name:           "wrong node",
			pod:            createDaemonPod("nginx", "default", "node1"),
			nodeName:       "node2",
			expectedStatus: http.StatusForbidden,
			expectedBody:   "Failed to check pod status",
			podName:        "nginx",
		},
		{
			name:           "deleting pod",
			pod:            createDeletingPod("nginx", "default", "node1"),
			nodeName:       "node1",
			expectedStatus: http.StatusForbidden,
			expectedBody:   "Failed to check pod status",
			podName:        "nginx",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var clientset *fake.Clientset
			if tt.pod != nil {
				clientset = fake.NewSimpleClientset(tt.pod)
			} else {
				clientset = fake.NewSimpleClientset()
			}

			req, err := http.NewRequest("POST", "/openyurt.io/v1/namespaces/default/pods/nginx/imagepull", nil)
			if err != nil {
				t.Fatal(err)
			}
			vars := map[string]string{
				"ns":      "default",
				"podname": tt.podName,
			}
			req = mux.SetURLVars(req, vars)
			rr := httptest.NewRecorder()

			PullPodImage(clientset, tt.nodeName).ServeHTTP(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)
			if tt.expectedBody != "" {
				assert.Contains(t, rr.Body.String(), tt.expectedBody)
			}
		})
	}
}
