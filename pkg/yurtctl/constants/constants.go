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
	// AnnotationAutonomy is used to identify if a node is automous
	AnnotationAutonomy = "node.beta.alibabacloud.com/autonomy"

	YurtctlLockConfigMapName = "yurtctl-lock"

	YurttunnelServerComponentName   = "yurt-tunnel-server"
	YurttunnelServerSvcName         = "x-tunnel-server-svc"
	YurttunnelServerInternalSvcName = "x-tunnel-server-internal-svc"
	YurttunnelServerCmName          = "yurt-tunnel-server-cfg"
	YurttunnelAgentComponentName    = "yurt-tunnel-agent"
	YurttunnelNamespace             = "kube-system"

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
	// ConvertServantJobTemplate defines the yurtctl convert servant job in yaml format
	ConvertServantJobTemplate = `
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
      volumes:
      - name: host-var-tmp
        hostPath:
          path: /var/tmp
          type: Directory
      containers:
      - name: yurtctl-servant
        image: {{.yurtctl_servant_image}}
        imagePullPolicy: Always
        command:
        - /bin/sh
        - -c
        args:
        - "cp /usr/local/bin/yurtctl /tmp && nsenter -t 1 -m -u -n -i -- /var/tmp/yurtctl convert edgenode --yurthub-image {{.yurthub_image}} --join-token {{.joinToken}} && rm /tmp/yurtctl"
        securityContext:
          privileged: true
        volumeMounts:
        - mountPath: /tmp
          name: host-var-tmp
        env:
        - name: NODE_NAME
          valueFrom:
            fieldRef:
              fieldPath: spec.nodeName
        - name: STATIC_POD_PATH
          value: {{.pod_manifest_path}}
          {{if  .kubeadm_conf_path }}
        - name: KUBELET_SVC
          value: {{.kubeadm_conf_path}}
          {{end}}
`
	// RevertServantJobTemplate defines the yurtctl revert servant job in yaml format
	RevertServantJobTemplate = `
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
      volumes:
      - name: host-var-tmp
        hostPath:
          path: /var/tmp
          type: Directory
      containers:
      - name: yurtctl-servant
        image: {{.yurtctl_servant_image}}
        imagePullPolicy: Always
        command:
        - /bin/sh
        - -c
        args:
        - "cp /usr/local/bin/yurtctl /tmp && nsenter -t 1 -m -u -n -i -- /var/tmp/yurtctl revert edgenode && rm /tmp/yurtctl"
        securityContext:
          privileged: true
        volumeMounts:
        - mountPath: /tmp
          name: host-var-tmp
        env:
        - name: NODE_NAME
          valueFrom:
            fieldRef:
              fieldPath: spec.nodeName
        - name: STATIC_POD_PATH
          value: {{.pod_manifest_path}}
          {{if  .kubeadm_conf_path }}
        - name: KUBELET_SVC
          value: {{.kubeadm_conf_path}}
          {{end}}
`
)
