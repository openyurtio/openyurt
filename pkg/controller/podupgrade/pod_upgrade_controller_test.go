/*
Copyright 2022 The OpenYurt Authors.

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

package podupgrade

import (
	"context"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apiserver/pkg/storage/names"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	ktest "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
)

var (
	simpleDaemonSetLabel = map[string]string{"foo": "bar"}
	alwaysReady          = func() bool { return true }
)

var controllerKind = appsv1.SchemeGroupVersion.WithKind("DaemonSet")

var (
	SyncDaemonset = "SyncDaemosnet"
	SyncNode      = "SyncNode"
)

type fixture struct {
	t               *testing.T
	client          *fake.Clientset
	podLister       []*corev1.Pod
	nodeLister      []*corev1.Node
	daemonsetLister []*appsv1.DaemonSet
	actions         []ktest.Action
	objects         []runtime.Object
}

func newFixture(t *testing.T) *fixture {
	return &fixture{
		t:       t,
		objects: []runtime.Object{},
	}
}

func (f *fixture) newController() (*Controller, informers.SharedInformerFactory, informers.SharedInformerFactory) {
	f.client = fake.NewSimpleClientset(f.objects...)

	dsInformer := informers.NewSharedInformerFactory(f.client, 0)
	nodeInformer := informers.NewSharedInformerFactory(f.client, 0)
	podInformer := informers.NewSharedInformerFactory(f.client, 0)

	for _, d := range f.daemonsetLister {
		dsInformer.Apps().V1().DaemonSets().Informer().GetIndexer().Add(d)
	}
	for _, n := range f.nodeLister {
		nodeInformer.Core().V1().Nodes().Informer().GetIndexer().Add(n)
	}
	for _, p := range f.podLister {
		podInformer.Core().V1().Pods().Informer().GetIndexer().Add(p)
	}

	c := NewController(f.client, dsInformer.Apps().V1().DaemonSets(), nodeInformer.Core().V1().Nodes(),
		podInformer.Core().V1().Pods())

	c.daemonsetSynced = alwaysReady
	c.nodeSynced = alwaysReady

	return c, dsInformer, nodeInformer
}

// run execute the controller logic
// kind=SyncDaemonset test daemonset update event
// kind=SyncNode test node ready event
func (f *fixture) run(key string, kind string) {
	f.testController(key, kind, false)
}

func (f *fixture) runExpectError(key string, kind string) {
	f.testController(key, kind, true)
}

func (f *fixture) testController(key string, kind string, expectError bool) {
	c, dsInformer, nodeInformer := f.newController()

	stopCh := make(chan struct{})
	defer close(stopCh)
	dsInformer.Start(stopCh)
	nodeInformer.Start(stopCh)

	var err error
	switch kind {
	case SyncDaemonset:
		err = c.syncDaemonsetHandler(key)

	case SyncNode:
		err = c.syncNodeHandler(key)
	}

	if !expectError && err != nil {
		f.t.Errorf("error syncing: %v", err)
	} else if expectError && err == nil {
		f.t.Error("expected error syncing, got nil")

	}

	// make sure all the expected action occurred during controller sync process
	for _, action := range f.actions {
		findAndCheckAction(action, f.client.Actions(), f.t)
	}
}

// findAndCheckAction search all the given actions to find whether the expected action exists
func findAndCheckAction(expected ktest.Action, actions []ktest.Action, t *testing.T) {
	for _, action := range actions {
		if checkAction(expected, action) {
			t.Logf("Check action %+v success", expected)
			return
		}
	}
	t.Errorf("Expected action %+v does not occur", expected)
}

// checkAction verifies that expected and actual actions are equal
func checkAction(expected, actual ktest.Action) bool {
	if !(expected.Matches(actual.GetVerb(), actual.GetResource().Resource)) ||
		reflect.TypeOf(actual) != reflect.TypeOf(expected) ||
		actual.GetSubresource() != expected.GetSubresource() {
		return false
	}

	switch a := actual.(type) {
	case ktest.DeleteAction:
		e, _ := expected.(ktest.DeleteActionImpl)
		expName := e.GetName()
		actualName := a.GetName()
		if expName != actualName {
			return false
		}
	}

	return true
}

func (f *fixture) expectDeletePodAction(p *corev1.Pod) {
	action := ktest.NewDeleteAction(schema.GroupVersionResource{Resource: "pods"}, p.Namespace, p.Name)
	f.actions = append(f.actions, action)
}

func newDaemonSet(name string, img string) *appsv1.DaemonSet {
	two := int32(2)
	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			UID:       uuid.NewUUID(),
			Name:      name,
			Namespace: metav1.NamespaceDefault,
		},
		Spec: appsv1.DaemonSetSpec{
			RevisionHistoryLimit: &two,
			UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
				Type: appsv1.OnDeleteDaemonSetStrategyType,
			},
			Selector: &metav1.LabelSelector{MatchLabels: simpleDaemonSetLabel},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: simpleDaemonSetLabel,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Image: img}},
				},
			},
		},
	}
}

func newPod(podName string, nodeName string, label map[string]string, ds *appsv1.DaemonSet) *corev1.Pod {
	// Add hash unique label to the pod
	newLabels := label
	var podSpec corev1.PodSpec
	// Copy pod spec from DaemonSet template, or use a default one if DaemonSet is nil
	if ds != nil {
		hash := ComputeHash(&ds.Spec.Template, ds.Status.CollisionCount)
		newLabels = CloneAndAddLabel(label, appsv1.DefaultDaemonSetUniqueLabelKey, hash)
		podSpec = ds.Spec.Template.Spec
	} else {
		podSpec = corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Image:                  "foo/bar",
					TerminationMessagePath: corev1.TerminationMessagePathDefault,
					ImagePullPolicy:        corev1.PullIfNotPresent,
				},
			},
		}
	}

	// Add node name to the pod
	if len(nodeName) > 0 {
		podSpec.NodeName = nodeName
	}

	pod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: podName,
			Labels:       newLabels,
			Namespace:    metav1.NamespaceDefault,
		},
		Spec: podSpec,
	}
	pod.Name = names.SimpleNameGenerator.GenerateName(podName)
	if ds != nil {
		pod.OwnerReferences = []metav1.OwnerReference{*metav1.NewControllerRef(ds, controllerKind)}
	}
	return pod
}

func newNode(name string) *corev1.Node {
	return &corev1.Node{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: metav1.NamespaceNone,
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
			},
			Allocatable: corev1.ResourceList{
				corev1.ResourcePods: resource.MustParse("100"),
			},
		},
	}
}

func setAutoUpgradeAnnotation(ds *appsv1.DaemonSet, annV string) {
	metav1.SetMetaDataAnnotation(&ds.ObjectMeta, UpgradeAnnotation, annV)
}

func getDaemonsetKey(ds *appsv1.DaemonSet, t *testing.T) string {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(ds)
	if err != nil {
		t.Errorf("Unexpected error getting key for foo %v: %v", ds.Name, err)
		return ""
	}
	return key
}

func TestAutoUpgradeWithDaemonsetUpdate(t *testing.T) {
	f := newFixture(t)
	ds := newDaemonSet("test-ds", "foo/bar:v1")
	setAutoUpgradeAnnotation(ds, "auto")
	node := newNode("test-node")
	pod := newPod("test-pod", "test-node", simpleDaemonSetLabel, ds)

	ds.Spec.Template.Spec.Containers[0].Image = "foo/bar:v2"

	f.daemonsetLister = append(f.daemonsetLister, ds)
	f.podLister = append(f.podLister, pod)

	f.objects = append(f.objects, ds, pod, node)

	f.expectDeletePodAction(pod)

	f.run(getDaemonsetKey(ds, t), SyncDaemonset)
}

func TestAutoUpgradeWithNodeReady(t *testing.T) {
	f := newFixture(t)
	ds := newDaemonSet("test-ds", "foo/bar")
	setAutoUpgradeAnnotation(ds, "auto")
	node := newNode("test-node")
	pod := newPod("test-pod", "test-node", simpleDaemonSetLabel, ds)

	ds.Spec.Template.Spec.Containers[0].Image = "foo/bar:v2"

	f.daemonsetLister = append(f.daemonsetLister, ds)
	f.nodeLister = append(f.nodeLister, node)
	f.podLister = append(f.podLister, pod)

	f.objects = append(f.objects, ds, pod, node)

	f.expectDeletePodAction(pod)

	f.run(node.Name, SyncNode)
}

func TestOTAUpgrade(t *testing.T) {
	f := newFixture(t)
	ds := newDaemonSet("test-ds", "foo/bar:v1")
	setAutoUpgradeAnnotation(ds, "ota")
	oldPod := newPod("old-pod", "test-node", simpleDaemonSetLabel, ds)

	ds.Spec.Template.Spec.Containers[0].Image = "foo/bar:v2"
	newPod := newPod("new-pod", "test-node", simpleDaemonSetLabel, ds)

	f.daemonsetLister = append(f.daemonsetLister, ds)
	f.podLister = append(f.podLister, oldPod, newPod)
	f.objects = append(f.objects, ds, oldPod, newPod)

	f.run(getDaemonsetKey(ds, t), SyncDaemonset)

	// check whether ota upgradable annotation set properly
	oldPodGot, err := f.client.CoreV1().Pods(ds.Namespace).Get(context.TODO(), oldPod.Name, metav1.GetOptions{})
	if err != nil {
		t.Errorf("get oldPod failed, %+v", err)
	}

	newPodGot, err := f.client.CoreV1().Pods(ds.Namespace).Get(context.TODO(), newPod.Name, metav1.GetOptions{})
	if err != nil {
		t.Errorf("get newPod failed, %+v", err)
	}

	annOldPodGot, oldPodOK := oldPodGot.Annotations[PodUpgradableAnnotation]
	assert.Equal(t, true, oldPodOK)
	assert.Equal(t, "true", annOldPodGot)

	annNewPodGot, newPodOK := newPodGot.Annotations[PodUpgradableAnnotation]
	assert.Equal(t, true, newPodOK)
	assert.Equal(t, "false", annNewPodGot)
}
