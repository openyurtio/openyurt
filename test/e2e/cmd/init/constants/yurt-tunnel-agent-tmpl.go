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
        kubernetes.io/os: linux
        {{.edgeWorkerLabel}}: "true"
      containers:
      - command:
        - yurt-tunnel-agent
        args:
        - --v=2
        - --node-name=$(NODE_NAME)
          {{if  .tunnelServerAddress }}
        - --tunnelserver-addr={{.tunnelServerAddress}}
          {{end}}
        image: {{.image}}
        imagePullPolicy: IfNotPresent
        name: yurt-tunnel-agent
        volumeMounts:
        - name: k8s-dir
          mountPath: /etc/kubernetes
        - name: kubelet-pki
          mountPath: /var/lib/kubelet/pki
        - name: tunnel-agent-dir
          mountPath: /var/lib/yurttunnel-agent
        env:
        - name: NODE_NAME
          valueFrom:
            fieldRef:
              fieldPath: spec.nodeName
        - name: POD_IP
          valueFrom:
            fieldRef:
              fieldPath: status.podIP
        - name: NODE_IP
          valueFrom:
            fieldRef:
              fieldPath: status.hostIP
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
      - name: tunnel-agent-dir
        hostPath:
          path: /var/lib/yurttunnel-agent
          type: DirectoryOrCreate
`
)
