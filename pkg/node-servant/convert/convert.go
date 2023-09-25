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
	"strings"
	"time"

	"github.com/openyurtio/openyurt/pkg/node-servant/components"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

// Config has the information that required by convert operation
type Config struct {
	yurthubImage              string
	yurthubHealthCheckTimeout time.Duration
	workingMode               util.WorkingMode
	joinToken                 string
	kubeadmConfPaths          []string
	openyurtDir               string
	enableDummyIf             bool
	enableNodePool            bool
}

// nodeConverter do the convert job
type nodeConverter struct {
	Config
}

// NewConverterWithOptions create nodeConverter
func NewConverterWithOptions(o *Options) *nodeConverter {
	return &nodeConverter{
		Config: Config{
			yurthubImage:              o.yurthubImage,
			yurthubHealthCheckTimeout: o.yurthubHealthCheckTimeout,
			workingMode:               util.WorkingMode(o.workingMode),
			joinToken:                 o.joinToken,
			kubeadmConfPaths:          strings.Split(o.kubeadmConfPaths, ","),
			openyurtDir:               o.openyurtDir,
			enableDummyIf:             o.enableDummyIf,
			enableNodePool:            o.enableNodePool,
		},
	}
}

// Do is used for the convert job.
// shall be implemented as idempotent, can execute multiple times with no side effect.
func (n *nodeConverter) Do() error {
	if err := n.installYurtHub(); err != nil {
		return err
	}
	if err := n.convertKubelet(); err != nil {
		return err
	}

	return nil
}

func (n *nodeConverter) installYurtHub() error {
	apiServerAddress, err := components.GetApiServerAddress(n.kubeadmConfPaths)
	if err != nil {
		return err
	}
	if apiServerAddress == "" {
		return fmt.Errorf("get apiServerAddress empty")
	}
	op := components.NewYurthubOperator(apiServerAddress, n.yurthubImage, n.joinToken,
		n.workingMode, n.yurthubHealthCheckTimeout, n.enableDummyIf, n.enableNodePool)
	return op.Install()
}

func (n *nodeConverter) convertKubelet() error {
	op := components.NewKubeletOperator(n.openyurtDir)
	return op.RedirectTrafficToYurtHub()
}
