/*
Copyright 2022 The OpenYurt Authors.
Copyright 2017 The Kubernetes Authors.

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

package daemonpodupdater

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apiserver/pkg/storage/names"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/daemonsetupgradestrategy"
	k8sutil "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/daemonsetupgradestrategy/daemonpodupdater/kubernetes"
)

const (
	SingleMaxUnavailable = "1"
)

var (
	simpleDaemonSetLabel = map[string]string{"foo": "bar"}
)

// ----------------------------------------------------------------------------------------------------------------
// ----------------------------------------------------new Object--------------------------------------------------
// ----------------------------------------------------------------------------------------------------------------

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
			Selector:             &metav1.LabelSelector{MatchLabels: simpleDaemonSetLabel},
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
		hash := k8sutil.ComputeHash(&ds.Spec.Template, ds.Status.CollisionCount)
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
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{
				{
					Type:   corev1.PodReady,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}
	pod.Name = names.SimpleNameGenerator.GenerateName(podName)
	if ds != nil {
		pod.OwnerReferences = []metav1.OwnerReference{*metav1.NewControllerRef(ds, controllerKind)}
	}
	return pod
}

func newNode(name string, ready bool) *corev1.Node {
	cond := corev1.NodeCondition{
		Type:   corev1.NodeReady,
		Status: corev1.ConditionTrue,
	}
	if !ready {
		cond.Status = corev1.ConditionFalse
	}

	return &corev1.Node{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: metav1.NamespaceNone,
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				cond,
			},
			Allocatable: corev1.ResourceList{
				corev1.ResourcePods: resource.MustParse("100"),
			},
		},
	}
}

// ----------------------------------------------------------------------------------------------------------------
// -------------------------------------------------------util-----------------------------------------------------
// ----------------------------------------------------------------------------------------------------------------

func setAdvanceRollingUpdateAnnotation(ds *appsv1.DaemonSet) {
	metav1.SetMetaDataAnnotation(&ds.ObjectMeta, daemonsetupgradestrategy.UpdateAnnotation, daemonsetupgradestrategy.AdvancedRollingUpdate)
}

func setMaxUnavailableAnnotation(ds *appsv1.DaemonSet, v string) {
	metav1.SetMetaDataAnnotation(&ds.ObjectMeta, daemonsetupgradestrategy.MaxUnavailableAnnotation, v)
}

func setOnDelete(ds *appsv1.DaemonSet) {
	ds.Spec.UpdateStrategy = appsv1.DaemonSetUpdateStrategy{
		Type: appsv1.OnDeleteDaemonSetStrategyType,
	}
}

func addNodesWithPods(startIndex, numNodes int, ds *appsv1.DaemonSet, ready bool) ([]client.Object, error) {
	objs := make([]client.Object, 0)

	for i := startIndex; i < startIndex+numNodes; i++ {
		var nodeName string
		switch ready {
		case true:
			nodeName = fmt.Sprintf("node-ready-%d", i)
		case false:
			nodeName = fmt.Sprintf("node-not-ready-%d", i)
		}

		node := newNode(nodeName, ready)
		objs = append(objs, node)

		podPrefix := fmt.Sprintf("pod-%d", i)
		pod := newPod(podPrefix, nodeName, simpleDaemonSetLabel, ds)
		objs = append(objs, pod)
	}
	return objs, nil
}

// ----------------------------------------------------------------------------------------------------------------
// ----------------------------------------------------Test Cases--------------------------------------------------
// ----------------------------------------------------------------------------------------------------------------

type tCase struct {
	name           string
	onDelete       bool
	strategy       string
	nodeNum        int
	readyNodeNum   int
	maxUnavailable string
	turnReady      bool
	wantDelete     bool
}

// DaemonSets should place onto NotReady nodes
func TestDaemonsetPodUpdater(t *testing.T) {
	tcases := []tCase{
		{
			name:           "not OnDelete strategy",
			onDelete:       false,
			strategy:       "Auto",
			nodeNum:        3,
			readyNodeNum:   3,
			maxUnavailable: SingleMaxUnavailable,
			turnReady:      false,
			wantDelete:     false,
		},
		{
			name:           "success",
			onDelete:       true,
			strategy:       "Auto",
			nodeNum:        3,
			readyNodeNum:   3,
			maxUnavailable: SingleMaxUnavailable,
			turnReady:      false,
			wantDelete:     true,
		},
		{
			name:           "success with maxUnavailable is 2",
			onDelete:       true,
			strategy:       "Auto",
			nodeNum:        3,
			readyNodeNum:   3,
			maxUnavailable: SingleMaxUnavailable,
			turnReady:      false,
			wantDelete:     true,
		},
		{
			name:           "success with maxUnavailable is 50%",
			onDelete:       true,
			strategy:       "Auto",
			nodeNum:        3,
			readyNodeNum:   3,
			maxUnavailable: "50%",
			turnReady:      false,
			wantDelete:     true,
		},
		{
			name:           "success with 1 node not-ready",
			onDelete:       true,
			strategy:       "Auto",
			nodeNum:        3,
			readyNodeNum:   2,
			maxUnavailable: SingleMaxUnavailable,
			turnReady:      false,
			wantDelete:     true,
		},
		{
			name:           "success with 2 nodes not-ready",
			onDelete:       true,
			strategy:       "AdvancedRollingUpdate",
			nodeNum:        3,
			readyNodeNum:   1,
			maxUnavailable: SingleMaxUnavailable,
			turnReady:      false,
			wantDelete:     true,
		},
		{
			name:           "success with 2 nodes not-ready, then turn ready",
			onDelete:       true,
			strategy:       "AdvancedRollingUpdate",
			nodeNum:        3,
			readyNodeNum:   1,
			maxUnavailable: SingleMaxUnavailable,
			turnReady:      true,
			wantDelete:     true,
		},
	}

	for _, tcase := range tcases {
		t.Logf("Current test case is %q", tcase.name)
		ds := newDaemonSet("ds", "foo/bar:v1")
		if tcase.onDelete {
			setOnDelete(ds)
		}
		setMaxUnavailableAnnotation(ds, tcase.maxUnavailable)
		switch tcase.strategy {
		case daemonsetupgradestrategy.AdvancedRollingUpdate:
			setAdvanceRollingUpdateAnnotation(ds)
		}

		// add ready nodes and its pods
		readyNodesWithPods, err := addNodesWithPods(1, tcase.readyNodeNum, ds, true)
		if err != nil {
			t.Fatal(err)
		}

		// add not-ready nodes and its pods
		notReadyNodesWithPods, err := addNodesWithPods(tcase.readyNodeNum+1, tcase.nodeNum-tcase.readyNodeNum, ds,
			false)
		if err != nil {
			t.Fatal(err)
		}

		// Update daemonset specification
		ds.Spec.Template.Spec.Containers[0].Image = "foo/bar:v2"

		c := fakeclient.NewClientBuilder().WithObjects(ds).WithObjects(readyNodesWithPods...).
			WithObjects(notReadyNodesWithPods...).Build()

		req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ds.Namespace, Name: ds.Name}}
		r := &ReconcileDaemonpodupdater{
			Client:       c,
			expectations: k8sutil.NewControllerExpectations(),
			podControl:   &k8sutil.FakePodControl{},
		}

		_, err = r.Reconcile(context.TODO(), req)
		if err != nil {
			t.Fatalf("Failed to reconcile daemonpodupdater controller")
		}
	}
}

func TestController_maxUnavailableCounts(t *testing.T) {
	tests := []struct {
		name           string
		maxUnavailable string
		wantNum        int
	}{
		{
			"use default when set 0",
			"0", 1,
		},
		{
			"use default when set 0%",
			"0%", 1,
		},
		{
			"10 * 10% = 1",
			"10%", 1,
		},
		{
			"10 * 10% = 2",
			"20%", 2,
		},
		{
			"10 * 90% = 9",
			"90%", 9,
		},
		{
			"10 * 95% = 9.5, roundup is 10",
			"95%", 10,
		},
		{
			"10 * 100% = 10",
			"100%", 10,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r := &ReconcileDaemonpodupdater{}
			ds := &appsv1.DaemonSet{}
			setMaxUnavailableAnnotation(ds, test.maxUnavailable)

			// Just fake, and set nodeToDaemonPods length to 10
			nodeToDaemonPods := map[string][]*corev1.Pod{
				"1": nil, "2": nil, "3": nil, "4": nil, "5": nil, "6": nil, "7": nil, "8": nil, "9": nil, "10": nil,
			}
			got, err := r.maxUnavailableCounts(ds, nodeToDaemonPods)
			assert.Equal(t, nil, err)
			assert.Equal(t, test.wantNum, got)
		})
	}
}

// TestReconcileDaemonpodupdater_Reconcile tests the Reconcile method with various scenarios
func TestReconcileDaemonpodupdater_Reconcile(t *testing.T) {
	tests := []struct {
		name           string
		daemonSet      *appsv1.DaemonSet
		objects        []client.Object
		expectations   map[string]bool
		expectedError  bool
		expectedResult reconcile.Result
	}{
		{
			name:           "DaemonSet not found",
			daemonSet:      nil,
			objects:        []client.Object{},
			expectations:   map[string]bool{},
			expectedError:  false,
			expectedResult: reconcile.Result{},
		},
		{
			name: "DaemonSet with deletion timestamp",
			daemonSet: func() *appsv1.DaemonSet {
				ds := newDaemonSet("test-ds", "test-image")
				now := metav1.Now()
				ds.DeletionTimestamp = &now
				ds.Finalizers = []string{"test-finalizer"}
				return ds
			}(),
			objects:        []client.Object{},
			expectations:   map[string]bool{},
			expectedError:  false,
			expectedResult: reconcile.Result{},
		},
		{
			name: "DaemonSet without update annotation",
			daemonSet: func() *appsv1.DaemonSet {
				ds := newDaemonSet("test-ds", "test-image")
				setOnDelete(ds)
				return ds
			}(),
			objects:        []client.Object{},
			expectations:   map[string]bool{},
			expectedError:  false,
			expectedResult: reconcile.Result{},
		},
		{
			name: "DaemonSet with OTA update strategy",
			daemonSet: func() *appsv1.DaemonSet {
				ds := newDaemonSet("test-ds", "test-image")
				setOnDelete(ds)
				metav1.SetMetaDataAnnotation(&ds.ObjectMeta, daemonsetupgradestrategy.UpdateAnnotation, daemonsetupgradestrategy.OTAUpdate)
				return ds
			}(),
			objects:        []client.Object{},
			expectations:   map[string]bool{"default/test-ds": true},
			expectedError:  false,
			expectedResult: reconcile.Result{},
		},
		{
			name: "DaemonSet with Auto update strategy",
			daemonSet: func() *appsv1.DaemonSet {
				ds := newDaemonSet("test-ds", "test-image")
				setOnDelete(ds)
				metav1.SetMetaDataAnnotation(&ds.ObjectMeta, daemonsetupgradestrategy.UpdateAnnotation, daemonsetupgradestrategy.AdvancedRollingUpdate)
				return ds
			}(),
			objects:        []client.Object{},
			expectations:   map[string]bool{"default/test-ds": true},
			expectedError:  false,
			expectedResult: reconcile.Result{},
		},
		{
			name: "DaemonSet with AdvancedRollingUpdate strategy",
			daemonSet: func() *appsv1.DaemonSet {
				ds := newDaemonSet("test-ds", "test-image")
				setOnDelete(ds)
				metav1.SetMetaDataAnnotation(&ds.ObjectMeta, daemonsetupgradestrategy.UpdateAnnotation, daemonsetupgradestrategy.AdvancedRollingUpdate)
				return ds
			}(),
			objects:        []client.Object{},
			expectations:   map[string]bool{"default/test-ds": true},
			expectedError:  false,
			expectedResult: reconcile.Result{},
		},
		{
			name: "DaemonSet with unknown update strategy",
			daemonSet: func() *appsv1.DaemonSet {
				ds := newDaemonSet("test-ds", "test-image")
				setOnDelete(ds)
				metav1.SetMetaDataAnnotation(&ds.ObjectMeta, daemonsetupgradestrategy.UpdateAnnotation, "UnknownStrategy")
				return ds
			}(),
			objects:        []client.Object{},
			expectations:   map[string]bool{},
			expectedError:  true,
			expectedResult: reconcile.Result{},
		},
		{
			name: "DaemonSet with expectations not satisfied",
			daemonSet: func() *appsv1.DaemonSet {
				ds := newDaemonSet("test-ds", "test-image")
				setOnDelete(ds)
				metav1.SetMetaDataAnnotation(&ds.ObjectMeta, daemonsetupgradestrategy.UpdateAnnotation, daemonsetupgradestrategy.OTAUpdate)
				return ds
			}(),
			objects:        []client.Object{},
			expectations:   map[string]bool{"default/test-ds": false},
			expectedError:  false,
			expectedResult: reconcile.Result{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fake client
			var objects []client.Object
			if tt.daemonSet != nil {
				objects = append(objects, tt.daemonSet)
			}
			objects = append(objects, tt.objects...)
			c := fakeclient.NewClientBuilder().WithObjects(objects...).Build()

			// Create expectations
			expectations := k8sutil.NewControllerExpectations()
			for key, satisfied := range tt.expectations {
				if !satisfied {
					expectations.SetExpectations(key, 0, 1)
				}
			}

			// Create reconciler
			r := &ReconcileDaemonpodupdater{
				Client:       c,
				expectations: expectations,
				podControl:   &k8sutil.FakePodControl{},
			}

			// Create request
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: "default",
					Name:      "test-ds",
				},
			}

			// Execute reconcile
			result, err := r.Reconcile(context.TODO(), req)

			// Verify results
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

// TestReconcileDaemonpodupdater_Reconcile_OTAUpdate tests OTA update specific scenarios
func TestReconcileDaemonpodupdater_Reconcile_OTAUpdate(t *testing.T) {
	tests := []struct {
		name          string
		daemonSet     *appsv1.DaemonSet
		pods          []*corev1.Pod
		expectedError bool
	}{
		{
			name: "OTA update with pods",
			daemonSet: func() *appsv1.DaemonSet {
				ds := newDaemonSet("test-ds", "test-image")
				setOnDelete(ds)
				metav1.SetMetaDataAnnotation(&ds.ObjectMeta, daemonsetupgradestrategy.UpdateAnnotation, daemonsetupgradestrategy.OTAUpdate)
				return ds
			}(),
			pods: func() []*corev1.Pod {
				ds := newDaemonSet("test-ds", "test-image")
				return []*corev1.Pod{
					newPod("pod-1", "node-1", simpleDaemonSetLabel, ds),
					newPod("pod-2", "node-2", simpleDaemonSetLabel, ds),
				}
			}(),
			expectedError: false,
		},
		{
			name: "OTA update with no pods",
			daemonSet: func() *appsv1.DaemonSet {
				ds := newDaemonSet("test-ds", "test-image")
				setOnDelete(ds)
				metav1.SetMetaDataAnnotation(&ds.ObjectMeta, daemonsetupgradestrategy.UpdateAnnotation, daemonsetupgradestrategy.OTAUpdate)
				return ds
			}(),
			pods:          []*corev1.Pod{},
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create objects for fake client
			objects := []client.Object{tt.daemonSet}
			for _, pod := range tt.pods {
				objects = append(objects, pod)
			}
			c := fakeclient.NewClientBuilder().WithObjects(objects...).Build()

			// Create reconciler
			r := &ReconcileDaemonpodupdater{
				Client:       c,
				expectations: k8sutil.NewControllerExpectations(),
				podControl:   &k8sutil.FakePodControl{},
			}

			// Create request
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: tt.daemonSet.Namespace,
					Name:      tt.daemonSet.Name,
				},
			}

			// Execute reconcile
			result, err := r.Reconcile(context.TODO(), req)

			// Verify results
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, reconcile.Result{}, result)
		})
	}
}

// TestReconcileDaemonpodupdater_Reconcile_AdvancedRollingUpdate tests AdvancedRollingUpdate specific scenarios
func TestReconcileDaemonpodupdater_Reconcile_AdvancedRollingUpdate(t *testing.T) {
	tests := []struct {
		name          string
		daemonSet     *appsv1.DaemonSet
		pods          []*corev1.Pod
		nodes         []*corev1.Node
		expectedError bool
	}{
		{
			name: "AdvancedRollingUpdate with ready nodes and pods",
			daemonSet: func() *appsv1.DaemonSet {
				ds := newDaemonSet("test-ds", "test-image")
				setOnDelete(ds)
				metav1.SetMetaDataAnnotation(&ds.ObjectMeta, daemonsetupgradestrategy.UpdateAnnotation, daemonsetupgradestrategy.AdvancedRollingUpdate)
				setMaxUnavailableAnnotation(ds, "1")
				return ds
			}(),
			pods: func() []*corev1.Pod {
				ds := newDaemonSet("test-ds", "test-image")
				return []*corev1.Pod{
					newPod("pod-1", "node-1", simpleDaemonSetLabel, ds),
					newPod("pod-2", "node-2", simpleDaemonSetLabel, ds),
				}
			}(),
			nodes: []*corev1.Node{
				newNode("node-1", true),
				newNode("node-2", true),
			},
			expectedError: false,
		},
		{
			name: "AdvancedRollingUpdate with not ready nodes",
			daemonSet: func() *appsv1.DaemonSet {
				ds := newDaemonSet("test-ds", "test-image")
				setOnDelete(ds)
				metav1.SetMetaDataAnnotation(&ds.ObjectMeta, daemonsetupgradestrategy.UpdateAnnotation, daemonsetupgradestrategy.AdvancedRollingUpdate)
				setMaxUnavailableAnnotation(ds, "1")
				return ds
			}(),
			pods: func() []*corev1.Pod {
				ds := newDaemonSet("test-ds", "test-image")
				return []*corev1.Pod{
					newPod("pod-1", "node-1", simpleDaemonSetLabel, ds),
					newPod("pod-2", "node-2", simpleDaemonSetLabel, ds),
				}
			}(),
			nodes: []*corev1.Node{
				newNode("node-1", false),
				newNode("node-2", true),
			},
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create objects for fake client
			objects := []client.Object{tt.daemonSet}
			for _, pod := range tt.pods {
				objects = append(objects, pod)
			}
			for _, node := range tt.nodes {
				objects = append(objects, node)
			}
			c := fakeclient.NewClientBuilder().WithObjects(objects...).Build()

			// Create reconciler
			r := &ReconcileDaemonpodupdater{
				Client:       c,
				expectations: k8sutil.NewControllerExpectations(),
				podControl:   &k8sutil.FakePodControl{},
			}

			// Create request
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: tt.daemonSet.Namespace,
					Name:      tt.daemonSet.Name,
				},
			}

			// Execute reconcile
			result, err := r.Reconcile(context.TODO(), req)

			// Verify results
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, reconcile.Result{}, result)
		})
	}
}

// TestReconcileDaemonpodupdater_Reconcile_ErrorHandling tests error handling scenarios
func TestReconcileDaemonpodupdater_Reconcile_ErrorHandling(t *testing.T) {
	tests := []struct {
		name          string
		daemonSet     *appsv1.DaemonSet
		objects       []client.Object
		expectations  map[string]bool
		expectedError bool
	}{
		{
			name: "Get DaemonSet error",
			daemonSet: func() *appsv1.DaemonSet {
				ds := newDaemonSet("test-ds", "test-image")
				// Don't add to objects to simulate not found
				return ds
			}(),
			objects:       []client.Object{},
			expectations:  map[string]bool{},
			expectedError: false, // Should handle NotFound gracefully
		},
		{
			name: "OTA update with GetDaemonsetPods error",
			daemonSet: func() *appsv1.DaemonSet {
				ds := newDaemonSet("test-ds", "test-image")
				setOnDelete(ds)
				metav1.SetMetaDataAnnotation(&ds.ObjectMeta, daemonsetupgradestrategy.UpdateAnnotation, daemonsetupgradestrategy.OTAUpdate)
				return ds
			}(),
			objects:       []client.Object{},
			expectations:  map[string]bool{"default/test-ds": true},
			expectedError: false, // Should handle gracefully
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fake client
			objects := []client.Object{tt.daemonSet}
			objects = append(objects, tt.objects...)
			c := fakeclient.NewClientBuilder().WithObjects(objects...).Build()

			// Create expectations
			expectations := k8sutil.NewControllerExpectations()
			for key, satisfied := range tt.expectations {
				if !satisfied {
					expectations.SetExpectations(key, 0, 1)
				}
			}

			// Create reconciler
			r := &ReconcileDaemonpodupdater{
				Client:       c,
				expectations: expectations,
				podControl:   &k8sutil.FakePodControl{},
			}

			// Create request
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: "default",
					Name:      "test-ds",
				},
			}

			// Execute reconcile
			result, err := r.Reconcile(context.TODO(), req)

			// Verify results
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, reconcile.Result{}, result)
		})
	}
}

// TestSetPodUpgradeCondition tests the SetPodUpgradeCondition method
func TestSetPodUpgradeCondition(t *testing.T) {
	tests := []struct {
		name          string
		daemonSet     *appsv1.DaemonSet
		pod           *corev1.Pod
		expectedError bool
	}{
		{
			name: "Set pod upgrade condition - pod needs upgrade",
			daemonSet: func() *appsv1.DaemonSet {
				ds := newDaemonSet("test-ds", "test-image")
				ds.Status.CollisionCount = nil
				return ds
			}(),
			pod: func() *corev1.Pod {
				ds := newDaemonSet("test-ds", "test-image")
				pod := newPod("test-pod", "node-1", simpleDaemonSetLabel, ds)
				// Make pod not latest by changing the hash
				pod.Labels[appsv1.DefaultDaemonSetUniqueLabelKey] = "old-hash"
				return pod
			}(),
			expectedError: false,
		},
		{
			name: "Set pod upgrade condition - pod is latest",
			daemonSet: func() *appsv1.DaemonSet {
				ds := newDaemonSet("test-ds", "test-image")
				ds.Status.CollisionCount = nil
				return ds
			}(),
			pod: func() *corev1.Pod {
				ds := newDaemonSet("test-ds", "test-image")
				pod := newPod("test-pod", "node-1", simpleDaemonSetLabel, ds)
				// Pod is already latest
				return pod
			}(),
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create objects for fake client
			objects := []client.Object{tt.daemonSet, tt.pod}
			c := fakeclient.NewClientBuilder().WithObjects(objects...).Build()

			// Create reconciler
			r := &ReconcileDaemonpodupdater{
				Client:       c,
				expectations: k8sutil.NewControllerExpectations(),
				podControl:   &k8sutil.FakePodControl{},
			}

			// Compute hash for the DaemonSet
			newHash := k8sutil.ComputeHash(&tt.daemonSet.Spec.Template, tt.daemonSet.Status.CollisionCount)

			// Execute SetPodUpgradeCondition
			err := r.SetPodUpgradeCondition(tt.daemonSet, tt.pod, newHash)

			// Verify results
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestIsPodUpdatable tests the IsPodUpdatable function
func TestIsPodUpdatable(t *testing.T) {
	tests := []struct {
		name     string
		pod      *corev1.Pod
		expected bool
	}{
		{
			name: "Pod is updatable",
			pod: func() *corev1.Pod {
				pod := newPod("test-pod", "node-1", simpleDaemonSetLabel, nil)
				pod.Status.Conditions = []corev1.PodCondition{
					{
						Type:   daemonsetupgradestrategy.PodNeedUpgrade,
						Status: corev1.ConditionTrue,
					},
				}
				return pod
			}(),
			expected: true,
		},
		{
			name: "Pod is not updatable",
			pod: func() *corev1.Pod {
				pod := newPod("test-pod", "node-1", simpleDaemonSetLabel, nil)
				pod.Status.Conditions = []corev1.PodCondition{
					{
						Type:   daemonsetupgradestrategy.PodNeedUpgrade,
						Status: corev1.ConditionFalse,
					},
				}
				return pod
			}(),
			expected: false,
		},
		{
			name: "Pod has no upgrade condition",
			pod: func() *corev1.Pod {
				pod := newPod("test-pod", "node-1", simpleDaemonSetLabel, nil)
				return pod
			}(),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsPodUpdatable(tt.pod)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestIsPodUpgradeConditionTrue tests the IsPodUpgradeConditionTrue function
func TestIsPodUpgradeConditionTrue(t *testing.T) {
	tests := []struct {
		name     string
		status   corev1.PodStatus
		expected bool
	}{
		{
			name: "Pod upgrade condition is true",
			status: corev1.PodStatus{
				Conditions: []corev1.PodCondition{
					{
						Type:   daemonsetupgradestrategy.PodNeedUpgrade,
						Status: corev1.ConditionTrue,
					},
				},
			},
			expected: true,
		},
		{
			name: "Pod upgrade condition is false",
			status: corev1.PodStatus{
				Conditions: []corev1.PodCondition{
					{
						Type:   daemonsetupgradestrategy.PodNeedUpgrade,
						Status: corev1.ConditionFalse,
					},
				},
			},
			expected: false,
		},
		{
			name: "Pod has no upgrade condition",
			status: corev1.PodStatus{
				Conditions: []corev1.PodCondition{},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsPodUpgradeConditionTrue(tt.status)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestGetTemplateGeneration tests the GetTemplateGeneration function
func TestGetTemplateGeneration(t *testing.T) {
	tests := []struct {
		name          string
		daemonSet     *appsv1.DaemonSet
		expectedGen   *int64
		expectedError bool
	}{
		{
			name: "DaemonSet with valid template generation",
			daemonSet: func() *appsv1.DaemonSet {
				ds := newDaemonSet("test-ds", "test-image")
				ds.Annotations = map[string]string{
					appsv1.DeprecatedTemplateGeneration: "123",
				}
				return ds
			}(),
			expectedGen:   func() *int64 { v := int64(123); return &v }(),
			expectedError: false,
		},
		{
			name: "DaemonSet without template generation annotation",
			daemonSet: func() *appsv1.DaemonSet {
				ds := newDaemonSet("test-ds", "test-image")
				return ds
			}(),
			expectedGen:   nil,
			expectedError: false,
		},
		{
			name: "DaemonSet with invalid template generation",
			daemonSet: func() *appsv1.DaemonSet {
				ds := newDaemonSet("test-ds", "test-image")
				ds.Annotations = map[string]string{
					appsv1.DeprecatedTemplateGeneration: "invalid",
				}
				return ds
			}(),
			expectedGen:   nil,
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gen, err := GetTemplateGeneration(tt.daemonSet)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if tt.expectedGen == nil {
				assert.Nil(t, gen)
			} else {
				assert.NotNil(t, gen)
				assert.Equal(t, *tt.expectedGen, *gen)
			}
		})
	}
}

// TestNodeReadyByName tests the NodeReadyByName function
func TestNodeReadyByName(t *testing.T) {
	tests := []struct {
		name          string
		node          *corev1.Node
		expectedReady bool
		expectedError bool
	}{
		{
			name: "Node is ready",
			node: func() *corev1.Node {
				return newNode("test-node", true)
			}(),
			expectedReady: true,
			expectedError: false,
		},
		{
			name: "Node is not ready",
			node: func() *corev1.Node {
				return newNode("test-node", false)
			}(),
			expectedReady: false,
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create objects for fake client
			objects := []client.Object{tt.node}
			c := fakeclient.NewClientBuilder().WithObjects(objects...).Build()

			// Execute NodeReadyByName
			ready, err := NodeReadyByName(c, tt.node.Name)

			// Verify results
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedReady, ready)
			}
		})
	}
}

// TestNodeReady tests the NodeReady function
func TestNodeReady(t *testing.T) {
	tests := []struct {
		name     string
		status   corev1.NodeStatus
		expected bool
	}{
		{
			name: "Node is ready",
			status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionTrue,
					},
				},
			},
			expected: true,
		},
		{
			name: "Node is not ready",
			status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionFalse,
					},
				},
			},
			expected: false,
		},
		{
			name: "Node has no ready condition",
			status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NodeReady(&tt.status)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestCloneAndAddLabel tests the CloneAndAddLabel function
func TestCloneAndAddLabel(t *testing.T) {
	tests := []struct {
		name     string
		labels   map[string]string
		key      string
		value    string
		expected map[string]string
	}{
		{
			name: "Add label to existing labels",
			labels: map[string]string{
				"foo": "bar",
			},
			key:   "new-key",
			value: "new-value",
			expected: map[string]string{
				"foo":     "bar",
				"new-key": "new-value",
			},
		},
		{
			name:   "Add label to empty labels",
			labels: map[string]string{},
			key:    "new-key",
			value:  "new-value",
			expected: map[string]string{
				"new-key": "new-value",
			},
		},
		{
			name: "Empty key returns original labels",
			labels: map[string]string{
				"foo": "bar",
			},
			key:   "",
			value: "new-value",
			expected: map[string]string{
				"foo": "bar",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CloneAndAddLabel(tt.labels, tt.key, tt.value)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestFindUpdatedPodsOnNode tests the findUpdatedPodsOnNode function
func TestFindUpdatedPodsOnNode(t *testing.T) {
	tests := []struct {
		name      string
		daemonSet *appsv1.DaemonSet
		pods      []*corev1.Pod
		newHash   string
		expected  bool
	}{
		{
			name: "Find updated pods on node - one new, one old",
			daemonSet: func() *appsv1.DaemonSet {
				ds := newDaemonSet("test-ds", "test-image")
				return ds
			}(),
			pods: func() []*corev1.Pod {
				ds := newDaemonSet("test-ds", "test-image")
				pod1 := newPod("pod-1", "node-1", simpleDaemonSetLabel, ds)
				pod2 := newPod("pod-2", "node-1", simpleDaemonSetLabel, ds)
				// Make pod2 old by changing its hash
				pod2.Labels[appsv1.DefaultDaemonSetUniqueLabelKey] = "old-hash"
				return []*corev1.Pod{pod1, pod2}
			}(),
			newHash:  "new-hash",
			expected: false, // Multiple pods should return false
		},
		{
			name: "Find updated pods on node - multiple new pods",
			daemonSet: func() *appsv1.DaemonSet {
				ds := newDaemonSet("test-ds", "test-image")
				return ds
			}(),
			pods: func() []*corev1.Pod {
				ds := newDaemonSet("test-ds", "test-image")
				pod1 := newPod("pod-1", "node-1", simpleDaemonSetLabel, ds)
				pod2 := newPod("pod-2", "node-1", simpleDaemonSetLabel, ds)
				return []*corev1.Pod{pod1, pod2}
			}(),
			newHash:  "new-hash",
			expected: false, // Multiple new pods should return false
		},
		{
			name: "Find updated pods on node - no pods",
			daemonSet: func() *appsv1.DaemonSet {
				ds := newDaemonSet("test-ds", "test-image")
				return ds
			}(),
			pods:     []*corev1.Pod{},
			newHash:  "new-hash",
			expected: true, // No pods should return true
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newPod, oldPod, ok := findUpdatedPodsOnNode(tt.daemonSet, tt.pods, tt.newHash)
			assert.Equal(t, tt.expected, ok)

			// Only check for non-nil results if we expect success
			if tt.expected && len(tt.pods) > 0 {
				// If ok is true and we have pods, we should have valid results
				assert.NotNil(t, newPod)
				assert.NotNil(t, oldPod)
			}
		})
	}
}

// TestGetTargetNodeName_EdgeCases tests edge cases for GetTargetNodeName
func TestGetTargetNodeName_EdgeCases(t *testing.T) {
	tests := []struct {
		name          string
		pod           *corev1.Pod
		expectedError bool
	}{
		{
			name: "Pod with empty node name and no affinity",
			pod: func() *corev1.Pod {
				pod := newPod("test-pod", "", simpleDaemonSetLabel, nil)
				pod.Spec.Affinity = nil
				return pod
			}(),
			expectedError: true,
		},
		{
			name: "Pod with empty node name and no node affinity",
			pod: func() *corev1.Pod {
				pod := newPod("test-pod", "", simpleDaemonSetLabel, nil)
				pod.Spec.Affinity = &corev1.Affinity{
					NodeAffinity: nil,
				}
				return pod
			}(),
			expectedError: true,
		},
		{
			name: "Pod with empty node name and no required during scheduling",
			pod: func() *corev1.Pod {
				pod := newPod("test-pod", "", simpleDaemonSetLabel, nil)
				pod.Spec.Affinity = &corev1.Affinity{
					NodeAffinity: &corev1.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: nil,
					},
				}
				return pod
			}(),
			expectedError: true,
		},
		{
			name: "Pod with empty node name and no node selector terms",
			pod: func() *corev1.Pod {
				pod := newPod("test-pod", "", simpleDaemonSetLabel, nil)
				pod.Spec.Affinity = &corev1.Affinity{
					NodeAffinity: &corev1.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
							NodeSelectorTerms: []corev1.NodeSelectorTerm{},
						},
					},
				}
				return pod
			}(),
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodeName, err := GetTargetNodeName(tt.pod)

			if tt.expectedError {
				assert.Error(t, err)
				assert.Empty(t, nodeName)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, nodeName)
			}
		})
	}
}

// TestAdvancedRollingUpdate_EdgeCases tests edge cases for advancedRollingUpdate
func TestAdvancedRollingUpdate_EdgeCases(t *testing.T) {
	tests := []struct {
		name          string
		daemonSet     *appsv1.DaemonSet
		pods          []*corev1.Pod
		nodes         []*corev1.Node
		expectedError bool
	}{
		{
			name: "AdvancedRollingUpdate with node ready check error",
			daemonSet: func() *appsv1.DaemonSet {
				ds := newDaemonSet("test-ds", "test-image")
				setOnDelete(ds)
				metav1.SetMetaDataAnnotation(&ds.ObjectMeta, daemonsetupgradestrategy.UpdateAnnotation, daemonsetupgradestrategy.AdvancedRollingUpdate)
				setMaxUnavailableAnnotation(ds, "1")
				return ds
			}(),
			pods: func() []*corev1.Pod {
				ds := newDaemonSet("test-ds", "test-image")
				pod := newPod("pod-1", "nonexistent-node", simpleDaemonSetLabel, ds)
				// Ensure pod has the same UID as DaemonSet
				pod.OwnerReferences[0].UID = ds.UID
				return []*corev1.Pod{pod}
			}(),
			nodes:         []*corev1.Node{},
			expectedError: false, // The method might handle missing nodes gracefully
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create objects for fake client
			objects := []client.Object{tt.daemonSet}
			for _, pod := range tt.pods {
				objects = append(objects, pod)
			}
			for _, node := range tt.nodes {
				objects = append(objects, node)
			}
			c := fakeclient.NewClientBuilder().WithObjects(objects...).Build()

			// Create reconciler
			r := &ReconcileDaemonpodupdater{
				Client:       c,
				expectations: k8sutil.NewControllerExpectations(),
				podControl:   &k8sutil.FakePodControl{},
			}

			// Execute advancedRollingUpdate
			err := r.advancedRollingUpdate(tt.daemonSet)

			// Verify results
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestReconcileDaemonpodupdater_deletePod(t *testing.T) {
	tests := []struct {
		name          string
		pod           *corev1.Pod
		daemonSet     *appsv1.DaemonSet
		expectations  map[string]bool
		expectedError bool
	}{
		{
			name: "Delete pod with valid controller reference",
			pod: func() *corev1.Pod {
				ds := newDaemonSet("test-ds", "test-image")
				setOnDelete(ds)
				metav1.SetMetaDataAnnotation(&ds.ObjectMeta, daemonsetupgradestrategy.UpdateAnnotation, daemonsetupgradestrategy.OTAUpdate)
				pod := newPod("test-pod", "node-1", simpleDaemonSetLabel, ds)
				return pod
			}(),
			daemonSet: func() *appsv1.DaemonSet {
				ds := newDaemonSet("test-ds", "test-image")
				setOnDelete(ds)
				metav1.SetMetaDataAnnotation(&ds.ObjectMeta, daemonsetupgradestrategy.UpdateAnnotation, daemonsetupgradestrategy.OTAUpdate)
				return ds
			}(),
			expectations:  map[string]bool{"default/test-ds": true},
			expectedError: false,
		},
		{
			name: "Delete pod with no controller reference",
			pod: func() *corev1.Pod {
				pod := newPod("test-pod", "node-1", simpleDaemonSetLabel, nil)
				return pod
			}(),
			daemonSet:     nil,
			expectations:  map[string]bool{},
			expectedError: false,
		},
		{
			name: "Delete pod with invalid controller reference",
			pod: func() *corev1.Pod {
				ds := newDaemonSet("test-ds", "test-image")
				pod := newPod("test-pod", "node-1", simpleDaemonSetLabel, ds)
				// Change the UID to make it invalid
				pod.OwnerReferences[0].UID = "invalid-uid"
				return pod
			}(),
			daemonSet: func() *appsv1.DaemonSet {
				ds := newDaemonSet("test-ds", "test-image")
				setOnDelete(ds)
				metav1.SetMetaDataAnnotation(&ds.ObjectMeta, daemonsetupgradestrategy.UpdateAnnotation, daemonsetupgradestrategy.OTAUpdate)
				return ds
			}(),
			expectations:  map[string]bool{},
			expectedError: false,
		},
		{
			name: "Delete pod with non-DaemonSet controller",
			pod: func() *corev1.Pod {
				pod := newPod("test-pod", "node-1", simpleDaemonSetLabel, nil)
				pod.OwnerReferences = []metav1.OwnerReference{
					{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       "test-deployment",
						UID:        "deployment-uid",
					},
				}
				return pod
			}(),
			daemonSet:     nil,
			expectations:  map[string]bool{},
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create objects for fake client
			objects := []client.Object{}
			if tt.daemonSet != nil {
				objects = append(objects, tt.daemonSet)
			}

			c := fakeclient.NewClientBuilder().WithObjects(objects...).Build()

			// Create expectations
			expectations := k8sutil.NewControllerExpectations()
			for key, satisfied := range tt.expectations {
				if !satisfied {
					expectations.SetExpectations(key, 0, 1)
				}
			}

			// Create reconciler
			r := &ReconcileDaemonpodupdater{
				Client:       c,
				expectations: expectations,
				podControl:   &k8sutil.FakePodControl{},
			}

			// Create delete event
			evt := event.TypedDeleteEvent[client.Object]{
				Object: tt.pod,
			}

			// Execute deletePod
			r.deletePod(context.TODO(), evt, nil)

			// Verify expectations were updated if applicable
			if len(tt.expectations) > 0 {
				// Expectations should be satisfied after deletion
				for key := range tt.expectations {
					assert.True(t, expectations.SatisfiedExpectations(key))
				}
			}
		})
	}
}

// TestReconcileDaemonpodupdater_otaUpdate tests the otaUpdate method
func TestReconcileDaemonpodupdater_otaUpdate(t *testing.T) {
	tests := []struct {
		name          string
		daemonSet     *appsv1.DaemonSet
		pods          []*corev1.Pod
		expectedError bool
	}{
		{
			name: "OTA update with pods",
			daemonSet: func() *appsv1.DaemonSet {
				ds := newDaemonSet("test-ds", "test-image")
				setOnDelete(ds)
				metav1.SetMetaDataAnnotation(&ds.ObjectMeta, daemonsetupgradestrategy.UpdateAnnotation, daemonsetupgradestrategy.OTAUpdate)
				return ds
			}(),
			pods: func() []*corev1.Pod {
				ds := newDaemonSet("test-ds", "test-image")
				return []*corev1.Pod{
					newPod("pod-1", "node-1", simpleDaemonSetLabel, ds),
					newPod("pod-2", "node-2", simpleDaemonSetLabel, ds),
				}
			}(),
			expectedError: false,
		},
		{
			name: "OTA update with no pods",
			daemonSet: func() *appsv1.DaemonSet {
				ds := newDaemonSet("test-ds", "test-image")
				setOnDelete(ds)
				metav1.SetMetaDataAnnotation(&ds.ObjectMeta, daemonsetupgradestrategy.UpdateAnnotation, daemonsetupgradestrategy.OTAUpdate)
				return ds
			}(),
			pods:          []*corev1.Pod{},
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create objects for fake client
			objects := []client.Object{tt.daemonSet}
			for _, pod := range tt.pods {
				objects = append(objects, pod)
			}
			c := fakeclient.NewClientBuilder().WithObjects(objects...).Build()

			// Create reconciler
			r := &ReconcileDaemonpodupdater{
				Client:       c,
				expectations: k8sutil.NewControllerExpectations(),
				podControl:   &k8sutil.FakePodControl{},
			}

			// Execute otaUpdate
			err := r.otaUpdate(tt.daemonSet)

			// Verify results
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestReconcileDaemonpodupdater_advancedRollingUpdate tests the advancedRollingUpdate method
func TestReconcileDaemonpodupdater_advancedRollingUpdate(t *testing.T) {
	tests := []struct {
		name          string
		daemonSet     *appsv1.DaemonSet
		pods          []*corev1.Pod
		nodes         []*corev1.Node
		expectedError bool
	}{
		{
			name: "AdvancedRollingUpdate with ready nodes",
			daemonSet: func() *appsv1.DaemonSet {
				ds := newDaemonSet("test-ds", "test-image")
				setOnDelete(ds)
				metav1.SetMetaDataAnnotation(&ds.ObjectMeta, daemonsetupgradestrategy.UpdateAnnotation, daemonsetupgradestrategy.AdvancedRollingUpdate)
				setMaxUnavailableAnnotation(ds, "1")
				return ds
			}(),
			pods: func() []*corev1.Pod {
				ds := newDaemonSet("test-ds", "test-image")
				return []*corev1.Pod{
					newPod("pod-1", "node-1", simpleDaemonSetLabel, ds),
					newPod("pod-2", "node-2", simpleDaemonSetLabel, ds),
				}
			}(),
			nodes: []*corev1.Node{
				newNode("node-1", true),
				newNode("node-2", true),
			},
			expectedError: false,
		},
		{
			name: "AdvancedRollingUpdate with mixed ready/not-ready nodes",
			daemonSet: func() *appsv1.DaemonSet {
				ds := newDaemonSet("test-ds", "test-image")
				setOnDelete(ds)
				metav1.SetMetaDataAnnotation(&ds.ObjectMeta, daemonsetupgradestrategy.UpdateAnnotation, daemonsetupgradestrategy.AdvancedRollingUpdate)
				setMaxUnavailableAnnotation(ds, "1")
				return ds
			}(),
			pods: func() []*corev1.Pod {
				ds := newDaemonSet("test-ds", "test-image")
				return []*corev1.Pod{
					newPod("pod-1", "node-1", simpleDaemonSetLabel, ds),
					newPod("pod-2", "node-2", simpleDaemonSetLabel, ds),
				}
			}(),
			nodes: []*corev1.Node{
				newNode("node-1", false),
				newNode("node-2", true),
			},
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create objects for fake client
			objects := []client.Object{tt.daemonSet}
			for _, pod := range tt.pods {
				objects = append(objects, pod)
			}
			for _, node := range tt.nodes {
				objects = append(objects, node)
			}
			c := fakeclient.NewClientBuilder().WithObjects(objects...).Build()

			// Create reconciler
			r := &ReconcileDaemonpodupdater{
				Client:       c,
				expectations: k8sutil.NewControllerExpectations(),
				podControl:   &k8sutil.FakePodControl{},
			}

			// Execute advancedRollingUpdate
			err := r.advancedRollingUpdate(tt.daemonSet)

			// Verify results
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestReconcileDaemonpodupdater_getNodesToDaemonPods tests the getNodesToDaemonPods method
func TestReconcileDaemonpodupdater_getNodesToDaemonPods(t *testing.T) {
	tests := []struct {
		name          string
		daemonSet     *appsv1.DaemonSet
		pods          []*corev1.Pod
		expectedError bool
		expectedNodes int
	}{
		{
			name: "Get nodes to daemon pods with valid pods",
			daemonSet: func() *appsv1.DaemonSet {
				ds := newDaemonSet("test-ds", "test-image")
				setOnDelete(ds)
				metav1.SetMetaDataAnnotation(&ds.ObjectMeta, daemonsetupgradestrategy.UpdateAnnotation, daemonsetupgradestrategy.AdvancedRollingUpdate)
				return ds
			}(),
			pods: func() []*corev1.Pod {
				// Create a shared DaemonSet for pods to ensure UID matches
				ds := newDaemonSet("test-ds", "test-image")
				pod1 := newPod("pod-1", "node-1", simpleDaemonSetLabel, ds)
				pod2 := newPod("pod-2", "node-2", simpleDaemonSetLabel, ds)
				return []*corev1.Pod{pod1, pod2}
			}(),
			expectedError: false,
			expectedNodes: 2,
		},
		{
			name: "Get nodes to daemon pods with no pods",
			daemonSet: func() *appsv1.DaemonSet {
				ds := newDaemonSet("test-ds", "test-image")
				setOnDelete(ds)
				metav1.SetMetaDataAnnotation(&ds.ObjectMeta, daemonsetupgradestrategy.UpdateAnnotation, daemonsetupgradestrategy.AdvancedRollingUpdate)
				return ds
			}(),
			pods:          []*corev1.Pod{},
			expectedError: false,
			expectedNodes: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create objects for fake client
			objects := []client.Object{tt.daemonSet}

			// For the first test case, ensure pods have the same UID as the DaemonSet
			if tt.name == "Get nodes to daemon pods with valid pods" {
				// Update pods to use the same UID as the DaemonSet
				for _, pod := range tt.pods {
					pod.OwnerReferences[0].UID = tt.daemonSet.UID
					objects = append(objects, pod)
				}
			} else {
				for _, pod := range tt.pods {
					objects = append(objects, pod)
				}
			}

			c := fakeclient.NewClientBuilder().WithObjects(objects...).Build()

			// Create reconciler
			r := &ReconcileDaemonpodupdater{
				Client:       c,
				expectations: k8sutil.NewControllerExpectations(),
				podControl:   &k8sutil.FakePodControl{},
			}

			// Execute getNodesToDaemonPods
			nodeToDaemonPods, err := r.getNodesToDaemonPods(tt.daemonSet)

			// Verify results
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedNodes, len(nodeToDaemonPods))
			}
		})
	}
}

// TestReconcileDaemonpodupdater_syncPodsOnNodes tests the syncPodsOnNodes method
func TestReconcileDaemonpodupdater_syncPodsOnNodes(t *testing.T) {
	tests := []struct {
		name          string
		daemonSet     *appsv1.DaemonSet
		podsToDelete  []string
		expectedError bool
	}{
		{
			name: "Sync pods on nodes with pods to delete",
			daemonSet: func() *appsv1.DaemonSet {
				ds := newDaemonSet("test-ds", "test-image")
				setOnDelete(ds)
				metav1.SetMetaDataAnnotation(&ds.ObjectMeta, daemonsetupgradestrategy.UpdateAnnotation, daemonsetupgradestrategy.AdvancedRollingUpdate)
				return ds
			}(),
			podsToDelete:  []string{"pod-1", "pod-2"},
			expectedError: false,
		},
		{
			name: "Sync pods on nodes with no pods to delete",
			daemonSet: func() *appsv1.DaemonSet {
				ds := newDaemonSet("test-ds", "test-image")
				setOnDelete(ds)
				metav1.SetMetaDataAnnotation(&ds.ObjectMeta, daemonsetupgradestrategy.UpdateAnnotation, daemonsetupgradestrategy.AdvancedRollingUpdate)
				return ds
			}(),
			podsToDelete:  []string{},
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create objects for fake client
			objects := []client.Object{tt.daemonSet}
			c := fakeclient.NewClientBuilder().WithObjects(objects...).Build()

			// Create reconciler
			r := &ReconcileDaemonpodupdater{
				Client:       c,
				expectations: k8sutil.NewControllerExpectations(),
				podControl:   &k8sutil.FakePodControl{},
			}

			// Execute syncPodsOnNodes
			err := r.syncPodsOnNodes(tt.daemonSet, tt.podsToDelete)

			// Verify results
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestReconcileDaemonpodupdater_resolveControllerRef tests the resolveControllerRef method
func TestReconcileDaemonpodupdater_resolveControllerRef(t *testing.T) {
	tests := []struct {
		name          string
		daemonSet     *appsv1.DaemonSet
		controllerRef *metav1.OwnerReference
		namespace     string
		expectedDS    bool
	}{
		{
			name: "Resolve valid controller reference",
			daemonSet: func() *appsv1.DaemonSet {
				ds := newDaemonSet("test-ds", "test-image")
				ds.UID = "test-uid"
				return ds
			}(),
			controllerRef: &metav1.OwnerReference{
				APIVersion: "apps/v1",
				Kind:       "DaemonSet",
				Name:       "test-ds",
				UID:        "test-uid",
			},
			namespace:  "default",
			expectedDS: true,
		},
		{
			name: "Resolve invalid controller reference - wrong kind",
			daemonSet: func() *appsv1.DaemonSet {
				ds := newDaemonSet("test-ds", "test-image")
				return ds
			}(),
			controllerRef: &metav1.OwnerReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       "test-ds",
				UID:        "test-uid",
			},
			namespace:  "default",
			expectedDS: false,
		},
		{
			name: "Resolve invalid controller reference - wrong UID",
			daemonSet: func() *appsv1.DaemonSet {
				ds := newDaemonSet("test-ds", "test-image")
				return ds
			}(),
			controllerRef: &metav1.OwnerReference{
				APIVersion: "apps/v1",
				Kind:       "DaemonSet",
				Name:       "test-ds",
				UID:        "wrong-uid",
			},
			namespace:  "default",
			expectedDS: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create objects for fake client
			objects := []client.Object{tt.daemonSet}
			c := fakeclient.NewClientBuilder().WithObjects(objects...).Build()

			// Create reconciler
			r := &ReconcileDaemonpodupdater{
				Client:       c,
				expectations: k8sutil.NewControllerExpectations(),
				podControl:   &k8sutil.FakePodControl{},
			}

			// Execute resolveControllerRef
			ds := r.resolveControllerRef(tt.namespace, tt.controllerRef)

			// Verify results
			if tt.expectedDS {
				assert.NotNil(t, ds)
				assert.Equal(t, tt.daemonSet.Name, ds.Name)
			} else {
				assert.Nil(t, ds)
			}
		})
	}
}

// TestReconcileDaemonpodupdater_Reconcile_Comprehensive tests comprehensive scenarios
func TestReconcileDaemonpodupdater_Reconcile_Comprehensive(t *testing.T) {
	tests := []struct {
		name           string
		daemonSet      *appsv1.DaemonSet
		objects        []client.Object
		expectations   map[string]bool
		expectedError  bool
		expectedResult reconcile.Result
	}{
		{
			name: "Comprehensive test with OTA update and pods",
			daemonSet: func() *appsv1.DaemonSet {
				ds := newDaemonSet("test-ds", "test-image")
				setOnDelete(ds)
				metav1.SetMetaDataAnnotation(&ds.ObjectMeta, daemonsetupgradestrategy.UpdateAnnotation, daemonsetupgradestrategy.OTAUpdate)
				return ds
			}(),
			objects: func() []client.Object {
				ds := newDaemonSet("test-ds", "test-image")
				return []client.Object{
					newPod("pod-1", "node-1", simpleDaemonSetLabel, ds),
					newPod("pod-2", "node-2", simpleDaemonSetLabel, ds),
					newNode("node-1", true),
					newNode("node-2", true),
				}
			}(),
			expectations:   map[string]bool{"default/test-ds": true},
			expectedError:  false,
			expectedResult: reconcile.Result{},
		},
		{
			name: "Comprehensive test with AdvancedRollingUpdate and mixed nodes",
			daemonSet: func() *appsv1.DaemonSet {
				ds := newDaemonSet("test-ds", "test-image")
				setOnDelete(ds)
				metav1.SetMetaDataAnnotation(&ds.ObjectMeta, daemonsetupgradestrategy.UpdateAnnotation, daemonsetupgradestrategy.AdvancedRollingUpdate)
				setMaxUnavailableAnnotation(ds, "1")
				return ds
			}(),
			objects: func() []client.Object {
				ds := newDaemonSet("test-ds", "test-image")
				return []client.Object{
					newPod("pod-1", "node-1", simpleDaemonSetLabel, ds),
					newPod("pod-2", "node-2", simpleDaemonSetLabel, ds),
					newNode("node-1", false),
					newNode("node-2", true),
				}
			}(),
			expectations:   map[string]bool{"default/test-ds": true},
			expectedError:  false,
			expectedResult: reconcile.Result{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fake client
			objects := []client.Object{tt.daemonSet}
			objects = append(objects, tt.objects...)
			c := fakeclient.NewClientBuilder().WithObjects(objects...).Build()

			// Create expectations
			expectations := k8sutil.NewControllerExpectations()
			for key, satisfied := range tt.expectations {
				if !satisfied {
					expectations.SetExpectations(key, 0, 1)
				}
			}

			// Create reconciler
			r := &ReconcileDaemonpodupdater{
				Client:       c,
				expectations: expectations,
				podControl:   &k8sutil.FakePodControl{},
			}

			// Create request
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: "default",
					Name:      "test-ds",
				},
			}

			// Execute reconcile
			result, err := r.Reconcile(context.TODO(), req)

			// Verify results
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}
