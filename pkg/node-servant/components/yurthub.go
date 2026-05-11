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

package components

import (
	"os"

	"github.com/openyurtio/openyurt/pkg/yurtadm/constants"
	"github.com/openyurtio/openyurt/pkg/yurtadm/util/edgenode"
	yurthubutil "github.com/openyurtio/openyurt/pkg/yurtadm/util/yurthub"
)

var (
	statYurthubBinaryFunc           = os.Stat
	removeYurthubBinaryFunc         = os.RemoveAll
	copyEmbeddedYurthubBinaryFunc   = edgenode.CopyFile
	checkAndInstallYurthubFunc      = installEmbeddedYurthub
	createYurthubSystemdServiceFunc = yurthubutil.CreateYurthubSystemdServiceWithConfig
	checkYurthubServiceHealthFunc   = yurthubutil.CheckYurthubServiceHealth
	checkYurthubReadyzFunc          = yurthubutil.CheckYurthubReadyz
	cleanYurthubHostArtifactsFunc   = yurthubutil.CleanYurthubHostArtifacts
)

type yurtHubOperator struct {
	cfg *yurthubutil.YurthubHostConfig
}

// NewYurthubOperator creates an operator that manages host-level yurthub
// lifecycle with the systemd + binary deployment path.
func NewYurthubOperator(cfg *yurthubutil.YurthubHostConfig) *yurtHubOperator {
	return &yurtHubOperator{cfg: cfg}
}

// Install installs yurthub on the host as a systemd service and waits for it
// to become healthy before kubelet traffic is redirected.
func (op *yurtHubOperator) Install() error {
	if err := checkAndInstallYurthubFunc(); err != nil {
		return err
	}

	if err := createYurthubSystemdServiceFunc(op.cfg); err != nil {
		return err
	}

	// 2. wait yurthub pod to be ready
	return hubHealthcheck(op.yurthubHealthCheckTimeout)
}

// UnInstall remove yaml and configs of yurthub
func (op *yurtHubOperator) UnInstall() error {
	// 1. remove the yurt-hub.yaml to delete the yurt-hub
	podManifestPath := enutil.GetPodManifestPath()
	yurthubYamlPath := getYurthubYaml(podManifestPath)
	if _, err := enutil.FileExists(yurthubYamlPath); os.IsNotExist(err) {
		klog.Infof("UnInstallYurthub: %s is not exists, skip delete", yurthubYamlPath)
	} else {
		err := os.Remove(yurthubYamlPath)
		if err != nil {
			return err
		}
		klog.Infof("UnInstallYurthub: %s has been removed", yurthubYamlPath)
	}

	// 2. remove yurt-hub config directory and certificates in it
	yurthubConf := getYurthubConf()
	if _, err := enutil.FileExists(yurthubConf); os.IsNotExist(err) {
		klog.Infof("UnInstallYurthub: dir %s does not exist, skip delete", yurthubConf)
	} else {
		if err := os.RemoveAll(yurthubConf); err != nil {
			return err
		}
		klog.Infof("UnInstallYurthub: config dir %s has been removed", yurthubConf)
	}

	// 3. remove yurthub cache dir
	// since k8s may takes a while to notice and remove yurthub pod, we have to wait for that.
	// because, if we delete dir before yurthub exit, yurthub may recreate cache/kubelet dir before exit.
	if err := waitUntilYurthubExit(time.Duration(60)*time.Second, time.Duration(1)*time.Second); err != nil {
		return err
	}
	cacheDir := getYurthubCacheDir()
	if err := os.RemoveAll(cacheDir); err != nil {
		return err
	}
	klog.Infof("UnInstallYurthub: cache dir %s has been removed", cacheDir)

	return nil
}

	return checkYurthubReadyzFunc(op.bindAddress())
}

// UnInstall removes the host-level yurthub artifacts created during conversion.
func (op *yurtHubOperator) UnInstall() error {
	return cleanYurthubHostArtifactsFunc()
}

func (op *yurtHubOperator) bindAddress() string {
	if op.cfg != nil && len(op.cfg.BindAddress) != 0 {
		return op.cfg.BindAddress
	}

	return constants.DefaultYurtHubServerAddr
}

func installEmbeddedYurthub() error {
	if _, err := statYurthubBinaryFunc(constants.YurthubExecStart); err == nil {
		if err := removeYurthubBinaryFunc(constants.YurthubExecStart); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	return copyEmbeddedYurthubBinaryFunc(constants.YurthubEmbeddedPath, constants.YurthubExecStart, 0755)
}
