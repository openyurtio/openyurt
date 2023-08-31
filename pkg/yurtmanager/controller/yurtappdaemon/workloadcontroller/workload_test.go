/*
Copyright 2021 The OpenYurt Authors.

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

package workloadcontroller

import (
	"reflect"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	unitv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/apps"
)

func TestGetRevision(t *testing.T) {
	wd := Workload{
		Name: "test",
		Spec: WorkloadSpec{
			Ref: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "kube-system",
					Name:      "kube-proxy",
					Labels: map[string]string{
						unitv1alpha1.ControllerRevisionHashLabelKey: "a",
					},
				},
				Spec: v1.PodSpec{},
			},
		},
	}

	tests := []struct {
		name   string
		expect string
	}{
		{
			"normal",
			"a",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			get := wd.GetRevision()
			t.Logf("get: %s", get)
			if !reflect.DeepEqual(get, tt.expect) {
				t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
			}
			t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)
		})
	}
}

func TestGetNodePoolName(t *testing.T) {
	wd := Workload{
		Name: "test",
		Spec: WorkloadSpec{
			Ref: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "kube-system",
					Name:      "kube-proxy",
					Annotations: map[string]string{
						unitv1alpha1.AnnotationRefNodePool: "a",
					},
				},
				Spec: v1.PodSpec{},
			},
		},
	}

	tests := []struct {
		name   string
		expect string
	}{
		{
			"normal",
			"a",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			get := wd.GetNodePoolName()
			t.Logf("get: %s", get)
			if !reflect.DeepEqual(get, tt.expect) {
				t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
			}
			t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)
		})
	}
}

func TestGetToleration(t *testing.T) {
	wd := Workload{
		Name: "test",
		Spec: WorkloadSpec{
			Ref: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "kube-system",
					Name:      "kube-proxy",
					Annotations: map[string]string{
						unitv1alpha1.AnnotationRefNodePool: "a",
					},
				},
				Spec: v1.PodSpec{},
			},
			Tolerations: []v1.Toleration{
				{
					Key: "a",
				},
			},
		},
	}

	tests := []struct {
		name   string
		expect string
	}{
		{
			"normal",
			"a",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			get := wd.GetToleration()
			t.Logf("get: %s", get[0].Key)
			if !reflect.DeepEqual(get[0].Key, tt.expect) {
				t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, string(get[0].Key))
			}
			t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)
		})
	}
}

func TestGetNodeSelector(t *testing.T) {
	wd := Workload{
		Name: "test",
		Spec: WorkloadSpec{
			Ref: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "kube-system",
					Name:      "kube-proxy",
					Annotations: map[string]string{
						unitv1alpha1.AnnotationRefNodePool: "a",
					},
				},
				Spec: v1.PodSpec{},
			},
			NodeSelector: map[string]string{
				"a": "a",
			},
		},
	}

	tests := []struct {
		name   string
		expect map[string]string
	}{
		{
			"normal",
			map[string]string{
				"a": "a",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			get := wd.GetNodeSelector()
			t.Logf("get: %s", get)
			if !reflect.DeepEqual(get, tt.expect) {
				t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
			}
			t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)
		})
	}
}

func TestGetKind(t *testing.T) {
	wd := Workload{
		Kind: "workload",
		Name: "test",
		Spec: WorkloadSpec{
			Ref: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "kube-system",
					Name:      "kube-proxy",
					Annotations: map[string]string{
						unitv1alpha1.AnnotationRefNodePool: "a",
					},
				},
				Spec: v1.PodSpec{},
			},
			NodeSelector: map[string]string{
				"a": "a",
			},
		},
	}

	tests := []struct {
		name   string
		expect string
	}{
		{
			"normal",
			"workload",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			get := wd.GetKind()
			t.Logf("get: %s", get)
			if !reflect.DeepEqual(get, tt.expect) {
				t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
			}
			t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)
		})
	}
}
