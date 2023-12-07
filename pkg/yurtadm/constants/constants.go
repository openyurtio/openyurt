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
	Hostname                      = "/etc/hostname"
	SysctlK8sConfig               = "/etc/sysctl.d/k8s.conf"
	StaticPodPath                 = "/etc/kubernetes/manifests"
	KubeletConfigureDir           = "/etc/kubernetes"
	KubeletWorkdir                = "/var/lib/kubelet"
	YurtHubWorkdir                = "/var/lib/yurthub"
	YurtHubBootstrapConfig        = "/var/lib/yurthub/bootstrap-hub.conf"
	OpenyurtDir                   = "/var/lib/openyurt"
	YurttunnelAgentWorkdir        = "/var/lib/yurttunnel-agent"
	YurttunnelServerWorkdir       = "/var/lib/yurttunnel-server"
	KubeCniDir                    = "/opt/cni/bin"
	KubeCniVersion                = "v0.8.0"
	KubeletServiceFilepath        = "/etc/systemd/system/kubelet.service"
	KubeletServiceConfPath        = "/etc/systemd/system/kubelet.service.d/10-kubeadm.conf"
	KubeletSvcPath                = "/usr/lib/systemd/system/kubelet.service.d/10-kubeadm.conf"
	PauseImagePath                = "registry.cn-hangzhou.aliyuncs.com/google_containers/pause:3.2"
	DefaultCertificatesDir        = "/etc/kubernetes/pki"
	DefaultDockerCRISocket        = "/var/run/dockershim.sock"
	YurthubYamlName               = "yurthub.yaml"
	YurthubStaticPodManifest      = "yurthub"
	YurthubNamespace              = "kube-system"
	YurthubYurtStaticSetName      = "yurt-hub"
	YurthubCloudYurtStaticSetName = "yurt-hub-cloud"
	// ManifestsSubDirName defines directory name to store manifests
	ManifestsSubDirName = "manifests"
	// KubeletKubeConfigFileName defines the file name for the kubeconfig that the control-plane kubelet will use for talking
	// to the API server
	KubeletKubeConfigFileName = "kubelet.conf"
	// KubeadmConfigConfigMap specifies in what ConfigMap in the kube-system namespace the `kubeadm init` configuration should be stored
	KubeadmConfigConfigMap = "kubeadm-config"
	// ClusterConfigurationConfigMapKey specifies in what ConfigMap key the cluster configuration should be stored
	ClusterConfigurationConfigMapKey = "ClusterConfiguration"
	// KubeadmJoinConfigFileName defines the file name for the JoinConfiguration that kubeadm will use for joining
	KubeadmJoinConfigFileName = "kubeadm-join.conf"
	// KubeadmJoinDiscoveryFileName defines the file name for the --discovery-file that kubeadm will use for joining
	KubeadmJoinDiscoveryFileName = "discovery.conf"

	KubeletHostname        = "--hostname-override=[^\"\\s]*"
	KubeletEnvironmentFile = "EnvironmentFile=.*"

	BootstrapTokenPattern = `\A([a-z0-9]{6})\.([a-z0-9]{16})\z`
	PlaceholderToken      = "abcdef.0123456789abcdef"

	DaemonReload      = "systemctl daemon-reload"
	RestartKubeletSvc = "systemctl restart kubelet"

	CniUrlFormat                    = "https://aliacs-edge-k8s-cn-hangzhou.oss-cn-hangzhou.aliyuncs.com/public/pkg/openyurt/cni/%s/cni-plugins-linux-%s-%s.tgz"
	DefaultKubernetesResourceServer = "dl.k8s.io"
	KubeadmUrlFormat                = "https://%s/release/%s/bin/linux/%s/kubeadm"
	KubeletUrlFormat                = "https://%s/release/%s/bin/linux/%s/kubelet"
	TmpDownloadDir                  = "/tmp"
	KubeadmInstallUrl               = "https://kubernetes.io/docs/setup/production-environment/tools/kubeadm/install-kubeadm/"

	EdgeNode  = "edge"
	CloudNode = "cloud"

	// CertificatesDir
	CertificatesDir = "cert-dir"
	// ForceReset
	ForceReset = "force"
	// IgnorePreflightErrors sets the path a list of checks whose errors will be shown as warnings. Example: 'IsPrivilegedUser,Swap'. Value 'all' ignores errors from all checks.
	IgnorePreflightErrors = "ignore-preflight-errors"
	// KubernetesResourceServer flag sets the address for download k8s node resources.
	KubernetesResourceServer = "kubernetes-resource-server"
	// NodeCRISocket flag sets the CRI socket to connect to.
	NodeCRISocket = "cri-socket"
	// NodeLabels flag sets the labels for worker node.
	NodeLabels = "node-labels"
	// NodeName flag sets the node name.
	NodeName = "node-name"
	// NodePoolName flag sets the nodePool name.
	NodePoolName = "nodepool-name"
	// NodeType flag sets the type of worker node to edge or cloud.
	NodeType = "node-type"
	// Organizations flag sets the extra organizations of hub agent client certificate.
	Organizations = "organizations"
	// PauseImage flag sets the pause image for worker node.
	PauseImage = "pause-image"
	// CfgPath flag sets the path to a JoinConfiguration file.
	CfgPath = "config"
	// TokenStr flag sets both the discovery-token and the tls-bootstrap-token when those values are not provided
	TokenStr = "token"
	// TokenDiscoveryCAHash flag instruct kubeadm to validate that the root CA public key matches this hash (for token-based discovery)
	TokenDiscoveryCAHash = "discovery-token-ca-cert-hash"
	// TokenDiscoverySkipCAHash flag instruct kubeadm to skip CA hash verification (for token-based discovery)
	TokenDiscoverySkipCAHash = "discovery-token-unsafe-skip-ca-verification"
	// Namespace flag sets the namespace of yurthub staticpod manifest.
	Namespace = "namespace"
	// YurtHubImage flag sets the yurthub image for worker node.
	YurtHubImage = "yurthub-image"
	// YurtHubServerAddr flag set the address of yurthub server (not proxy server!)
	YurtHubServerAddr = "yurthub-server-addr"
	// ServerAddr flag set the address of kubernetes kube-apiserver
	ServerAddr = "server-addr"
	// ReuseCNIBin flag sets whether to reuse local CNI binaries or not.
	ReuseCNIBin = "reuse-cni-bin"
	// StaticPods flag set the specified static pods on this node want to install
	StaticPods = "static-pods"

	KubeletConfFileAvailableError = "FileAvailable--etc-kubernetes-kubelet.conf"
	ManifestsDirAvailableError    = "DirAvailable--etc-kubernetes-manifests"

	DefaultServerAddr            = "https://127.0.0.1:6443"
	ServerHealthzServer          = "127.0.0.1:10267"
	ServerHealthzURLPath         = "/v1/healthz"
	ServerReadyzURLPath          = "/v1/readyz"
	DefaultOpenYurtImageRegistry = "registry.cn-hangzhou.aliyuncs.com/openyurt"
	Yurthub                      = "yurthub"
	DefaultOpenYurtVersion       = "latest"
	DefaultYurtHubServerAddr     = "127.0.0.1"
	DirMode                      = 0755
	KubeletServiceContent        = `
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

	KubeadmJoinConf = `
apiVersion: {{.apiVersion}}
kind: JoinConfiguration
discovery:
  file:
    kubeConfigPath: {{.kubeConfigPath}}
  tlsBootstrapToken: {{.tlsBootstrapToken}}
nodeRegistration:
  criSocket: {{.criSocket}}
  name: {{.name}}
  ignorePreflightErrors:
    {{- range $index, $value := .ignorePreflightErrors}}
    - {{$value}}
    {{- end}}
  kubeletExtraArgs:
    rotate-certificates: "false"
    pod-infra-container-image: {{.podInfraContainerImage}}
    node-labels: {{.nodeLabels}}
    {{- if .networkPlugin}}
    network-plugin: {{.networkPlugin}}
    {{end}}
    {{- if .containerRuntime}}
    container-runtime: {{.containerRuntime}}
    {{end}}
    {{- if .containerRuntimeEndpoint}}
    container-runtime-endpoint: {{.containerRuntimeEndpoint}}
    {{end}}
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
    image: {{.image}}
    imagePullPolicy: IfNotPresent
    volumeMounts:
    - name: hub-dir
      mountPath: /var/lib/yurthub
    - name: kubernetes
      mountPath: /etc/kubernetes
    command:
    - yurthub
    - --v=2
    - --bind-address={{.yurthubBindingAddr}}
    - --server-addr={{.kubernetesServerAddr}}
    - --node-name=$(NODE_NAME)
    - --bootstrap-file=/var/lib/yurthub/bootstrap-hub.conf
    - --working-mode={{.workingMode}}
    - --namespace={{.namespace}}
      {{if .enableDummyIf }}
    - --enable-dummy-if={{.enableDummyIf}}
      {{end}}
      {{if .enableNodePool }}
    - --enable-node-pool={{.enableNodePool}}
      {{end}}
      {{if .organizations }}
    - --hub-cert-organizations={{.organizations}}
      {{end}}
      {{if .nodePoolName }}
    - --nodepool-name={{.nodePoolName}}
      {{end}}
    livenessProbe:
      httpGet:
        host: {{.yurthubBindingAddr}}
        path: /v1/healthz
        port: 10267
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
