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

package convert

import (
	"fmt"

	"github.com/openyurtio/openyurt/pkg/node-servant/components"
	enutil "github.com/openyurtio/openyurt/pkg/yurtctl/util/edgenode"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

// NodeConverter do the convert job
type nodeConverter struct {
	Options
}

// NewConverterWithOptions create nodeConverter
func NewConverterWithOptions(o *Options) *nodeConverter {
	return &nodeConverter{
		*o,
	}
}

// Do, do the convert job.
// shall be implemented as idempotent, can execute multiple times with no side-affect.
func (n *nodeConverter) Do() error {
	if err := n.validateOptions(); err != nil {
		return err
	}
	if err := n.preflightCheck(); err != nil {
		return err
	}

	if err := n.installYurtHub(); err != nil {
		return err
	}
	if err := n.convertKubelet(); err != nil {
		return err
	}

	return nil
}

func (n *nodeConverter) validateOptions() error {
	if !util.IsSupportedWorkingMode(n.workingMode) {
		return fmt.Errorf("workingMode must be pointed out as cloud or edge. got %s", n.workingMode)
	}

	return nil
}

func (n *nodeConverter) preflightCheck() error {
	// 1. check if critical files exist
	if _, err := enutil.FileExists(n.kubeadmConfPath); err != nil {
		return err
	}

	return nil
}

func (n *nodeConverter) installYurtHub() error {
	apiServerAddress, err := components.GetApiServerAddress(n.kubeadmConfPath)
	if err != nil {
		return err
	}
	if apiServerAddress == "" {
		return fmt.Errorf("get apiServerAddress empty")
	}
	op := components.NewYurthubOperator(apiServerAddress, n.yurthubImage, n.joinToken,
		n.workingMode, n.yurthubHealthCheckTimeout)
	return op.Install()
}

func (n *nodeConverter) convertKubelet() error {
	op := components.NewKubeletOperator(n.openyurtDir, n.kubeadmConfPath)
	return op.RedirectTrafficToYurtHub()
}
