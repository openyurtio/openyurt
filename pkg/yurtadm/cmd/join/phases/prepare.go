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
	"os"
	"path/filepath"

	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurtadm/cmd/join/joindata"
	"github.com/openyurtio/openyurt/pkg/yurtadm/kubernetes/kubeadm/app/cmd/options"
	"github.com/openyurtio/openyurt/pkg/yurtadm/kubernetes/kubeadm/app/cmd/phases/workflow"
	"github.com/openyurtio/openyurt/pkg/yurtadm/kubernetes/kubeadm/app/constants"
	"github.com/openyurtio/openyurt/pkg/yurtadm/util/kubernetes"
	"github.com/openyurtio/openyurt/pkg/yurtadm/util/system"
)

// NewPreparePhase creates a yurtadm workflow phase that initialize the node environment.
func NewPreparePhase() workflow.Phase {
	return workflow.Phase{
		Name:  "Initialize system environment.",
		Short: "Initialize system environment.",
		Run:   runPrepare,
		InheritFlags: []string{
			options.TokenStr,
		},
	}
}

//runPrepare executes the node initialization process.
func runPrepare(c workflow.RunData) error {
	data, ok := c.(joindata.YurtJoinData)
	if !ok {
		return fmt.Errorf("Prepare phase invoked with an invalid data struct. ")
	}

	// cleanup at first
	staticPodsPath := filepath.Join(constants.KubernetesDir, constants.ManifestsSubDirName)
	if err := os.RemoveAll(staticPodsPath); err != nil {
		klog.Warningf("remove %s: %v", staticPodsPath, err)
	}

	if err := system.SetIpv4Forward(); err != nil {
		return err
	}
	if err := system.SetBridgeSetting(); err != nil {
		return err
	}
	if err := system.SetSELinux(); err != nil {
		return err
	}
	if err := kubernetes.CheckAndInstallKubelet(data.KubernetesResourceServer(), data.KubernetesVersion()); err != nil {
		return err
	}
	if err := kubernetes.SetKubeletService(); err != nil {
		return err
	}
	if err := kubernetes.SetKubeletUnitConfig(); err != nil {
		return err
	}
	if err := kubernetes.SetKubeletConfigForNode(); err != nil {
		return err
	}
	if err := kubernetes.SetKubeletCaCert(data.TLSBootstrapCfg()); err != nil {
		return err
	}
	return nil
}
