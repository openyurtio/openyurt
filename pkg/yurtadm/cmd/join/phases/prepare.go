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
	"os"
	"path/filepath"

	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurtadm/cmd/join/joindata"
	"github.com/openyurtio/openyurt/pkg/yurtadm/constants"
	"github.com/openyurtio/openyurt/pkg/yurtadm/util/edgenode"
	yurtadmutil "github.com/openyurtio/openyurt/pkg/yurtadm/util/kubernetes"
	"github.com/openyurtio/openyurt/pkg/yurtadm/util/system"
	"github.com/openyurtio/openyurt/pkg/yurtadm/util/yurthub"
)

// RunPrepare executes the node initialization process.
func RunPrepare(data joindata.YurtJoinData) error {
	// cleanup at first
	staticPodsPath := filepath.Join(constants.KubeletConfigureDir, constants.ManifestsSubDirName)
	if err := os.RemoveAll(staticPodsPath); err != nil {
		klog.Warningf("remove %s: %v", staticPodsPath, err)
	}

	if err := system.SetIpv4Forward(); err != nil {
		return err
	}
	if err := system.SetBridgeSetting(); err != nil {
		return err
	}
	if err := system.SetSELinux(); err != nil {
		return err
	}
	if err := yurtadmutil.CheckAndInstallKubelet(data.KubernetesResourceServer(), data.KubernetesVersion()); err != nil {
		return err
	}
	if err := yurtadmutil.CheckAndInstallKubeadm(data.KubernetesResourceServer(), data.KubernetesVersion()); err != nil {
		return err
	}
	if err := yurtadmutil.CheckAndInstallKubernetesCni(data.ReuseCNIBin()); err != nil {
		return err
	}
	if err := yurtadmutil.SetKubeletService(); err != nil {
		return err
	}
	if err := yurtadmutil.EnableKubeletService(); err != nil {
		return err
	}
	if err := yurtadmutil.SetKubeletUnitConfig(); err != nil {
		return err
	}
	if err := yurtadmutil.SetKubeletConfigForNode(); err != nil {
		return err
	}
	if err := yurthub.SetHubBootstrapConfig(data.ServerAddr(), data.JoinToken(), data.CaCertHashes()); err != nil {
		return err
	}
	if err := yurthub.AddYurthubStaticYaml(data, constants.StaticPodPath); err != nil {
		return err
	}
	if len(data.StaticPodTemplateList()) != 0 {
		// deploy user specified static pods
		if err := edgenode.DeployStaticYaml(data.StaticPodManifestList(), data.StaticPodTemplateList(), constants.StaticPodPath); err != nil {
			return err
		}
	}
	if err := yurtadmutil.SetDiscoveryConfig(data); err != nil {
		return err
	}
	if data.CfgPath() == "" {
		if err := yurtadmutil.SetKubeadmJoinConfig(data); err != nil {
			return err
		}
	}
	return nil
}
