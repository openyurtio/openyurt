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

	YurtManagerCertsSecret = `
apiVersion: v1
kind: Secret
metadata:
  name: yurt-manager-webhook-certs
  namespace: kube-system
`

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

	YurtManagerRoleBinding = `
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: yurt-manager-role-binding
  namespace: kube-system
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
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
            - --metrics-addr=:10271
            - --health-probe-addr=:10272
            - --webhook-port=10273
            - --controllers=*
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
          livenessProbe:
            httpGet:
              path: /healthz
              port: 10272
            initialDelaySeconds: 60
            timeoutSeconds: 2
            periodSeconds: 10
            failureThreshold: 3
          readinessProbe:
            httpGet:
              path: /readyz
              port: 10272
            initialDelaySeconds: 60
            timeoutSeconds: 2
            periodSeconds: 10
            failureThreshold: 2
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

	YurthubCloudYurtStaticSet = `
apiVersion: apps.openyurt.io/v1alpha1
kind: YurtStaticSet
metadata:
  name: yurt-hub-cloud
  namespace: "kube-system"
spec:
  staticPodManifest: yurthub
  template:
    metadata:
      labels:
        k8s-app: yurt-hub-cloud
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
          image: {{.yurthub_image}}
          imagePullPolicy: IfNotPresent
          volumeMounts:
            - name: hub-dir
              mountPath: /var/lib/yurthub
            - name: kubernetes
              mountPath: /etc/kubernetes
          command:
            - yurthub
            - --v=2
            - --bind-address=127.0.0.1
            - --server-addr={{.kubernetesServerAddr}}
            - --node-name=$(NODE_NAME)
            - --bootstrap-file=/var/lib/yurthub/bootstrap-hub.conf
            - --working-mode=cloud
            - --namespace="kube-system"
          livenessProbe:
            httpGet:
              host: 127.0.0.1
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
              add: [ "NET_ADMIN", "NET_RAW" ]
          env:
            - name: NODE_NAME
              valueFrom:
                fieldRef:
                  fieldPath: spec.nodeName
      hostNetwork: true
      priorityClassName: system-node-critical
      priority: 2000001000
`
	YurthubYurtStaticSet = `
apiVersion: apps.openyurt.io/v1alpha1
kind: YurtStaticSet
metadata:
  name: yurt-hub
  namespace: "kube-system"
spec:
  staticPodManifest: yurthub
  template:
    metadata:
      labels:
        k8s-app: yurt-hub
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
          image: {{.yurthub_image}}
          imagePullPolicy: IfNotPresent
          volumeMounts:
            - name: hub-dir
              mountPath: /var/lib/yurthub
            - name: kubernetes
              mountPath: /etc/kubernetes
          command:
            - yurthub
            - --v=2
            - --bind-address=127.0.0.1
            - --server-addr={{.kubernetesServerAddr}}
            - --node-name=$(NODE_NAME)
            - --bootstrap-file=/var/lib/yurthub/bootstrap-hub.conf
            - --working-mode=edge
            - --namespace="kube-system"
          livenessProbe:
            httpGet:
              host: 127.0.0.1
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
              add: [ "NET_ADMIN", "NET_RAW" ]
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
