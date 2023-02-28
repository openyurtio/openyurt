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
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

const (
	DefaultUpgradeDir                   = "openyurtio-upgrade"
	defaultStaticPodRunningCheckTimeout = 2 * time.Minute

	// TODO: use constant value of static-pod controller
	OTA  = "ota"
	Auto = "auto"
)

var (
	DefaultConfigmapPath = "/data"
	DefaultManifestPath  = "/etc/kubernetes/manifests"
)

type Controller struct {
	client kubernetes.Interface

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

	// Manifest path of static pod, default `/etc/kubernetes/manifests/manifestName.yaml`
	manifestPath string
	// The backup manifest path, default `/etc/kubernetes/manifests/openyurtio-upgrade/manifestName.bak`
	bakManifestPath string
	// Default is `/data/podName`
	configMapDataPath string
	// The latest manifest path, default `/etc/kubernetes/manifests/openyurtio-upgrade/manifestName.upgrade`
	upgradeManifestPath string
}

func New(client kubernetes.Interface, name, namespace, manifest, hash, mode string) (*Controller, error) {
	ctrl := &Controller{
		client:      client,
		name:        name,
		namespace:   namespace,
		manifest:    manifest,
		hash:        hash,
		upgradeMode: mode,
	}

	ctrl.manifestPath = filepath.Join(DefaultManifestPath, WithYamlSuffix(ctrl.manifest))
	ctrl.bakManifestPath = filepath.Join(DefaultManifestPath, DefaultUpgradeDir, WithBackupSuffix(ctrl.manifest))
	ctrl.configMapDataPath = filepath.Join(DefaultConfigmapPath, ctrl.manifest)
	ctrl.upgradeManifestPath = filepath.Join(DefaultManifestPath, DefaultUpgradeDir, WithUpgradeSuffix(ctrl.manifest))

	return ctrl, nil
}

func (ctrl *Controller) Upgrade() error {
	// 1. Check the target static pod exist
	if err := ctrl.checkStaticPodExist(); err != nil {
		return err
	}
	klog.Info("Check static pod existence success")

	// 2. Check old manifest and the latest manifest exist
	if err := ctrl.checkManifestFileExist(); err != nil {
		return err
	}
	klog.Info("Check old manifest and new manifest files existence success")

	// 3. prepare the latest manifest
	if err := ctrl.prepareManifest(); err != nil {
		return err
	}
	klog.Info("Prepare upgrade manifest success")

	// 4. execute upgrade operations
	switch ctrl.upgradeMode {
	case Auto:
		return ctrl.AutoUpgrade()

	case OTA:
		return ctrl.OTAUpgrade()
	}

	return nil
}

func (ctrl *Controller) AutoUpgrade() error {
	// (1) Back up the old manifest in case of upgrade failure
	if err := ctrl.backupManifest(); err != nil {
		return err
	}
	klog.Info("Auto upgrade backupManifest success")

	// (2) Replace manifest and kubelet will upgrade the static pod automatically
	if err := ctrl.replaceManifest(); err != nil {
		return err
	}
	klog.Info("Auto upgrade replaceManifest success")

	// (3) Verify the new static pod is running
	ok, err := ctrl.verify()
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("the latest static pod is not running")
	}
	klog.Info("Auto upgrade verify success")

	return nil
}

// In ota mode, just need to set the latest upgrade manifest version
func (ctrl *Controller) OTAUpgrade() error {
	if err := ctrl.setLatestManifestHash(); err != nil {
		return err
	}
	klog.Info("OTA upgrade set latest manifest hash success")
	return nil
}

// checkStaticPodExist check if the target static pod exist in cluster
func (ctrl *Controller) checkStaticPodExist() error {
	if errs := validation.IsDNS1123Subdomain(ctrl.name); len(errs) > 0 {
		return fmt.Errorf("pod name %s is invalid: %v", ctrl.name, errs)
	}
	_, err := ctrl.client.CoreV1().Pods(ctrl.namespace).Get(context.TODO(), ctrl.name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	return nil
}

// checkManifestFileExist check if the specified files exist
func (ctrl *Controller) checkManifestFileExist() error {
	check := []string{ctrl.manifestPath, ctrl.configMapDataPath}
	for _, c := range check {
		_, err := os.Stat(c)
		if os.IsNotExist(err) {
			return fmt.Errorf("manifest %s does not exist", c)
		}
	}

	return nil
}

// prepareManifest move the latest manifest to DefaultUpgradeDir and set `.upgrade` suffix
// TODO: In kubernetes when mount configmap file to the sub path of hostpath mount, it will not be persistent
// TODO: Init configmap(latest manifest) to a default place and move it to `DefaultUpgradeDir` to save it persistent
func (ctrl *Controller) prepareManifest() error {
	// Make sure upgrade dir exist
	if _, err := os.Stat(filepath.Join(DefaultManifestPath, DefaultUpgradeDir)); os.IsNotExist(err) {
		if err = os.Mkdir(filepath.Join(DefaultManifestPath, DefaultUpgradeDir), 0755); err != nil {
			return err
		}
	}

	return CopyFile(ctrl.configMapDataPath, ctrl.upgradeManifestPath)
}

// backUpManifest backup the old manifest in order to roll back when errors occur
func (ctrl *Controller) backupManifest() error {
	return CopyFile(ctrl.manifestPath, ctrl.bakManifestPath)
}

// replaceManifest replace old manifest with the latest one, it achieves static pod upgrade
func (ctrl *Controller) replaceManifest() error {
	return CopyFile(ctrl.upgradeManifestPath, ctrl.manifestPath)
}

// verify make sure the latest static pod is running
// return false when the latest static pod failed or check status time out
func (ctrl *Controller) verify() (bool, error) {
	return WaitForPodRunning(ctrl.client, ctrl.name, ctrl.namespace, ctrl.hash, defaultStaticPodRunningCheckTimeout)
}

// setLatestManifestHash set the latest manifest hash value to target static pod annotation
// TODO: In ota mode, it's hard for controller to check whether the latest manifest file has been issued to nodes
// TODO: Use annotation `openyurt.io/ota-manifest-version` to indicate the version of manifest issued to nodes
func (ctrl *Controller) setLatestManifestHash() error {
	pod, err := ctrl.client.CoreV1().Pods(ctrl.namespace).Get(context.TODO(), ctrl.name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	metav1.SetMetaDataAnnotation(&pod.ObjectMeta, OTALatestManifestAnnotation, ctrl.hash)
	_, err = ctrl.client.CoreV1().Pods(ctrl.namespace).Update(context.TODO(), pod, metav1.UpdateOptions{})
	return err
}

func GetClient() (kubernetes.Interface, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	c, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return c, nil
}
