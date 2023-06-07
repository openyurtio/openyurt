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

func newNode(name string) *corev1.Node {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	return node
}

func newNodes(nodeNames []string) []client.Object {
	var nodes []client.Object
	for _, n := range nodeNames {
		nodes = append(nodes, client.Object(newNode(n)))
	}
	return nodes
}

func newPod(podName string, nodeName string, namespace string, isStaticPod bool) *corev1.Pod {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      UpgradeWorkerPodPrefix + podName + "-" + rand.String(10),
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

func newStaticPod() *appsv1alpha1.YurtStaticSet {
	return &appsv1alpha1.YurtStaticSet{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fakeStaticPodName,
			Namespace: metav1.NamespaceDefault},
		Spec:   appsv1alpha1.YurtStaticSetSpec{},
		Status: appsv1alpha1.YurtStaticSetStatus{},
	}
}

func Test_ConstructStaticPodsUpgradeInfoList(t *testing.T) {
	staticPods := newPods(fakeStaticPodNodes, metav1.NamespaceDefault, true)
	workerPods := newPods(fakeWorkerPodNodes, metav1.NamespaceDefault, false)
	nodes := newNodes(fakeStaticPodNodes)
	expect := map[string]*UpgradeInfo{
		"node1": {
			StaticPod:        staticPods[0].(*corev1.Pod),
			WorkerPod:        workerPods[0].(*corev1.Pod),
			WorkerPodRunning: true,
		},
		"node2": {
			StaticPod:        staticPods[1].(*corev1.Pod),
			WorkerPod:        workerPods[1].(*corev1.Pod),
			WorkerPodRunning: true,
		},
		"node3": {
			StaticPod: staticPods[2].(*corev1.Pod),
		},
		"node4": {
			StaticPod: staticPods[3].(*corev1.Pod),
		},
	}

	objects := append(staticPods, workerPods...)
	objects = append(objects, nodes...)
	c := fake.NewClientBuilder().WithObjects(objects...).Build()

	t.Run("test", func(t *testing.T) {
		spi, _ := New(c, newStaticPod(), UpgradeWorkerPodPrefix, "")

		if !reflect.DeepEqual(spi, expect) {
			t.Fatalf("Fail to test ConstructStaticPodsUpgradeInfoList, got %v, want %v", spi, expect)
		}
	})
}

func TestNodes(t *testing.T) {
	spi := map[string]*UpgradeInfo{
		"node1": {
			WorkerPodRunning: true,
			UpgradeNeeded:    true,
			NodeReady:        true,
		},
		"node2": {
			StaticPod:     &corev1.Pod{},
			UpgradeNeeded: true,
			NodeReady:     true,
		},
		"node3": {
			StaticPod:     &corev1.Pod{},
			UpgradeNeeded: true,
			NodeReady:     true,
		},
		"node4": {
			NodeReady: true,
		},
		"node5": {
			WorkerPod: &corev1.Pod{},
		},
		"node6": {
			WorkerPod: &corev1.Pod{
				Spec: corev1.PodSpec{
					NodeName: "node6",
				},
			},
			WorkerPodDeleteNeeded: true,
		},
		"node7": {
			WorkerPod:            &corev1.Pod{},
			WorkerPodStatusPhase: corev1.PodPending,
		},
		"node8": {
			StaticPod:      &corev1.Pod{},
			StaticPodReady: true,
		},
		"node9": {
			StaticPod:     &corev1.Pod{},
			UpgradeNeeded: false,
		},
	}

	expectReadyUpgradeWaitingNodes := map[string]struct{}{"node2": {}, "node3": {}}
	expectUpgradeNeededNodes := map[string]struct{}{"node1": {}, "node2": {}, "node3": {}}
	expectUpgradedNodes := map[string]struct{}{"node4": {}, "node5": {}, "node6": {}, "node7": {}, "node8": {}, "node9": {}}
	expectDeletePods := []string{"node6"}
	expectStaticReadyPods := []string{"node8"}

	t.Run("TestReadyUpgradeWaitingNodes", func(t *testing.T) {
		if got := ReadyUpgradeWaitingNodes(spi); !hasCommonElement(got, expectReadyUpgradeWaitingNodes) {
			t.Fatalf("ReadyUpgradeWaitingNodes = %v, want %v", got, expectReadyUpgradeWaitingNodes)
		}
	})

	t.Run("ListOutUpgradeNeededNodesAndUpgradedNodes", func(t *testing.T) {
		nGot, got := ListOutUpgradeNeededNodesAndUpgradedNodes(spi)
		if !hasCommonElement(nGot, expectUpgradeNeededNodes) {
			t.Fatalf("UpgradeNeededNodes got %v, want %v", nGot, expectUpgradeNeededNodes)
		}
		if !hasCommonElement(got, expectUpgradedNodes) {
			t.Fatalf("UpgradedNodes got %v, want %v", got, expectUpgradedNodes)
		}
	})

	t.Run("TestCalculateOperateInfoFromUpgradeInfoMap", func(t *testing.T) {
		upgradedNumber, readyNumber, allSucceeded, deletePods := CalculateOperateInfoFromUpgradeInfoMap(spi)
		if upgradedNumber != 2 {
			t.Fatalf("UpgradedNumber got %v, want 0", upgradedNumber)
		}
		if readyNumber != int32(len(expectStaticReadyPods)) {
			t.Fatalf("ReadyNumber got %v, want 0", readyNumber)
		}
		if allSucceeded {
			t.Fatalf("AllSucceeded got %v, want false", allSucceeded)
		}
		if !hasCommonElementForPod(expectDeletePods, deletePods) {
			t.Fatalf("DeletePods got %v, want empty", deletePods)
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

func hasCommonElementForPod(a []string, b []*corev1.Pod) bool {
	if len(a) != len(b) {
		return false
	}

	for i, name := range a {
		if name != b[i].Spec.NodeName {
			return false
		}
	}
	return true
}
