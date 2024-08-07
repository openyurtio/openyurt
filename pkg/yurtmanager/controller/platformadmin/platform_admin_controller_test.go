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

package platformadmin

import (
	"context"
	"testing"

	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/platformadmin/config"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	//"k8s.io/apimachinery/pkg/runtime/serializer/json"
	kjson "k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openyurtio/openyurt/pkg/apis"
	"github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
	"github.com/openyurtio/openyurt/pkg/apis/iot/v1alpha2"
)

func getFakeScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	apis.AddToScheme(scheme)
	appsv1.AddToScheme(scheme)
	corev1.AddToScheme(scheme)
	v1beta1.AddToScheme(scheme)
	v1alpha2.AddToScheme(scheme)
	return scheme
}

var fakeScheme = getFakeScheme()

func TestReconcile(t *testing.T) {
	tests := []struct {
		name              string
		configmap         *corev1.ConfigMap
		platformAdmin     *v1alpha2.PlatformAdmin
		yurtAppSet        *v1beta1.YurtAppSet
		expectedExists    bool
		expectedNodePools int
		expectedErr       bool
	}{
		{
			name: "Create YurtAppSet with Single Node Pool",
			configmap: &corev1.ConfigMap{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ComfigMaps",
					APIVersion: "iot.openyurt.io/v1alpha2",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "platformadmin-framework",
					Namespace: "default",
				},
			},
			platformAdmin: &v1alpha2.PlatformAdmin{
				TypeMeta: metav1.TypeMeta{
					Kind:       "PlatformAdmin",
					APIVersion: "iot.openyurt.io/v1alpha2",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-platformadmin",
					Namespace: "default",
				},
				Spec: v1alpha2.PlatformAdminSpec{
					Version:       "minnesota",
					ImageRegistry: "default",
					Pools:         []string{"pool1"},
					Components: []v1alpha2.Component{
						{
							Name: "edgex-core-command",
						},
					},
				},
			},
			yurtAppSet:        nil, // YurtAppSet should be created by Reconcile
			expectedExists:    true,
			expectedNodePools: 1,
			expectedErr:       false,
		},
		// {
		// 	name: "Create YurtAppSet with Multiple Node Pools",
		// 	platformAdmin: &v1alpha2.PlatformAdmin{
		// 		ObjectMeta: metav1.ObjectMeta{
		// 			Name:      "test-platformadmin",
		// 			Namespace: "default",
		// 		},
		// 		Spec: v1alpha2.PlatformAdminSpec{
		// 			Pools: "pool1,pool2",
		// 		},
		// 	},
		// 	yurtAppSet:        nil, // YurtAppSet should be created by Reconcile
		// 	expectedExists:    true,
		// 	expectedNodePools: 2,
		// 	expectedErr:       false,
		// },
		// {
		// 	name: "Delete YurtAppSet",
		// 	platformAdmin: &v1alpha2.PlatformAdmin{
		// 		ObjectMeta: metav1.ObjectMeta{
		// 			Name:      "test-platformadmin",
		// 			Namespace: "default",
		// 		},
		// 		Spec: v1alpha2.PlatformAdminSpec{
		// 			Pools: "", // No pools specified, should delete YurtAppSet
		// 		},
		// 	},
		// 	yurtAppSet: &v1beta1.YurtAppSet{
		// 		ObjectMeta: metav1.ObjectMeta{
		// 			Name:      "test-yurtappset",
		// 			Namespace: "default",
		// 		},
		// 		Spec: v1beta1.YurtAppSetSpec{
		// 			Pools: []string{"pool1"},
		// 			Workload: v1beta1.Workload{
		// 				WorkloadTemplate: v1beta1.WorkloadTemplate{
		// 					DeploymentTemplate: &v1beta1.DeploymentTemplateSpec{
		// 						Spec: appsv1.DeploymentSpec{
		// 							Template: corev1.PodTemplateSpec{
		// 								Spec: corev1.PodSpec{
		// 									Containers: []corev1.Container{
		// 										{
		// 											Name:  "test-container",
		// 											Image: "test-image",
		// 										},
		// 									},
		// 								},
		// 							},
		// 						},
		// 					},
		// 				},
		// 			},
		// 		},
		// 	},
		// 	expectedExists: false,
		// 	expectedErr:    false,
		// },
		// {
		// 	name: "Add Node Pool to Existing YurtAppSet",
		// 	platformAdmin: &v1alpha2.PlatformAdmin{
		// 		ObjectMeta: metav1.ObjectMeta{
		// 			Name:      "test-platformadmin",
		// 			Namespace: "default",
		// 		},
		// 		Spec: v1alpha2.PlatformAdminSpec{
		// 			Pools: "pool1,pool2",
		// 		},
		// 	},
		// 	yurtAppSet: &v1beta1.YurtAppSet{
		// 		ObjectMeta: metav1.ObjectMeta{
		// 			Name:      "test-yurtappset",
		// 			Namespace: "default",
		// 		},
		// 		Spec: v1beta1.YurtAppSetSpec{
		// 			Pools: []string{"pool1"},
		// 			Workload: v1beta1.Workload{
		// 				WorkloadTemplate: v1beta1.WorkloadTemplate{
		// 					DeploymentTemplate: &v1beta1.DeploymentTemplateSpec{
		// 						Spec: appsv1.DeploymentSpec{
		// 							Template: corev1.PodTemplateSpec{
		// 								Spec: corev1.PodSpec{
		// 									Containers: []corev1.Container{
		// 										{
		// 											Name:  "test-container",
		// 											Image: "test-image",
		// 										},
		// 									},
		// 								},
		// 							},
		// 						},
		// 					},
		// 				},
		// 			},
		// 		},
		// 	},
		// 	expectedExists:    true,
		// 	expectedNodePools: 2,
		// 	expectedErr:       false,
		// },
		// {
		// 	name: "Remove Node Pool from Existing YurtAppSet",
		// 	platformAdmin: &v1alpha2.PlatformAdmin{
		// 		ObjectMeta: metav1.ObjectMeta{
		// 			Name:      "test-platformadmin",
		// 			Namespace: "default",
		// 		},
		// 		Spec: v1alpha2.PlatformAdminSpec{
		// 			Pools: "pool1",
		// 		},
		// 	},
		// 	yurtAppSet: &v1beta1.YurtAppSet{
		// 		ObjectMeta: metav1.ObjectMeta{
		// 			Name:      "test-yurtappset",
		// 			Namespace: "default",
		// 		},
		// 		Spec: v1beta1.YurtAppSetSpec{
		// 			Pools: []string{"pool1", "pool2"},
		// 			Workload: v1beta1.Workload{
		// 				WorkloadTemplate: v1beta1.WorkloadTemplate{
		// 					DeploymentTemplate: &v1beta1.DeploymentTemplateSpec{
		// 						Spec: appsv1.DeploymentSpec{
		// 							Template: corev1.PodTemplateSpec{
		// 								Spec: corev1.PodSpec{
		// 									Containers: []corev1.Container{
		// 										{
		// 											Name:  "test-container",
		// 											Image: "test-image",
		// 										},
		// 									},
		// 								},
		// 							},
		// 						},
		// 					},
		// 				},
		// 			},
		// 		},
		// 	},
		// 	expectedExists:    true,
		// 	expectedNodePools: 1,
		// 	expectedErr:       false,
		// },
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(fakeScheme).WithObjects(tt.platformAdmin).Build()
			if tt.yurtAppSet != nil {
				fakeClient = fake.NewClientBuilder().WithScheme(fakeScheme).WithObjects(tt.platformAdmin, tt.yurtAppSet).Build()
			}
			r := &ReconcilePlatformAdmin{
				Client:         fakeClient,
				scheme:         fakeScheme,
				recorder:       record.NewFakeRecorder(10), // 使用假的事件记录器
				yamlSerializer: kjson.NewSerializerWithOptions(kjson.DefaultMetaFactory, fakeScheme, fakeScheme, kjson.SerializerOptions{Yaml: true, Pretty: true}),
				Configration: config.PlatformAdminControllerConfiguration{
					NoSectyConfigMaps: map[string][]corev1.ConfigMap{
						"minnesota": {
							corev1.ConfigMap{
								ObjectMeta: metav1.ObjectMeta{
									Name:      "common-variables",
									Namespace: "default",
								},
							},
						},
					},
					NoSectyComponents: map[string][]*config.Component{
						"minnesota": {
							{Name: "edgex-core-command"},
							{Name: "edgex-core-consul"},
							{Name: "edgex-core-metadata"},
							{Name: "edgex-redis"},
							{Name: "edgex-core-common-config-bootstrapper"},
						},
					},
				},
			}

			request := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "test-platformadmin",
					Namespace: "default",
				},
			}

			_, err := r.Reconcile(context.TODO(), request)
			if tt.expectedErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			updatedYurtAppSet := &v1beta1.YurtAppSet{}
			err = fakeClient.Get(context.TODO(), types.NamespacedName{Name: "test-component", Namespace: "default"}, updatedYurtAppSet)
			if tt.expectedExists {
				assert.NoError(t, err)
				assert.Len(t, updatedYurtAppSet.Spec.Pools, tt.expectedNodePools)
			} else {
				assert.True(t, errors.IsNotFound(err))
			}
		})
	}
}
