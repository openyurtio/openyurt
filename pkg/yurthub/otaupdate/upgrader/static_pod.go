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

	upgrade "github.com/openyurtio/openyurt/pkg/node-servant/static-pod-upgrade"
	upgradeutil "github.com/openyurtio/openyurt/pkg/node-servant/static-pod-upgrade/util"
	"github.com/openyurtio/openyurt/pkg/yurthub/otaupdate/util"
	spctrlutil "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/yurtstaticset/util"
)

const OTA = "OTA"

var (
	DefaultUpgradePath = "/tmp/manifests"
)

type StaticPodUpgrader struct {
	kubernetes.Interface
	types.NamespacedName
	// Name format of static pod is `staticName-nodeName`
	StaticName string
}

func (s *StaticPodUpgrader) Apply() error {
	cm, err := s.CoreV1().ConfigMaps(s.Namespace).Get(context.TODO(),
		spctrlutil.WithConfigMapPrefix(s.StaticName), metav1.GetOptions{})
	if err != nil {
		return err
	}
	var manifest, data string
	for k, v := range cm.Data {
		manifest = k
		data = v
	}
	if len(data) == 0 {
		return fmt.Errorf("empty manifest in configmap %v", spctrlutil.WithConfigMapPrefix(s.StaticName))
	}

	// Make sure upgrade dir exist
	if _, err := os.Stat(DefaultUpgradePath); os.IsNotExist(err) {
		if err = os.Mkdir(DefaultUpgradePath, 0755); err != nil {
			return err
		}
	}

	upgradeManifestPath := filepath.Join(DefaultUpgradePath, upgradeutil.WithUpgradeSuffix(manifest))
	if err := genUpgradeManifest(upgradeManifestPath, data); err != nil {
		return err
	}
	klog.V(5).Info("Generate upgrade manifest")

	ctrl := upgrade.New(s.Name, s.Namespace, manifest, OTA)
	return ctrl.Upgrade()
}

func PreCheck(name, nodename, namespace string, c kubernetes.Interface) (bool, string, error) {
	ok, staticName := util.RemoveNodeNameFromStaticPod(name, nodename)
	if !ok {
		return false, "", fmt.Errorf("pod name doesn't meet static pod's format")
	}
	_, err := c.CoreV1().ConfigMaps(namespace).Get(context.TODO(),
		spctrlutil.WithConfigMapPrefix(staticName), metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, "", nil
		}
		return false, "", err
	}
	return true, staticName, nil
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
