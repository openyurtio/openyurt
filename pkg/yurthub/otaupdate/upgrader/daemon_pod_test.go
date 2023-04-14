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
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/openyurtio/openyurt/pkg/yurthub/otaupdate/util"
)

func TestDaemonPodUpgrader_Apply(t *testing.T) {
	clientset := fake.NewSimpleClientset(util.NewPodWithCondition("nginx", "DaemonSet", corev1.ConditionTrue))
	upgrader := DaemonPodUpgrader{
		Interface:      clientset,
		NamespacedName: types.NamespacedName{Name: "nginx", Namespace: "default"},
	}
	t.Run("TestDaemonPodUpgrader_Apply", func(t *testing.T) {
		if err := upgrader.Apply(); err != nil {
			t.Fatalf("Fail to ota upgrade Daemonset pod")
		}
	})

	upgrader.Interface = fake.NewSimpleClientset()
	t.Run("TestDaemonPodUpgrader_ApplyErr", func(t *testing.T) {
		if err := upgrader.Apply(); err == nil {
			t.Fatalf("Should fail to ota upgrade Daemonset pod")
		}
	})

}
