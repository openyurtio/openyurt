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

package utils

import (
	"reflect"
	"sort"
	"testing"
)

func TestNodeMap(t *testing.T) {
	nm := NewNodepoolMap()
	nm.Add("pool1", "node1")
	nm.Add("pool1", "node2")
	nm.Add("pool2", "node3")
	nm.Add("pool2", "node4")
	nm.Add("pool2", "node5")

	if nm.Count("pool1") != 2 {
		t.Errorf("expect %v, but %v returned", 2, nm.Count("pool1"))
	}
	if nm.Count("pool2") != 3 {
		t.Errorf("expect %v, but %v returned", 3, nm.Count("pool2"))
	}
	nm.Del("pool2", "node4")
	if nm.Count("pool2") != 2 {
		t.Errorf("expect %v, but %v returned", 2, nm.Count("pool2"))
	}
	pool, ok := nm.GetPool("node1")
	if !ok {
		t.Errorf("node1's pool should be pool1")
	}
	if pool != "pool1" {
		t.Errorf("node1's pool should be pool1")
	}

	nodes := nm.Nodes("pool2")
	sort.Sort(sort.StringSlice(nodes))
	expected := []string{"node3", "node5"}
	if !reflect.DeepEqual(nodes, expected) {
		t.Errorf("expect %v, but %v returned", expected, nodes)
	}
	nm.DelNode("node3")
	nodes = nm.Nodes("pool2")
	sort.Sort(sort.StringSlice(nodes))
	expected = []string{"node5"}
	if !reflect.DeepEqual(nodes, expected) {
		t.Errorf("expect %v, but %v returned", expected, nodes)
	}
	nm.DelNode("node5")
	nodes = nm.Nodes("pool2")
	sort.Sort(sort.StringSlice(nodes))
	expected = []string{}
	if !reflect.DeepEqual(nodes, expected) {
		t.Errorf("expect %v, but %v returned", expected, nodes)
	}

	nm.Del("pool1", "node1")
	nm.Del("pool1", "node2")
	if nm.Count("pool1") != 0 {
		t.Errorf("expect %v, but %v returned", 0, nm.Count("pool1"))
	}
	nodes = nm.Nodes("pool1")
	expected = []string{}
	if !reflect.DeepEqual(nodes, expected) {
		t.Errorf("expect %v, but %v returned", expected, nodes)
	}

	nm.DelNode("node5")
	nm.DelNode("node1")
	nodes = nm.Nodes("pool2")
	sort.Sort(sort.StringSlice(nodes))
	expected = []string{}
	if !reflect.DeepEqual(nodes, expected) {
		t.Errorf("expect %v, but %v returned", expected, nodes)
	}

}
