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

package revert

import (
	"time"

	"github.com/openyurtio/openyurt/pkg/node-servant/components"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

// NodeReverter do the revert job
type nodeReverter struct {
	Options
}

// NewReverterWithOptions creates nodeReverter
func NewReverterWithOptions(o *Options) *nodeReverter {
	return &nodeReverter{
		*o,
	}
}

// Do is used for the revert job
// shall be implemented as idempotent, can execute multiple times with no side-affect.
func (n *nodeReverter) Do() error {

	if err := n.revertKubelet(); err != nil {
		return err
	}
	if err := n.unInstallYurtHub(); err != nil {
		return err
	}
	if err := n.unInstallYurtTunnelAgent(); err != nil {
		return err
	}
	if err := n.unInstallYurtTunnelServer(); err != nil {
		return err
	}

	return nil
}

func (n *nodeReverter) revertKubelet() error {
	op := components.NewKubeletOperator(n.openyurtDir)
	return op.UndoRedirectTrafficToYurtHub()
}

func (n *nodeReverter) unInstallYurtHub() error {
	op := components.NewYurthubOperator("", "", "",
		util.WorkingModeCloud, time.Duration(1), true, true) // params is not important here
	return op.UnInstall()
}

func (n *nodeReverter) unInstallYurtTunnelAgent() error {
	return components.UnInstallYurtTunnelAgent()
}

func (n *nodeReverter) unInstallYurtTunnelServer() error {
	return components.UnInstallYurtTunnelServer()
}
