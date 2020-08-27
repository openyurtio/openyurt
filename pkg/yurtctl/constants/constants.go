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

	YurttunnelServerComponentName = "yurt-tunnel-server"
	YurttunnelServerSvcName       = "x-tunnel-server-svc"
	YurttunnelAgentComponentName  = "yurt-tunnel-agent"
	YurttunnelNamespace           = "kube-system"

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
	// ServantJobTemplate defines the servant job in yaml format
	ServantJobTemplate = `
apiVersion: batch/v1
kind: Job
metadata:
  name: {{.jobName}}
  namespace: kube-system
spec:
  template:
    spec:
      hostPID: true
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
        - "sed -i 's|__kubernetes_service_host__|$(KUBERNETES_SERVICE_HOST)|g;s|__kubernetes_service_port_https__|$(KUBERNETES_SERVICE_PORT_HTTPS)|g;s|__node_name__|$(NODE_NAME)|g;s|__yurthub_image__|{{.yurthub_image}}|g' /var/lib/openyurt/setup_edgenode && cp /var/lib/openyurt/setup_edgenode /tmp && nsenter -t 1 -m -u -n -i /var/tmp/setup_edgenode {{.action}} {{.provider}}"
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
`
)
