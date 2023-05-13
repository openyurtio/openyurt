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
	DefaultOpenYurtVersion   = "latest"
	TmpDownloadDir           = "/tmp"
	DirMode                  = 0755
	FileMode                 = 0666

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
      targetPort: 10273
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
            - --webhook-port=10273
            - --logtostderr=true
            - --v=4
          command:
            - /usr/local/bin/yurt-manager
          image: {{.image}}
          imagePullPolicy: IfNotPresent
          name: yurt-manager
          ports:
            - containerPort: 10273
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
    verbs:
      - get
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
