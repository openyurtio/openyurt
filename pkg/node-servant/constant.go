/*
Copyright 2026 The OpenYurt Authors.

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
	// ConversionJobNameBase is the unified prefix of the node conversion Job name.
	ConversionJobNameBase = "node-servant-conversion"
	// ConversionNodeLabelKey is the label key used to correlate conversion Jobs with Nodes.
	ConversionNodeLabelKey = "openyurt.io/conversion-node"
	// DefaultConversionJobBackoffLimit is the proposal-aligned retry budget for conversion Jobs.
	DefaultConversionJobBackoffLimit = 3
	// DefaultConversionJobTTLSecondsAfterFinished keeps finished Jobs long enough for diagnosis.
	DefaultConversionJobTTLSecondsAfterFinished = 7200
	// DefaultConversionJobNamespace is the namespace used by node conversion Jobs.
	DefaultConversionJobNamespace = "kube-system"

	defaultWorkingMode = "edge"

	// ServantJobTemplate defines the unified node-servant Job in yaml format.
	ServantJobTemplate = `
apiVersion: batch/v1
kind: Job
metadata:
  name: {{.jobName}}
  namespace: {{.jobNamespace}}
  labels:
    {{.conversionNodeLabelKey}}: {{.nodeName}}
spec:
  backoffLimit: {{.backoffLimit}}
  ttlSecondsAfterFinished: {{.ttlSecondsAfterFinished}}
  template:
    metadata:
      labels:
        {{.conversionNodeLabelKey}}: {{.nodeName}}
    spec:
      hostPID: true
      hostNetwork: true
      restartPolicy: Never
      nodeName: {{.nodeName}}
      tolerations:
      - operator: Exists
        effect: NoSchedule
      - operator: Exists
        effect: NoExecute
      volumes:
      - name: host-root
        hostPath:
          path: /
          type: Directory
      containers:
      - name: node-servant
        image: {{.nodeServantImage}}
        imagePullPolicy: IfNotPresent
        command:
        - /bin/sh
        - -c
        args:
        - "/usr/local/bin/entry.sh {{.servantCommand}}"
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
`
)
