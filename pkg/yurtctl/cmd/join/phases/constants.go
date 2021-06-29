package phases

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

const (
	ip_forward = "/proc/sys/net/ipv4/ip_forward"

	bridgenf               = "/proc/sys/net/bridge/bridge-nf-call-iptables"
	bridgenf6              = "/proc/sys/net/bridge/bridge-nf-call-ip6tables"
	sysctl_k8s_config      = "/etc/sysctl.d/k8s.conf"
	kubernetsBridgeSetting = `net.bridge.bridge-nf-call-ip6tables = 1
net.bridge.bridge-nf-call-iptables = 1`
	tmpDownloadDir           = "/tmp"
	yurtHubStaticPodYamlFile = "/etc/kubernetes/manifests/yurthub.yaml"
	defaultYurthubImage      = "registry.cn-hangzhou.aliyuncs.com/openyurt/yurthub:v0.4.0"

	kubeCniDir     = "/opt/cni/bin"
	kubeCniVersion = "v0.8.0"
	cniUrlFormat   = "https://aliacs-edge-k8s-cn-hangzhou.oss-cn-hangzhou.aliyuncs.com/public/pkg/openyurt/cni/%s/cni-plugins-linux-%s-%s.tgz"
	kubeUrlFormat  = "https://dl.k8s.io/%s/kubernetes-node-linux-%s.tar.gz"
	staticPodPath  = "/etc/kubernetes/manifests"

	EdgeNode  = "edge-node"
	CloudNode = "cloud-node"
)

const (
	KubeletServiceFilepath string = "/etc/systemd/system/kubelet.service"
	kubeletServiceContent         = `[Unit]
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

	edgeKubeletUnitConfig = `
[Service]
Environment="KUBELET_KUBECONFIG_ARGS=--kubeconfig=/etc/kubernetes/kubelet.conf"
Environment="KUBELET_CONFIG_ARGS=--config=/var/lib/kubelet/config.yaml"
EnvironmentFile=-/var/lib/kubelet/kubeadm-flags.env
EnvironmentFile=-/etc/default/kubelet
ExecStart=
ExecStart=/usr/bin/kubelet $KUBELET_KUBECONFIG_ARGS $KUBELET_CONFIG_ARGS $KUBELET_KUBEADM_ARGS $KUBELET_EXTRA_ARGS
`
	cloudKubeletUnitConfig = `
[Service]
Environment="KUBELET_KUBECONFIG_ARGS=--bootstrap-kubeconfig=/etc/kubernetes/bootstrap-kubelet.conf --kubeconfig=/etc/kubernetes/kubelet.conf"
Environment="KUBELET_CONFIG_ARGS=--config=/var/lib/kubelet/config.yaml"
EnvironmentFile=-/var/lib/kubelet/kubeadm-flags.env
EnvironmentFile=-/etc/default/kubelet
ExecStart=
ExecStart=/usr/bin/kubelet $KUBELET_KUBECONFIG_ARGS $KUBELET_CONFIG_ARGS $KUBELET_KUBEADM_ARGS $KUBELET_EXTRA_ARGS
`

	kubeletConfForEdgeNode = `
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
)
