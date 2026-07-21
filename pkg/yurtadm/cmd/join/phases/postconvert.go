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

package phases

import (
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/node-servant/components"
	"github.com/openyurtio/openyurt/pkg/yurtadm/cmd/join/joindata"
	"github.com/openyurtio/openyurt/pkg/yurtadm/constants"
	yurthubutil "github.com/openyurtio/openyurt/pkg/yurtadm/util/yurthub"
)

const bootstrapModeKubeletCertificate = "kubeletcertificate"

var (
	createYurthubSystemdServiceFunc = yurthubutil.CreateYurthubSystemdServiceWithConfig
	checkYurthubServiceHealthFunc   = yurthubutil.CheckYurthubServiceHealth
	checkYurthubReadyzFunc          = yurthubutil.CheckYurthubReadyz
	redirectKubeletTrafficFunc      = func(openyurtDir string) error {
		return components.NewKubeletOperator(openyurtDir).RedirectTrafficToYurtHub()
	}
)

// RunConvert installs yurthub and redirects kubelet traffic on edge and cloud
// nodes after kubeadm join has completed.
func RunConvert(data joindata.YurtJoinData) error {
	if data.NodeRegistration().WorkingMode == constants.LocalNode {
		return nil
	}

	mode := data.NodeRegistration().WorkingMode
	klog.Infof("[post-convert] starting yurthub installation and kubelet redirect for %s node", mode)

	// 1. Configure yurthub with kubeletcertificate bootstrap mode, identical to what node-servant convert uses.
	yurthubCfg := &yurthubutil.YurthubHostConfig{
		BootstrapMode: bootstrapModeKubeletCertificate,
		Namespace:     data.Namespace(),
		NodeName:      data.NodeRegistration().Name,
		NodePoolName:  data.NodeRegistration().NodePoolName,
		ServerAddr:    data.ServerAddr(),
		WorkingMode:   mode,
	}

	// 2. Create and start yurthub systemd service.
	klog.Info("[post-convert] creating yurthub systemd service")
	if err := createYurthubSystemdServiceFunc(yurthubCfg); err != nil {
		return err
	}

	// 3. Wait for yurthub to become healthy.
	klog.Info("[post-convert] waiting for yurthub to become healthy")
	if err := checkYurthubServiceHealthFunc(data.YurtHubServer()); err != nil {
		return err
	}
	klog.Info("[post-convert] yurthub service is healthy")

	// 4. Wait for yurthub certificates to be ready.
	if err := checkYurthubReadyzFunc(data.YurtHubServer()); err != nil {
		return err
	}
	klog.Info("[post-convert] yurthub certificates are ready")

	// 5. Redirect kubelet traffic to yurthub, using the exact same method as node-servant convert.
	klog.Info("[post-convert] redirecting kubelet traffic to yurthub")
	if err := redirectKubeletTrafficFunc(constants.OpenyurtDir); err != nil {
		return err
	}
	klog.Info("[post-convert] kubelet traffic redirected to yurthub successfully")

	// 6. Verify kubelet is healthy after the redirect restart.
	if err := checkKubeletStatusFunc(); err != nil {
		return err
	}

	return nil
}
