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

	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

var (
	DefaultManifestPath = "/etc/kubernetes/manifests"
)

const (
	// TODO: use upgrade-static-pod
	ConfigMapPrefix   = "yurt-cm-"
	DefaultUpgradeDir = "openyurtio-upgrade"
	UpgradeSuffix     = ".upgrade"
	YamlSuffix        = ".yaml"
	BackupSuffix      = ".bak"
)

type StaticPodUpgrader struct {
	kubernetes.Interface
	types.NamespacedName
}

func (s *StaticPodUpgrader) Apply() error {
	manifestPath := filepath.Join(DefaultManifestPath, WithYamlSuffix(s.Name))
	upgradeDir := filepath.Join(DefaultManifestPath, DefaultUpgradeDir)
	upgradeManifestPath := filepath.Join(upgradeDir, WithUpgradeSuffix(s.Name))
	backupManifestPath := filepath.Join(upgradeDir, WithBackupSuffix(s.Name))

	// 1. Make sure upgrade dir exist
	if _, err := os.Stat(upgradeDir); os.IsNotExist(err) {
		if err = os.Mkdir(upgradeDir, 0755); err != nil {
			return err
		}
		klog.V(5).Infof("Create upgrade dir %v", upgradeDir)
	}

	// 2. Check upgrade manifest existence.
	ok, err := util.FileExists(upgradeManifestPath)
	if err != nil {
		return err
	}

	// 3. Upgrade manifest does not exist, get from api-server.
	if !ok {
		cm, err := s.CoreV1().ConfigMaps(metav1.NamespaceSystem).Get(context.TODO(), WithConfigMapPrefix(s.Namespace+"-"+s.Name), metav1.GetOptions{})
		if err != nil {
			return err
		}

		data := cm.Data[s.Name]
		if len(data) == 0 {
			return fmt.Errorf("empty manifest in configmap %v", WithConfigMapPrefix(s.Namespace+"-"+s.Name))
		}

		if err := genUpgradeManifest(upgradeManifestPath, data); err != nil {
			return err
		}
		klog.V(5).Info("Generate upgrade manifest")
	}

	// 4. Backup manifest
	if err := CopyFile(manifestPath, backupManifestPath); err != nil {
		return err
	}
	klog.V(5).Info("Backup upgrade manifest success")

	// 5. Execute upgrade
	if err := CopyFile(upgradeManifestPath, manifestPath); err != nil {
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

// TODO: use upgrade-static-pod
func WithYamlSuffix(path string) string {
	return path + YamlSuffix
}

// TODO: use upgrade-static-pod
func WithBackupSuffix(path string) string {
	return path + BackupSuffix
}

// TODO: use upgrade-static-pod
func WithUpgradeSuffix(path string) string {
	return path + UpgradeSuffix
}

// TODO: use upgrade-static-pod
func CopyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0666)
	if err != nil {
		return err
	}

	defer out.Close()

	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}
	return nil
}

// TODO: use upgrade-static-pod
func WithConfigMapPrefix(str string) string {
	return ConfigMapPrefix + str
}
