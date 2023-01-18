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

package utils

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestVerifyAgainstWhitelist(t *testing.T) {
	toadd := []corev1.Toleration{
		{Key: "node.kubernetes.io/unreachable",
			Operator:          "Exists",
			Effect:            "NoExecute",
			TolerationSeconds: nil},
	}
	current := []corev1.Toleration{
		{Key: "node.kubernetes.io/unreachable",
			Operator:          "Exists",
			Effect:            "NoExecute",
			TolerationSeconds: nil},
		{Key: "node.kubernetes.io/not-ready",
			Operator:          "Exists",
			Effect:            "NoExecute",
			TolerationSeconds: nil},
	}
	if VerifyAgainstWhitelist(toadd, current) != true {
		t.Fail()
	}
}

func TestMergeTolerations(t *testing.T) {
	toadd := []corev1.Toleration{
		{Key: "node.kubernetes.io/unreachable",
			Operator:          "Exists",
			Effect:            "NoExecute",
			TolerationSeconds: nil},
		{Key: "node.kubernetes.io/not-ready",
			Operator:          "Exists",
			Effect:            "NoExecute",
			TolerationSeconds: nil},
	}
	var ss int64 = 300
	current := []corev1.Toleration{
		{Key: "node.kubernetes.io/unreachable",
			Operator:          "Exists",
			Effect:            "NoExecute",
			TolerationSeconds: &ss},
		{Key: "node.kubernetes.io/not-ready",
			Operator:          "Exists",
			Effect:            "NoExecute",
			TolerationSeconds: &ss},
	}
	r, m := MergeTolerations(toadd, current)
	if m != true {
		t.Fail()
	}
	if reflect.DeepEqual(r, toadd) != true {
		t.Fail()
	}
}

func TestIsSuperset(t *testing.T) {
	t1 := corev1.Toleration{Key: "node.kubernetes.io/unreachable",
		Operator:          "Exists",
		Effect:            "NoExecute",
		TolerationSeconds: nil}
	var ss int64 = 300
	t2 := corev1.Toleration{Key: "node.kubernetes.io/unreachable",
		Operator:          "Exists",
		Effect:            "NoExecute",
		TolerationSeconds: &ss}
	if isSuperset(t1, t2) != true {
		t.Fail()
	}
}
