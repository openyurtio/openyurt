/*
Copyright 2017 The Kubernetes Authors.
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

package scheduler

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	flag "github.com/spf13/pflag"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	clienttesting "k8s.io/client-go/testing"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/testutil"
)

var timeForControllerToProgressForSanityCheck = 20 * time.Millisecond

func init() {
	klog.InitFlags(nil)
	flag.Set("v", "5")
	flag.Parse()
}
func getPodsAssignedToNode(ctx context.Context, c client.Client) GetPodsByNodeNameFunc {
	return func(nodeName string) ([]*v1.Pod, error) {
		var pods v1.PodList
		err := c.List(ctx, &pods, &client.ListOptions{})
		if err != nil {
			return []*v1.Pod{}, fmt.Errorf("failed to get Pods assigned to node %v", nodeName)
		}
		rPods := make([]*v1.Pod, 0)
		for i := range pods.Items {
			if pods.Items[i].Spec.NodeName == nodeName {
				rPods = append(rPods, &pods.Items[i])
			}
		}
		return rPods, nil
	}
}

func createNoExecuteTaint(index int) v1.Taint {
	now := metav1.Now()
	return v1.Taint{
		Key:       "testTaint" + fmt.Sprintf("%v", index),
		Value:     "test" + fmt.Sprintf("%v", index),
		Effect:    v1.TaintEffectNoExecute,
		TimeAdded: &now,
	}
}

func addToleration(pod *v1.Pod, index int, duration int64) *v1.Pod {
	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}
	if duration < 0 {
		pod.Spec.Tolerations = []v1.Toleration{{Key: "testTaint" + fmt.Sprintf("%v", index), Value: "test" + fmt.Sprintf("%v", index), Effect: v1.TaintEffectNoExecute}}

	} else {
		pod.Spec.Tolerations = []v1.Toleration{{Key: "testTaint" + fmt.Sprintf("%v", index), Value: "test" + fmt.Sprintf("%v", index), Effect: v1.TaintEffectNoExecute, TolerationSeconds: &duration}}
	}
	return pod
}

func addTaintsToNode(node *v1.Node, key, value string, indices []int) *v1.Node {
	taints := []v1.Taint{}
	for _, index := range indices {
		taints = append(taints, createNoExecuteTaint(index))
	}
	node.Spec.Taints = taints
	return node
}

type timestampedPod struct {
	names     []string
	timestamp time.Duration
}

type durationSlice []timestampedPod

func (a durationSlice) Len() int           { return len(a) }
func (a durationSlice) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a durationSlice) Less(i, j int) bool { return a[i].timestamp < a[j].timestamp }

func TestFilterNoExecuteTaints(t *testing.T) {
	taints := []v1.Taint{
		{
			Key:    "one",
			Value:  "one",
			Effect: v1.TaintEffectNoExecute,
		},
		{
			Key:    "two",
			Value:  "two",
			Effect: v1.TaintEffectNoSchedule,
		},
	}
	taints = getNoExecuteTaints(taints)
	if len(taints) != 1 || taints[0].Key != "one" {
		t.Errorf("Filtering doesn't work. Got %v", taints)
	}
}

func TestCreatePod(t *testing.T) {
	testCases := []struct {
		description                   string
		pod                           *v1.Pod
		taintedNodes                  map[string][]v1.Taint
		expectPatch                   bool
		expectDelete                  bool
		enablePodDisruptionConditions bool
	}{
		{
			description:  "not scheduled - ignore",
			pod:          NewPod("pod1", ""),
			taintedNodes: map[string][]v1.Taint{},
			expectDelete: false,
		},
		{
			description:  "scheduled on untainted Node",
			pod:          NewPod("pod1", "node1"),
			taintedNodes: map[string][]v1.Taint{},
			expectDelete: false,
		},
		{
			description: "schedule on tainted Node",
			pod:         NewPod("pod1", "node1"),
			taintedNodes: map[string][]v1.Taint{
				"node1": {createNoExecuteTaint(1)},
			},
			expectDelete: true,
		},
		//{
		//	description: "schedule on tainted Node; PodDisruptionConditions enabled",
		//	pod:         NewPod("pod1", "node1"),
		//	taintedNodes: map[string][]v1.Taint{
		//		"node1": {createNoExecuteTaint(1)},
		//	},
		//	expectPatch:                   true,
		//	expectDelete:                  true,
		//	enablePodDisruptionConditions: true,
		//},
		{
			description: "schedule on tainted Node with finite toleration",
			pod:         addToleration(NewPod("pod1", "node1"), 1, 100),
			taintedNodes: map[string][]v1.Taint{
				"node1": {createNoExecuteTaint(1)},
			},
			expectDelete: false,
		},
		{
			description: "schedule on tainted Node with infinite toleration",
			pod:         addToleration(NewPod("pod1", "node1"), 1, -1),
			taintedNodes: map[string][]v1.Taint{
				"node1": {createNoExecuteTaint(1)},
			},
			expectDelete: false,
		},
		{
			description: "schedule on tainted Node with infinite invalid toleration",
			pod:         addToleration(NewPod("pod1", "node1"), 2, -1),
			taintedNodes: map[string][]v1.Taint{
				"node1": {createNoExecuteTaint(1)},
			},
			expectDelete: true,
		},
	}

	for _, item := range testCases {
		t.Run(item.description, func(t *testing.T) {
			//defer featuregatetesting.SetFeatureGateDuringTest(t, feature.DefaultFeatureGate, features.PodDisruptionConditions, item.enablePodDisruptionConditions)()
			ctx, cancel := context.WithCancel(context.Background())
			//fakeClientset := fake.NewSimpleClientset(&v1.PodList{Items: []v1.Pod{*item.pod}})
			//controller, podIndexer, _ := setupNewNoExecuteTaintManager(ctx, fakeActionClient)
			//controller.recorder = testutil.NewFakeRecorder(0)
			scheme := runtime.NewScheme()
			clientgoscheme.AddToScheme(scheme)
			fakeClient := fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(item.pod).Build()
			fakeClientset := testutil.NewClientWrapper(fakeClient, scheme)
			controller := NewNoExecuteTaintManager(testutil.NewFakeRecorder(), fakeClientset, getPodsAssignedToNode(ctx, fakeClientset))
			go controller.Run(ctx)
			controller.taintedNodes = item.taintedNodes

			//podIndexer.Add(item.pod)
			controller.PodUpdated(nil, item.pod)

			verifyPodActions(t, item.description, fakeClientset, item.expectPatch, item.expectDelete)

			cancel()
		})
	}
}

func TestDeletePod(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	//fakeClientset := fake.NewSimpleClientset()
	//controller, _, _ := setupNewNoExecuteTaintManager(ctx, fakeClientset)
	//controller.recorder = testutil.NewFakeRecorder(0)
	scheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(scheme)
	fakeClient := fakeclient.NewClientBuilder().WithScheme(scheme).Build()
	fakeClientset := testutil.NewClientWrapper(fakeClient, scheme)
	controller := NewNoExecuteTaintManager(testutil.NewFakeRecorder(), fakeClientset, getPodsAssignedToNode(ctx, fakeClientset))
	go controller.Run(ctx)
	controller.taintedNodes = map[string][]v1.Taint{
		"node1": {createNoExecuteTaint(1)},
	}
	controller.PodUpdated(NewPod("pod1", "node1"), nil)
	// wait a bit to see if nothing will panic
	time.Sleep(timeForControllerToProgressForSanityCheck)
}

func TestUpdatePod(t *testing.T) {
	testCases := []struct {
		description                   string
		prevPod                       *v1.Pod
		awaitForScheduledEviction     bool
		newPod                        *v1.Pod
		taintedNodes                  map[string][]v1.Taint
		expectPatch                   bool
		expectDelete                  bool
		enablePodDisruptionConditions bool
	}{
		//{
		//	description: "scheduling onto tainted Node results in patch and delete when PodDisruptionConditions enabled",
		//	prevPod:     NewPod("pod1", ""),
		//	newPod:      NewPod("pod1", "node1"),
		//	taintedNodes: map[string][]v1.Taint{
		//		"node1": {createNoExecuteTaint(1)},
		//	},
		//	expectPatch:                   true,
		//	expectDelete:                  true,
		//	enablePodDisruptionConditions: true,
		//},
		{
			description: "scheduling onto tainted Node",
			prevPod:     NewPod("pod1", ""),
			newPod:      NewPod("pod1", "node1"),
			taintedNodes: map[string][]v1.Taint{
				"node1": {createNoExecuteTaint(1)},
			},
			expectDelete: true,
		},
		{
			description: "scheduling onto tainted Node with toleration",
			prevPod:     addToleration(NewPod("pod1", ""), 1, -1),
			newPod:      addToleration(NewPod("pod1", "node1"), 1, -1),
			taintedNodes: map[string][]v1.Taint{
				"node1": {createNoExecuteTaint(1)},
			},
			expectDelete: false,
		},
		{
			description:               "removing toleration",
			prevPod:                   addToleration(NewPod("pod1", "node1"), 1, 100),
			newPod:                    NewPod("pod1", "node1"),
			awaitForScheduledEviction: true,
			taintedNodes: map[string][]v1.Taint{
				"node1": {createNoExecuteTaint(1)},
			},
			expectDelete: true,
		},
		{
			description:               "lengthening toleration shouldn't work",
			prevPod:                   addToleration(NewPod("pod1", "node1"), 1, 1),
			newPod:                    addToleration(NewPod("pod1", "node1"), 1, 100),
			awaitForScheduledEviction: true,
			taintedNodes: map[string][]v1.Taint{
				"node1": {createNoExecuteTaint(1)},
			},
			expectDelete: true,
		},
	}

	for _, item := range testCases {
		t.Run(item.description, func(t *testing.T) {
			//defer featuregatetesting.SetFeatureGateDuringTest(t, feature.DefaultFeatureGate, features.PodDisruptionConditions, item.enablePodDisruptionConditions)()
			ctx, cancel := context.WithCancel(context.Background())
			//fakeClientset := fake.NewSimpleClientset(&v1.PodList{Items: []v1.Pod{*item.prevPod}})
			//controller, podIndexer, _ := setupNewNoExecuteTaintManager(context.TODO(), fakeClientset)
			//controller.recorder = testutil.NewFakeRecorder(0)
			scheme := runtime.NewScheme()
			clientgoscheme.AddToScheme(scheme)
			fakeClient := fakeclient.NewClientBuilder().WithScheme(scheme).WithLists(&v1.PodList{Items: []v1.Pod{*item.prevPod}}).Build()
			fakeClientset := testutil.NewClientWrapper(fakeClient, scheme)
			controller := NewNoExecuteTaintManager(testutil.NewFakeRecorder(), fakeClientset, getPodsAssignedToNode(ctx, fakeClientset))
			controller.taintedNodes = item.taintedNodes
			go controller.Run(ctx)

			//podIndexer.Add(item.prevPod)
			controller.PodUpdated(nil, item.prevPod)

			if item.awaitForScheduledEviction {
				nsName := types.NamespacedName{Namespace: item.prevPod.Namespace, Name: item.prevPod.Name}
				err := wait.PollUntilContextTimeout(ctx, time.Millisecond*10, time.Second, true, func(ctx context.Context) (bool, error) {
					scheduledEviction := controller.taintEvictionQueue.GetWorkerUnsafe(nsName.String())
					return scheduledEviction != nil, nil
				})
				if err != nil {
					t.Fatalf("Failed to await for scheduled eviction: %q", err)
				}
			}

			//podIndexer.Update(item.newPod)
			fakeClientset.Update(ctx, item.newPod, &client.UpdateOptions{})
			controller.PodUpdated(item.prevPod, item.newPod)

			verifyPodActions(t, item.description, fakeClientset, item.expectPatch, item.expectDelete)
			cancel()
		})
	}
}

func TestCreateNode(t *testing.T) {
	testCases := []struct {
		description  string
		pods         []v1.Pod
		node         *v1.Node
		expectPatch  bool
		expectDelete bool
	}{
		{
			description: "Creating Node matching already assigned Pod",
			pods: []v1.Pod{
				*NewPod("pod1", "node1"),
			},
			node:         NewNode("node1"),
			expectPatch:  false,
			expectDelete: false,
		},
		{
			description: "Creating tainted Node matching already assigned Pod",
			pods: []v1.Pod{
				*NewPod("pod1", "node1"),
			},
			node: addTaintsToNode(NewNode("node1"), "testTaint1", "taint1", []int{1}),
			//expectPatch:  true,
			expectDelete: true,
		},
		{
			description: "Creating tainted Node matching already assigned tolerating Pod",
			pods: []v1.Pod{
				*addToleration(NewPod("pod1", "node1"), 1, -1),
			},
			node:         addTaintsToNode(NewNode("node1"), "testTaint1", "taint1", []int{1}),
			expectPatch:  false,
			expectDelete: false,
		},
	}

	for _, item := range testCases {
		ctx, cancel := context.WithCancel(context.Background())
		//fakeClientset := fake.NewSimpleClientset(&v1.PodList{Items: item.pods})
		//controller, _, nodeIndexer := setupNewNoExecuteTaintManager(ctx, fakeClientset)
		//nodeIndexer.Add(item.node)
		//controller.recorder = testutil.NewFakeRecorder(0)
		scheme := runtime.NewScheme()
		clientgoscheme.AddToScheme(scheme)
		fakeClient := fakeclient.NewClientBuilder().WithScheme(scheme).WithLists(&v1.PodList{Items: item.pods}).WithObjects(item.node).Build()
		fakeClientset := testutil.NewClientWrapper(fakeClient, scheme)
		controller := NewNoExecuteTaintManager(testutil.NewFakeRecorder(), fakeClientset, getPodsAssignedToNode(ctx, fakeClientset))
		go controller.Run(ctx)
		controller.NodeUpdated(nil, item.node)

		verifyPodActions(t, item.description, fakeClientset, item.expectPatch, item.expectDelete)

		cancel()
	}
}

func TestDeleteNode(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	//fakeClientset := fake.NewSimpleClientset()
	//controller, _, _ := setupNewNoExecuteTaintManager(ctx, fakeClientset)
	//controller.recorder = testutil.NewFakeRecorder(0)
	scheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(scheme)
	fakeClient := fakeclient.NewClientBuilder().WithScheme(scheme).Build()
	fakeClientset := testutil.NewClientWrapper(fakeClient, scheme)
	controller := NewNoExecuteTaintManager(testutil.NewFakeRecorder(), fakeClientset, getPodsAssignedToNode(ctx, fakeClientset))
	controller.taintedNodes = map[string][]v1.Taint{
		"node1": {createNoExecuteTaint(1)},
	}
	go controller.Run(ctx)
	controller.NodeUpdated(NewNode("node1"), nil)

	// await until controller.taintedNodes is empty
	err := wait.PollUntilContextTimeout(ctx, 10*time.Millisecond, time.Second, true, func(ctx context.Context) (bool, error) {
		controller.taintedNodesLock.Lock()
		defer controller.taintedNodesLock.Unlock()
		_, ok := controller.taintedNodes["node1"]
		return !ok, nil
	})
	if err != nil {
		t.Errorf("Failed to await for processing node deleted: %q", err)
	}
	cancel()
}

func TestUpdateNode(t *testing.T) {
	testCases := []struct {
		description                   string
		pods                          []v1.Pod
		oldNode                       *v1.Node
		newNode                       *v1.Node
		expectPatch                   bool
		expectDelete                  bool
		additionalSleep               time.Duration
		enablePodDisruptionConditions bool
	}{
		//{
		//	description: "Added taint, expect node patched and deleted when PodDisruptionConditions is enabled",
		//	pods: []v1.Pod{
		//		*NewPod("pod1", "node1"),
		//	},
		//	oldNode:                       NewNode("node1"),
		//	newNode:                       addTaintsToNode(NewNode("node1"), "testTaint1", "taint1", []int{1}),
		//	expectPatch:                   true,
		//	expectDelete:                  true,
		//	enablePodDisruptionConditions: true,
		//},
		{
			description: "Added taint",
			pods: []v1.Pod{
				*NewPod("pod1", "node1"),
			},
			oldNode:      NewNode("node1"),
			newNode:      addTaintsToNode(NewNode("node1"), "testTaint1", "taint1", []int{1}),
			expectDelete: true,
		},
		{
			description: "Added tolerated taint",
			pods: []v1.Pod{
				*addToleration(NewPod("pod1", "node1"), 1, 100),
			},
			oldNode:      NewNode("node1"),
			newNode:      addTaintsToNode(NewNode("node1"), "testTaint1", "taint1", []int{1}),
			expectDelete: false,
		},
		{
			description: "Only one added taint tolerated",
			pods: []v1.Pod{
				*addToleration(NewPod("pod1", "node1"), 1, 100),
			},
			oldNode:      NewNode("node1"),
			newNode:      addTaintsToNode(NewNode("node1"), "testTaint1", "taint1", []int{1, 2}),
			expectDelete: true,
		},
		{
			description: "Taint removed",
			pods: []v1.Pod{
				*addToleration(NewPod("pod1", "node1"), 1, 1),
			},
			oldNode:         addTaintsToNode(NewNode("node1"), "testTaint1", "taint1", []int{1}),
			newNode:         NewNode("node1"),
			expectDelete:    false,
			additionalSleep: 1500 * time.Millisecond,
		},
		{
			description: "Pod with multiple tolerations are evicted when first one runs out",
			pods: []v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "pod1",
					},
					Spec: v1.PodSpec{
						NodeName: "node1",
						Tolerations: []v1.Toleration{
							{Key: "testTaint1", Value: "test1", Effect: v1.TaintEffectNoExecute, TolerationSeconds: &[]int64{1}[0]},
							{Key: "testTaint2", Value: "test2", Effect: v1.TaintEffectNoExecute, TolerationSeconds: &[]int64{100}[0]},
						},
					},
					Status: v1.PodStatus{
						Conditions: []v1.PodCondition{
							{
								Type:   v1.PodReady,
								Status: v1.ConditionTrue,
							},
						},
					},
				},
			},
			oldNode:      NewNode("node1"),
			newNode:      addTaintsToNode(NewNode("node1"), "testTaint1", "taint1", []int{1, 2}),
			expectDelete: true,
		},
	}

	for _, item := range testCases {
		t.Run(item.description, func(t *testing.T) {
			//defer featuregatetesting.SetFeatureGateDuringTest(t, feature.DefaultFeatureGate, features.PodDisruptionConditions, item.enablePodDisruptionConditions)()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			//fakeClientset := fake.NewSimpleClientset(&v1.PodList{Items: item.pods})
			//controller, _, nodeIndexer := setupNewNoExecuteTaintManager(ctx, fakeClientset)
			//nodeIndexer.Add(item.newNode)
			//controller.recorder = testutil.NewFakeRecorder(0)
			scheme := runtime.NewScheme()
			clientgoscheme.AddToScheme(scheme)
			fakeClient := fakeclient.NewClientBuilder().WithScheme(scheme).WithLists(&v1.PodList{Items: item.pods}).WithObjects(item.newNode).Build()
			fakeClientset := testutil.NewClientWrapper(fakeClient, scheme)
			controller := NewNoExecuteTaintManager(testutil.NewFakeRecorder(), fakeClientset, getPodsAssignedToNode(ctx, fakeClientset))
			go controller.Run(ctx)
			controller.NodeUpdated(item.oldNode, item.newNode)

			if item.additionalSleep > 0 {
				time.Sleep(item.additionalSleep)
			}

			verifyPodActions(t, item.description, fakeClientset, item.expectPatch, item.expectDelete)
		})
	}
}

func TestUpdateNodeWithMultipleTaints(t *testing.T) {
	taint1 := createNoExecuteTaint(1)
	taint2 := createNoExecuteTaint(2)

	minute := int64(60)
	pod := NewPod("pod1", "node1")
	pod.Spec.Tolerations = []v1.Toleration{
		{Key: taint1.Key, Operator: v1.TolerationOpExists, Effect: v1.TaintEffectNoExecute},
		{Key: taint2.Key, Operator: v1.TolerationOpExists, Effect: v1.TaintEffectNoExecute, TolerationSeconds: &minute},
	}
	podNamespacedName := types.NamespacedName{Namespace: pod.Namespace, Name: pod.Name}

	untaintedNode := NewNode("node1")

	doubleTaintedNode := NewNode("node1")
	doubleTaintedNode.Spec.Taints = []v1.Taint{taint1, taint2}

	singleTaintedNode := NewNode("node1")
	singleTaintedNode.Spec.Taints = []v1.Taint{taint1}

	ctx, cancel := context.WithCancel(context.TODO())
	//fakeClientset := fake.NewSimpleClientset(pod)
	//controller, _, nodeIndexer := setupNewNoExecuteTaintManager(ctx, fakeClientset)
	//controller.recorder = testutil.NewFakeRecorder(0)
	scheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(scheme)
	fakeClient := fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(pod).WithObjects(untaintedNode).Build()
	fakeClientset := testutil.NewClientWrapper(fakeClient, scheme)
	controller := NewNoExecuteTaintManager(testutil.NewFakeRecorder(), fakeClientset, getPodsAssignedToNode(ctx, fakeClientset))
	go controller.Run(ctx)

	// no taint
	//nodeIndexer.Add(untaintedNode)
	controller.handleNodeUpdate(ctx, nodeUpdateItem{"node1"})
	// verify pod is not queued for deletion
	if controller.taintEvictionQueue.GetWorkerUnsafe(podNamespacedName.String()) != nil {
		t.Fatalf("pod queued for deletion with no taints")
	}

	// no taint -> infinitely tolerated taint
	//nodeIndexer.Update(singleTaintedNode)
	nodeCopy := singleTaintedNode.DeepCopy()
	fakeClientset.Update(ctx, nodeCopy, &client.UpdateOptions{})
	controller.handleNodeUpdate(ctx, nodeUpdateItem{"node1"})
	// verify pod is not queued for deletion
	if controller.taintEvictionQueue.GetWorkerUnsafe(podNamespacedName.String()) != nil {
		t.Fatalf("pod queued for deletion with permanently tolerated taint")
	}

	// infinitely tolerated taint -> temporarily tolerated taint
	//nodeIndexer.Update(doubleTaintedNode)
	fakeClientset.Update(ctx, doubleTaintedNode, &client.UpdateOptions{})
	controller.handleNodeUpdate(ctx, nodeUpdateItem{"node1"})
	// verify pod is queued for deletion
	if controller.taintEvictionQueue.GetWorkerUnsafe(podNamespacedName.String()) == nil {
		t.Fatalf("pod not queued for deletion after addition of temporarily tolerated taint")
	}

	// temporarily tolerated taint -> infinitely tolerated taint
	//nodeIndexer.Update(singleTaintedNode)
	if err := fakeClientset.Update(ctx, singleTaintedNode, &client.UpdateOptions{}); err != nil {
		t.Errorf("failed to update singleTaintedNode, %v", err)
	}
	controller.handleNodeUpdate(ctx, nodeUpdateItem{"node1"})
	// verify pod is not queued for deletion
	if controller.taintEvictionQueue.GetWorkerUnsafe(podNamespacedName.String()) != nil {
		t.Fatalf("pod queued for deletion after removal of temporarily tolerated taint")
	}

	// verify pod is not deleted
	for _, action := range fakeClientset.Actions() {
		if action.GetVerb() == "delete" && action.GetResource().Resource == "pods" {
			t.Error("Unexpected deletion")
		}
	}
	cancel()
}

func TestUpdateNodeWithMultiplePods(t *testing.T) {
	testCases := []struct {
		description         string
		pods                []v1.Pod
		oldNode             *v1.Node
		newNode             *v1.Node
		expectedDeleteTimes durationSlice
	}{
		{
			description: "Pods with different toleration times are evicted appropriately",
			pods: []v1.Pod{
				*NewPod("pod1", "node1"),
				*addToleration(NewPod("pod2", "node1"), 1, 1),
				*addToleration(NewPod("pod3", "node1"), 1, -1),
			},
			oldNode: NewNode("node1"),
			newNode: addTaintsToNode(NewNode("node1"), "testTaint1", "taint1", []int{1}),
			expectedDeleteTimes: durationSlice{
				{[]string{"pod1"}, 0},
				{[]string{"pod2"}, time.Second},
			},
		},
		{
			description: "Evict all pods not matching all taints instantly",
			pods: []v1.Pod{
				*NewPod("pod1", "node1"),
				*addToleration(NewPod("pod2", "node1"), 1, 1),
				*addToleration(NewPod("pod3", "node1"), 1, -1),
			},
			oldNode: NewNode("node1"),
			newNode: addTaintsToNode(NewNode("node1"), "testTaint1", "taint1", []int{1, 2}),
			expectedDeleteTimes: durationSlice{
				{[]string{"pod1", "pod2", "pod3"}, 0},
			},
		},
	}

	for _, item := range testCases {
		t.Run(item.description, func(t *testing.T) {
			t.Logf("Starting testcase %q", item.description)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			//fakeClientset := fake.NewSimpleClientset(&v1.PodList{Items: item.pods})
			sort.Sort(item.expectedDeleteTimes)
			//controller, _, nodeIndexer := setupNewNoExecuteTaintManager(ctx, fakeClientset)
			//nodeIndexer.Add(item.newNode)
			//controller.recorder = testutil.NewFakeRecorder(0)
			scheme := runtime.NewScheme()
			clientgoscheme.AddToScheme(scheme)
			fakeClient := fakeclient.NewClientBuilder().WithScheme(scheme).WithLists(&v1.PodList{Items: item.pods}).WithObjects(item.newNode).Build()
			fakeClientset := testutil.NewClientWrapper(fakeClient, scheme)
			controller := NewNoExecuteTaintManager(testutil.NewFakeRecorder(), fakeClientset, getPodsAssignedToNode(ctx, fakeClientset))
			go controller.Run(ctx)
			controller.NodeUpdated(item.oldNode, item.newNode)

			startedAt := time.Now()
			for i := range item.expectedDeleteTimes {
				if i == 0 || item.expectedDeleteTimes[i-1].timestamp != item.expectedDeleteTimes[i].timestamp {
					// compute a grace duration to give controller time to process updates. Choose big
					// enough intervals in the test cases above to avoid flakes.
					var increment time.Duration
					if i == len(item.expectedDeleteTimes)-1 || item.expectedDeleteTimes[i+1].timestamp == item.expectedDeleteTimes[i].timestamp {
						increment = 500 * time.Millisecond
					} else {
						increment = ((item.expectedDeleteTimes[i+1].timestamp - item.expectedDeleteTimes[i].timestamp) / time.Duration(2))
					}

					sleepTime := item.expectedDeleteTimes[i].timestamp - time.Since(startedAt) + increment
					if sleepTime < 0 {
						sleepTime = 0
					}
					t.Logf("Sleeping for %v", sleepTime)
					time.Sleep(sleepTime)
				}

				for delay, podName := range item.expectedDeleteTimes[i].names {
					deleted := false
					for _, action := range fakeClientset.Actions() {
						deleteAction, ok := action.(clienttesting.DeleteActionImpl)
						if !ok {
							t.Logf("Found not-delete action with verb %v. Ignoring.", action.GetVerb())
							continue
						}
						if deleteAction.GetResource().Resource != "pods" {
							continue
						}
						if podName == deleteAction.GetName() {
							deleted = true
						}
					}
					if !deleted {
						t.Errorf("Failed to deleted pod %v after %v", podName, delay)
					}
				}
				for _, action := range fakeClientset.Actions() {
					deleteAction, ok := action.(clienttesting.DeleteActionImpl)
					if !ok {
						t.Logf("Found not-delete action with verb %v. Ignoring.", action.GetVerb())
						continue
					}
					if deleteAction.GetResource().Resource != "pods" {
						continue
					}
					deletedPodName := deleteAction.GetName()
					expected := false
					for _, podName := range item.expectedDeleteTimes[i].names {
						if podName == deletedPodName {
							expected = true
						}
					}
					if !expected {
						t.Errorf("Pod %v was deleted even though it shouldn't have", deletedPodName)
					}
				}
				fakeClientset.ClearActions()
			}
		})
	}
}

func TestGetMinTolerationTime(t *testing.T) {
	one := int64(1)
	two := int64(2)
	oneSec := 1 * time.Second

	tests := []struct {
		tolerations []v1.Toleration
		expected    time.Duration
	}{
		{
			tolerations: []v1.Toleration{},
			expected:    0,
		},
		{
			tolerations: []v1.Toleration{
				{
					TolerationSeconds: nil,
				},
			},
			expected: -1,
		},
		{
			tolerations: []v1.Toleration{
				{
					TolerationSeconds: &one,
				},
				{
					TolerationSeconds: &two,
				},
			},
			expected: oneSec,
		},

		{
			tolerations: []v1.Toleration{
				{
					TolerationSeconds: &one,
				},
				{
					TolerationSeconds: nil,
				},
			},
			expected: oneSec,
		},
		{
			tolerations: []v1.Toleration{
				{
					TolerationSeconds: nil,
				},
				{
					TolerationSeconds: &one,
				},
			},
			expected: oneSec,
		},
	}

	for _, test := range tests {
		got := getMinTolerationTime(test.tolerations)
		if got != test.expected {
			t.Errorf("Incorrect min toleration time: got %v, expected %v", got, test.expected)
		}
	}
}

// TestEventualConsistency verifies if getPodsAssignedToNode returns incomplete data
// (e.g. due to watch latency), it will reconcile the remaining pods eventually.
// This scenario is partially covered by TestUpdatePods, but given this is an important
// property of TaintManager, it's better to have explicit test for this.
func TestEventualConsistency(t *testing.T) {
	testCases := []struct {
		description  string
		pods         []v1.Pod
		prevPod      *v1.Pod
		newPod       *v1.Pod
		oldNode      *v1.Node
		newNode      *v1.Node
		expectPatch  bool
		expectDelete bool
	}{
		{
			description: "existing pod2 scheduled onto tainted Node",
			pods: []v1.Pod{
				*NewPod("pod1", "node1"),
			},
			prevPod: NewPod("pod2", ""),
			newPod:  NewPod("pod2", "node1"),
			oldNode: NewNode("node1"),
			newNode: addTaintsToNode(NewNode("node1"), "testTaint1", "taint1", []int{1}),
			//expectPatch:  true,
			expectDelete: true,
		},
		{
			description: "existing pod2 with taint toleration scheduled onto tainted Node",
			pods: []v1.Pod{
				*NewPod("pod1", "node1"),
			},
			prevPod: addToleration(NewPod("pod2", ""), 1, 100),
			newPod:  addToleration(NewPod("pod2", "node1"), 1, 100),
			oldNode: NewNode("node1"),
			newNode: addTaintsToNode(NewNode("node1"), "testTaint1", "taint1", []int{1}),
			//expectPatch:  true,
			expectDelete: true,
		},
		{
			description: "new pod2 created on tainted Node",
			pods: []v1.Pod{
				*NewPod("pod1", "node1"),
			},
			prevPod: nil,
			newPod:  NewPod("pod2", "node1"),
			oldNode: NewNode("node1"),
			newNode: addTaintsToNode(NewNode("node1"), "testTaint1", "taint1", []int{1}),
			//expectPatch:  true,
			expectDelete: true,
		},
		{
			description: "new pod2 with tait toleration created on tainted Node",
			pods: []v1.Pod{
				*NewPod("pod1", "node1"),
			},
			prevPod: nil,
			newPod:  addToleration(NewPod("pod2", "node1"), 1, 100),
			oldNode: NewNode("node1"),
			newNode: addTaintsToNode(NewNode("node1"), "testTaint1", "taint1", []int{1}),
			//expectPatch:  true,
			expectDelete: true,
		},
	}

	for _, item := range testCases {
		t.Run(item.description, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			//fakeClientset := fake.NewSimpleClientset(&v1.PodList{Items: item.pods})
			//controller, podIndexer, nodeIndexer := setupNewNoExecuteTaintManager(ctx, fakeClientset)
			//nodeIndexer.Add(item.newNode)
			//controller.recorder = testutil.NewFakeRecorder(0)
			scheme := runtime.NewScheme()
			clientgoscheme.AddToScheme(scheme)
			fakeClient := fakeclient.NewClientBuilder().WithScheme(scheme).WithLists(&v1.PodList{Items: item.pods}).WithObjects(item.newNode).Build()
			fakeClientset := testutil.NewClientWrapper(fakeClient, scheme)
			controller := NewNoExecuteTaintManager(testutil.NewFakeRecorder(), fakeClientset, getPodsAssignedToNode(ctx, fakeClientset))
			go controller.Run(ctx)

			if item.prevPod != nil {
				//podIndexer.Add(item.prevPod)
				fakeClientset.Update(ctx, item.prevPod, &client.UpdateOptions{})
				controller.PodUpdated(nil, item.prevPod)
			}

			// First we simulate NodeUpdate that should delete 'pod1'. It doesn't know about 'pod2' yet.
			controller.NodeUpdated(item.oldNode, item.newNode)

			verifyPodActions(t, item.description, fakeClientset, item.expectPatch, item.expectDelete)
			fakeClientset.ClearActions()

			// And now the delayed update of 'pod2' comes to the TaintManager. We should delete it as well.
			//podIndexer.Update(item.newPod)
			fakeClientset.Update(ctx, item.newPod, &client.UpdateOptions{})
			controller.PodUpdated(item.prevPod, item.newPod)
			// wait a bit
			time.Sleep(timeForControllerToProgressForSanityCheck)
		})
	}
}

func verifyPodActions(t *testing.T, description string, fakeClientset *testutil.ClientWrapper, expectPatch, expectDelete bool) {
	t.Helper()
	podPatched := false
	podDeleted := false
	// use Poll instead of PollImmediate to give some processing time to the controller that the expected
	// actions are likely to be already sent
	err := wait.PollUntilContextTimeout(context.Background(), 10*time.Millisecond, 5*time.Second, true, func(ctx context.Context) (bool, error) {
		for _, action := range fakeClientset.Actions() {
			if action.GetVerb() == "patch" && action.GetResource().Resource == "pods" {
				podPatched = true
			}
			if action.GetVerb() == "delete" && action.GetResource().Resource == "pods" {
				podDeleted = true
			}
		}
		return podPatched == expectPatch && podDeleted == expectDelete, nil
	})
	if err != nil {
		t.Errorf("Failed waiting for the expected actions: %q", err)
	}
	if podPatched != expectPatch {
		t.Errorf("[%v]Unexpected test result. Expected patch %v, got %v", description, expectPatch, podPatched)
	}
	if podDeleted != expectDelete {
		t.Errorf("[%v]Unexpected test result. Expected delete %v, got %v", description, expectDelete, podDeleted)
	}
}

// TestPodDeletionEvent Verify that the output events are as expected
func TestPodDeletionEvent(t *testing.T) {
	f := func(path cmp.Path) bool {
		switch path.String() {
		// These fields change at runtime, so ignore it
		case "LastTimestamp", "FirstTimestamp", "ObjectMeta.Name":
			return true
		}
		return false
	}

	t.Run("emitPodDeletionEvent", func(t *testing.T) {
		controller := &NoExecuteTaintManager{}
		recorder := testutil.NewFakeRecorder()
		controller.recorder = recorder
		controller.emitPodDeletionEvent(types.NamespacedName{
			Name:      "test",
			Namespace: "test",
		})
		want := []*v1.Event{
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test",
				},
				InvolvedObject: v1.ObjectReference{
					Kind:       "Pod",
					APIVersion: "v1",
					Namespace:  "test",
					Name:       "test",
				},
				Reason:  "TaintManagerEviction",
				Type:    "Normal",
				Count:   1,
				Message: "Marking for deletion Pod test/test",
				Source:  v1.EventSource{Component: "nodeControllerTest"},
			},
		}
		if diff := cmp.Diff(want, recorder.Events, cmp.FilterPath(f, cmp.Ignore())); len(diff) > 0 {
			t.Errorf("emitPodDeletionEvent() returned data (-want,+got):\n%s", diff)
		}
	})

	t.Run("emitCancelPodDeletionEvent", func(t *testing.T) {
		controller := &NoExecuteTaintManager{}
		recorder := testutil.NewFakeRecorder()
		controller.recorder = recorder
		controller.emitCancelPodDeletionEvent(types.NamespacedName{
			Name:      "test",
			Namespace: "test",
		})
		want := []*v1.Event{
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test",
				},
				InvolvedObject: v1.ObjectReference{
					Kind:       "Pod",
					APIVersion: "v1",
					Namespace:  "test",
					Name:       "test",
				},
				Reason:  "TaintManagerEviction",
				Type:    "Normal",
				Count:   1,
				Message: "Cancelling deletion of Pod test/test",
				Source:  v1.EventSource{Component: "nodeControllerTest"},
			},
		}
		if diff := cmp.Diff(want, recorder.Events, cmp.FilterPath(f, cmp.Ignore())); len(diff) > 0 {
			t.Errorf("emitPodDeletionEvent() returned data (-want,+got):\n%s", diff)
		}
	})
}

// NewNode is a helper function for creating Nodes for testing.
func NewNode(name string) *v1.Node {
	return &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Status: v1.NodeStatus{
			Capacity: v1.ResourceList{
				v1.ResourceName(v1.ResourceCPU):    resource.MustParse("10"),
				v1.ResourceName(v1.ResourceMemory): resource.MustParse("10G"),
			},
		},
	}
}

// NewPod is a helper function for creating Pods for testing.
func NewPod(name, host string) *v1.Pod {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      name,
		},
		Spec: v1.PodSpec{
			NodeName: host,
		},
		Status: v1.PodStatus{
			Conditions: []v1.PodCondition{
				{
					Type:   v1.PodReady,
					Status: v1.ConditionTrue,
				},
			},
		},
	}

	return pod
}
