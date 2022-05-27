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

package constants

import "github.com/openyurtio/openyurt/pkg/projectinfo"

var (
	// AnnotationAutonomy is used to identify if a node is autonomous
	AnnotationAutonomy = projectinfo.GetAutonomyAnnotation()
)

const (
	YurtctlLockConfigMapName = "yurtctl-lock"

	DefaultOpenYurtVersion = "latest"

	TmpDownloadDir = "/tmp"

	DirMode  = 0755
	FileMode = 0666

	YurthubComponentName                = "yurt-hub"
	YurthubNamespace                    = "kube-system"
	YurthubCmName                       = "yurt-hub-cfg"
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
  verbs:
    - update
- apiGroups:
    - certificates.k8s.io
  resourceNames:
    - kubernetes.io/kube-apiserver-client
    - kubernetes.io/kubelet-serving
  resources:
    - signers
  verbs:
    - approve
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
`
	YurthubClusterRole = `
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: yurt-hub
rules:
  - apiGroups:
      - ""
    resources:
      - events
    verbs:
      - get
  - apiGroups:
      - apps.openyurt.io
    resources:
      - nodepools
    verbs:
      - list
      - watch
  - apiGroups:
      - ""
    resources:
      - configmaps
    resourceNames:
      - yurt-hub-cfg
    verbs:
      - list
      - watch
`
	YurthubClusterRoleBinding = `
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: yurt-hub
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: yurt-hub
subjects:
  - apiGroup: rbac.authorization.k8s.io
    kind: Group
    name: system:nodes
`
	YurthubConfigMap = `
apiVersion: v1
kind: ConfigMap
metadata:
  name: yurt-hub-cfg
  namespace: kube-system
data:
  cache_agents: ""
  filter_endpoints: coredns/endpoints#list;watch
  filter_servicetopology: coredns/endpointslices#list;watch
  filter_discardcloudservice: ""
  filter_masterservice: ""
`
)
