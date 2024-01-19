/*
Copyright 2024 The OpenYurt Authors.

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

package yurtappset

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/openyurtio/openyurt/pkg/apis"
	yurtapps "github.com/openyurtio/openyurt/pkg/apis/apps"
	"github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
	beta1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
)

const (
	failed  = "\u2717"
	succeed = "\u2713"
)

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

func TestGetYurtAppSetPatch(t *testing.T) {

	type test struct {
		name string
		yas  *beta1.YurtAppSet
		err  bool
	}

	var tests = []test{
		{
			name: "Yas is nil",
			yas:  nil,
			err:  true,
		},
		{
			name: "No 'spec' field in original data",
			yas:  &beta1.YurtAppSet{},
			err:  false,
		},
		{
			name: "No 'spec.workloadTemplate' & 'spec.workloadTweaks' field in original data",
			yas: &beta1.YurtAppSet{
				Spec: beta1.YurtAppSetSpec{},
			},
			err: false,
		},
		{
			name: "No 'spec.workloadTemplate' field in original data",
			yas: &beta1.YurtAppSet{
				Spec: beta1.YurtAppSetSpec{
					Workload: beta1.Workload{
						WorkloadTweaks: []beta1.WorkloadTweak{
							{
								Pools: []string{"pool1"},
							},
						},
					},
				},
			},
			err: false,
		},
		{
			name: "No 'spec.workloadTweaks' field in original data",
			yas: &beta1.YurtAppSet{
				Spec: beta1.YurtAppSetSpec{
					Workload: beta1.Workload{
						WorkloadTemplate: beta1.WorkloadTemplate{
							DeploymentTemplate: &beta1.DeploymentTemplateSpec{
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
						}},
				},
			},
			err: false,
		},
		{
			name: "All fields exist",
			yas: &beta1.YurtAppSet{
				Spec: beta1.YurtAppSetSpec{
					Workload: beta1.Workload{
						WorkloadTemplate: beta1.WorkloadTemplate{
							DeploymentTemplate: &beta1.DeploymentTemplateSpec{
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
						WorkloadTweaks: []beta1.WorkloadTweak{
							{
								Pools: []string{"pool1"},
							},
						},
					},
				},
			},
			err: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := getYurtAppSetPatch(tt.yas)
			if (err != nil) != tt.err {
				t.Errorf("getYurtAppSetPatch() error = %v, wantErr %v", err, tt.err)
				return
			}
		})
	}
}

func getFakeScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	apis.AddToScheme(scheme)
	apps.AddToScheme(scheme)
	return scheme
}

var fakeScheme = getFakeScheme()

func TestNewRevision(t *testing.T) {

	type args struct {
		yas         *beta1.YurtAppSet
		revision    int64
		collision   *int32
		expectedErr bool
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "Valid YurtAppSet",
			args: args{
				yas: &beta1.YurtAppSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-yurtappset",
						Namespace: "default",
						Labels: map[string]string{
							"a": "a",
						},
					},
					Spec: beta1.YurtAppSetSpec{},
				},
				revision:    1,
				collision:   nil,
				expectedErr: false,
			},
		},
		{
			name: "Error in getting YurtAppSet patch",
			args: args{
				yas:         nil,
				revision:    1,
				collision:   nil,
				expectedErr: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cr, err := newRevision(tt.args.yas, tt.args.revision, tt.args.collision, fakeScheme)
			if tt.args.expectedErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Subset(t, cr.Labels, tt.args.yas.Labels)
			}
		})
	}
}

var (
	yas = beta1.YurtAppSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-yurtappset",
			Namespace: "default",
		},
	}

	cr1 = apps.ControllerRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-yurtappset-",
			Namespace: "default",
			Labels: map[string]string{
				yurtapps.YurtAppSetOwnerLabelKey: "test-yurtappset",
			},
		},
		Data: runtime.RawExtension{
			Raw: []byte(`{"a":"a"}`),
		},
		Revision: 2,
	}

	cr2 = apps.ControllerRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-yurtappset-7fcd4f8557",
			Namespace: "default",
		},
		Data: runtime.RawExtension{
			Raw: []byte(`{"a":"a"}`),
		},
		Revision: 0,
	}

	cr3 = apps.ControllerRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-yurtappset-7fcd4f8557",
			Namespace: "default",
		},
		Data: runtime.RawExtension{
			Raw: []byte(`{"a":"b"}`),
		},
	}

	cr4 = apps.ControllerRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-yurtappset-694bbcc68",
			Namespace: "default",
			Labels: map[string]string{
				yurtapps.YurtAppSetOwnerLabelKey: "test-yurtappset",
			},
		},
		Data: runtime.RawExtension{
			Raw: []byte("{\"spec\":{\"workload\":{\"$patch\":\"replace\",\"workloadTemplate\":{}}}}"),
		},
		Revision: 1,
	}

	cr5 = apps.ControllerRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-yurtappset-694bbcc68",
			Namespace: "default",
			Labels: map[string]string{
				yurtapps.YurtAppSetOwnerLabelKey: "test-yurtappset",
			},
		},
		Data: runtime.RawExtension{
			Raw: []byte("{\"spec\":{\"workload\":{\"$patch\":\"replace\",\"workloadTemplate\":{}}}}"),
		},
		Revision: 0,
	}
)

func TestCreateControllerRevision(t *testing.T) {

	collisionCount := int32(0)

	tests := []struct {
		name      string
		cli       client.Client
		yas       *beta1.YurtAppSet
		revision  *apps.ControllerRevision
		collision *int32
		err       bool
	}{
		{
			name: "CollisionCount is nil",
			yas: &beta1.YurtAppSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-yurtappset",
					Namespace: "default",
				},
			},
			revision:  nil,
			collision: nil,
			err:       true,
		},
		{
			name: "create success without error",
			yas: &beta1.YurtAppSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-yurtappset",
					Namespace: "default",
				},
			},
			revision:  cr1.DeepCopy(),
			collision: &collisionCount,
			err:       false,
			cli:       fake.NewClientBuilder().WithScheme(fakeScheme).WithObjects().Build(),
		},
		{
			name: "create success with already exist",
			yas: &beta1.YurtAppSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-yurtappset",
					Namespace: "default",
				},
			},
			revision:  cr1.DeepCopy(),
			collision: &collisionCount,
			err:       false,
			cli:       fake.NewClientBuilder().WithScheme(fakeScheme).WithObjects(cr2.DeepCopy()).Build(),
		},
		{
			name: "create success with already exist and collison occurs",
			yas: &beta1.YurtAppSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-yurtappset",
					Namespace: "default",
				},
			},
			revision:  cr1.DeepCopy(),
			collision: &collisionCount,
			err:       false,
			cli:       fake.NewClientBuilder().WithScheme(fakeScheme).WithObjects(cr3.DeepCopy()).Build(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := createControllerRevision(tt.cli, tt.yas, tt.revision, tt.collision)
			if !tt.err {
				assert.NoError(t, err)
			}
		})
	}

}

func TestCleanRevisions(t *testing.T) {

	itemRevisionHistoryLimit := int32(0)

	tests := []struct {
		name      string
		cli       client.Client
		yas       *beta1.YurtAppSet
		revisions []*apps.ControllerRevision
		err       bool
	}{
		{
			name: "clean success and clear success",
			yas: &beta1.YurtAppSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-yurtappset",
					Namespace: "default",
				},
				Spec: v1beta1.YurtAppSetSpec{
					RevisionHistoryLimit: &itemRevisionHistoryLimit,
				},
			},
			revisions: []*apps.ControllerRevision{
				cr1.DeepCopy(), cr2.DeepCopy(),
			},
			err: false,
			cli: fake.NewClientBuilder().WithScheme(fakeScheme).WithObjects(
				cr1.DeepCopy(),
				cr2.DeepCopy(),
			).Build(),
		},
		{
			name: "clean success with yas revisionHistoryLimit is nil",
			yas: &beta1.YurtAppSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-yurtappset",
					Namespace: "default",
				},
			},
			revisions: []*apps.ControllerRevision{
				cr1.DeepCopy(), cr2.DeepCopy(),
			},
			err: false,
			cli: fake.NewClientBuilder().WithScheme(fakeScheme).WithObjects(
				cr1.DeepCopy(),
				cr2.DeepCopy(),
			).Build(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := cleanRevisions(tt.cli, tt.yas, tt.revisions)
			if !tt.err {
				assert.NoError(t, err)
			}
		})
	}
}

func TestControlledHistories(t *testing.T) {
	// cr_should_adopt is a cr has owner label but doesnot has owner reference
	cr_should_adopt := apps.ControllerRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-yurtappset-1",
			Namespace: "default",
			Labels:    map[string]string{yurtapps.YurtAppSetOwnerLabelKey: "test-yurtappset"},
		},
		Data: runtime.RawExtension{
			Raw: []byte(`{"a":"a"}`),
		},
	}
	// cr_should_release is a cr not has owner label but has owner reference
	cr_should_release := apps.ControllerRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-yurtappset-2",
			Namespace: "default",
		},
		Data: runtime.RawExtension{
			Raw: []byte(`{"a":"a"}`),
		},
		Revision: 0,
	}
	controllerutil.SetControllerReference(&yas, &cr_should_release, fakeScheme)

	crs, err := controlledHistories(fake.NewClientBuilder().WithScheme(fakeScheme).WithObjects(
		&cr_should_adopt,
		&cr_should_release,
		&yas,
	).Build(), fakeScheme, &yas)
	assert.NoError(t, err)

	assert.Len(t, crs, 1)
	assert.Equal(t, cr_should_adopt.Name, crs[0].Name)
}

func TestConstructYurtAppSetRevisions(t *testing.T) {
	tests := []struct {
		name string
		yas  *beta1.YurtAppSet
		cli  client.Client
		err  bool
	}{
		{
			name: "construct success and create new revision",
			yas:  &yas,
			cli:  fake.NewClientBuilder().WithScheme(fakeScheme).WithObjects(cr1.DeepCopy(), cr2.DeepCopy(), &yas).Build(),
			err:  false,
		},
		{
			name: "construct success and update old revision",
			yas:  &yas,
			cli:  fake.NewClientBuilder().WithScheme(fakeScheme).WithObjects(cr1.DeepCopy(), cr4.DeepCopy(), &yas).Build(),
			err:  false,
		},
		{
			name: "construct success and reuse invalid revision",
			yas:  &yas,
			cli:  fake.NewClientBuilder().WithScheme(fakeScheme).WithObjects(cr1.DeepCopy(), cr5.DeepCopy(), &yas).Build(),
			err:  false,
		},
		{
			name: "construct success and no need to update old revision",
			yas:  &yas,
			cli:  fake.NewClientBuilder().WithScheme(fakeScheme).WithObjects(cr2.DeepCopy(), cr4.DeepCopy(), &yas).Build(),
			err:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testReconcile := &ReconcileYurtAppSet{
				Client: tt.cli,
				scheme: fakeScheme,
			}
			_, _, _, err := testReconcile.constructYurtAppSetRevisions(tt.yas)
			if !tt.err {
				assert.NoError(t, err)
			}
		})
	}
}
