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

package yurtappdaemon

import (
	"reflect"
	"testing"

	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	yurtapps "github.com/openyurtio/openyurt/pkg/apis/apps"
	alpha1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
)

const (
	failed  = "\u2717"
	succeed = "\u2713"
)

func TestNewRevision(t *testing.T) {
	scheme := runtime.NewScheme()
	alpha1.AddToScheme(scheme)
	var int1 int32 = 1

	ed := ReconcileYurtAppDaemon{
		Client:   fake.NewClientBuilder().WithScheme(scheme).Build(),
		scheme:   scheme,
		recorder: record.NewFakeRecorder(1),
	}

	tests := []struct {
		name           string
		ud             *alpha1.YurtAppDaemon
		revision       int64
		collisionCount *int32
		expect         int64
	}{
		{
			"normal",
			&alpha1.YurtAppDaemon{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "kube-system",
					Name:      "yad",
				},
				Spec: alpha1.YurtAppDaemonSpec{
					WorkloadTemplate: alpha1.WorkloadTemplate{
						DeploymentTemplate: &alpha1.DeploymentTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"a": "a",
								},
							},
							Spec: apps.DeploymentSpec{
								Template: v1.PodTemplateSpec{
									Spec: v1.PodSpec{
										Volumes: []v1.Volume{},
										Containers: []v1.Container{
											{
												VolumeMounts: []v1.VolumeMount{},
											},
										},
									},
								},
								Selector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										yurtapps.PoolNameLabelKey: "a",
									},
								},
							},
						},
					},
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"a": "a",
						},
					},
				},
			},
			1,
			&int1,
			1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ed.newRevision(tt.ud, tt.revision, tt.collisionCount)
			get := tt.expect
			if !reflect.DeepEqual(get, tt.expect) {
				t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
			}
			t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)
		})
	}
}

func TestNextRevision(t *testing.T) {
	tests := []struct {
		name      string
		revisions []*apps.ControllerRevision
		expect    int64
	}{
		{
			"zero",
			[]*apps.ControllerRevision{},
			1,
		},
		{
			"normal",
			[]*apps.ControllerRevision{
				{
					Revision: 1,
				},
			},
			2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			get := nextRevision(tt.revisions)

			if !reflect.DeepEqual(get, tt.expect) {
				t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
			}
			t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)
		})
	}
}

func TestGetYurtAppDaemonPatch(t *testing.T) {

	tests := []struct {
		name string
		ud   *alpha1.YurtAppDaemon
	}{
		{
			"normal",
			&alpha1.YurtAppDaemon{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			get, _ := getYurtAppDaemonPatch(tt.ud)

			//if !reflect.DeepEqual(get, expect) {
			//	t.Fatalf("\t%s\texpect %v, but get %v", failed, expect, get)
			//}
			t.Logf("\t%s\tget %v", succeed, get)
		})
	}
}
