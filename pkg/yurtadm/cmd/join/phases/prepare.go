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
	"os"
	"path/filepath"
	"strings"

	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/util/templates"
	"github.com/openyurtio/openyurt/pkg/yurtadm/cmd/join/joindata"
	"github.com/openyurtio/openyurt/pkg/yurtadm/constants"
	yurtadmutil "github.com/openyurtio/openyurt/pkg/yurtadm/util/kubernetes"
	"github.com/openyurtio/openyurt/pkg/yurtadm/util/system"
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
	if err := yurtadmutil.CheckAndInstallKubernetesCni(); err != nil {
		return err
	}
	if err := yurtadmutil.SetKubeletService(); err != nil {
		return err
	}
	if err := yurtadmutil.SetKubeletUnitConfig(); err != nil {
		return err
	}
	if err := yurtadmutil.SetKubeletConfigForNode(); err != nil {
		return err
	}
	if err := addYurthubStaticYaml(data, filepath.Join(constants.KubeletConfigureDir, constants.ManifestsSubDirName)); err != nil {
		return err
	}
	if err := yurtadmutil.SetDiscoveryConfig(data); err != nil {
		return err
	}
	if err := yurtadmutil.SetKubeadmJoinConfig(data); err != nil {
		return err
	}
	return nil
}

// addYurthubStaticYaml generate YurtHub static yaml for worker node.
func addYurthubStaticYaml(data joindata.YurtJoinData, podManifestPath string) error {
	klog.Info("[join-node] Adding edge hub static yaml")
	if _, err := os.Stat(podManifestPath); err != nil {
		if os.IsNotExist(err) {
			err = os.MkdirAll(podManifestPath, os.ModePerm)
			if err != nil {
				return err
			}
		} else {
			klog.Errorf("Describe dir %s fail: %v", podManifestPath, err)
			return err
		}
	}

	// There can be multiple master IP addresses
	serverAddrs := strings.Split(data.ServerAddr(), ",")
	for i := 0; i < len(serverAddrs); i++ {
		serverAddrs[i] = fmt.Sprintf("https://%s", serverAddrs[i])
	}

	kubernetesServerAddrs := strings.Join(serverAddrs, ",")

	ctx := map[string]string{
		"kubernetesServerAddr": kubernetesServerAddrs,
		"image":                data.YurtHubImage(),
		"joinToken":            data.JoinToken(),
		"workingMode":          data.NodeRegistration().WorkingMode,
		"organizations":        data.NodeRegistration().Organizations,
		"yurthubServerAddr":    data.YurtHubServer(),
	}

	yurthubTemplate, err := templates.SubsituteTemplate(constants.YurthubTemplate, ctx)
	if err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(podManifestPath, constants.YurthubStaticPodFileName), []byte(yurthubTemplate), 0600); err != nil {
		return err
	}
	klog.Info("[join-node] Add hub agent static yaml is ok")
	return nil
}
