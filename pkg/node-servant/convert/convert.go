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

	nodeservant "github.com/openyurtio/openyurt/pkg/node-servant"
	"github.com/openyurtio/openyurt/pkg/node-servant/components"
	"github.com/openyurtio/openyurt/pkg/yurtadm/constants"
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
	restartContainersFunc = func(nodeName string) error {
		return components.RestartNonPauseContainers(nodeName, conversionJobPodPrefix(nodeName))
	}
)

// Config has the information that required by convert operation.
type Config struct {
	kubeadmConfPaths []string
	nodeName         string
	nodePoolName     string
	openyurtDir      string
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
			nodeName:         o.nodeName,
			nodePoolName:     o.nodePoolName,
			openyurtDir:      o.openyurtDir,
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
	if err := n.restartContainers(); err != nil {
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

func (n *nodeConverter) restartContainers() error {
	return restartContainersFunc(n.nodeName)
}

func (n *nodeConverter) yurthubHostConfig(apiServerAddress string) *yurthubutil.YurthubHostConfig {
	return &yurthubutil.YurthubHostConfig{
		BootstrapMode: bootstrapModeKubeletCertificate,
		Namespace:     constants.YurthubNamespace,
		NodeName:      n.nodeName,
		NodePoolName:  n.nodePoolName,
		ServerAddr:    apiServerAddress,
		WorkingMode:   constants.EdgeNode,
	}
}

func conversionJobPodPrefix(nodeName string) string {
	return fmt.Sprintf("%s-%s", nodeservant.ConversionJobNameBase, nodeName)
}
