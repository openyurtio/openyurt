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

package static

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	spctrlutil "github.com/openyurtio/openyurt/pkg/controller/staticpod/util"
	upgradeutil "github.com/openyurtio/openyurt/pkg/node-servant/static-pod-upgrade/util"
)

var (
	DefaultManifestPath = "/etc/kubernetes/manifests"
	DefaultUpgradePath  = "/tmp/manifests"
)

type StaticPodUpgrader struct {
	kubernetes.Interface
	types.NamespacedName
}

func (s *StaticPodUpgrader) Apply() error {
	manifestPath := filepath.Join(DefaultManifestPath, upgradeutil.WithYamlSuffix(s.Name))
	upgradeManifestPath := filepath.Join(DefaultUpgradePath, upgradeutil.WithUpgradeSuffix(s.Name))
	backupManifestPath := filepath.Join(DefaultUpgradePath, upgradeutil.WithBackupSuffix(s.Name))

	// 1. Make sure upgrade dir exist
	if _, err := os.Stat(DefaultUpgradePath); os.IsNotExist(err) {
		if err = os.Mkdir(DefaultUpgradePath, 0755); err != nil {
			return err
		}
		klog.V(5).Infof("Create upgrade dir %v", DefaultUpgradePath)
	}

	// 2. Get upgrade manifest from api-serv
	cm, err := s.CoreV1().ConfigMaps(s.Namespace).Get(context.TODO(),
		spctrlutil.WithConfigMapPrefix(spctrlutil.Hyphen(s.Namespace, s.Name)), metav1.GetOptions{})
	if err != nil {
		return err
	}
	data := cm.Data[s.Name]
	if len(data) == 0 {
		return fmt.Errorf("empty manifest in configmap %v",
			spctrlutil.WithConfigMapPrefix(spctrlutil.Hyphen(s.Namespace, s.Name)))
	}
	if err := genUpgradeManifest(upgradeManifestPath, data); err != nil {
		return err
	}
	klog.V(5).Info("Generate upgrade manifest")

	// 3. Backup manifest
	if err := upgradeutil.CopyFile(manifestPath, backupManifestPath); err != nil {
		return err
	}
	klog.V(5).Info("Backup upgrade manifest success")

	// 4. Execute upgrade
	if err := upgradeutil.CopyFile(upgradeManifestPath, manifestPath); err != nil {
		return err
	}
	klog.V(5).Info("Replace upgrade manifest success")

	return nil
}

func genUpgradeManifest(path, data string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.WriteString(file, data)
	if err != nil {
		return err
	}
	return nil
}
