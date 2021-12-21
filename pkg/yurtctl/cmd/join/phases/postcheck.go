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
	"path/filepath"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurtctl/cmd/join/joindata"
	"github.com/openyurtio/openyurt/pkg/yurtctl/kubernetes/kubeadm/app/cmd/options"
	"github.com/openyurtio/openyurt/pkg/yurtctl/kubernetes/kubeadm/app/cmd/phases/workflow"
	"github.com/openyurtio/openyurt/pkg/yurtctl/kubernetes/kubeadm/app/constants"
	"github.com/openyurtio/openyurt/pkg/yurtctl/kubernetes/kubeadm/app/util/apiclient"
	"github.com/openyurtio/openyurt/pkg/yurtctl/kubernetes/kubeadm/app/util/initsystem"
	"github.com/openyurtio/openyurt/pkg/yurtctl/kubernetes/kubeadm/app/util/kubeconfig"
	"github.com/openyurtio/openyurt/pkg/yurtctl/util/edgenode"
)

//NewPostcheckPhase creates a yurtctl workflow phase that check the health status of node components.
func NewPostcheckPhase() workflow.Phase {
	return workflow.Phase{
		Name:  "postcheck",
		Short: "postcheck",
		Run:   runPostCheck,
		InheritFlags: []string{
			options.TokenStr,
		},
	}
}

// runPostCheck executes the node health check process.
func runPostCheck(c workflow.RunData) error {
	j, ok := c.(joindata.YurtJoinData)
	if !ok {
		return fmt.Errorf("Postcheck edge-node phase invoked with an invalid data struct. ")
	}

	klog.V(1).Infof("check kubelet status.")
	if err := checkKubeletStatus(); err != nil {
		return err
	}
	klog.V(1).Infof("kubelet service is active")

	klog.V(1).Infof("waiting hub agent ready.")
	if err := checkYurthubHealthz(); err != nil {
		return err
	}
	klog.V(1).Infof("hub agent is ready")

	nodeRegistration := j.NodeRegistration()
	return patchNode(nodeRegistration.Name, nodeRegistration.CRISocket)
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

//patchNode patch annotations for worker node.
func patchNode(nodeName, criSocket string) error {
	client, err := kubeconfig.ClientSetFromFile(filepath.Join(constants.KubernetesDir, constants.KubeletKubeConfigFileName))
	if err != nil {
		return err
	}

	return apiclient.PatchNode(client, nodeName, func(n *v1.Node) {
		if n.ObjectMeta.Annotations == nil {
			n.ObjectMeta.Annotations = make(map[string]string)
		}
		n.ObjectMeta.Annotations[constants.AnnotationKubeadmCRISocket] = criSocket
	})
}
