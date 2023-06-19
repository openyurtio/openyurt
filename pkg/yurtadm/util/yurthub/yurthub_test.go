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
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/sets"
	clientset "k8s.io/client-go/kubernetes"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/openyurtio/openyurt/pkg/yurtadm/cmd/join/joindata"
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
			want: setAddr,
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

func (j *testData) IgnorePreflightErrors() sets.String {
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
