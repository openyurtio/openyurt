/*
Copyright 2021 The OpenYurt Authors.

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

package node_servant

const (

	// ConvertJobNameBase is the prefix of the convert ServantJob name
	ConvertJobNameBase = "node-servant-convert"
	// RevertJobNameBase is the prefix of the revert ServantJob name
	RevertJobNameBase = "node-servant-revert"

	// ConvertServantJobTemplate defines the node convert servant job in yaml format
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
      - name: host-root
        hostPath:
          path: /
          type: Directory
      - name: configmap
        configMap:
          defaultMode: 420
          name: {{.configmap_name}}
      containers:
      - name: node-servant-servant
        image: {{.node_servant_image}}
        imagePullPolicy: IfNotPresent
        command:
        - /bin/sh
        - -c
        args:
        - "/usr/local/bin/entry.sh convert --working-mode={{.working_mode}} --yurthub-image={{.yurthub_image}} {{if .yurthub_healthcheck_timeout}}--yurthub-healthcheck-timeout={{.yurthub_healthcheck_timeout}} {{end}}--join-token={{.joinToken}} {{if .enable_dummy_if}}--enable-dummy-if={{.enable_dummy_if}}{{end}} {{if .enable_node_pool}}--enable-node-pool={{.enable_node_pool}}{{end}}"
        securityContext:
          privileged: true
        volumeMounts:
        - mountPath: /openyurt
          name: host-root
        - mountPath: /openyurt/data
          name: configmap
        env:
        - name: NODE_NAME
          valueFrom:
            fieldRef:
              fieldPath: spec.nodeName
          {{if  .kubeadm_conf_path }}
        - name: KUBELET_SVC
          value: {{.kubeadm_conf_path}}
          {{end}}
`
	// RevertServantJobTemplate defines the node revert servant job in yaml format
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
      - name: host-root
        hostPath:
          path: /
          type: Directory
      containers:
      - name: node-servant
        image: {{.node_servant_image}}
        imagePullPolicy: IfNotPresent
        command:
        - /bin/sh
        - -c
        args:
        - "/usr/local/bin/entry.sh revert"
        securityContext:
          privileged: true
        volumeMounts:
        - mountPath: /openyurt
          name: host-root
        env:
        - name: NODE_NAME
          valueFrom:
            fieldRef:
              fieldPath: spec.nodeName
          {{if  .kubeadm_conf_path }}
        - name: KUBELET_SVC
          value: {{.kubeadm_conf_path}}
          {{end}}
`
)
