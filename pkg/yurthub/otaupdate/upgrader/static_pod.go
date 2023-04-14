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

package upgrader

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	spctrlutil "github.com/openyurtio/openyurt/pkg/controller/staticpod/util"
	upgrade "github.com/openyurtio/openyurt/pkg/node-servant/static-pod-upgrade"
	upgradeutil "github.com/openyurtio/openyurt/pkg/node-servant/static-pod-upgrade/util"
)

const OTA = "ota"

var (
	DefaultUpgradePath = "/tmp/manifests"
)

type StaticPodUpgrader struct {
	kubernetes.Interface
	types.NamespacedName
}

func (s *StaticPodUpgrader) Apply() error {
	upgradeManifestPath := filepath.Join(DefaultUpgradePath, upgradeutil.WithUpgradeSuffix(s.Name))
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

	ctrl := upgrade.New(s.Name, s.Namespace, s.Name, OTA)
	return ctrl.Upgrade()
}

func PreCheck(name, namespace string, c kubernetes.Interface) (bool, error) {
	_, err := c.CoreV1().ConfigMaps(namespace).Get(context.TODO(),
		spctrlutil.WithConfigMapPrefix(spctrlutil.Hyphen(namespace, name)), metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
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
