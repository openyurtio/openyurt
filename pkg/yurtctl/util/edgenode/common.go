/*
Copyright 2021 The OpenYurt Authors.

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

package edgenode

const (
	KubeletSvcPath   = "/etc/systemd/system/kubelet.service.d/10-kubeadm.conf"
	OpenyurtDir      = "/var/lib/openyurt"
	StaticPodPath    = "/etc/kubernetes/manifests"
	KubeCondfigPath  = "/etc/kubernetes/kubelet.conf"
	YurthubYamlName  = "yurt-hub.yaml"
	KubeletConfName  = "kubelet.conf"
	KubeletSvcBackup = "%s.bk"

	Hostname               = "/etc/hostname"
	KubeletHostname        = "--hostname-override=[^\"\\s]*"
	KubeletEnvironmentFile = "EnvironmentFile=.*"

	DaemonReload      = "systemctl daemon-reload"
	RestartKubeletSvc = "systemctl restart kubelet"

	ServerHealthzServer  = "127.0.0.1:10261"
	ServerHealthzURLPath = "/v1/healthz"
	OpenyurtKubeletConf  = `
apiVersion: v1
clusters:
- cluster:
    server: http://127.0.0.1:10261
  name: default-cluster
contexts:
- context:
    cluster: default-cluster
    namespace: default
    user: default-auth
  name: default-context
current-context: default-context
kind: Config
preferences: {}
`
	YurthubTemplate = `
apiVersion: v1
kind: Pod
metadata:
  labels:
    k8s-app: yurt-hub
  name: yurt-hub
  namespace: kube-system
spec:
  volumes:
  - name: hub-dir
    hostPath:
      path: /var/lib/yurthub
      type: DirectoryOrCreate
  - name: kubernetes
    hostPath:
      path: /etc/kubernetes
      type: Directory
  containers:
  - name: yurt-hub
    image: __yurthub_image__
    imagePullPolicy: IfNotPresent
    volumeMounts:
    - name: hub-dir
      mountPath: /var/lib/yurthub
    - name: kubernetes
      mountPath: /etc/kubernetes
    command:
    - yurthub
    - --v=2
    - --server-addr=__kubernetes_service_addr__
    - --node-name=$(NODE_NAME)
    - --join-token=__join_token__
    livenessProbe:
      httpGet:
        host: 127.0.0.1
        path: /v1/healthz
        port: 10261
      initialDelaySeconds: 300
      periodSeconds: 5
      failureThreshold: 3
    resources:
      requests:
        cpu: 150m
        memory: 150Mi
      limits:
        memory: 300Mi
    securityContext:
      capabilities:
        add: ["NET_ADMIN", "NET_RAW"]
    env:
    - name: NODE_NAME
      valueFrom:
        fieldRef:
          fieldPath: spec.nodeName
  hostNetwork: true
  priorityClassName: system-node-critical
  priority: 2000001000
`
)
