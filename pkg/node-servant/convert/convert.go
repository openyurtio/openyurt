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

package convert

import (
	"fmt"
	"strings"

	"github.com/openyurtio/openyurt/pkg/node-servant/components"
	yurthubutil "github.com/openyurtio/openyurt/pkg/yurtadm/util/yurthub"
)

const bootstrapModeKubeletCertificate = "kubeletcertificate"

var (
	getAPIServerAddressFunc = components.GetApiServerAddress
	installYurthubFunc      = func(cfg *yurthubutil.YurthubHostConfig) error {
		return components.NewYurthubOperator(cfg).Install()
	}
	redirectKubeletFunc = func(openyurtDir string) error {
		return components.NewKubeletOperator(openyurtDir).RedirectTrafficToYurtHub()
	}
)

// Config has the information that required by convert operation.
type Config struct {
	kubeadmConfPaths []string
	namespace        string
	nodeName         string
	nodePoolName     string
	openyurtDir      string
	workingMode      string
	yurthubBinaryURL string
	yurthubVersion   string
}

// nodeConverter do the convert job.
type nodeConverter struct {
	Config
}

// NewConverterWithOptions create nodeConverter.
func NewConverterWithOptions(o *Options) *nodeConverter {
	return &nodeConverter{
		Config: Config{
			kubeadmConfPaths: strings.Split(o.kubeadmConfPaths, ","),
			namespace:        o.namespace,
			nodeName:         o.nodeName,
			nodePoolName:     o.nodePoolName,
			openyurtDir:      o.openyurtDir,
			workingMode:      o.workingMode,
			yurthubBinaryURL: o.yurthubBinaryURL,
			yurthubVersion:   o.yurthubVersion,
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
	apiServerAddress, err := getAPIServerAddressFunc(n.kubeadmConfPaths)
	if err != nil {
		return err
	}
	if apiServerAddress == "" {
		return fmt.Errorf("get apiServerAddress empty")
	}

	return installYurthubFunc(n.yurthubHostConfig(apiServerAddress))
}

func (n *nodeConverter) convertKubelet() error {
	return redirectKubeletFunc(n.openyurtDir)
}

func (n *nodeConverter) yurthubHostConfig(apiServerAddress string) *yurthubutil.YurthubHostConfig {
	return &yurthubutil.YurthubHostConfig{
		BinaryURL:     n.yurthubBinaryURL,
		BootstrapMode: bootstrapModeKubeletCertificate,
		Namespace:     n.namespace,
		NodeName:      n.nodeName,
		NodePoolName:  n.nodePoolName,
		ServerAddr:    apiServerAddress,
		Version:       n.yurthubVersion,
		WorkingMode:   n.workingMode,
	}
}
