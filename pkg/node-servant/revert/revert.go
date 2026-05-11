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

package revert

import (
	"fmt"

	nodeservant "github.com/openyurtio/openyurt/pkg/node-servant"
	"github.com/openyurtio/openyurt/pkg/node-servant/components"
)

var (
	undoKubeletRedirectFunc = func(openyurtDir string) error {
		return components.NewKubeletOperator(openyurtDir).UndoRedirectTrafficToYurtHub()
	}
	uninstallYurthubFunc = func() error {
		return components.NewYurthubOperator(nil).UnInstall()
	}
	restartContainersFunc = func(nodeName string) error {
		return components.RestartNonPauseContainers(nodeName, conversionJobPodPrefix(nodeName))
	}
)

// NodeReverter do the revert job.
type nodeReverter struct {
	Options
}

// NewReverterWithOptions creates nodeReverter.
func NewReverterWithOptions(o *Options) *nodeReverter {
	return &nodeReverter{
		*o,
	}
}

// Do is used for the revert job.
// shall be implemented as idempotent, can execute multiple times with no side-affect.
func (n *nodeReverter) Do() error {
	if err := n.revertKubelet(); err != nil {
		return err
	}
	if err := n.restartContainers(); err != nil {
		return err
	}
	if err := n.unInstallYurtHub(); err != nil {
		return err
	}

	return nil
}

func (n *nodeReverter) revertKubelet() error {
	return undoKubeletRedirectFunc(n.openyurtDir)
}

func (n *nodeReverter) unInstallYurtHub() error {
	return uninstallYurthubFunc()
}

func (n *nodeReverter) restartContainers() error {
	return restartContainersFunc(n.nodeName)
}

func conversionJobPodPrefix(nodeName string) string {
	return fmt.Sprintf("%s-%s", nodeservant.ConversionJobNameBase, nodeName)
}
