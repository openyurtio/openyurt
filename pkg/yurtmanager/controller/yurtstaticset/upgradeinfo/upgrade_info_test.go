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
	utilpointer "k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	appsv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
	podutil "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/pod"
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
			Annotations: map[string]string{
				podutil.ConfigSourceAnnotationKey: "true",
			},
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
			Annotations: map[string]string{
				podutil.ConfigSourceAnnotationKey: "true",
			},
			Name:      fakeStaticPodName,
			Namespace: metav1.NamespaceDefault,
		},
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

var (
	hostPathDirectoryOrCreate = corev1.HostPathDirectoryOrCreate
	testYss                   = &appsv1alpha1.YurtStaticSet{
		Spec: appsv1alpha1.YurtStaticSetSpec{
			StaticPodManifest: "yurthub",
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"k8s-app": "yurthub",
					},
					Name:      "yurthub",
					Namespace: "kube-system",
				},
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						{
							Name: "hub-dir",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/var/lib/yurthub",
									Type: &hostPathDirectoryOrCreate,
								},
							},
						},
					},
					HostNetwork:       true,
					PriorityClassName: "system-node-critical",
					Priority:          utilpointer.Int32Ptr(2000001000),
				},
			},
		},
	}
)

func preparePods() []*corev1.Pod {
	podList := make([]*corev1.Pod, 0)
	testPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"k8s-app": "yurthub",
			},
			Name:            "yurt-hub-host-475424",
			Namespace:       "kube-system",
			ResourceVersion: "111112",
		},
		Spec: corev1.PodSpec{
			Volumes: []corev1.Volume{
				{
					Name: "hub-dir",
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: "/var/lib/yurthub",
							Type: &hostPathDirectoryOrCreate,
						},
					},
				},
			},
			HostNetwork:       true,
			PriorityClassName: "system-node-critical",
			Priority:          utilpointer.Int32Ptr(2000001000),
			NodeName:          "aaa",
			SchedulerName:     "default-scheduler",
			RestartPolicy:     "Always",
		},
	}
	podList = append(podList, testPod)

	testPod2 := testPod.DeepCopy()
	testPod2.Spec.PriorityClassName = "aaaa"
	podList = append(podList, testPod2)

	testPod3 := testPod.DeepCopy()
	testPod3.Spec = corev1.PodSpec{
		Volumes: []corev1.Volume{
			{
				Name: "hub-dir",
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: "/var/lib/yurthub",
						Type: &hostPathDirectoryOrCreate,
					},
				},
			},
		},
		PriorityClassName: "system-node-critical",
		Priority:          utilpointer.Int32Ptr(2000001000),
		NodeName:          "aaa",
		SchedulerName:     "default-scheduler",
		RestartPolicy:     "Always",
	}
	podList = append(podList, testPod3)

	testPod4 := testPod.DeepCopy()
	testPod4.Namespace = "fffff"
	podList = append(podList, testPod4)

	testPod5 := testPod.DeepCopy()
	testPod5.ObjectMeta = metav1.ObjectMeta{
		Labels: map[string]string{
			"k8s-app": "yurthub",
		},
		Name:            "yurt-hub-host-475424",
		ResourceVersion: "111112",
	}
	podList = append(podList, testPod5)

	return podList
}

func TestMatch(t *testing.T) {
	pods := preparePods()
	tests := []struct {
		name     string
		instance *appsv1alpha1.YurtStaticSet
		pod      *corev1.Pod
		want     bool
	}{
		{
			name:     "test1",
			instance: testYss,
			pod:      pods[0],
			want:     true,
		},
		{
			name:     "test2",
			instance: testYss,
			pod:      pods[1],
			want:     false,
		},
		{
			name:     "test3",
			instance: testYss,
			pod:      pods[2],
			want:     false,
		},
		{
			name:     "test4",
			instance: testYss,
			pod:      pods[3],
			want:     false,
		},
		{
			name:     "test5",
			instance: testYss,
			pod:      pods[4],
			want:     false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := match(tt.instance, tt.pod); got != tt.want {
				t.Errorf("match() = %v, want %v", got, tt.want)
			}
		})
	}
}
