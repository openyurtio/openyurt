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
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	k8sutil "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/daemonpodupdater/kubernetes"
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

func setAutoUpdateAnnotation(ds *appsv1.DaemonSet) {
	metav1.SetMetaDataAnnotation(&ds.ObjectMeta, UpdateAnnotation, AutoUpdate)
}

func setMaxUnavailableAnnotation(ds *appsv1.DaemonSet, v string) {
	metav1.SetMetaDataAnnotation(&ds.ObjectMeta, MaxUnavailableAnnotation, v)
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
			name:           "failed with not OnDelete strategy",
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
		case AutoUpdate, AdvancedRollingUpdate:
			setAutoUpdateAnnotation(ds)
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
