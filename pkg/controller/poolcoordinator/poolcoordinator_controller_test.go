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

package poolcoordinator

import (
	"testing"

	coordv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
)

func TestTaintNode(t *testing.T) {
	c := NewController(nil, nil)
	node := &corev1.Node{}
	node = c.doDeTaintNodeNotSchedulable(node)
	if len(node.Spec.Taints) != 0 {
		t.Fail()
	}
	node = c.doTaintNodeNotSchedulable(node)
	node = c.doTaintNodeNotSchedulable(node)
	if len(node.Spec.Taints) == 0 {
		t.Fail()
	}
	node = c.doDeTaintNodeNotSchedulable(node)
	if len(node.Spec.Taints) != 0 {
		t.Fail()
	}
}

func TestOnLeaseCreate(t *testing.T) {
	c := NewController(nil, nil)
	l := &coordv1.Lease{}
	c.onLeaseCreate(l)
	l.Namespace = corev1.NamespaceNodeLease
	l.Name = "ai-ice-vm05"
	c.onLeaseCreate(l)
}

func TestOnLeaseUpdate(t *testing.T) {
	c := NewController(nil, nil)
	l := &coordv1.Lease{}
	c.onLeaseUpdate(l, l)
	l.Namespace = corev1.NamespaceNodeLease
	l.Name = "ai-ice-vm05"
	c.onLeaseUpdate(l, l)
}
