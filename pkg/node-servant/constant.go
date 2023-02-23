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

	//ConvertPreflightJobNameBase is the prefix of the preflight-convert ServantJob name
	ConvertPreflightJobNameBase = "node-servant-preflight-convert"

	// ConfigControlPlaneJobNameBase is the prefix of the config control-plane ServantJob name
	ConfigControlPlaneJobNameBase = "config-control-plane"

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
	// ConvertPreflightJobTemplate defines the node convert preflight checks servant job in yaml format
	ConvertPreflightJobTemplate = `
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
        - "/usr/local/bin/entry.sh preflight-convert {{if .ignore_preflight_errors}}--ignore-preflight-errors {{.ignore_preflight_errors}} {{end}}"
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

	// ConfigControlPlaneJobTemplate defines the node-servant config control-plane for configuring kube-apiserver and kube-controller-manager
	ConfigControlPlaneJobTemplate = `
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
        - "/usr/local/bin/entry.sh config control-plane"
        securityContext:
          privileged: true
        volumeMounts:
        - mountPath: /openyurt
          name: host-root
`
)
