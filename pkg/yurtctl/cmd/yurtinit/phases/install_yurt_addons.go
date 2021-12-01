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
	"net"
	"runtime"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/kubernetes/cmd/kubeadm/app/cmd/phases/workflow"
	kubeContants "k8s.io/kubernetes/cmd/kubeadm/app/constants"

	YurtContants "github.com/openyurtio/openyurt/pkg/yurtctl/constants"
	kubeutil "github.com/openyurtio/openyurt/pkg/yurtctl/util/kubernetes"
)

func NewInstallYurtAddonsPhase() workflow.Phase {
	return workflow.Phase{
		Name:  "Install OpenYurt component.",
		Short: "Install OpenYurt component.",
		Run:   runInstallYurtAddons,
	}
}

func runInstallYurtAddons(c workflow.RunData) error {
	data, ok := c.(YurtInitData)
	if !ok {
		return fmt.Errorf("Install yurt addons phase invoked with an invalid data struct. ")
	}
	if !data.IsConvertYurtCluster() {
		return nil
	}

	restCfg, err := clientcmd.BuildConfigFromFlags("", kubeContants.GetAdminKubeConfigPath())
	if err != nil {
		return err
	}
	client, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return err
	}
	dynamicClient, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		return err
	}
	imageRegistry := data.OpenYurtImageRegistry()
	version := data.OpenYurtVersion()
	tunnelServerAddress := data.YurtTunnelAddress()

	if len(imageRegistry) == 0 {
		imageRegistry = YurtContants.DefaultOpenYurtImageRegistry
	}
	if len(version) == 0 {
		version = YurtContants.DefaultOpenYurtVersion
	}
	if err := kubeutil.DeployYurtControllerManager(client, fmt.Sprintf("%s/%s:%s", imageRegistry, YurtContants.YurtControllerManager, version)); err != nil {
		return err
	}
	if err := kubeutil.DeployYurtAppManager(client, fmt.Sprintf("%s/%s:%s", imageRegistry, YurtContants.YurtAppManager, version), dynamicClient, runtime.GOARCH); err != nil {
		return err
	}
	var certIP string
	if data.YurtTunnelAddress() != "" {
		certIP, _, _ = net.SplitHostPort(data.YurtTunnelAddress())
	}
	if err := kubeutil.DeployYurttunnelServer(client, certIP, fmt.Sprintf("%s/%s:%s", imageRegistry, YurtContants.YurtTunnelServer, version), runtime.GOARCH); err != nil {
		return err
	}
	if err := kubeutil.DeployYurttunnelAgent(client, tunnelServerAddress, fmt.Sprintf("%s/%s:%s", imageRegistry, YurtContants.YurtTunnelAgent, version)); err != nil {
		return err
	}
	return nil
}
