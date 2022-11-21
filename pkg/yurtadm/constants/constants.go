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

	Hostname                 = "/etc/hostname"
	SysctlK8sConfig          = "/etc/sysctl.d/k8s.conf"
	StaticPodPath            = "/etc/kubernetes/manifests"
	KubeletConfigureDir      = "/etc/kubernetes"
	KubeletWorkdir           = "/var/lib/kubelet"
	YurtHubWorkdir           = "/var/lib/yurthub"
	OpenyurtDir              = "/var/lib/openyurt"
	YurttunnelAgentWorkdir   = "/var/lib/yurttunnel-agent"
	YurttunnelServerWorkdir  = "/var/lib/yurttunnel-server"
	KubeCondfigPath          = "/etc/kubernetes/kubelet.conf"
	KubeCniDir               = "/opt/cni/bin"
	KubeCniVersion           = "v0.8.0"
	KubeletServiceFilepath   = "/etc/systemd/system/kubelet.service"
	KubeletServiceConfPath   = "/etc/systemd/system/kubelet.service.d/10-kubeadm.conf"
	KubeletSvcPath           = "/usr/lib/systemd/system/kubelet.service.d/10-kubeadm.conf"
	YurthubStaticPodFileName = "yurthub.yaml"
	PauseImagePath           = "registry.cn-hangzhou.aliyuncs.com/google_containers/pause:3.2"
	DefaultDockerCRISocket   = "/var/run/dockershim.sock"
	YurthubYamlName          = "yurt-hub.yaml"
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

	DaemonReload      = "systemctl daemon-reload"
	RestartKubeletSvc = "systemctl restart kubelet"

	CniUrlFormat                    = "https://aliacs-edge-k8s-cn-hangzhou.oss-cn-hangzhou.aliyuncs.com/public/pkg/openyurt/cni/%s/cni-plugins-linux-%s-%s.tgz"
	DefaultKubernetesResourceServer = "dl.k8s.io"
	KubeadmUrlFormat                = "https://%s/release/%s/bin/linux/%s/kubeadm"
	KubeletUrlFormat                = "https://%s/release/%s/bin/linux/%s/kubelet"
	TmpDownloadDir                  = "/tmp"
	KubeadmInstallUrl               = "https://kubernetes.io/docs/setup/production-environment/tools/kubeadm/install-kubeadm/"
	FlannelIntallFile               = "https://aliacs-edge-k8s-cn-hangzhou.oss-cn-hangzhou.aliyuncs.com/public/pkg/openyurt/flannel.yaml"

	EdgeNode  = "edge"
	CloudNode = "cloud"

	ServerHealthzServer          = "127.0.0.1:10267"
	ServerHealthzURLPath         = "/v1/healthz"
	DefaultOpenYurtImageRegistry = "registry.cn-hangzhou.aliyuncs.com/openyurt"
	DefaultOpenYurtVersion       = "latest"
	YurtControllerManager        = "yurt-controller-manager"
	YurtTunnelServer             = "yurt-tunnel-server"
	YurtTunnelAgent              = "yurt-tunnel-agent"
	Yurthub                      = "yurthub"
	DefaultYurtHubServerAddr     = "127.0.0.1"
	YurtAppManager               = "yurt-app-manager"
	YurtAppManagerNamespace      = "kube-system"
	DirMode                      = 0755
	FileMode                     = 0666
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
apiVersion: kubeadm.k8s.io/v1beta3
kind: JoinConfiguration
discovery:
  file:
    kubeConfigPath: {{.kubeConfigPath}}
  tlsBootstrapToken: {{.tlsBootstrapToken}}
nodeRegistration:
  criSocket: {{.criSocket}}
  name: {{.name}}
  ignorePreflightErrors:
    - FileAvailable--etc-kubernetes-kubelet.conf
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
  - name: pem-dir
    hostPath:
      path: /var/lib/kubelet/pki
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
    - name: pem-dir
      mountPath: /var/lib/kubelet/pki
    command:
    - yurthub
    - --v=2
    - --bind-address={{.yurthubServerAddr}}
    - --server-addr={{.kubernetesServerAddr}}
    - --node-name=$(NODE_NAME)
    - --join-token={{.joinToken}}
    - --working-mode={{.workingMode}}
      {{if .enableDummyIf }}
    - --enable-dummy-if={{.enableDummyIf}}
      {{end}}
      {{if .enableNodePool }}
    - --enable-node-pool={{.enableNodePool}}
      {{end}}
      {{if .organizations }}
    - --hub-cert-organizations={{.organizations}}
      {{end}}
    livenessProbe:
      httpGet:
        host: {{.yurthubServerAddr}}
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
