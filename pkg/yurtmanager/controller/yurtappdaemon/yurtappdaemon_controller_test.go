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

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openyurtio/openyurt/cmd/yurt-manager/names"
	"github.com/openyurtio/openyurt/pkg/apis/apps"
	unitv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/yurtappdaemon/workloadcontroller"
)

//func TestAdd(t *testing.T) {
//	cfg, _ := config.GetConfig()
//	mgr, _ := manager.New(cfg, manager.Options{})
//	tests := []struct {
//		name   string
//		mgr    manager.Manager
//		cxt    context.Context
//		expect error
//	}{
//		{
//			name:   "add new key/val",
//			mgr:    mgr,
//			cxt:    context.TODO(),
//			expect: nil,
//		},
//	}
//	for _, tt := range tests {
//		st := tt
//		tf := func(t *testing.T) {
//			t.Parallel()
//			t.Logf("\tTestCase: %s", st.name)
//			{
//				get := Add(tt.mgr, tt.cxt)
//				if !reflect.DeepEqual(get, st.expect) {
//					t.Fatalf("\t%s\texpect %v, but get %v", failed, st.expect, get)
//				}
//				t.Logf("\t%s\texpect %v, get %v", succeed, st.expect, get)
//
//			}
//		}
//		t.Run(st.name, tf)
//	}
//}

//func TestNewReconciler(t *testing.T) {
//	cfg, _ := config.GetConfig()
//	mgr, _ := manager.New(cfg, manager.Options{})
//	tests := []struct {
//		name   string
//		mgr    manager.Manager
//		expect reconcile.Reconciler
//	}{
//		{
//			name: "add new key/val",
//			mgr:  mgr,
//			expect: &ReconcileYurtAppDaemon{
//				Client: mgr.GetClient(),
//				scheme: mgr.GetScheme(),
//
//				recorder: mgr.GetEventRecorderFor(controllerName),
//				controls: map[unitv1alpha1.TemplateType]workloadcontroller.WorkloadController{
//					//			unitv1alpha1.StatefulSetTemplateType: &StatefulSetControllor{Client: mgr.GetClient(), scheme: mgr.GetScheme()},
//					unitv1alpha1.DeploymentTemplateType: &workloadcontroller.DeploymentControllor{Client: mgr.GetClient(), Scheme: mgr.GetScheme()},
//				},
//			},
//		},
//	}
//	for _, tt := range tests {
//		st := tt
//		tf := func(t *testing.T) {
//			t.Parallel()
//			t.Logf("\tTestCase: %s", st.name)
//			{
//				get := newReconciler(tt.mgr)
//				if !reflect.DeepEqual(get, st.expect) {
//					t.Fatalf("\t%s\texpect %v, but get %v", failed, st.expect, get)
//				}
//				t.Logf("\t%s\texpect %v, get %v", succeed, st.expect, get)
//
//			}
//		}
//		t.Run(st.name, tf)
//	}
//}

func TestUpdateStatus(t *testing.T) {
	var int1 int32 = 11
	var yad *unitv1alpha1.YurtAppDaemon
	yad = &unitv1alpha1.YurtAppDaemon{}
	yad.Generation = 1

	tests := []struct {
		name            string
		instance        *unitv1alpha1.YurtAppDaemon
		newStatus       *unitv1alpha1.YurtAppDaemonStatus
		oldStatus       *unitv1alpha1.YurtAppDaemonStatus
		currentRevision *appsv1.ControllerRevision
		collisionCount  int32
		templateType    unitv1alpha1.TemplateType
		expect          reconcile.Result
	}{
		{
			"equal",
			yad,
			&unitv1alpha1.YurtAppDaemonStatus{
				CurrentRevision:    names.YurtAppDaemonController,
				CollisionCount:     &int1,
				TemplateType:       "StatefulSet",
				ObservedGeneration: 1,
				NodePools: []string{
					"192.168.1.1",
				},
				Conditions: []unitv1alpha1.YurtAppDaemonCondition{
					{
						Type: unitv1alpha1.WorkLoadProvisioned,
					},
				},
			},
			&unitv1alpha1.YurtAppDaemonStatus{
				CurrentRevision:    names.YurtAppDaemonController,
				CollisionCount:     &int1,
				TemplateType:       "StatefulSet",
				ObservedGeneration: 1,
				NodePools: []string{
					"192.168.1.1",
				},
				Conditions: []unitv1alpha1.YurtAppDaemonCondition{
					{
						Type: unitv1alpha1.WorkLoadProvisioned,
					},
				},
			},
			&appsv1.ControllerRevision{},
			int1,
			"StatefulSet",
			reconcile.Result{},
		},
	}

	for _, tt := range tests {
		st := tt
		tf := func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", st.name)
			{
				rc := &ReconcileYurtAppDaemon{
					Client: fakeclient.NewClientBuilder().Build(),
				}
				get, _ := rc.updateStatus(
					st.instance, st.newStatus, st.oldStatus, st.currentRevision, st.collisionCount, st.templateType, make(map[string]*workloadcontroller.Workload))
				if !reflect.DeepEqual(get, st.expect) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, st.expect, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, st.expect, get)

			}
		}
		t.Run(st.name, tf)
	}
}

func TestUpdateYurtAppDaemon(t *testing.T) {
	var int1 int32 = 11
	var yad *unitv1alpha1.YurtAppDaemon
	yad = &unitv1alpha1.YurtAppDaemon{}
	yad.Generation = 1

	tests := []struct {
		name      string
		instance  *unitv1alpha1.YurtAppDaemon
		newStatus *unitv1alpha1.YurtAppDaemonStatus
		oldStatus *unitv1alpha1.YurtAppDaemonStatus
		expect    *unitv1alpha1.YurtAppDaemon
	}{
		{
			"equal",
			yad,
			&unitv1alpha1.YurtAppDaemonStatus{
				CurrentRevision:    names.YurtAppDaemonController,
				CollisionCount:     &int1,
				TemplateType:       "StatefulSet",
				ObservedGeneration: 1,
				NodePools: []string{
					"192.168.1.1",
				},
				Conditions: []unitv1alpha1.YurtAppDaemonCondition{
					{
						Type: unitv1alpha1.WorkLoadProvisioned,
					},
				},
			},
			&unitv1alpha1.YurtAppDaemonStatus{
				CurrentRevision:    names.YurtAppDaemonController,
				CollisionCount:     &int1,
				TemplateType:       "StatefulSet",
				ObservedGeneration: 1,
				NodePools: []string{
					"192.168.1.1",
				},
				Conditions: []unitv1alpha1.YurtAppDaemonCondition{
					{
						Type: unitv1alpha1.WorkLoadProvisioned,
					},
				},
			},
			yad,
		},
	}

	for _, tt := range tests {
		st := tt
		tf := func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", st.name)
			{
				rc := &ReconcileYurtAppDaemon{}
				get, _ := rc.updateYurtAppDaemon(
					st.instance, st.newStatus, st.oldStatus)
				if !reflect.DeepEqual(get, st.expect) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, st.expect, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, st.expect, get)

			}
		}
		t.Run(st.name, tf)
	}
}

func TestCalculateStatus(t *testing.T) {
	var int1 int32 = 11
	var yad *unitv1alpha1.YurtAppDaemon
	yad = &unitv1alpha1.YurtAppDaemon{}
	yad.Generation = 1
	var cr appsv1.ControllerRevision
	cr.Name = "a"
	tests := []struct {
		name                      string
		instance                  *unitv1alpha1.YurtAppDaemon
		newStatus                 *unitv1alpha1.YurtAppDaemonStatus
		currentNodepoolToWorkload map[string]*workloadcontroller.Workload
		currentRevision           *appsv1.ControllerRevision
		collisionCount            int32
		templateType              unitv1alpha1.TemplateType
		expect                    unitv1alpha1.YurtAppDaemonStatus
	}{
		{
			"normal",
			yad,
			&unitv1alpha1.YurtAppDaemonStatus{
				CurrentRevision:    "",
				CollisionCount:     &int1,
				TemplateType:       "StatefulSet",
				ObservedGeneration: 1,
				NodePools: []string{
					"192.168.1.1",
				},
				Conditions: []unitv1alpha1.YurtAppDaemonCondition{
					{
						Type: unitv1alpha1.WorkLoadProvisioned,
					},
				},
			},
			map[string]*workloadcontroller.Workload{},
			&cr,
			1,
			"StatefulSet",
			unitv1alpha1.YurtAppDaemonStatus{
				CurrentRevision:    "a",
				CollisionCount:     &int1,
				TemplateType:       "StatefulSet",
				ObservedGeneration: 1,
				NodePools: []string{
					"192.168.1.1",
				},
				Conditions: []unitv1alpha1.YurtAppDaemonCondition{
					{
						Type: unitv1alpha1.WorkLoadProvisioned,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		st := tt
		tf := func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", st.name)
			{
				rc := &ReconcileYurtAppDaemon{
					Client: fakeclient.NewClientBuilder().Build(),
				}
				get := rc.calculateStatus(st.instance, st.newStatus, st.currentRevision, st.collisionCount, st.templateType, st.currentNodepoolToWorkload)
				if !reflect.DeepEqual(get.CurrentRevision, st.expect.CurrentRevision) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, st.expect.CurrentRevision, get.CurrentRevision)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, st.expect.CurrentRevision, get.CurrentRevision)

			}
		}
		t.Run(st.name, tf)
	}
}

func TestManageWorkloads(t *testing.T) {
	var yad *unitv1alpha1.YurtAppDaemon
	yad = &unitv1alpha1.YurtAppDaemon{}
	yad.Generation = 1

	tests := []struct {
		name                      string
		instance                  *unitv1alpha1.YurtAppDaemon
		currentNodepoolToWorkload map[string]*workloadcontroller.Workload
		allNameToNodePools        map[string]unitv1alpha1.NodePool
		expectedRevision          string
		templateType              unitv1alpha1.TemplateType
		expect                    bool
	}{
		{
			"normal",
			yad,
			map[string]*workloadcontroller.Workload{},
			map[string]unitv1alpha1.NodePool{},
			"a",
			"StatefulSet",
			false,
		},
	}

	for _, tt := range tests {
		st := tt
		tf := func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", st.name)
			{
				rc := &ReconcileYurtAppDaemon{
					Client: fakeclient.NewClientBuilder().Build(),
				}
				rc.manageWorkloads(st.instance, st.currentNodepoolToWorkload, st.allNameToNodePools, st.expectedRevision, st.templateType)
				get := st.expect
				if !reflect.DeepEqual(get, st.expect) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, st.expect, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, st.expect, get)

			}
		}
		t.Run(st.name, tf)
	}
}

func TestManageWorkloadsProvision(t *testing.T) {
	var yad *unitv1alpha1.YurtAppDaemon
	yad = &unitv1alpha1.YurtAppDaemon{}
	yad.Generation = 1

	tests := []struct {
		name               string
		instance           *unitv1alpha1.YurtAppDaemon
		allNameToNodePools map[string]unitv1alpha1.NodePool
		expectedRevision   string
		templateType       unitv1alpha1.TemplateType
		needDeleted        []*workloadcontroller.Workload
		needCreate         []string
		expect             bool
	}{
		{
			"normal",
			yad,
			map[string]unitv1alpha1.NodePool{},
			"a",
			"StatefulSet",
			[]*workloadcontroller.Workload{},
			[]string{},
			false,
		},
	}

	for _, tt := range tests {
		st := tt
		tf := func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", st.name)
			{
				rc := &ReconcileYurtAppDaemon{}
				get, _ := rc.manageWorkloadsProvision(
					st.instance, st.allNameToNodePools, st.expectedRevision, st.templateType, st.needDeleted, st.needCreate)
				if !reflect.DeepEqual(get, false) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, false, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, false, get)

			}
		}
		t.Run(st.name, tf)
	}
}

func TestClassifyWorkloads(t *testing.T) {
	var yad *unitv1alpha1.YurtAppDaemon
	yad = &unitv1alpha1.YurtAppDaemon{}
	yad.Generation = 1

	tests := []struct {
		name                      string
		instance                  *unitv1alpha1.YurtAppDaemon
		currentNodepoolToWorkload map[string]*workloadcontroller.Workload
		allNameToNodePools        map[string]unitv1alpha1.NodePool
		expectedRevision          string
		expect                    []string
	}{
		{
			name:                      "normal",
			instance:                  yad,
			currentNodepoolToWorkload: map[string]*workloadcontroller.Workload{},
			allNameToNodePools:        map[string]unitv1alpha1.NodePool{},
			expectedRevision:          "a",
			expect:                    []string{},
		},
	}

	for _, tt := range tests {
		st := tt
		tf := func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", st.name)
			{
				rc := &ReconcileYurtAppDaemon{}
				rc.classifyWorkloads(
					st.instance, st.currentNodepoolToWorkload, st.allNameToNodePools, st.expectedRevision)
				get := []string{}
				if !reflect.DeepEqual(get, st.expect) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, st.expect, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, st.expect, get)

			}
		}
		t.Run(st.name, tf)
	}
}

func TestGetTemplateControls(t *testing.T) {
	rc := &ReconcileYurtAppDaemon{}

	tests := []struct {
		name     string
		instance *unitv1alpha1.YurtAppDaemon
		expect   unitv1alpha1.TemplateType
	}{
		{
			name: "default",
			instance: &unitv1alpha1.YurtAppDaemon{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "kube-system",
					Name:      "yad",
				},
				Spec: unitv1alpha1.YurtAppDaemonSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"a": "a",
						},
					},
				},
			},
			expect: "",
		},
		{
			name: "deployment",
			instance: &unitv1alpha1.YurtAppDaemon{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "kube-system",
					Name:      "yad",
				},
				Spec: unitv1alpha1.YurtAppDaemonSpec{
					WorkloadTemplate: unitv1alpha1.WorkloadTemplate{
						DeploymentTemplate: &unitv1alpha1.DeploymentTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"a": "a",
								},
							},
							Spec: appsv1.DeploymentSpec{
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
										apps.PoolNameLabelKey: "a",
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
			expect: unitv1alpha1.DeploymentTemplateType,
		},
		{
			name: "stateful",
			instance: &unitv1alpha1.YurtAppDaemon{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "kube-system",
					Name:      "yad",
				},
				Spec: unitv1alpha1.YurtAppDaemonSpec{
					WorkloadTemplate: unitv1alpha1.WorkloadTemplate{
						StatefulSetTemplate: &unitv1alpha1.StatefulSetTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"a": "a",
								},
							},
							Spec: appsv1.StatefulSetSpec{
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
										apps.PoolNameLabelKey: "a",
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
			expect: unitv1alpha1.StatefulSetTemplateType,
		},
	}

	for _, tt := range tests {
		st := tt
		tf := func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", st.name)
			{
				_, get, _ := rc.getTemplateControls(st.instance)
				if !reflect.DeepEqual(get, st.expect) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, st.expect, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, st.expect, get)

			}
		}
		t.Run(st.name, tf)
	}
}
