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
	"io/ioutil"
	"net/http"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog"
	"k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm"
	"k8s.io/kubernetes/cmd/kubeadm/app/cmd/phases/workflow"
	kubeadmconstants "k8s.io/kubernetes/cmd/kubeadm/app/constants"
	patchnodephase "k8s.io/kubernetes/cmd/kubeadm/app/phases/patchnode"
	"k8s.io/kubernetes/cmd/kubeadm/app/util/apiclient"
	"k8s.io/kubernetes/cmd/kubeadm/app/util/initsystem"
	kubeconfigutil "k8s.io/kubernetes/cmd/kubeadm/app/util/kubeconfig"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/yurtctl/constants"
	"github.com/openyurtio/openyurt/pkg/yurtctl/util/edgenode"
)

//NewPostcheckPhase creates a yurtctl workflow phase that check the health status of node components.
func NewPostcheckPhase() workflow.Phase {
	return workflow.Phase{
		Name:  "postcheck",
		Short: "postcheck",
		Run:   runPostcheck,
	}
}

//runPostcheck executes the node health check process.
func runPostcheck(c workflow.RunData) error {
	j, ok := c.(YurtJoinData)
	if !ok {
		return fmt.Errorf("Postcheck edge-node phase invoked with an invalid data struct. ")
	}

	klog.V(1).Infof("check kubelet status.")
	if err := checkKubeletStatus(); err != nil {
		return err
	}

	cfg := j.Cfg()
	if j.NodeType() == constants.EdgeNode {
		klog.V(1).Infof("waiting yurt hub ready.")
		if err := checkYurthubHealthz(); err != nil {
			return err
		}
		return patchEdgeNode(cfg, j.MarkAutonomous())
	}
	return patchCloudNode(cfg)
}

//checkKubeletStatus check if kubelet is healthy.
func checkKubeletStatus() error {
	initSystem, err := initsystem.GetInitSystem()
	if err != nil {
		return err
	}
	if ok := initSystem.ServiceIsActive("kubelet"); !ok {
		return fmt.Errorf("kubelet is not active. ")
	}
	return nil
}

//checkYurthubHealthz check if YurtHub is healthy.
func checkYurthubHealthz() error {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s%s", edgenode.ServerHealthzServer, edgenode.ServerHealthzURLPath), nil)
	if err != nil {
		return err
	}
	client := &http.Client{}
	return wait.PollImmediate(time.Second*5, 300*time.Second, func() (bool, error) {
		resp, err := client.Do(req)
		if err != nil {
			return false, nil
		}
		ok, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return false, nil
		}
		return string(ok) == "OK", nil
	})
}

//patchEdgeNode patch labels and annotations for edge-node.
func patchEdgeNode(cfg *kubeadm.JoinConfiguration, markAutonomous bool) error {
	client, err := kubeconfigutil.ClientSetFromFile(kubeadmconstants.GetKubeletKubeConfigPath())
	if err != nil {
		return err
	}
	if err := patchnodephase.AnnotateCRISocket(client, cfg.NodeRegistration.Name, cfg.NodeRegistration.CRISocket); err != nil {
		return err
	}

	if markAutonomous {
		if err := apiclient.PatchNode(client, cfg.NodeRegistration.Name, annotateNodeWithAutonomousNode); err != nil {
			return err
		}
	}

	if err := apiclient.PatchNode(client, cfg.NodeRegistration.Name, func(n *v1.Node) {
		n.Labels[projectinfo.GetEdgeWorkerLabelKey()] = "true"
	}); err != nil {
		return err
	}
	return nil
}

//patchCloudNode patch labels and annotations for cloud-node.
func patchCloudNode(cfg *kubeadm.JoinConfiguration) error {
	client, err := kubeconfigutil.ClientSetFromFile(kubeadmconstants.GetKubeletKubeConfigPath())
	if err != nil {
		return err
	}
	if err := patchnodephase.AnnotateCRISocket(client, cfg.NodeRegistration.Name, cfg.NodeRegistration.CRISocket); err != nil {
		return err
	}
	if err := apiclient.PatchNode(client, cfg.NodeRegistration.Name, func(n *v1.Node) {
		n.Labels[projectinfo.GetEdgeWorkerLabelKey()] = "false"
	}); err != nil {
		return err
	}
	return nil
}

func annotateNodeWithAutonomousNode(n *v1.Node) {
	klog.V(1).Infof("[patchnode] mark autonomous to the Node API object %q as an annotation\n", n.Name)

	if n.ObjectMeta.Annotations == nil {
		n.ObjectMeta.Annotations = make(map[string]string)
	}
	n.ObjectMeta.Annotations[constants.AnnotationAutonomy] = "true"
}
