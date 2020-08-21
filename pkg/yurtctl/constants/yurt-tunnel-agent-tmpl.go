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
	YurttunnelAgentClusterRole = `
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  annotations:
    rbac.authorization.kubernetes.io/autoupdate: "true"
  name: yurt-tunnel-agent
rules:
- apiGroups:
  - ""
  resources:
  - nodes/stats
  - nodes/metrics
  - nodes/log
  - nodes/spec
  - nodes/proxy
  verbs:
  - create
  - get
  - list
  - watch
  - delete
  - update
  - patch
`
	YurttunnelAgentClusterRoleBinding = `
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: yurt-tunnel-agent
subjects:
- kind: Group
  name: system:nodes
  apiGroup: rbac.authorization.k8s.io
roleRef:
  kind: ClusterRole
  name: yurt-tunnel-agent
  apiGroup: rbac.authorization.k8s.io
`
	YurttunnelAgentDaemonSet = `
apiVersion: apps/v1
kind: DaemonSet
metadata:
  labels:
    k8s-app: yurt-tunnel-agent
  name: yurt-tunnel-agent
  namespace: kube-system
spec:
  selector:
    matchLabels:
      k8s-app: yurt-tunnel-agent
  template:
    metadata:
      labels:
        k8s-app: yurt-tunnel-agent
    spec:
      nodeSelector:
        beta.kubernetes.io/arch: amd64
        beta.kubernetes.io/os: linux
        {{.edgeWorkerLabel}}: "true"
      containers:
      - command:
        - yurt-tunnel-agent
        args:
        - --node-name=$(NODE_NAME)
        image: {{.image}}
        imagePullPolicy: Always
        name: yurt-tunnel-agent
        volumeMounts:
        - name: k8s-dir
          mountPath: /etc/kubernetes
        - name: kubelet-pki
          mountPath: /var/lib/kubelet/pki
        env:
        - name: NODE_NAME
          valueFrom:
            fieldRef:
              fieldPath: spec.nodeName
        - name: POD_IP
          valueFrom:
            fieldRef:
              fieldPath: status.podIP
      hostNetwork: true
      restartPolicy: Always
      volumes:
      - name: k8s-dir
        hostPath:
          path: /etc/kubernetes
          type: Directory
      - name: kubelet-pki
        hostPath:
          path: /var/lib/kubelet/pki
          type: Directory
`
)
