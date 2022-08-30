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
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
)

func TestGetNodePods(t *testing.T) {
	node := newNode("node1")
	pod1 := newPod("test-pod1", "node1", simpleDaemonSetLabel, nil)
	pod2 := newPod("test-pod2", "node2", simpleDaemonSetLabel, nil)

	expectPods := []*corev1.Pod{pod1}
	clientset := fake.NewSimpleClientset(node, pod1, pod2)
	podInformer := informers.NewSharedInformerFactory(clientset, 0)

	podInformer.Core().V1().Pods().Informer().GetIndexer().Add(pod1)
	podInformer.Core().V1().Pods().Informer().GetIndexer().Add(pod2)

	gotPods, err := GetNodePods(podInformer.Core().V1().Pods().Lister(), node)

	assert.Equal(t, nil, err)
	assert.Equal(t, expectPods, gotPods)
}

func TestGetDaemonsetPods(t *testing.T) {
	ds1 := newDaemonSet("daemosnet1", "foo/bar:v1")

	pod1 := newPod("pod1", "", simpleDaemonSetLabel, ds1)
	pod2 := newPod("pod2", "", simpleDaemonSetLabel, nil)

	expectPods := []*corev1.Pod{pod1}
	clientset := fake.NewSimpleClientset(ds1, pod1, pod2)
	podInformer := informers.NewSharedInformerFactory(clientset, 0)

	podInformer.Core().V1().Pods().Informer().GetIndexer().Add(pod1)
	podInformer.Core().V1().Pods().Informer().GetIndexer().Add(pod2)

	gotPods, err := GetDaemonsetPods(podInformer.Core().V1().Pods().Lister(), ds1)

	assert.Equal(t, nil, err)
	assert.Equal(t, expectPods, gotPods)
}

func TestIsDaemonsetPodLatest(t *testing.T) {
	daemosnetV1 := newDaemonSet("daemonset", "foo/bar:v1")
	daemosnetV2 := daemosnetV1.DeepCopy()
	daemosnetV2.Spec.Template.Spec.Containers[0].Image = "foo/bar:v2"

	tests := []struct {
		name       string
		ds         *appsv1.DaemonSet
		pod        *corev1.Pod
		wantLatest bool
	}{
		{
			name:       "latest",
			ds:         daemosnetV1,
			pod:        newPod("pod", "", simpleDaemonSetLabel, daemosnetV1),
			wantLatest: true,
		},
		{
			name:       "not latest",
			ds:         daemosnetV2,
			pod:        newPod("pod", "", simpleDaemonSetLabel, daemosnetV1),
			wantLatest: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotLatest, _ := IsDaemonsetPodLatest(tt.ds, tt.pod)
			assert.Equal(t, tt.wantLatest, gotLatest)
		})
	}
}
