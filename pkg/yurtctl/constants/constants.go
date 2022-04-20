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

import (
	"k8s.io/apimachinery/pkg/util/version"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
)

var (
	// AnnotationAutonomy is used to identify if a node is autonomous
	AnnotationAutonomy    = projectinfo.GetAutonomyAnnotation()
	MinimumKubeletVersion = version.MustParseSemantic("v1.17.0")
)

const (
	YurtctlLockConfigMapName = "yurtctl-lock"

	YurttunnelServerComponentName   = "yurt-tunnel-server"
	YurttunnelServerSvcName         = "x-tunnel-server-svc"
	YurttunnelServerInternalSvcName = "x-tunnel-server-internal-svc"
	YurttunnelServerCmName          = "yurt-tunnel-server-cfg"
	YurttunnelAgentComponentName    = "yurt-tunnel-agent"
	YurttunnelNamespace             = "kube-system"

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

	CniUrlFormat                    = "https://aliacs-edge-k8s-cn-hangzhou.oss-cn-hangzhou.aliyuncs.com/public/pkg/openyurt/cni/%s/cni-plugins-linux-%s-%s.tgz"
	DefaultKubernetesResourceServer = "dl.k8s.io"
	KubeUrlFormat                   = "https://%s/%s/kubernetes-node-linux-%s.tar.gz"
	TmpDownloadDir                  = "/tmp"
	FlannelIntallFile               = "https://aliacs-edge-k8s-cn-hangzhou.oss-cn-hangzhou.aliyuncs.com/public/pkg/openyurt/flannel.yaml"

	EdgeNode  = "edge"
	CloudNode = "cloud"

	DefaultOpenYurtImageRegistry = "registry.cn-hangzhou.aliyuncs.com/openyurt"
	DefaultOpenYurtVersion       = "latest"
	YurtControllerManager        = "yurt-controller-manager"
	YurtTunnelServer             = "yurt-tunnel-server"
	YurtTunnelAgent              = "yurt-tunnel-agent"
	Yurthub                      = "yurthub"
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

	YurtControllerManagerServiceAccount = `
apiVersion: v1
kind: ServiceAccount
metadata:
  name: yurt-controller-manager
  namespace: kube-system
`
	// YurtControllerManagerClusterRole has the same privilege as the
	// system:controller:node-controller and has the right to manipulate
	// the leases resource
	YurtControllerManagerClusterRole = `
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  annotations:
    rbac.authorization.kubernetes.io/autoupdate: "true"
  name: yurt-controller-manager 
rules:
- apiGroups:
  - ""
  resources:
  - nodes
  verbs:
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - ""
  resources:
  - nodes/status
  verbs:
  - patch
  - update
- apiGroups:
  - ""
  resources:
  - pods/status
  verbs:
  - update
- apiGroups:
  - ""
  resources:
  - pods
  verbs:
  - delete
  - list
  - watch
- apiGroups:
  - ""
  - events.k8s.io
  resources:
  - events
  verbs:
  - create
  - patch
  - update
- apiGroups:
  - coordination.k8s.io
  resources:
  - leases
  verbs:
  - create
  - delete
  - get
  - patch
  - update
  - list
  - watch
- apiGroups:
  - ""
  - apps
  resources:
  - daemonsets
  verbs:
  - list
  - watch
- apiGroups:
    - certificates.k8s.io
  resources:
    - certificatesigningrequests
  verbs:
    - get
    - list
    - watch
- apiGroups:
    - certificates.k8s.io
  resources:
    - certificatesigningrequests/approval
    - certificatesigningrequests/status
  verbs:
    - update
- apiGroups:
    - certificates.k8s.io
  resourceNames:
    - kubernetes.io/*
    - openyurt.io/*
  resources:
    - signers
  verbs:
    - approve
    - sign
`
	YurtControllerManagerClusterRoleBinding = `
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: yurt-controller-manager 
subjects:
  - kind: ServiceAccount
    name: yurt-controller-manager
    namespace: kube-system
roleRef:
  kind: ClusterRole
  name: yurt-controller-manager 
  apiGroup: rbac.authorization.k8s.io
`
	// YurtControllerManagerDeployment defines the yurt controller manager
	// deployment in yaml format
	YurtControllerManagerDeployment = `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: yurt-controller-manager
  namespace: kube-system
spec:
  replicas: 1
  selector:
    matchLabels:
      app: yurt-controller-manager
  template:
    metadata:
      labels:
        app: yurt-controller-manager
    spec:
      serviceAccountName: yurt-controller-manager
      nodeSelector:
        node-role.kubernetes.io/master: ""
      hostNetwork: true
      tolerations:
      - operator: "Exists"
      affinity:
        nodeAffinity:
          # we prefer allocating ecm on cloud node
          preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 1
            preference:
              matchExpressions:
              - key: {{.edgeNodeLabel}}
                operator: In
                values:
                - "false"
      containers:
      - name: yurt-controller-manager
        image: {{.image}}
        command:
        - yurt-controller-manager	
        volumeMounts:
        - mountPath: /etc/kubernetes/pki
          name: k8s-certs
          readOnly: true
      volumes:
      - hostPath:
          path: /etc/kubernetes/pki
          type: DirectoryOrCreate
        name: k8s-certs
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
