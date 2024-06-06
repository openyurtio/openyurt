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
)

// Config has the information that required by convert operation
type Config struct {
	yurthubHealthCheckTimeout time.Duration
	joinToken                 string
	kubeadmConfPaths          []string
	openyurtDir               string
}

// nodeConverter do the convert job
type nodeConverter struct {
	Config
}

// NewConverterWithOptions create nodeConverter
func NewConverterWithOptions(o *Options) *nodeConverter {
	return &nodeConverter{
		Config: Config{
			yurthubHealthCheckTimeout: o.yurthubHealthCheckTimeout,
			joinToken:                 o.joinToken,
			kubeadmConfPaths:          strings.Split(o.kubeadmConfPaths, ","),
			openyurtDir:               o.openyurtDir,
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
	op := components.NewYurthubOperator(apiServerAddress, n.joinToken, n.yurthubHealthCheckTimeout)
	return op.Install()
}

func (n *nodeConverter) convertKubelet() error {
	op := components.NewKubeletOperator(n.openyurtDir)
	return op.RedirectTrafficToYurtHub()
}
