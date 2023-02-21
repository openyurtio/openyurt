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

package upgradeinfo

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	appsv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
)

const (
	UpgradeWorkerPodPrefix = "static-pod-upgrade-worker-"
)

var (
	fakeStaticPodNodes = []string{"node1", "node2", "node3", "node4"}
	fakeWorkerPodNodes = []string{"node1", "node2"}
	fakeStaticPodName  = "nginx"
)

func newPod(podName string, nodeName string, namespace string, isStaticPod bool) *corev1.Pod {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      UpgradeWorkerPodPrefix + rand.String(10),
			Namespace: namespace,
		},
		Spec: corev1.PodSpec{NodeName: nodeName},
	}

	if isStaticPod {
		pod.Name = podName + "-" + nodeName
		pod.ObjectMeta.OwnerReferences = []metav1.OwnerReference{{Kind: "Node"}}
	}

	return pod
}

func newPods(nodes []string, namespace string, isStaticPod bool) []client.Object {
	var pods []client.Object
	for _, n := range nodes {
		pods = append(pods, client.Object(newPod(fakeStaticPodName, n, namespace, isStaticPod)))
	}
	return pods
}

func newStaticPod() *appsv1alpha1.StaticPod {
	return &appsv1alpha1.StaticPod{
		TypeMeta:   metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{},
		Spec: appsv1alpha1.StaticPodSpec{
			StaticPodName: fakeStaticPodName,
		},
		Status: appsv1alpha1.StaticPodStatus{},
	}
}

func Test_ConstructStaticPodsUpgradeInfoList(t *testing.T) {
	staticPods := newPods(fakeStaticPodNodes, metav1.NamespaceDefault, true)
	workerPods := newPods(fakeWorkerPodNodes, metav1.NamespaceSystem, false)
	expect := map[string]*UpgradeInfo{
		"node1": {
			StaticPod: staticPods[0].(*corev1.Pod),
			WorkerPod: workerPods[0].(*corev1.Pod),
		},
		"node2": {
			StaticPod: staticPods[1].(*corev1.Pod),

			WorkerPod: workerPods[1].(*corev1.Pod),
		},
		"node3": {
			StaticPod: staticPods[2].(*corev1.Pod),
		},
		"node4": {
			StaticPod: staticPods[3].(*corev1.Pod),
		},
	}

	pods := append(staticPods, workerPods...)
	c := fake.NewClientBuilder().WithObjects(pods...).Build()

	t.Run("test", func(t *testing.T) {
		spi, _ := New(c, newStaticPod(), UpgradeWorkerPodPrefix)

		if !reflect.DeepEqual(spi, expect) {
			t.Fatalf("Fail to test ConstructStaticPodsUpgradeInfoList, got %v, want %v", spi, expect)
		}
	})
}

func TestNodes(t *testing.T) {
	tHash := "tHash"
	fHash := "fHash"
	spi := map[string]*UpgradeInfo{
		"node1": {
			WorkerPodRunning: true,
			UpgradeNeeded:    true,
			Ready:            true,
		},
		"node2": {
			StaticPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						OTALatestManifestAnnotation: tHash,
					},
				},
			},
			UpgradeNeeded: true,
			Ready:         true,
		},
		"node3": {
			StaticPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						OTALatestManifestAnnotation: fHash,
					},
				},
			},
			UpgradeNeeded: true,
			Ready:         true,
		},
		"node4": {
			Ready: true,
		},
		"node5": {},
	}

	expectReadyUpgradeWaitingNodes := map[string]struct{}{"node2": {}, "node3": {}}
	expectOTAReadyUpgradeWaitingNodes := map[string]struct{}{"node3": {}}
	expectReadyNodes := map[string]struct{}{"node1": {}, "node2": {}, "node3": {}, "node4": {}}
	expectUpgradeNeededNodes := map[string]struct{}{"node1": {}, "node2": {}, "node3": {}}
	expectUpgradedNodes := map[string]struct{}{"node4": {}, "node5": {}}

	t.Run("TestReadyUpgradeWaitingNodes", func(t *testing.T) {
		if got := ReadyUpgradeWaitingNodes(spi); !hasCommonElement(got, expectReadyUpgradeWaitingNodes) {
			t.Fatalf("ReadyUpgradeWaitingNodes = %v, want %v", got, expectReadyUpgradeWaitingNodes)
		}
	})

	t.Run("OTAReadyUpgradeWaitingNodes", func(t *testing.T) {
		if got := OTAReadyUpgradeWaitingNodes(spi, tHash); !hasCommonElement(got, expectOTAReadyUpgradeWaitingNodes) {
			t.Fatalf("OTAReadyUpgradeWaitingNodes got %v, want %v", got, expectOTAReadyUpgradeWaitingNodes)
		}
	})

	t.Run("ReadyNodes", func(t *testing.T) {
		if got := ReadyNodes(spi); !hasCommonElement(got, expectReadyNodes) {
			t.Fatalf("ReadyNodes got %v, want %v", got, expectReadyNodes)
		}
	})

	t.Run("UpgradeNeededNodes", func(t *testing.T) {
		if got := UpgradeNeededNodes(spi); !hasCommonElement(got, expectUpgradeNeededNodes) {
			t.Fatalf("UpgradeNeededNodes got %v, want %v", got, expectUpgradeNeededNodes)
		}
	})

	t.Run("UpgradedNodes", func(t *testing.T) {
		if got := UpgradedNodes(spi); !hasCommonElement(got, expectUpgradedNodes) {
			t.Fatalf("UpgradedNodes got %v, want %v", got, expectUpgradedNodes)
		}
	})
}

func hasCommonElement(a []string, b map[string]struct{}) bool {
	if len(a) != len(b) {
		return false
	}

	for _, i := range a {
		if b[i] != struct{}{} {
			return false
		}
	}
	return true
}
