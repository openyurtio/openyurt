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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
)

func TestGetNodePods(t *testing.T) {
	// Note: fake client does not support filtering by field selector, just ignore
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
