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
package workloadmanager

import (
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openyurtio/openyurt/pkg/apis/apps"
	"github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
)

var stsYAS = &v1beta1.YurtAppSet{
	ObjectMeta: metav1.ObjectMeta{
		Name: "test-yas",
	},
	Spec: v1beta1.YurtAppSetSpec{
		Pools: []string{"test-nodepool"},
		Workload: v1beta1.Workload{
			WorkloadTemplate: v1beta1.WorkloadTemplate{
				StatefulSetTemplate: &v1beta1.StatefulSetTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-statefulSet",
					},
					Spec: appsv1.StatefulSetSpec{
						Replicas: &itemReplicas,
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app": "test",
							},
						},
						ServiceName: "test-service",
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"app": "test",
								},
							},
							Spec: corev1.PodSpec{
								InitContainers: []corev1.Container{
									{
										Name:  "initContainer",
										Image: "initOld",
									},
								},
								Containers: []corev1.Container{
									{
										Name:  "nginx",
										Image: "nginx",
									},
								},
								Volumes: []corev1.Volume{
									{
										Name: "config",
										VolumeSource: corev1.VolumeSource{
											ConfigMap: &corev1.ConfigMapVolumeSource{
												LocalObjectReference: corev1.LocalObjectReference{
													Name: "configMapSource",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	},
}

var stsNp = &v1beta1.NodePool{
	ObjectMeta: metav1.ObjectMeta{
		Name: "test-nodepool",
	},
	Spec: v1beta1.NodePoolSpec{
		HostNetwork: false,
	},
}

func TestStatefulSetManager(t *testing.T) {
	var fakeScheme = newOpenYurtScheme()
	var fakeClient = fake.NewClientBuilder().WithScheme(fakeScheme).WithObjects(stsYAS, stsNp).Build()

	mgr := &StatefulSetManager{
		Client: fakeClient,
		Scheme: fakeScheme,
	}

	// test create
	err := mgr.Create(stsYAS, "test-nodepool", "test-revision")
	assert.Nil(t, err)

	// test list
	statefulSets, err := mgr.List(stsYAS)
	assert.Nil(t, err)
	assert.Equal(t, len(statefulSets), 1)
	assert.Equal(t, GetWorkloadRefNodePool(statefulSets[0]), "test-nodepool")

	// test update
	err = mgr.Update(stsYAS, statefulSets[0], "test-nodepool", "test-revision-1")
	assert.Nil(t, err)

	statefulSets, err = mgr.List(stsYAS)
	assert.Nil(t, err)
	assert.Equal(t, len(statefulSets), 1)
	assert.Equal(t, statefulSets[0].GetLabels()[apps.ControllerRevisionHashLabelKey], "test-revision-1")

	// test delete
	err = mgr.Delete(stsYAS, statefulSets[0])
	assert.Nil(t, err)

	statefulSets, err = mgr.List(stsYAS)
	assert.Nil(t, err)
	assert.Equal(t, len(statefulSets), 0)
}
