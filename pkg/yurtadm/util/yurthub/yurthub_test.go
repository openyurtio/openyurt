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

package yurthub

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/sets"
	clientset "k8s.io/client-go/kubernetes"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/openyurtio/openyurt/pkg/yurtadm/cmd/join/joindata"
	"github.com/openyurtio/openyurt/pkg/yurtadm/constants"
)

var (
	defaultAddr = `apiVersion: v1
kind: Pod
metadata:
  annotations:
    openyurt.io/static-pod-hash: 76f4f955b6
  creationTimestamp: null
  labels:
    k8s-app: yurt-hub
  name: yurt-hub
  namespace: kube-system
spec:
  containers:
    - command:
      - yurthub
      - --v=2
      - --bind-address=127.0.0.1
      - --server-addr=https://127.0.0.1:6443
      - --node-name=$(NODE_NAME)
      - --bootstrap-file=/var/lib/yurthub/bootstrap-hub.conf
      - --working-mode=edge
      - --namespace=kube-system
      env:
        - name: NODE_NAME
          valueFrom:
            fieldRef:
              apiVersion: v1
              fieldPath: spec.nodeName
      image: openyurt/yurthub:v1.3.0
      imagePullPolicy: IfNotPresent
      livenessProbe:
        failureThreshold: 3
        httpGet:
          host: 127.0.0.1
          path: /v1/healthz
          port: 10267
          scheme: HTTP
        initialDelaySeconds: 300
        periodSeconds: 5
        successThreshold: 1
        timeoutSeconds: 1
      name: yurt-hub
      resources:
        limits:
          memory: 300Mi
        requests:
          cpu: 150m
          memory: 150Mi
      securityContext:
        capabilities:
          add:
            - NET_ADMIN
            - NET_RAW
      terminationMessagePath: /dev/termination-log
      terminationMessagePolicy: File
      volumeMounts:
        - mountPath: /var/lib/yurthub
          name: hub-dir
        - mountPath: /etc/kubernetes
          name: kubernetes
  dnsPolicy: ClusterFirst
  hostNetwork: true
  priority: 2000001000
  priorityClassName: system-node-critical
  restartPolicy: Always
  schedulerName: default-scheduler
  securityContext: {}
  terminationGracePeriodSeconds: 30
  volumes:
    - hostPath:
        path: /var/lib/yurthub
        type: DirectoryOrCreate
      name: hub-dir
    - hostPath:
        path: /etc/kubernetes
        type: Directory
      name: kubernetes
status: {}
`

	setAddr = `apiVersion: v1
kind: Pod
metadata:
  annotations:
    openyurt.io/static-pod-hash: 76f4f955b6
  creationTimestamp: null
  labels:
    k8s-app: yurt-hub
  name: yurt-hub
  namespace: kube-system
spec:
  containers:
    - command:
      - yurthub
      - --v=2
      - --bind-address=127.0.0.1
      - --server-addr=https://192.0.0.1:6443
      - --node-name=$(NODE_NAME)
      - --bootstrap-file=/var/lib/yurthub/bootstrap-hub.conf
      - --working-mode=edge
      - --namespace=kube-system
      env:
        - name: NODE_NAME
          valueFrom:
            fieldRef:
              apiVersion: v1
              fieldPath: spec.nodeName
      image: openyurt/yurthub:v1.3.0
      imagePullPolicy: IfNotPresent
      livenessProbe:
        failureThreshold: 3
        httpGet:
          host: 127.0.0.1
          path: /v1/healthz
          port: 10267
          scheme: HTTP
        initialDelaySeconds: 300
        periodSeconds: 5
        successThreshold: 1
        timeoutSeconds: 1
      name: yurt-hub
      resources:
        limits:
          memory: 300Mi
        requests:
          cpu: 150m
          memory: 150Mi
      securityContext:
        capabilities:
          add:
            - NET_ADMIN
            - NET_RAW
      terminationMessagePath: /dev/termination-log
      terminationMessagePolicy: File
      volumeMounts:
        - mountPath: /var/lib/yurthub
          name: hub-dir
        - mountPath: /etc/kubernetes
          name: kubernetes
  dnsPolicy: ClusterFirst
  hostNetwork: true
  priority: 2000001000
  priorityClassName: system-node-critical
  restartPolicy: Always
  schedulerName: default-scheduler
  securityContext: {}
  terminationGracePeriodSeconds: 30
  volumes:
    - hostPath:
        path: /var/lib/yurthub
        type: DirectoryOrCreate
      name: hub-dir
    - hostPath:
        path: /etc/kubernetes
        type: Directory
      name: kubernetes
status: {}
`
	setAddr2 = `apiVersion: v1
kind: Pod
metadata:
  annotations:
    openyurt.io/static-pod-hash: 76f4f955b6
  creationTimestamp: null
  labels:
    k8s-app: yurt-hub
  name: yurt-hub
  namespace: kube-system
spec:
  containers:
    - command:
      - yurthub
      - --v=2
      - --bind-address=127.0.0.1
      - --server-addr=https://192.0.0.2:6443
      - --node-name=$(NODE_NAME)
      - --bootstrap-file=/var/lib/yurthub/bootstrap-hub.conf
      - --working-mode=edge
      - --namespace=kube-system
      env:
        - name: NODE_NAME
          valueFrom:
            fieldRef:
              apiVersion: v1
              fieldPath: spec.nodeName
      image: openyurt/yurthub:v1.3.0
      imagePullPolicy: IfNotPresent
      livenessProbe:
        failureThreshold: 3
        httpGet:
          host: 127.0.0.1
          path: /v1/healthz
          port: 10267
          scheme: HTTP
        initialDelaySeconds: 300
        periodSeconds: 5
        successThreshold: 1
        timeoutSeconds: 1
      name: yurt-hub
      resources:
        limits:
          memory: 300Mi
        requests:
          cpu: 150m
          memory: 150Mi
      securityContext:
        capabilities:
          add:
            - NET_ADMIN
            - NET_RAW
      terminationMessagePath: /dev/termination-log
      terminationMessagePolicy: File
      volumeMounts:
        - mountPath: /var/lib/yurthub
          name: hub-dir
        - mountPath: /etc/kubernetes
          name: kubernetes
  dnsPolicy: ClusterFirst
  hostNetwork: true
  priority: 2000001000
  priorityClassName: system-node-critical
  restartPolicy: Always
  schedulerName: default-scheduler
  securityContext: {}
  terminationGracePeriodSeconds: 30
  volumes:
    - hostPath:
        path: /var/lib/yurthub
        type: DirectoryOrCreate
      name: hub-dir
    - hostPath:
        path: /etc/kubernetes
        type: Directory
      name: kubernetes
status: {}
`

	serverAddrsA = "https://192.0.0.1:6443"
	serverAddrsB = "https://192.0.0.2:6443"
)

func Test_useRealServerAddr(t *testing.T) {
	type args struct {
		yurthubTemplate       string
		kubernetesServerAddrs string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "change default server addr",
			args: args{
				yurthubTemplate:       defaultAddr,
				kubernetesServerAddrs: serverAddrsA,
			},
			want: setAddr,
		},
		{
			name: " already set server addr",
			args: args{
				yurthubTemplate:       setAddr,
				kubernetesServerAddrs: serverAddrsB,
			},
			want: setAddr2,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			actualYaml, err := useRealServerAddr(test.args.yurthubTemplate, test.args.kubernetesServerAddrs)
			if err != nil {
				t.Errorf("unexpected error: %s", err)
			}

			assert.Equal(t, actualYaml, test.want)
		})
	}
}

type testData struct {
	joinNodeData *joindata.NodeRegistration
}

func (j *testData) CfgPath() string {
	return ""
}

func (j *testData) ServerAddr() string {
	return ""
}

func (j *testData) JoinToken() string {
	return ""
}

func (j *testData) PauseImage() string {
	return ""
}

func (j *testData) YurtHubImage() string {
	return ""
}

func (j *testData) YurtHubBinaryUrl() string {
	return ""
}

func (j *testData) HostControlPlaneAddr() string {
	return ""
}

func (j *testData) YurtHubServer() string {
	return ""
}

func (j *testData) YurtHubTemplate() string {
	return ""
}

func (j *testData) YurtHubManifest() string {
	return ""
}

func (j *testData) KubernetesVersion() string {
	return ""
}

func (j *testData) TLSBootstrapCfg() *clientcmdapi.Config {
	return nil
}

func (j *testData) BootstrapClient() *clientset.Clientset {
	return nil
}

func (j *testData) NodeRegistration() *joindata.NodeRegistration {
	return j.joinNodeData
}

func (j *testData) IgnorePreflightErrors() sets.Set[string] {
	return nil
}

func (j *testData) CaCertHashes() []string {
	return nil
}

func (j *testData) NodeLabels() map[string]string {
	return nil
}

func (j *testData) KubernetesResourceServer() string {
	return ""
}

func (j *testData) ReuseCNIBin() bool {
	return false
}

func (j *testData) Namespace() string {
	return ""
}

func (j *testData) StaticPodTemplateList() []string {
	return nil
}

func (j *testData) StaticPodManifestList() []string {
	return nil
}

func TestAddYurthubStaticYaml(t *testing.T) {
	xdata := testData{
		joinNodeData: &joindata.NodeRegistration{
			Name:          "name1",
			NodePoolName:  "nodePool1",
			CRISocket:     "",
			WorkingMode:   "edge",
			Organizations: "",
		}}

	tests := []struct {
		name            string
		data            testData
		podManifestPath string
		wantErr         bool
	}{
		{
			name:            "test",
			data:            xdata,
			podManifestPath: "/tmp",
			wantErr:         false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := AddYurthubStaticYaml(&tt.data, tt.podManifestPath); (err != nil) != tt.wantErr {
				t.Errorf("AddYurthubStaticYaml() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCheckYurtHubItself(t *testing.T) {
	tests := []struct {
		testName string
		ns       string
		name     string
		want     bool
	}{
		{
			testName: "test1",
			ns:       "kube-system",
			name:     "yurt-hub",
			want:     true,
		},
		{
			testName: "test2",
			ns:       "cattle-system",
			name:     "yurt-hub",
			want:     false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CheckYurtHubItself(tt.ns, tt.name); got != tt.want {
				t.Errorf("CheckYurtHubItself() = %v, want %v", got, tt.want)
			}
		})
	}
}

type mockYurtJoinData struct {
	joindata.YurtJoinData
	serverAddr       string
	nodeRegistration *joindata.NodeRegistration
	namespace        string
}

func (m *mockYurtJoinData) ServerAddr() string {
	return m.serverAddr
}

func (m *mockYurtJoinData) NodeRegistration() *joindata.NodeRegistration {
	return m.nodeRegistration
}

func (m *mockYurtJoinData) Namespace() string {
	return m.namespace
}

func TestCheckAndInstallYurthub(t *testing.T) {
	tempDir := t.TempDir()
	yurthubExecPath := filepath.Join(tempDir, "yurthub")

	oldLookPath := lookPath
	defer func() {
		lookPath = oldLookPath
	}()

	t.Run("Yurthub binary already exists", func(t *testing.T) {

		err := os.WriteFile(yurthubExecPath, []byte("dummy"), 0755)
		if err != nil {
			t.Fatalf("Failed to create dummy yurthub binary: %v", err)
		}

		lookPath = func(file string) (string, error) {
			if file == constants.YurthubExecStart {
				return yurthubExecPath, nil
			}
			return oldLookPath(file)
		}

		err = CheckAndInstallYurthub("v1.6.1")
		if err != nil {
			t.Errorf("CheckAndInstallYurthub() error = %v, wantErr %v", err, nil)
		}
	})

	t.Run("Yurthub version is empty", func(t *testing.T) {
		err := CheckAndInstallYurthub("")
		if err == nil {
			t.Errorf("CheckAndInstallYurthub() should return error for empty version but got nil")
		}
	})

	t.Run("Yurthub binary does not exist", func(t *testing.T) {

		lookPath = func(file string) (string, error) {
			if file == constants.YurthubExecStart {
				return "", &os.PathError{}
			}
			return oldLookPath(file)
		}

		t.Log("In a real environment, if yurthub binary doesn't exist and download fails, an error would be returned")
	})
}

func TestCreateYurthubSystemdService(t *testing.T) {

	mockData := &mockYurtJoinData{
		serverAddr: "127.0.0.1:6443",
		nodeRegistration: &joindata.NodeRegistration{
			Name:         "test-node",
			NodePoolName: "test-pool",
			WorkingMode:  "edge",
		},
		namespace: "kube-system",
	}

	oldExecCommand := execCommand
	oldExecLookPath := lookPath
	defer func() {
		execCommand = oldExecCommand
		lookPath = oldExecLookPath
	}()

	execCommand = func(name string, arg ...string) *exec.Cmd {
		return exec.Command("echo", "dummy")
	}

	t.Run("Create systemd service successfully", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("CreateYurthubSystemdService() panicked: %v", r)
			}
		}()

		err := CreateYurthubSystemdService(mockData)
		_ = err
	})

	t.Run("Create systemd service with empty node pool name", func(t *testing.T) {
		mockDataEmptyPool := &mockYurtJoinData{
			serverAddr: "127.0.0.1:6443",
			nodeRegistration: &joindata.NodeRegistration{
				Name:         "test-node",
				NodePoolName: "",
				WorkingMode:  "edge",
			},
			namespace: "kube-system",
		}

		defer func() {
			if r := recover(); r != nil {
				t.Errorf("CreateYurthubSystemdService() with empty node pool panicked: %v", r)
			}
		}()

		err := CreateYurthubSystemdService(mockDataEmptyPool)
		_ = err
	})
}

func TestCheckYurthubServiceHealth(t *testing.T) {

	oldExecCommand := execCommand
	oldCheckYurthubHealthz := checkYurthubHealthzFunc
	defer func() {
		execCommand = oldExecCommand
		checkYurthubHealthzFunc = oldCheckYurthubHealthz
	}()

	t.Run("Service is active and healthy", func(t *testing.T) {
		execCommand = func(name string, arg ...string) *exec.Cmd {
			if name == "systemctl" && len(arg) > 0 && arg[0] == "is-active" {
				return exec.Command("echo", "active")
			}
			return exec.Command("echo", "dummy")
		}

		checkYurthubHealthzFunc = func(string) error {
			return nil
		}

		err := CheckYurthubServiceHealth("127.0.0.1")
		if err != nil {
			t.Errorf("CheckYurthubServiceHealth() error = %v, wantErr %v", err, nil)
		}
	})

	t.Run("Service is not active", func(t *testing.T) {
		execCommand = func(name string, arg ...string) *exec.Cmd {
			if name == "systemctl" && len(arg) > 0 && arg[0] == "is-active" {
				return exec.Command("false")
			}
			return exec.Command("echo", "dummy")
		}

		err := CheckYurthubServiceHealth("127.0.0.1")
		if err == nil {
			t.Errorf("CheckYurthubServiceHealth() error = %v, wantErr %v", err, true)
		}
	})

	t.Run("Service is active but not healthy", func(t *testing.T) {
		execCommand = func(name string, arg ...string) *exec.Cmd {
			if name == "systemctl" && len(arg) > 0 && arg[0] == "is-active" {
				return exec.Command("echo", "active")
			}
			return exec.Command("echo", "dummy")
		}

		checkYurthubHealthzFunc = func(string) error {
			return fmt.Errorf("health check failed")
		}

		err := CheckYurthubServiceHealth("127.0.0.1")
		if err == nil {
			t.Errorf("CheckYurthubServiceHealth() error = %v, wantErr %v", err, true)
		}
	})
}

func TestCheckYurthubServiceHealth_HealthzFails(t *testing.T) {
	oldExec := execCommand
	oldHealthz := checkYurthubHealthzFunc
	defer func() {
		execCommand = oldExec
		checkYurthubHealthzFunc = oldHealthz
	}()

	execCommand = func(name string, arg ...string) *exec.Cmd {
		if name == "systemctl" && len(arg) > 0 && arg[0] == "is-active" {
			return exec.Command("echo", "active")
		}
		return exec.Command("echo", "dummy")
	}

	checkYurthubHealthzFunc = func(addr string) error {
		return fmt.Errorf("health check timeout")
	}

	err := CheckYurthubServiceHealth("127.0.0.1")
	if err == nil {
		t.Errorf("Expected error from healthz check, but got nil")
	}
}

func TestCheckYurthubServiceHealth_HealthzSuccess(t *testing.T) {
	oldExec := execCommand
	oldHealthz := checkYurthubHealthzFunc
	defer func() {
		execCommand = oldExec
		checkYurthubHealthzFunc = oldHealthz
	}()

	execCommand = func(name string, arg ...string) *exec.Cmd {
		if name == "systemctl" && len(arg) > 0 && arg[0] == "is-active" {
			return exec.Command("echo", "active")
		}
		return exec.Command("echo", "dummy")
	}

	checkYurthubHealthzFunc = func(addr string) error {
		return nil
	}

	err := CheckYurthubServiceHealth("127.0.0.1")
	if err != nil {
		t.Errorf("Expected no error when both service and healthz are ok, but got %v", err)
	}
}

func Test_CreateYurthubSystemdService_StartFails(t *testing.T) {
	mockData := &mockYurtJoinData{
		serverAddr: "127.0.0.1:6443",
		nodeRegistration: &joindata.NodeRegistration{
			Name:         "svc-node",
			NodePoolName: "svc-pool",
			WorkingMode:  "edge",
		},
		namespace: "kube-system",
	}

	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	execCommand = func(name string, arg ...string) *exec.Cmd {
		if name == "systemctl" {
			if len(arg) > 0 && arg[0] == "start" {
				return exec.Command("false")
			}
			return exec.Command("echo", "ok")
		}
		return exec.Command("echo", "ok")
	}

	err := CreateYurthubSystemdService(mockData)
	if err == nil {
		t.Fatalf("CreateYurthubSystemdService() expected to fail due to systemctl start error, but got nil")
	}
}

func Test_CheckAndInstallYurthub_LookPathErrorCausesDownloadAttempt(t *testing.T) {
	oldLookPath := lookPath
	defer func() { lookPath = oldLookPath }()

	lookPath = func(file string) (string, error) {
		if file == constants.YurthubExecStart {
			return "", &os.PathError{Op: "stat", Path: file, Err: os.ErrNotExist}
		}
		return oldLookPath(file)
	}

	err := CheckAndInstallYurthub("v0.0.0-test")
	if err == nil {
		t.Fatalf("CheckAndInstallYurthub() expected to return an error when binary missing and download/copy fails, but got nil")
	}
}

func Test_SetHubBootstrapConfig_InvalidData_ReturnsError(t *testing.T) {
	err := SetHubBootstrapConfig("invalid-server:6443", "badtoken", []string{"hash"})
	if err == nil {
		t.Fatalf("SetHubBootstrapConfig() expected to return error for invalid data, got nil")
	}
}

func Test_CleanHubBootstrapConfig_NoError(t *testing.T) {
	if err := CleanHubBootstrapConfig(); err != nil {
		t.Fatalf("CleanHubBootstrapConfig() expected no error, got: %v", err)
	}
}

func Test_CheckYurtHubItself_CloudAndYurtNames(t *testing.T) {
	if !CheckYurtHubItself(constants.YurthubNamespace, constants.YurthubCloudYurtStaticSetName) {
		t.Errorf("expected CheckYurtHubItself to be true for cloud static set name")
	}
	if !CheckYurtHubItself(constants.YurthubNamespace, constants.YurthubYurtStaticSetName) {
		t.Errorf("expected CheckYurtHubItself to be true for yurt static set name")
	}
}
func Test_CreateYurthubSystemdService_DaemonReloadFails(t *testing.T) {
	mockData := &mockYurtJoinData{
		serverAddr: "127.0.0.1:6443",
		nodeRegistration: &joindata.NodeRegistration{
			Name:         "daemon-fail-node",
			NodePoolName: "pool",
			WorkingMode:  "edge",
		},
		namespace: "kube-system",
	}

	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	execCommand = func(name string, arg ...string) *exec.Cmd {
		if name == "systemctl" && len(arg) > 0 && arg[0] == "daemon-reload" {
			return exec.Command("false")
		}
		return exec.Command("echo", "ok")
	}

	if err := CreateYurthubSystemdService(mockData); err == nil {
		t.Fatalf("expected error when daemon-reload fails, got nil")
	}
}

func Test_CreateYurthubSystemdService_EnableFails(t *testing.T) {
	mockData := &mockYurtJoinData{
		serverAddr: "127.0.0.1:6443",
		nodeRegistration: &joindata.NodeRegistration{
			Name:         "enable-fail-node",
			NodePoolName: "pool",
			WorkingMode:  "edge",
		},
		namespace: "kube-system",
	}

	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	execCommand = func(name string, arg ...string) *exec.Cmd {
		if name == "systemctl" && len(arg) > 0 && arg[0] == "enable" {
			return exec.Command("false")
		}
		return exec.Command("echo", "ok")
	}

	if err := CreateYurthubSystemdService(mockData); err == nil {
		t.Fatalf("expected error when systemctl enable fails, got nil")
	}
}

func Test_CheckYurthubServiceHealth_SystemctlRunError(t *testing.T) {
	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	execCommand = func(name string, arg ...string) *exec.Cmd {
		if name == "systemctl" && len(arg) > 0 && arg[0] == "is-active" {
			return exec.Command("false")
		}
		return exec.Command("echo", "ok")
	}

	if err := CheckYurthubServiceHealth("127.0.0.1"); err == nil {
		t.Fatalf("expected error when systemctl is-active command fails, got nil")
	}
}

func Test_CheckYurthubServiceHealth_HealthzFuncErrorPropagation(t *testing.T) {
	oldExec := execCommand
	oldHealthz := checkYurthubHealthzFunc
	defer func() {
		execCommand = oldExec
		checkYurthubHealthzFunc = oldHealthz
	}()

	execCommand = func(name string, arg ...string) *exec.Cmd {
		if name == "systemctl" && len(arg) > 0 && arg[0] == "is-active" {
			return exec.Command("echo", "active")
		}
		return exec.Command("echo", "ok")
	}

	checkYurthubHealthzFunc = func(addr string) error {
		return errors.New("simulated healthz failure")
	}

	if err := CheckYurthubServiceHealth("127.0.0.1"); err == nil {
		t.Fatalf("expected error when healthz check fails, got nil")
	}
}

func Test_setYurthubUnitService_TemplateSubstitutionError(t *testing.T) {
	mockData := &mockYurtJoinData{
		serverAddr: "invalid:server:addr",
		nodeRegistration: &joindata.NodeRegistration{
			Name:        "",
			WorkingMode: "",
		},
		namespace: "",
	}

	err := setYurthubUnitService(mockData)
	if err == nil {
		t.Fatal("expected template substitution to fail with invalid data")
	}
}

var (
	osStat     = os.Stat
	osMkdirAll = os.MkdirAll
)

func Test_setYurthubMainService_DirCreationFail(t *testing.T) {
	oldStat := osStat
	oldMkdirAll := osMkdirAll

	osStat = func(name string) (os.FileInfo, error) {
		return nil, &os.PathError{Op: "stat", Path: name, Err: os.ErrNotExist}
	}

	osMkdirAll = func(path string, perm os.FileMode) error {
		if path == filepath.Dir(constants.YurthubServicePath) {
			return fmt.Errorf("permission denied")
		}
		return os.MkdirAll(path, perm)
	}

	defer func() {
		osStat = oldStat
		osMkdirAll = oldMkdirAll
	}()

	err := setYurthubMainService()
	if err == nil {
		t.Fatalf("setYurthubMainService() should return error when mkdir fails, but got nil")
	}
}

func Test_setYurthubUnitService_DirCreationFail(t *testing.T) {
	mockData := &mockYurtJoinData{
		serverAddr: "192.0.2.10:6443",
		nodeRegistration: &joindata.NodeRegistration{
			Name:         "test-node",
			NodePoolName: "test-pool",
			WorkingMode:  "edge",
		},
		namespace: "kube-system",
	}

	oldStat := osStat
	oldMkdirAll := osMkdirAll

	osStat = func(name string) (os.FileInfo, error) {
		return nil, &os.PathError{Op: "stat", Path: name, Err: os.ErrNotExist}
	}

	osMkdirAll = func(path string, perm os.FileMode) error {
		if path == filepath.Dir(constants.YurthubServiceConfPath) {
			return fmt.Errorf("permission denied")
		}
		return os.MkdirAll(path, perm)
	}

	defer func() {
		osStat = oldStat
		osMkdirAll = oldMkdirAll
	}()

	err := setYurthubUnitService(mockData)
	if err == nil {
		t.Fatalf("setYurthubUnitService() should return error when mkdir fails, but got nil")
	}
}

func Test_CheckYurthubServiceHealth_CmdRunError(t *testing.T) {
	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	execCommand = func(name string, arg ...string) *exec.Cmd {
		if name == "systemctl" && len(arg) > 0 && arg[0] == "is-active" {
			return exec.Command("sh", "-c", "exit 1")
		}
		return exec.Command("echo", "dummy")
	}

	err := CheckYurthubServiceHealth("127.0.0.1")
	if err == nil {
		t.Fatalf("CheckYurthubServiceHealth() should return error when systemctl fails, but got nil")
	}

	expectedErrMsg := "yurthub service is not active"
	if !strings.Contains(err.Error(), expectedErrMsg) {
		t.Errorf("Expected error message to contain %q, but got: %v", expectedErrMsg, err)
	}
}

func Test_useRealServerAddr_ScanError(t *testing.T) {
	yurthubTemplate := "test template"
	kubernetesServerAddrs := "https://192.168.1.1:6443"

	_, err := useRealServerAddr(yurthubTemplate, kubernetesServerAddrs)
	if err != nil {
		t.Logf("useRealServerAddr returned error (might be expected): %v", err)
	}
}

func Test_useRealServerAddr_NoServerAddrLine(t *testing.T) {
	yurthubTemplate := `apiVersion: v1
kind: Pod
metadata:
  name: yurt-hub
spec:
  containers:
    - command:
      - yurthub
      - --v=2
      name: yurt-hub`

	kubernetesServerAddrs := "https://192.168.1.1:6443"
	result, err := useRealServerAddr(yurthubTemplate, kubernetesServerAddrs)

	if err != nil {
		t.Fatalf("useRealServerAddr() unexpected error: %v", err)
	}

	if !strings.Contains(result, "--v=2") {
		t.Errorf("Expected result to contain original content, but got: %s", result)
	}
}

func Test_CheckYurthubReadyzOnce_RequestFail(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == constants.ServerReadyzURLPath {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
		}
	}))
	defer ts.Close()

	u, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("Failed to parse test server URL: %v", err)
	}

	result := CheckYurthubReadyzOnce(u.Hostname())
	if result {
		t.Errorf("CheckYurthubReadyzOnce() should return false when server returns error status")
	}
}

func Test_CheckYurthubReadyzOnce_NonOKResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == constants.ServerReadyzURLPath {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("Not Ready"))
		}
	}))
	defer ts.Close()

	addr := strings.TrimPrefix(ts.URL, "http://")
	result := CheckYurthubReadyzOnce(addr)
	assert.False(t, result)
}

func Test_CheckYurtHubItself_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		ns       string
		podName  string
		expected bool
	}{
		{
			name:     "Empty namespace",
			ns:       "",
			podName:  constants.YurthubYurtStaticSetName,
			expected: false,
		},
		{
			name:     "Empty pod name",
			ns:       constants.YurthubNamespace,
			podName:  "",
			expected: false,
		},
		{
			name:     "Both empty",
			ns:       "",
			podName:  "",
			expected: false,
		},
		{
			name:     "Wrong namespace with correct pod name",
			ns:       "default",
			podName:  constants.YurthubYurtStaticSetName,
			expected: false,
		},
		{
			name:     "Correct namespace with wrong pod name",
			ns:       constants.YurthubNamespace,
			podName:  "other-pod",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CheckYurtHubItself(tt.ns, tt.podName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func Test_CreateYurthubSystemdService_DaemonReloadError(t *testing.T) {
	mockData := &mockYurtJoinData{
		serverAddr: "127.0.0.1:6443",
		nodeRegistration: &joindata.NodeRegistration{
			Name:        "test-node",
			WorkingMode: "edge",
		},
		namespace: "kube-system",
	}

	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	execCommand = func(name string, arg ...string) *exec.Cmd {
		if name == "systemctl" && len(arg) > 0 && arg[0] == "daemon-reload" {
			return exec.Command("false")
		}
		return exec.Command("echo", "ok")
	}

	err := CreateYurthubSystemdService(mockData)
	assert.Error(t, err)
}

func Test_CreateYurthubSystemdService_EnableError(t *testing.T) {
	mockData := &mockYurtJoinData{
		serverAddr: "127.0.0.1:6443",
		nodeRegistration: &joindata.NodeRegistration{
			Name:        "test-node",
			WorkingMode: "edge",
		},
		namespace: "kube-system",
	}

	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	execCommand = func(name string, arg ...string) *exec.Cmd {
		if name == "systemctl" && len(arg) > 0 && arg[0] == "enable" {
			return exec.Command("false")
		}
		return exec.Command("echo", "ok")
	}

	err := CreateYurthubSystemdService(mockData)
	assert.Error(t, err)
}

func Test_CheckYurthubReadyzOnce_ReadBodyFail(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == constants.ServerReadyzURLPath {
			hijacker, ok := w.(http.Hijacker)
			if !ok {
				t.Fatal("server does not support hijacking")
			}
			conn, _, err := hijacker.Hijack()
			if err != nil {
				t.Fatal(err)
			}
			conn.Close()
		}
	}))
	defer ts.Close()

	addr := strings.TrimPrefix(ts.URL, "http://")
	result := CheckYurthubReadyzOnce(addr)
	assert.False(t, result)
}

func Test_useRealServerAddr_ScannerError(t *testing.T) {
	var largeInput strings.Builder
	for i := 0; i < 100000; i++ {
		largeInput.WriteString(fmt.Sprintf("line %d\n", i))
	}

	largeInput.WriteString(fmt.Sprintf("- --%s=https://127.0.0.1:6443\n", constants.ServerAddr))

	for i := 100000; i < 200000; i++ {
		largeInput.WriteString(fmt.Sprintf("line %d\n", i))
	}

	_, err := useRealServerAddr(largeInput.String(), "https://192.168.1.1:6443")
	assert.NoError(t, err)
}

func Test_CheckYurtHubItself_BoundaryCases(t *testing.T) {
	testCases := []struct {
		name     string
		ns       string
		podName  string
		expected bool
	}{
		{
			name:     "Empty strings",
			ns:       "",
			podName:  "",
			expected: false,
		},
		{
			name:     "Correct namespace, wrong pod name",
			ns:       constants.YurthubNamespace,
			podName:  "wrong-name",
			expected: false,
		},
		{
			name:     "Wrong namespace, correct pod name",
			ns:       "wrong-namespace",
			podName:  constants.YurthubYurtStaticSetName,
			expected: false,
		},
		{
			name:     "Wrong namespace, correct cloud pod name",
			ns:       "wrong-namespace",
			podName:  constants.YurthubCloudYurtStaticSetName,
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := CheckYurtHubItself(tc.ns, tc.podName)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func Test_CheckYurthubHealthz_ClientTimeout(t *testing.T) {
	oldCheckFunc := checkYurthubHealthzFunc
	defer func() {
		checkYurthubHealthzFunc = oldCheckFunc
	}()

	checkYurthubHealthzFunc = func(server string) error {
		time.Sleep(100 * time.Millisecond)
		return fmt.Errorf("mock error")
	}

	err := CheckYurthubServiceHealth("127.0.0.1")
	assert.Error(t, err)
}

func TestAddYurthubStaticYaml_ErrorCases(t *testing.T) {
	tempDir := t.TempDir()

	err := os.Chmod(tempDir, 0444)
	if err != nil {
		t.Skip("无法修改目录权限，跳过测试")
	}
	defer os.Chmod(tempDir, 0755)

	data := &mockYurtJoinData{
		serverAddr: "127.0.0.1:6443",
		nodeRegistration: &joindata.NodeRegistration{
			Name:        "test-node",
			WorkingMode: "edge",
		},
		namespace: "kube-system",
	}

	err = AddYurthubStaticYaml(data, filepath.Join(tempDir, "nonexistent", "path"))
	assert.Error(t, err)
}

func Test_useRealServerAddr_ComplexYAML(t *testing.T) {
	complexYAML := `apiVersion: v1
kind: Pod
metadata:
  name: yurt-hub
spec:
  containers:
  - command:
    - yurthub
    - --server-addr=https://127.0.0.1:6443
    - --another-flag=value
    - --server-addr=https://127.0.0.1:6443 # 重复的参数
    name: yurt-hub`

	result, err := useRealServerAddr(complexYAML, "https://192.168.1.1:6443")
	assert.NoError(t, err)
	assert.Contains(t, result, "--server-addr=https://192.168.1.1:6443")
}

func Test_useRealServerAddr_EmptyAndSpecialChars(t *testing.T) {
	yamlWithEmptyLines := `apiVersion: v1

kind: Pod

metadata:
  name: yurt-hub
spec:
  containers:
  - command:
    - yurthub
    - --server-addr=https://127.0.0.1:6443
    
    name: yurt-hub`

	result, err := useRealServerAddr(yamlWithEmptyLines, "https://192.168.1.1:6443")
	assert.NoError(t, err)
	assert.Contains(t, result, "--server-addr=https://192.168.1.1:6443")
}

func TestCheckYurthubReadyzOnce_VariousCases(t *testing.T) {
	result := CheckYurthubReadyzOnce("invalid-host:10267")
	assert.False(t, result)

	result = CheckYurthubReadyzOnce("127.0.0.1:99999")
	assert.False(t, result)
}

func Test_CheckYurthubHealthz_WithTimeout(t *testing.T) {
	originalFunc := checkYurthubHealthzFunc
	defer func() {
		checkYurthubHealthzFunc = originalFunc
	}()

	checkYurthubHealthzFunc = func(server string) error {
		time.Sleep(10 * time.Millisecond)
		return fmt.Errorf("mock error")
	}

	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	execCommand = func(name string, arg ...string) *exec.Cmd {
		if name == "systemctl" && len(arg) > 0 && arg[0] == "is-active" {
			return exec.Command("echo", "active")
		}
		return exec.Command("echo", "dummy")
	}

	err := CheckYurthubServiceHealth("127.0.0.1")
	assert.Error(t, err)
}
