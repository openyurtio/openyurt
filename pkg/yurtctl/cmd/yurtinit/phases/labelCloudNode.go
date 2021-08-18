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

package phases

import (
	"fmt"

	v1 "k8s.io/api/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/cmd/kubeadm/app/cmd/phases/workflow"
	"k8s.io/kubernetes/cmd/kubeadm/app/util/apiclient"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
)

func NewMarkCloudNode() workflow.Phase {
	return workflow.Phase{
		Name:  "mark cloud node",
		Short: "Mark a node as a cloud-node",
		Run:   runMarkCloudNode,
	}
}

func runMarkCloudNode(c workflow.RunData) error {
	data, ok := c.(YurtInitData)
	if !ok {
		return fmt.Errorf("Label cloud node phase invoked with an invalid data struct. ")
	}
	client, err := data.Client()
	if err != nil {
		return err
	}
	nodeRegistration := data.Cfg().NodeRegistration
	return LabelCloudNode(client, nodeRegistration.Name,
		map[string]string{projectinfo.GetEdgeWorkerLabelKey(): "false"})
}

// LabelCloudNode set cloud-node label
func LabelCloudNode(client clientset.Interface, nodeName string, label map[string]string) error {
	return apiclient.PatchNode(client, nodeName, func(n *v1.Node) {
		labelCloudNode(n, label)
	})
}

func labelCloudNode(n *v1.Node, labels map[string]string) {
	for k, v := range labels {
		n.ObjectMeta.Labels[k] = v
	}
}
