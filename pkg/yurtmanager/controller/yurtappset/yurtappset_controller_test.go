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
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openyurtio/openyurt/pkg/apis/apps"
	"github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/yurtappset/workloadmanager"
)

func TestGetWorkloadManagerFromYurtAppSet(t *testing.T) {
	type args struct {
		yas *v1beta1.YurtAppSet
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "StatefulSetTemplate is set in YurtAppSet's Spec.Workload.WorkloadTemplate.",
			args: args{
				yas: &v1beta1.YurtAppSet{
					Spec: v1beta1.YurtAppSetSpec{
						Workload: v1beta1.Workload{
							WorkloadTemplate: v1beta1.WorkloadTemplate{
								StatefulSetTemplate: &v1beta1.StatefulSetTemplateSpec{},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "DeploymentTemplate is set in YurtAppSet's Spec.Workload.WorkloadTemplate.",
			args: args{
				yas: &v1beta1.YurtAppSet{
					Spec: v1beta1.YurtAppSetSpec{
						Workload: v1beta1.Workload{
							WorkloadTemplate: v1beta1.WorkloadTemplate{
								DeploymentTemplate: &v1beta1.DeploymentTemplateSpec{},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Neither StatefulSetTemplate nor DeploymentTemplate is set in YurtAppSet's Spec.Workload.WorkloadTemplate.",
			args: args{
				yas: &v1beta1.YurtAppSet{
					Spec: v1beta1.YurtAppSetSpec{
						Workload: v1beta1.Workload{},
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := ReconcileYurtAppSet{}
			_, err := r.getWorkloadManagerFromYurtAppSet(tt.args.yas)
			if (err != nil) != tt.wantErr {
				t.Errorf("getWorkloadManagerFromYurtAppSet() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}

func TestClassifyWorkloads(t *testing.T) {
	yas := &v1beta1.YurtAppSet{}
	expectedNodePools := sets.NewString("test-np1", "test-np3")
	expectedRevision := "test-revision-2"
	workloadTobeDeleted := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-deployment-1",
			Labels: map[string]string{
				apps.PoolNameLabelKey: "test-np2",
			},
		},
	}
	workloadTobeUpdated := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-deployment-2",
			Labels: map[string]string{
				apps.PoolNameLabelKey:               "test-np1",
				apps.ControllerRevisionHashLabelKey: "test-revision-1",
			},
		},
	}
	currentWorkloads := []metav1.Object{workloadTobeDeleted, workloadTobeUpdated}

	needDeleted, needUpdate, needCreate := classifyWorkloads(yas, currentWorkloads, expectedNodePools, expectedRevision)
	if len(needDeleted) != 1 || needDeleted[0].GetName() != workloadTobeDeleted.GetName() {
		t.Errorf("classifyWorkloads() needDeleted = %v, want %v", needDeleted, workloadTobeDeleted)
	}
	if len(needUpdate) != 1 || needUpdate[0].GetName() != workloadTobeUpdated.GetName() {
		t.Errorf("classifyWorkloads() needUpdate = %v, want %v", needUpdate, workloadTobeUpdated)
	}
	if len(needCreate) != 1 || needCreate[0] != "test-np3" {
		t.Errorf("classifyWorkloads() needCreate = %v, want %v", needCreate, []string{"test-np3"})
	}
}

type fakeEventRecorder struct {
}

func (f *fakeEventRecorder) Event(object runtime.Object, eventtype, reason, message string) {
}

func (f *fakeEventRecorder) Eventf(object runtime.Object, eventtype, reason, messageFmt string, args ...interface{}) {
}

func (f *fakeEventRecorder) AnnotatedEventf(object runtime.Object, annotations map[string]string, eventtype, reason, messageFmt string, args ...interface{}) {
}

func TestReconcile(t *testing.T) {
	tests := []struct {
		name       string
		request    reconcile.Request
		yas        *v1beta1.YurtAppSet
		npList     []client.Object
		deployList []client.Object

		expectedDeployNum int
		expectedErr       bool
		isUpdated         bool
	}{
		{
			name: "yas create 2 deployment",
			request: reconcile.Request{
				NamespacedName: client.ObjectKey{
					Name:      "test-yurtappset",
					Namespace: "default",
				},
			},
			yas: &v1beta1.YurtAppSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-yurtappset",
					Namespace: "default",
				},
				Spec: v1beta1.YurtAppSetSpec{
					Pools: []string{"test-np1", "test-np2"},
					Workload: v1beta1.Workload{
						WorkloadTemplate: v1beta1.WorkloadTemplate{
							DeploymentTemplate: &v1beta1.DeploymentTemplateSpec{
								Spec: appsv1.DeploymentSpec{
									Selector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"app": "test-yurtappset",
										},
									},
								},
							},
						},
					},
				},
			},
			npList: []client.Object{
				&v1beta1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-np1",
					},
				},
				&v1beta1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-np2",
					},
				},
			},
			expectedDeployNum: 2,
			expectedErr:       false,
		},
		{
			name: "no np found, should delete 1 deployment",
			request: reconcile.Request{
				NamespacedName: client.ObjectKey{
					Name:      "test-yurtappset",
					Namespace: "default",
				},
			},
			yas: &v1beta1.YurtAppSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-yurtappset",
					Namespace: "default",
				},
				Spec: v1beta1.YurtAppSetSpec{
					Pools: []string{"test-np1"},
					Workload: v1beta1.Workload{
						WorkloadTemplate: v1beta1.WorkloadTemplate{
							DeploymentTemplate: &v1beta1.DeploymentTemplateSpec{
								Spec: appsv1.DeploymentSpec{
									Selector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"app": "test-yurtappset",
										},
									},
								},
							},
						},
					},
				},
			},
			deployList: []client.Object{
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-deployment-1",
						Namespace: "default",
						Labels: map[string]string{
							apps.PoolNameLabelKey:        "test-np2",
							apps.YurtAppSetOwnerLabelKey: "test-yurtappset",
						},
					},
				},
			},
			expectedDeployNum: 0,
			expectedErr:       false,
		},
		{
			name: "1 np found and 1 np miss, should create 1 deployment and delete 1 deployment",
			request: reconcile.Request{
				NamespacedName: client.ObjectKey{
					Name:      "test-yurtappset",
					Namespace: "default",
				},
			},
			yas: &v1beta1.YurtAppSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-yurtappset",
					Namespace: "default",
				},
				Spec: v1beta1.YurtAppSetSpec{
					Pools: []string{"test-np1"},
					Workload: v1beta1.Workload{
						WorkloadTemplate: v1beta1.WorkloadTemplate{
							DeploymentTemplate: &v1beta1.DeploymentTemplateSpec{
								Spec: appsv1.DeploymentSpec{
									Selector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"app": "test-yurtappset",
										},
									},
								},
							},
						},
					},
				},
			},
			npList: []client.Object{
				&v1beta1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-np1",
					},
				},
			},
			deployList: []client.Object{
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-deployment-1",
						Namespace: "default",
						Labels: map[string]string{
							apps.PoolNameLabelKey:        "test-np2",
							apps.YurtAppSetOwnerLabelKey: "test-yurtappset",
						},
					},
				},
			},
			expectedDeployNum: 1,
			expectedErr:       false,
		},
		{
			name: "update 1 deployment",
			request: reconcile.Request{
				NamespacedName: client.ObjectKey{
					Name:      "test-yurtappset",
					Namespace: "default",
				},
			},
			yas: &v1beta1.YurtAppSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-yurtappset",
					Namespace: "default",
				},
				Spec: v1beta1.YurtAppSetSpec{
					Pools: []string{"test-np1"},
					Workload: v1beta1.Workload{
						WorkloadTemplate: v1beta1.WorkloadTemplate{
							DeploymentTemplate: &v1beta1.DeploymentTemplateSpec{
								Spec: appsv1.DeploymentSpec{
									Template: corev1.PodTemplateSpec{
										Spec: corev1.PodSpec{
											Containers: []corev1.Container{
												{
													Name:  "test-container",
													Image: "test-image",
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
			npList: []client.Object{
				&v1beta1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-np1",
					},
				},
			},
			deployList: []client.Object{
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-deployment-1",
						Namespace: "default",
						Labels: map[string]string{
							apps.PoolNameLabelKey:               "test-np1",
							apps.YurtAppSetOwnerLabelKey:        "test-yurtappset",
							apps.ControllerRevisionHashLabelKey: "test-revision-1",
						},
					},
				},
			},
			expectedDeployNum: 1,
			expectedErr:       false,
			isUpdated:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objList := []client.Object{tt.yas}
			if len(tt.npList) > 0 {
				objList = append(objList, tt.npList...)
			}
			if len(tt.deployList) > 0 {
				objList = append(objList, tt.deployList...)
			}

			fakeClient := fake.NewClientBuilder().WithScheme(fakeScheme).WithObjects(objList...).Build()
			r := &ReconcileYurtAppSet{
				scheme:   fakeScheme,
				Client:   fakeClient,
				recorder: &fakeEventRecorder{},
				workloadManagers: map[workloadmanager.TemplateType]workloadmanager.WorkloadManager{
					workloadmanager.DeploymentTemplateType: &workloadmanager.DeploymentManager{
						Client: fakeClient,
						Scheme: fakeScheme,
					},
				},
			}

			_, err := r.Reconcile(context.TODO(), tt.request)
			if tt.expectedErr {
				assert.NotNil(t, err)
			}

			deployList := &appsv1.DeploymentList{}
			if err := fakeClient.List(context.TODO(), deployList); err == nil {
				assert.Len(t, deployList.Items, tt.expectedDeployNum)
			}

			if tt.isUpdated {
				for _, deploy := range deployList.Items {
					assert.NotEqual(t, deploy.Labels[apps.ControllerRevisionHashLabelKey], tt.yas.Status.CurrentRevision)
				}
			}
		})
	}
}
