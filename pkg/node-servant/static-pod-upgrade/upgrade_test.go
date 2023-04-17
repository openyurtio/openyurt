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
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	upgradeUtil "github.com/openyurtio/openyurt/pkg/node-servant/static-pod-upgrade/util"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

const (
	TestPodName   = "nginx"
	TestHashValue = "789c7f9f47"
	TestManifest  = "manifest"
)

func Test(t *testing.T) {
	// Temporarily modify the manifest path in order to test
	DefaultManifestPath = t.TempDir()
	DefaultConfigmapPath = t.TempDir()
	DefaultUpgradePath = t.TempDir()
	_, _ = os.Create(filepath.Join(DefaultManifestPath, upgradeUtil.WithYamlSuffix(TestManifest)))
	_, _ = os.Create(filepath.Join(DefaultConfigmapPath, TestManifest))

	runningStaticPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      TestPodName,
			Namespace: metav1.NamespaceDefault,
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}

	modes := []string{"auto"}

	for _, mode := range modes {
		/*
			1. Prepare the test environment
		*/
		c := fake.NewSimpleClientset(runningStaticPod)
		// Add watch event for verify
		watcher := watch.NewFake()
		c.PrependWatchReactor("pods", k8stesting.DefaultWatchReactor(watcher, nil))
		go func() {
			watcher.Add(runningStaticPod)
		}()

		/*
			2. Test
		*/
		o := &Options{
			name:      TestPodName,
			namespace: metav1.NamespaceDefault,
			manifest:  TestManifest,
			hash:      TestHashValue,
			mode:      mode,
			timeout:   time.Second,
		}
		ctrl, err := NewWithOptions(o)
		if err != nil {
			t.Errorf("Fail to get upgrade controller, %v", err)
		}

		if err := ctrl.Upgrade(); err != nil {
			if !strings.Contains(err.Error(), "timeout waiting for static pod") {
				t.Errorf("Fail to upgrade, %v", err)
			}
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
		}
	}
}
