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

	YurthubComponentName = "yurt-hub"
	YurthubNamespace     = "kube-system"
	YurthubCmName        = "yurt-hub-cfg"

	YurtManagerServiceAccount = `
apiVersion: v1
kind: ServiceAccount
metadata:
  name: yurt-manager
  namespace: kube-system
`

	YurtManagerClusterRoleBinding = `
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: yurt-manager-rolebinding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: yurt-manager-role
subjects:
- kind: ServiceAccount
  name: yurt-manager
  namespace: kube-system
`

	YurtManagerService = `
apiVersion: v1
kind: Service
metadata:
  name: yurt-manager-webhook-service
  namespace: kube-system
spec:
  ports:
    - port: 443
      protocol: TCP
      targetPort: 10270
  selector:
    app.kubernetes.io/name: yurt-manager
`
	// YurtManagerDeployment defines the yurt manager deployment in yaml format
	YurtManagerDeployment = `
apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    app.kubernetes.io/name: yurt-manager
    app.kubernetes.io/version: v1.3.0
  name: yurt-manager
  namespace: "kube-system"
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: yurt-manager
  template:
    metadata:
      labels:
        app.kubernetes.io/name: yurt-manager
    spec:
      tolerations:
        - operator: "Exists"
      hostNetwork: true
      containers:
        - args:
            - --enable-leader-election
            - --metrics-addr=:10271
            - --health-probe-addr=:10272
            - --logtostderr=true
            - --v=4
          command:
            - /usr/local/bin/yurt-manager
          image: {{.image}}
          imagePullPolicy: IfNotPresent
          name: yurt-manager
          ports:
            - containerPort: 10270
              name: webhook-server
              protocol: TCP
            - containerPort: 10271
              name: metrics
              protocol: TCP
            - containerPort: 10272
              name: health
              protocol: TCP
          readinessProbe:
            httpGet:
              path: /readyz
              port: 10272
      serviceAccountName: yurt-manager
      affinity:
        nodeAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            nodeSelectorTerms:
            - matchExpressions:
              - key: {{.edgeNodeLabel}}
                operator: In
                values:
                - "false"
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
  - get
  - update
  - list
  - watch
- apiGroups:
  - ""
  resources:
  - pods
  verbs:
  - update
  - list
  - watch
- apiGroups:
  - ""
  resources:
  - secrets
  verbs:
  - get
  - create
  - update
  - list
  - patch
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
- apiGroups:
    - discovery.k8s.io
  resources:
    - endpointslices
  verbs:
    - get
    - list
    - watch
    - patch
- apiGroups:
    - ""
  resources:
    - endpoints
  verbs:
    - get
    - list
    - watch
    - patch
- apiGroups:
    - "apps.openyurt.io"
  resources:
    - nodepools
  verbs:
    - get
    - list
    - watch
- apiGroups:
    - ""
  resources:
    - services
  verbs:
    - get
    - list
    - watch
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
        - --controllers=-servicetopologycontroller,-poolcoordinatorcertmanager,*
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
  servicetopology: ""
  discardcloudservice: ""
  masterservice: ""
`
)
