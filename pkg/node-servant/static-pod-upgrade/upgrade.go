/*
Copyright 2023 The OpenYurt Authors.

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

package upgrade

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"k8s.io/klog/v2"

	appsv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
	"github.com/openyurtio/openyurt/pkg/node-servant/static-pod-upgrade/util"
)

var (
	DefaultConfigmapPath = "/data"
	DefaultManifestPath  = "/etc/kubernetes/manifests"
	DefaultUpgradePath   = "/tmp/manifests"
)

type Controller struct {
	// Name of static pod
	name string
	// Namespace of static pod
	namespace string
	// Manifest file name of static pod
	manifest string
	// The latest static pod hash
	hash string
	// Only support `OTA` and `Auto`
	upgradeMode string
	// Timeout for upgrade success check
	timeout time.Duration

	// Manifest path of static pod, default `/etc/kubernetes/manifests/manifestName.yaml`
	manifestPath string
	// The backup manifest path, default `/etc/kubernetes/manifests/openyurtio-upgrade/manifestName.bak`
	bakManifestPath string
	// Default is `/data/podName`
	configMapDataPath string
	// The latest manifest path, default `/etc/kubernetes/manifests/openyurtio-upgrade/manifestName.upgrade`
	upgradeManifestPath string
}

func NewWithOptions(o *Options) (*Controller, error) {
	ctrl := New(o.name, o.namespace, o.manifest, o.mode)
	ctrl.hash = o.hash
	ctrl.timeout = o.timeout
	return ctrl, nil
}

func New(name, namespace, manifest, mode string) *Controller {
	ctrl := &Controller{
		name:        name,
		namespace:   namespace,
		manifest:    manifest,
		upgradeMode: mode,
	}
	ctrl.manifestPath = filepath.Join(DefaultManifestPath, util.WithYamlSuffix(ctrl.manifest))
	ctrl.bakManifestPath = filepath.Join(DefaultUpgradePath, util.WithBackupSuffix(ctrl.manifest))
	ctrl.configMapDataPath = filepath.Join(DefaultConfigmapPath, ctrl.manifest)
	ctrl.upgradeManifestPath = filepath.Join(DefaultUpgradePath, util.WithUpgradeSuffix(ctrl.manifest))

	return ctrl
}

func (ctrl *Controller) Upgrade() error {
	if err := ctrl.createUpgradeSpace(); err != nil {
		return err
	}
	klog.Info("Create upgrade space success")

	// execute upgrade operations
	switch strings.ToLower(ctrl.upgradeMode) {
	case strings.ToLower(string(appsv1alpha1.AdvancedRollingUpdateUpgradeStrategyType)):
		return ctrl.AutoUpgrade()
	case strings.ToLower(string(appsv1alpha1.OTAUpgradeStrategyType)):
		return ctrl.OTAUpgrade()
	}

	return nil
}

func (ctrl *Controller) AutoUpgrade() error {
	// (1) Prepare the latest manifest
	if err := ctrl.prepareManifest(); err != nil {
		return err
	}
	klog.Info("Auto prepare upgrade manifest success")

	// (2) Back up the old manifest in case of upgrade failure
	if err := ctrl.backupManifest(); err != nil {
		return err
	}
	klog.Info("Auto upgrade backupManifest success")

	// (3) Replace manifest and kubelet will upgrade the static pod automatically
	if err := ctrl.replaceManifest(); err != nil {
		return err
	}
	klog.Info("Auto upgrade replaceManifest success")

	// (4) Verify the new static pod is running
	ok, err := ctrl.verify()
	if err != nil {
		if err := ctrl.rollbackManifest(); err != nil {
			klog.Errorf("could not rollback manifest when upgrade failed, %v", err)
		}
		return err
	}
	if !ok {
		if err := ctrl.rollbackManifest(); err != nil {
			klog.Errorf("could not rollback manifest when upgrade failed, %v", err)
		}
		return fmt.Errorf("the latest static pod is not running")
	}
	klog.Info("Auto upgrade verify success")

	return nil
}

func (ctrl *Controller) OTAUpgrade() error {
	// (1) Back up the old manifest in case of upgrade failure
	if err := ctrl.backupManifest(); err != nil {
		return err
	}
	klog.Info("OTA upgrade backupManifest success")

	// (2) Replace manifest and kubelet will upgrade the static pod automatically
	if err := ctrl.replaceManifest(); err != nil {
		return err
	}
	klog.Info("OTA upgrade replaceManifest success")

	return nil
}

// createUpgradeSpace creates DefaultUpgradePath if it doesn't exist
func (ctrl *Controller) createUpgradeSpace() error {
	// Make sure upgrade dir exist
	if _, err := os.Stat(DefaultUpgradePath); os.IsNotExist(err) {
		if err = os.Mkdir(DefaultUpgradePath, 0755); err != nil {
			return err
		}
	}
	return nil
}

// prepareManifest move the latest manifest to DefaultUpgradePath and set `.upgrade` suffix
func (ctrl *Controller) prepareManifest() error {
	return util.CopyFile(ctrl.configMapDataPath, ctrl.upgradeManifestPath)
}

// backUpManifest backup the old manifest in order to roll back when errors occur
func (ctrl *Controller) backupManifest() error {
	return util.CopyFile(ctrl.manifestPath, ctrl.bakManifestPath)
}

// replaceManifest replace old manifest with the latest one, it achieves static pod upgrade
func (ctrl *Controller) replaceManifest() error {
	return util.CopyFile(ctrl.upgradeManifestPath, ctrl.manifestPath)
}

// rollbackManifest replace new manifest with the backup
func (ctrl *Controller) rollbackManifest() error {
	return util.CopyFile(ctrl.bakManifestPath, ctrl.manifestPath)
}

// verify make sure the latest static pod is running
// return false when the latest static pod failed or check status time out
func (ctrl *Controller) verify() (bool, error) {
	return util.WaitForPodRunning(ctrl.namespace, ctrl.name, ctrl.hash, ctrl.timeout)
}
