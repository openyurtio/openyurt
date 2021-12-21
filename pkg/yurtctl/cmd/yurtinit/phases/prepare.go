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

	"k8s.io/kubernetes/cmd/kubeadm/app/cmd/phases/workflow"

	"github.com/openyurtio/openyurt/pkg/yurtctl/util/kubernetes"
	"github.com/openyurtio/openyurt/pkg/yurtctl/util/system"
)

// NewEdgeNodePhase creates a yurtctl workflow phase that initialize the node environment.
func NewPreparePhase() workflow.Phase {
	return workflow.Phase{
		Name:  "Initialize system environment.",
		Short: "Initialize system environment.",
		Run:   runPrepare,
	}
}

//runPrepare executes the node initialization process.
func runPrepare(c workflow.RunData) error {
	data, ok := c.(YurtInitData)
	if !ok {
		return fmt.Errorf("Prepare phase invoked with an invalid data struct. ")
	}
	initCfg := data.Cfg()

	if err := system.SetIpv4Forward(); err != nil {
		return err
	}
	if err := system.SetBridgeSetting(); err != nil {
		return err
	}
	if err := system.SetSELinux(); err != nil {
		return err
	}
	if err := kubernetes.CheckAndInstallKubelet(initCfg.ClusterConfiguration.KubernetesVersion); err != nil {
		return err
	}
	if err := kubernetes.SetKubeletService(); err != nil {
		return err
	}
	if err := kubernetes.SetKubeletUnitConfig(); err != nil {
		return err
	}
	return nil
}
