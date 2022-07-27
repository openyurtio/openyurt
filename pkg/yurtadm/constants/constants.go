/*
Copyright 2020 The OpenYurt Authors.

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

package constants

const (
	YurttunnelServerComponentName   = "yurt-tunnel-server"
	YurttunnelServerSvcName         = "x-tunnel-server-svc"
	YurttunnelServerInternalSvcName = "x-tunnel-server-internal-svc"
	YurttunnelServerCmName          = "yurt-tunnel-server-cfg"
	YurttunnelAgentComponentName    = "yurt-tunnel-agent"
	YurttunnelNamespace             = "kube-system"

	EtcHostsFile   = "/etc/hosts"
	SealerRegistry = "sea.hub"

	SysctlK8sConfig          = "/etc/sysctl.d/k8s.conf"
	KubeletConfigureDir      = "/etc/kubernetes"
	KubeletWorkdir           = "/var/lib/kubelet"
	YurtHubWorkdir           = "/var/lib/yurthub"
	YurttunnelAgentWorkdir   = "/var/lib/yurttunnel-agent"
	YurttunnelServerWorkdir  = "/var/lib/yurttunnel-server"
	KubeCniDir               = "/opt/cni/bin"
	KubeCniVersion           = "v0.8.0"
	KubeletServiceFilepath   = "/etc/systemd/system/kubelet.service"
	KubeletServiceConfPath   = "/etc/systemd/system/kubelet.service.d/10-kubeadm.conf"
	YurthubStaticPodFileName = "yurthub.yaml"
	PauseImagePath           = "registry.cn-hangzhou.aliyuncs.com/google_containers/pause:3.2"

	OpenYurtCniUrl                  = "https://github.com/openyurtio/openyurt/releases/download/v0.7.0/openyurt-cni-0.8.7-0.x86_64.rpm"
	CniUrlFormat                    = "https://aliacs-edge-k8s-cn-hangzhou.oss-cn-hangzhou.aliyuncs.com/public/pkg/openyurt/cni/%s/cni-plugins-linux-%s-%s.tgz"
	DefaultKubernetesResourceServer = "dl.k8s.io"
	KubeUrlFormat                   = "https://%s/%s/kubernetes-node-linux-%s.tar.gz"
	TmpDownloadDir                  = "/tmp"
	FlannelIntallFile               = "https://aliacs-edge-k8s-cn-hangzhou.oss-cn-hangzhou.aliyuncs.com/public/pkg/openyurt/flannel.yaml"

	EdgeNode  = "edge"
	CloudNode = "cloud"

	DefaultOpenYurtImageRegistry = "registry.cn-hangzhou.aliyuncs.com/openyurt"
	DefaultOpenYurtVersion       = "latest"
	DefaultK8sVersion            = "1.21.14"
	DefaultPodSubnet             = "10.244.0.0/16"
	DefaultServiceSubnet         = "10.96.0.0/12"
	DefaultClusterCIDR           = "10.244.0.0/16"
	DefaultKubeProxyBindAddress  = "0.0.0.0"

	YurtControllerManager   = "yurt-controller-manager"
	YurtTunnelServer        = "yurt-tunnel-server"
	YurtTunnelAgent         = "yurt-tunnel-agent"
	Yurthub                 = "yurthub"
	YurtAppManager          = "yurt-app-manager"
	YurtAppManagerNamespace = "kube-system"
	DirMode                 = 0755
	FileMode                = 0666
	KubeletServiceContent   = `
[Unit]
Description=kubelet: The Kubernetes Node Agent
Documentation=http://kubernetes.io/docs/

[Service]
ExecStartPre=/sbin/swapoff -a
ExecStart=/usr/bin/kubelet
Restart=always
StartLimitInterval=0
RestartSec=10

[Install]
WantedBy=multi-user.target`

	KubeletUnitConfig = `
[Service]
Environment="KUBELET_KUBECONFIG_ARGS=--kubeconfig=/etc/kubernetes/kubelet.conf"
Environment="KUBELET_CONFIG_ARGS=--config=/var/lib/kubelet/config.yaml"
EnvironmentFile=-/var/lib/kubelet/kubeadm-flags.env
EnvironmentFile=-/etc/default/kubelet
ExecStart=
ExecStart=/usr/bin/kubelet $KUBELET_KUBECONFIG_ARGS $KUBELET_CONFIG_ARGS $KUBELET_KUBEADM_ARGS $KUBELET_EXTRA_ARGS
`

	KubeletConfForNode = `
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
	// DisableNodeControllerJobTemplate defines the node-controller disable job in yaml format
	DisableNodeControllerJobTemplate = `
apiVersion: batch/v1
kind: Job
metadata:
  name: {{.jobName}}
  namespace: kube-system
spec:
  template:
    spec:
      hostPID: true
      hostNetwork: true
      restartPolicy: OnFailure
      nodeName: {{.nodeName}}
      containers:
      - name: yurtctl-disable-node-controller
        image: {{.node_servant_image}}
        imagePullPolicy: IfNotPresent
        command:
        - /bin/sh
        - -c
        args:
        - "nsenter -t 1 -m -u -n -i -- sed -i 's/--controllers=/--controllers=-nodelifecycle,/g' {{.pod_manifest_path}}/kube-controller-manager.yaml"
        securityContext:
          privileged: true
`
	// EnableNodeControllerJobTemplate defines the node-controller enable job in yaml format
	EnableNodeControllerJobTemplate = `
apiVersion: batch/v1
kind: Job
metadata:
  name: {{.jobName}}
  namespace: kube-system
spec:
  template:
    spec:
      hostPID: true
      hostNetwork: true
      restartPolicy: OnFailure
      nodeName: {{.nodeName}}
      containers:
      - name: yurtctl-enable-node-controller
        image: {{.node_servant_image}}
        imagePullPolicy: IfNotPresent
        command:
        - /bin/sh
        - -c
        args:
        - "nsenter -t 1 -m -u -n -i -- sed -i 's/--controllers=-nodelifecycle,/--controllers=/g' {{.pod_manifest_path}}/kube-controller-manager.yaml"
        securityContext:
          privileged: true
`
)
