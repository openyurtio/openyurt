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
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

func Test(t *testing.T) {
	// Temporarily modify the manifest path in order to test
	DefaultManifestPath = t.TempDir()
	DefaultConfigmapPath = t.TempDir()
	_, _ = os.Create(filepath.Join(DefaultManifestPath, WithYamlSuffix("nginxManifest")))
	_, _ = os.Create(filepath.Join(DefaultConfigmapPath, "nginxManifest"))

	viper.Set("name", "nginx-node")
	viper.Set("namespace", "default")
	viper.Set("manifest", "nginxManifest")
	viper.Set("hash", "789c7f9f47")

	runningStaticPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx-node",
			Namespace: "default",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}

	modes := []string{"ota", "auto"}

	for _, mode := range modes {
		/*
			1. Prepare the test environment
		*/
		if mode == "auto" {
			runningStaticPod.Annotations = map[string]string{
				StaticPodHashAnnotation: "789c7f9f47",
			}
		}
		c := fake.NewSimpleClientset(runningStaticPod)
		// Add watch event for verify
		watcher := watch.NewFake()
		c.PrependWatchReactor("pods", k8stesting.DefaultWatchReactor(watcher, nil))
		go func() {
			watcher.Add(runningStaticPod)
		}()

		viper.Set("mode", mode)

		/*
			2. Test
		*/
		ctrl, err := New(c)
		if err != nil {
			t.Errorf("Fail to get upgrade controller, %v", err)
		}

		if err := ctrl.Upgrade(); err != nil {
			t.Errorf("Fail to upgrade, %v", err)
		}

		/*
			3. Verify OTA upgrade mode
		*/
		if mode == "ota" {
			ok, err := util.FileExists(ctrl.upgradeManifestPath)
			if err != nil {
				t.Errorf("Fail to check manifest existence for ota upgrade, %v", err)
			}
			if !ok {
				t.Errorf("Manifest for ota upgrade does not exist")
			}

			pod, err := ctrl.client.CoreV1().Pods(runningStaticPod.Namespace).
				Get(context.TODO(), runningStaticPod.Name, metav1.GetOptions{})
			if err != nil {
				t.Errorf("Fail to get the running static pod, %v", err)
			}

			if pod.Annotations[OTALatestManifestAnnotation] != "789c7f9f47" {
				t.Errorf("Fail to verify hash annotation for ota upgrade, %v", err)
			}
		}
		/*
			4. Verify Auto upgrade mode
		*/
		if mode == "auto" {
			checkFiles := []string{ctrl.upgradeManifestPath, ctrl.bakManifestPath}
			for _, file := range checkFiles {
				ok, err := util.FileExists(file)
				if err != nil {
					t.Errorf("Fail to check %s manifest existence for auto upgrade, %v", file, err)
				}
				if !ok {
					t.Errorf("Manifest %s for auto upgrade does not exist", file)
				}
			}
		}
	}
}
