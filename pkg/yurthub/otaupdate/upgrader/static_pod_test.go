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
	"os"
	"path/filepath"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/kubernetes/fake"

	upgrade "github.com/openyurtio/openyurt/pkg/node-servant/static-pod-upgrade"
	upgradeutil "github.com/openyurtio/openyurt/pkg/node-servant/static-pod-upgrade/util"
	"github.com/openyurtio/openyurt/pkg/yurthub/otaupdate/util"
	spctrlutil "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/yurtstaticset/util"
)

func TestStaticPodUpgrader_ApplyManifestNotExist(t *testing.T) {
	// Temporarily modify the manifest path in order to test
	upgrade.DefaultUpgradePath = t.TempDir()
	upgrade.DefaultManifestPath = t.TempDir()
	DefaultUpgradePath = upgrade.DefaultUpgradePath
	_, _ = os.Create(filepath.Join(upgrade.DefaultManifestPath, upgradeutil.WithYamlSuffix("nginx")))

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: metav1.NamespaceDefault,
			Name:      spctrlutil.WithConfigMapPrefix("nginx"),
		},
		Data: map[string]string{
			"nginx": `
apiVersion: v1
kind: Pod
metadata:
  name: nginx
spec:
  containers:
    - name: web
      image: nginx:1.19.2
`,
		},
	}

	clientset := fake.NewSimpleClientset(util.NewPodWithCondition("nginx-node", "Node", corev1.ConditionTrue), cm)
	upgrader := StaticPodUpgrader{
		Interface:      clientset,
		NamespacedName: types.NamespacedName{Namespace: metav1.NamespaceDefault, Name: "nginx-node"},
		StaticName:     "nginx",
	}

	t.Run("TestStaticPodUpgrader_ApplyManifestNotExist", func(t *testing.T) {
		if err := upgrader.Apply(); err != nil {
			t.Fatalf("Fail to ota upgrade static pod, %v", err)
		}
	})
}

func Test_genUpgradeManifest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, rand.String(10))
	data := "test data"

	if err := genUpgradeManifest(path, data); err != nil {
		t.Fatalf("Fail to genUpgradeManifest, %v", err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("Fail to gen file")

	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Fail to read file content")
	}
	if string(content) != data {
		t.Fatalf("Fail to match file content")
	}

}
