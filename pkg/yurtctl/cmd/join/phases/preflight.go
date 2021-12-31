/*
Copyright 2017 The Kubernetes Authors.

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
	"github.com/pkg/errors"
	"k8s.io/klog/v2"
	utilsexec "k8s.io/utils/exec"

	"github.com/openyurtio/openyurt/pkg/yurtctl/cmd/join/joindata"
	"github.com/openyurtio/openyurt/pkg/yurtctl/kubernetes/kubeadm/app/cmd/options"
	"github.com/openyurtio/openyurt/pkg/yurtctl/kubernetes/kubeadm/app/cmd/phases/workflow"
	"github.com/openyurtio/openyurt/pkg/yurtctl/kubernetes/kubeadm/app/preflight"
)

// NewPreflightPhase creates a kubeadm workflow phase that implements preflight checks for a new node join
func NewPreflightPhase() workflow.Phase {
	return workflow.Phase{
		Name:  "preflight [api-server-endpoint]",
		Short: "Run join pre-flight checks",
		Long:  "Run pre-flight checks for kubeadm join.",
		Run:   runPreflight,
		InheritFlags: []string{
			options.TokenStr,
			options.NodeCRISocket,
			options.NodeName,
			options.IgnorePreflightErrors,
		},
	}
}

// runPreflight executes preflight checks logic.
func runPreflight(c workflow.RunData) error {
	data, ok := c.(joindata.YurtJoinData)
	if !ok {
		return errors.New("preflight phase invoked with an invalid data struct")
	}

	// Start with general checks
	klog.V(1).Infoln("[preflight] Running general checks")
	if err := preflight.RunJoinNodeChecks(utilsexec.New(), data); err != nil {
		return err
	}

	return nil
}
