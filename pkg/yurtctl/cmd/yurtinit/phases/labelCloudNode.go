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

	"k8s.io/kubernetes/cmd/kubeadm/app/constants"

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
		return fmt.Errorf("Label cloud-node-phase invoked with an invalid data struct. ")
	}
	client, err := data.Client()
	if err != nil {
		return err
	}
	nodeRegistration := data.Cfg().NodeRegistration
	return MarkCloudNodePhase(client, nodeRegistration.Name, nodeRegistration.Taints)
}

// MarkCloudNodePhase taints the control-plane and sets the control-plane label
func MarkCloudNodePhase(client clientset.Interface, cloudName string, taints []v1.Taint) error {
	fmt.Printf("[mark-control-plane] Marking the node %s as cloud-node-plane by adding the label \"%s=''\"\n", cloudName, constants.LabelNodeRoleMaster)

	if len(taints) > 0 {
		taintStrs := []string{}
		for _, taint := range taints {
			taintStrs = append(taintStrs, taint.ToString())
		}
		fmt.Printf("[mark-control-plane] Marking the node %s as control-plane by adding the taints %v\n", cloudName, taintStrs)
	}

	return apiclient.PatchNode(client, cloudName, func(n *v1.Node) {
		markCloudNode(n, taints)
	})
}

func taintExists(taint v1.Taint, taints []v1.Taint) bool {
	for _, t := range taints {
		if t == taint {
			return true
		}
	}

	return false
}

func markCloudNode(n *v1.Node, taints []v1.Taint) {
	n.ObjectMeta.Labels[constants.LabelNodeRoleMaster] = ""
	n.ObjectMeta.Labels[projectinfo.GetEdgeWorkerLabelKey()] = "false"
	for _, nt := range n.Spec.Taints {
		if !taintExists(nt, taints) {
			taints = append(taints, nt)
		}
	}

	n.Spec.Taints = taints
}
