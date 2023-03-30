/*
Copyright 2020 The OpenYurt Authors.
Copyright 2019 The Kruise Authors.

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

/*

import (
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/onsi/gomega"
	"golang.org/x/net/context"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	unitv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
	yurtctlutil "github.com/openyurtio/yurt-app-manager/pkg/yurtappmanager/pkg/controller/util"
)

var c client.Client

const timeout = time.Second * 2

var (
	one int32 = 1
	two int32 = 2
	ten int32 = 10
)

func TestStsReconcile(t *testing.T) {
	g, requests, stopMgr, mgrStopped := setUp(t)
	defer func() {
		clean(g, c)
		close(stopMgr)
		mgrStopped.Wait()
	}()

	caseName := "sts-reconcile"
	instance := &appsv1alpha1.YurtAppSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      caseName,
			Namespace: "default",
		},
		Spec: appsv1alpha1.YurtAppSetSpec{
			Replicas: &one,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name": caseName,
				},
			},
			WorkloadTemplate: appsv1alpha1.WorkloadTemplate{
				StatefulSetTemplate: &appsv1alpha1.StatefulSetTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"name": caseName,
						},
					},
					Spec: appsv1.StatefulSetSpec{
						WorkloadTemplate: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"name": caseName,
								},
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "container-a",
										Image: "nginx:1.0",
									},
								},
							},
						},
					},
				},
			},
			Topology: appsv1alpha1.Topology{
				Pools: []appsv1alpha1.Pool{
					{
						Name: "pool-a",
						NodeSelectorTerm: corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "node-name",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"node-a"},
								},
							},
						},
					},
				},
			},
			RevisionHistoryLimit: &ten,
		},
	}

	// Create the YurtAppSet object and expect the Reconcile and Deployment to be created
	err := c.Create(context.TODO(), instance)
	// The instance object may not be a valid object because it might be missing some required fields.
	// Please modify the instance object by adding required fields and then remove the following if statement.
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer c.Delete(context.TODO(), instance)
	waitReconcilerProcessFinished(g, requests, 3)
	expectedStsCount(g, instance, 1)
}

func TestStsPoolProvision(t *testing.T) {
	g, requests, stopMgr, mgrStopped := setUp(t)
	defer func() {
		clean(g, c)
		close(stopMgr)
		mgrStopped.Wait()
	}()

	caseName := "test-sts-pool-provision"
	instance := &appsv1alpha1.YurtAppSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      caseName,
			Namespace: "default",
		},
		Spec: appsv1alpha1.YurtAppSetSpec{
			Replicas: &one,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name": caseName,
				},
			},
			WorkloadTemplate: appsv1alpha1.WorkloadTemplate{
				StatefulSetTemplate: &appsv1alpha1.StatefulSetTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"name": caseName,
						},
					},
					Spec: appsv1.StatefulSetSpec{
						WorkloadTemplate: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"name": caseName,
								},
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "container-a",
										Image: "nginx:1.0",
									},
								},
							},
						},
					},
				},
			},
			Topology: appsv1alpha1.Topology{
				Pools: []appsv1alpha1.Pool{
					{
						Name: "pool-a",
						NodeSelectorTerm: corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "node-name",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"node-a"},
								},
							},
						},
					},
				},
			},
			RevisionHistoryLimit: &ten,
		},
	}

	// Create the YurtAppSet object and expect the Reconcile and Deployment to be created
	err := c.Create(context.TODO(), instance)
	// The instance object may not be a valid object because it might be missing some required fields.
	// Please modify the instance object by adding required fields and then remove the following if statement.
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer c.Delete(context.TODO(), instance)
	waitReconcilerProcessFinished(g, requests, 3)

	stsList := expectedStsCount(g, instance, 1)
	sts := &stsList.Items[0]
	g.Expect(sts.Spec.WorkloadTemplate.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution).ShouldNot(gomega.BeNil())
	g.Expect(len(sts.Spec.WorkloadTemplate.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms)).Should(gomega.BeEquivalentTo(1))
	g.Expect(len(sts.Spec.WorkloadTemplate.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions)).Should(gomega.BeEquivalentTo(1))
	g.Expect(sts.Spec.WorkloadTemplate.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Key).Should(gomega.BeEquivalentTo("node-name"))
	g.Expect(sts.Spec.WorkloadTemplate.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Operator).Should(gomega.BeEquivalentTo(corev1.NodeSelectorOpIn))
	g.Expect(len(sts.Spec.WorkloadTemplate.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Values)).Should(gomega.BeEquivalentTo(1))
	g.Expect(sts.Spec.WorkloadTemplate.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Values[0]).Should(gomega.BeEquivalentTo("node-a"))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	instance.Spec.Topology.Pools = append(instance.Spec.Topology.Pools, appsv1alpha1.Pool{
		Name: "pool-b",
		NodeSelectorTerm: corev1.NodeSelectorTerm{
			MatchExpressions: []corev1.NodeSelectorRequirement{
				{
					Key:      "node-name",
					Operator: corev1.NodeSelectorOpIn,
					Values:   []string{"node-b"},
				},
			},
		},
	})
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	stsList = expectedStsCount(g, instance, 2)
	sts = getPoolByName(stsList, "pool-a")
	g.Expect(sts.Spec.WorkloadTemplate.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution).ShouldNot(gomega.BeNil())
	g.Expect(len(sts.Spec.WorkloadTemplate.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms)).Should(gomega.BeEquivalentTo(1))
	g.Expect(len(sts.Spec.WorkloadTemplate.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions)).Should(gomega.BeEquivalentTo(1))
	g.Expect(sts.Spec.WorkloadTemplate.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Key).Should(gomega.BeEquivalentTo("node-name"))
	g.Expect(sts.Spec.WorkloadTemplate.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Operator).Should(gomega.BeEquivalentTo(corev1.NodeSelectorOpIn))
	g.Expect(len(sts.Spec.WorkloadTemplate.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Values)).Should(gomega.BeEquivalentTo(1))
	g.Expect(sts.Spec.WorkloadTemplate.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Values[0]).Should(gomega.BeEquivalentTo("node-a"))

	sts = getPoolByName(stsList, "pool-b")
	g.Expect(sts.Spec.WorkloadTemplate.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution).ShouldNot(gomega.BeNil())
	g.Expect(len(sts.Spec.WorkloadTemplate.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms)).Should(gomega.BeEquivalentTo(1))
	g.Expect(len(sts.Spec.WorkloadTemplate.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions)).Should(gomega.BeEquivalentTo(1))
	g.Expect(sts.Spec.WorkloadTemplate.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Key).Should(gomega.BeEquivalentTo("node-name"))
	g.Expect(sts.Spec.WorkloadTemplate.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Operator).Should(gomega.BeEquivalentTo(corev1.NodeSelectorOpIn))
	g.Expect(len(sts.Spec.WorkloadTemplate.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Values)).Should(gomega.BeEquivalentTo(1))
	g.Expect(sts.Spec.WorkloadTemplate.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Values[0]).Should(gomega.BeEquivalentTo("node-b"))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	instance.Spec.Topology.Pools = instance.Spec.Topology.Pools[1:]
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	stsList = expectedStsCount(g, instance, 1)
	sts = &stsList.Items[0]
	g.Expect(sts.Spec.WorkloadTemplate.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution).ShouldNot(gomega.BeNil())
	g.Expect(len(sts.Spec.WorkloadTemplate.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms)).Should(gomega.BeEquivalentTo(1))
	g.Expect(len(sts.Spec.WorkloadTemplate.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions)).Should(gomega.BeEquivalentTo(1))
	g.Expect(sts.Spec.WorkloadTemplate.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Key).Should(gomega.BeEquivalentTo("node-name"))
	g.Expect(sts.Spec.WorkloadTemplate.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Operator).Should(gomega.BeEquivalentTo(corev1.NodeSelectorOpIn))
	g.Expect(len(sts.Spec.WorkloadTemplate.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Values)).Should(gomega.BeEquivalentTo(1))
	g.Expect(sts.Spec.WorkloadTemplate.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Values[0]).Should(gomega.BeEquivalentTo("node-b"))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	instance.Spec.WorkloadTemplate.StatefulSetTemplate.Spec.WorkloadTemplate.Spec.Affinity = &corev1.Affinity{}
	instance.Spec.WorkloadTemplate.StatefulSetTemplate.Spec.WorkloadTemplate.Spec.Affinity.NodeAffinity = &corev1.NodeAffinity{}

	nodeSelector := &corev1.NodeSelector{}
	nodeSelector.NodeSelectorTerms = append(nodeSelector.NodeSelectorTerms, corev1.NodeSelectorTerm{
		MatchExpressions: []corev1.NodeSelectorRequirement{
			{
				Key:      "test",
				Operator: corev1.NodeSelectorOpExists,
			},
			{
				Key:      "region",
				Operator: corev1.NodeSelectorOpIn,
				Values:   []string{caseName},
			},
		},
	})
	nodeSelector.NodeSelectorTerms = append(nodeSelector.NodeSelectorTerms, corev1.NodeSelectorTerm{
		MatchExpressions: []corev1.NodeSelectorRequirement{
			{
				Key:      "test",
				Operator: corev1.NodeSelectorOpDoesNotExist,
			},
		},
	})
	instance.Spec.WorkloadTemplate.StatefulSetTemplate.Spec.WorkloadTemplate.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = nodeSelector

	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	stsList = expectedStsCount(g, instance, 1)
	sts = &stsList.Items[0]
	g.Expect(sts.Spec.WorkloadTemplate.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution).ShouldNot(gomega.BeNil())
	g.Expect(len(sts.Spec.WorkloadTemplate.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms)).Should(gomega.BeEquivalentTo(2))

	g.Expect(len(sts.Spec.WorkloadTemplate.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions)).Should(gomega.BeEquivalentTo(3))
	g.Expect(sts.Spec.WorkloadTemplate.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Key).Should(gomega.BeEquivalentTo("test"))
	g.Expect(sts.Spec.WorkloadTemplate.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Operator).Should(gomega.BeEquivalentTo(corev1.NodeSelectorOpExists))
	g.Expect(len(sts.Spec.WorkloadTemplate.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Values)).Should(gomega.BeEquivalentTo(0))
	g.Expect(sts.Spec.WorkloadTemplate.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[1].Key).Should(gomega.BeEquivalentTo("region"))
	g.Expect(sts.Spec.WorkloadTemplate.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[1].Operator).Should(gomega.BeEquivalentTo(corev1.NodeSelectorOpIn))
	g.Expect(len(sts.Spec.WorkloadTemplate.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[1].Values)).Should(gomega.BeEquivalentTo(1))
	g.Expect(sts.Spec.WorkloadTemplate.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[1].Values[0]).Should(gomega.BeEquivalentTo(caseName))
	g.Expect(sts.Spec.WorkloadTemplate.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[2].Key).Should(gomega.BeEquivalentTo("node-name"))
	g.Expect(sts.Spec.WorkloadTemplate.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[2].Operator).Should(gomega.BeEquivalentTo(corev1.NodeSelectorOpIn))
	g.Expect(len(sts.Spec.WorkloadTemplate.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[2].Values)).Should(gomega.BeEquivalentTo(1))
	g.Expect(sts.Spec.WorkloadTemplate.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[2].Values[0]).Should(gomega.BeEquivalentTo("node-b"))

	g.Expect(len(sts.Spec.WorkloadTemplate.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[1].MatchExpressions)).Should(gomega.BeEquivalentTo(2))
	g.Expect(sts.Spec.WorkloadTemplate.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[1].MatchExpressions[0].Key).Should(gomega.BeEquivalentTo("test"))
	g.Expect(sts.Spec.WorkloadTemplate.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[1].MatchExpressions[0].Operator).Should(gomega.BeEquivalentTo(corev1.NodeSelectorOpDoesNotExist))
	g.Expect(len(sts.Spec.WorkloadTemplate.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[1].MatchExpressions[0].Values)).Should(gomega.BeEquivalentTo(0))
	g.Expect(sts.Spec.WorkloadTemplate.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[1].MatchExpressions[1].Key).Should(gomega.BeEquivalentTo("node-name"))
	g.Expect(sts.Spec.WorkloadTemplate.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[1].MatchExpressions[1].Operator).Should(gomega.BeEquivalentTo(corev1.NodeSelectorOpIn))
	g.Expect(len(sts.Spec.WorkloadTemplate.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[1].MatchExpressions[1].Values)).Should(gomega.BeEquivalentTo(1))
	g.Expect(sts.Spec.WorkloadTemplate.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[1].MatchExpressions[1].Values[0]).Should(gomega.BeEquivalentTo("node-b"))
}

func TestStsPoolProvisionWithToleration(t *testing.T) {
	g, requests, stopMgr, mgrStopped := setUp(t)
	defer func() {
		clean(g, c)
		close(stopMgr)
		mgrStopped.Wait()
	}()

	caseName := "test-sts-pool-provision-with-toleration"
	instance := &appsv1alpha1.YurtAppSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      caseName,
			Namespace: "default",
		},
		Spec: appsv1alpha1.YurtAppSetSpec{
			Replicas: &one,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name": caseName,
				},
			},
			WorkloadTemplate: appsv1alpha1.WorkloadTemplate{
				StatefulSetTemplate: &appsv1alpha1.StatefulSetTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"name": caseName,
						},
					},
					Spec: appsv1.StatefulSetSpec{
						WorkloadTemplate: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"name": caseName,
								},
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "container-a",
										Image: "nginx:1.0",
									},
								},
							},
						},
					},
				},
			},
			Topology: appsv1alpha1.Topology{
				Pools: []appsv1alpha1.Pool{
					{
						Name: "pool-a",
						Tolerations: []corev1.Toleration{
							{
								Key:      "taint-a",
								Operator: corev1.TolerationOpExists,
								Effect:   corev1.TaintEffectNoSchedule,
							},
							{
								Key:      "taint-b",
								Operator: corev1.TolerationOpEqual,
								Value:    "taint-b-value",
							},
						},
					},
				},
			},
			RevisionHistoryLimit: &ten,
		},
	}

	// Create the YurtAppSet object and expect the Reconcile and Deployment to be created
	err := c.Create(context.TODO(), instance)
	// The instance object may not be a valid object because it might be missing some required fields.
	// Please modify the instance object by adding required fields and then remove the following if statement.
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer c.Delete(context.TODO(), instance)
	waitReconcilerProcessFinished(g, requests, 3)

	stsList := expectedStsCount(g, instance, 1)
	sts := &stsList.Items[0]
	g.Expect(sts.Spec.WorkloadTemplate.Spec.Tolerations).ShouldNot(gomega.BeNil())
	g.Expect(len(sts.Spec.WorkloadTemplate.Spec.Tolerations)).Should(gomega.BeEquivalentTo(2))
	g.Expect(reflect.DeepEqual(sts.Spec.WorkloadTemplate.Spec.Tolerations[0], instance.Spec.Topology.Pools[0].Tolerations[0])).Should(gomega.BeTrue())
	g.Expect(reflect.DeepEqual(sts.Spec.WorkloadTemplate.Spec.Tolerations[1], instance.Spec.Topology.Pools[0].Tolerations[1])).Should(gomega.BeTrue())

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	instance.Spec.WorkloadTemplate.StatefulSetTemplate.Spec.WorkloadTemplate.Spec.Tolerations = append(instance.Spec.WorkloadTemplate.StatefulSetTemplate.Spec.WorkloadTemplate.Spec.Tolerations, corev1.Toleration{
		Key:      "taint-0",
		Operator: corev1.TolerationOpExists,
	})

	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	stsList = expectedStsCount(g, instance, 1)
	sts = &stsList.Items[0]
	g.Expect(sts.Spec.WorkloadTemplate.Spec.Tolerations).ShouldNot(gomega.BeNil())
	g.Expect(len(sts.Spec.WorkloadTemplate.Spec.Tolerations)).Should(gomega.BeEquivalentTo(3))
	g.Expect(reflect.DeepEqual(sts.Spec.WorkloadTemplate.Spec.Tolerations[1], instance.Spec.Topology.Pools[0].Tolerations[0])).Should(gomega.BeTrue())
	g.Expect(reflect.DeepEqual(sts.Spec.WorkloadTemplate.Spec.Tolerations[2], instance.Spec.Topology.Pools[0].Tolerations[1])).Should(gomega.BeTrue())
}

func TestStsDupPool(t *testing.T) {
	g, requests, stopMgr, mgrStopped := setUp(t)
	defer func() {
		clean(g, c)
		close(stopMgr)
		mgrStopped.Wait()
	}()

	caseName := "test-sts-dup-pool"
	instance := &appsv1alpha1.YurtAppSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      caseName,
			Namespace: "default",
		},
		Spec: appsv1alpha1.YurtAppSetSpec{
			Replicas: &one,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name": caseName,
				},
			},
			WorkloadTemplate: appsv1alpha1.WorkloadTemplate{
				StatefulSetTemplate: &appsv1alpha1.StatefulSetTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"name": caseName,
						},
					},
					Spec: appsv1.StatefulSetSpec{
						WorkloadTemplate: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"name": caseName,
								},
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "container-a",
										Image: "nginx:1.0",
									},
								},
							},
						},
					},
				},
			},
			Topology: appsv1alpha1.Topology{
				Pools: []appsv1alpha1.Pool{
					{
						Name: "pool-a",
						NodeSelectorTerm: corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "node-name",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"node-a"},
								},
							},
						},
					},
				},
			},
			RevisionHistoryLimit: &ten,
		},
	}

	// Create the YurtAppSet object and expect the Reconcile and Deployment to be created
	err := c.Create(context.TODO(), instance)
	// The instance object may not be a valid object because it might be missing some required fields.
	// Please modify the instance object by adding required fields and then remove the following if statement.
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer c.Delete(context.TODO(), instance)
	waitReconcilerProcessFinished(g, requests, 3)

	stsList := expectedStsCount(g, instance, 1)

	poolA := stsList.Items[0]
	dupSts := poolA.DeepCopy()
	dupSts.Name = "dup-pool-a"
	dupSts.ResourceVersion = ""
	g.Expect(c.Create(context.TODO(), dupSts)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 3)
	expectedStsCount(g, instance, 1)
}

func TestStsScale(t *testing.T) {
	g, requests, stopMgr, mgrStopped := setUp(t)
	defer func() {
		clean(g, c)
		close(stopMgr)
		mgrStopped.Wait()
	}()

	caseName := "test-sts-scale"
	instance := &appsv1alpha1.YurtAppSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      caseName,
			Namespace: "default",
		},
		Spec: appsv1alpha1.YurtAppSetSpec{
			Replicas: &one,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name": caseName,
				},
			},
			WorkloadTemplate: appsv1alpha1.WorkloadTemplate{
				StatefulSetTemplate: &appsv1alpha1.StatefulSetTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"name": caseName,
						},
					},
					Spec: appsv1.StatefulSetSpec{
						WorkloadTemplate: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"name": caseName,
								},
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "container-a",
										Image: "nginx:1.0",
									},
								},
							},
						},
					},
				},
			},
			Topology: appsv1alpha1.Topology{
				Pools: []appsv1alpha1.Pool{
					{
						Name: "pool-a",
						NodeSelectorTerm: corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "node-name",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"nodeA"},
								},
							},
						},
					},
					{
						Name: "pool-b",
						NodeSelectorTerm: corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "node-name",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"nodeB"},
								},
							},
						},
					},
				},
			},
			RevisionHistoryLimit: &ten,
		},
	}

	// Create the YurtAppSet object and expect the Reconcile and Deployment to be created
	err := c.Create(context.TODO(), instance)
	// The instance object may not be a valid object because it might be missing some required fields.
	// Please modify the instance object by adding required fields and then remove the following if statement.
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer c.Delete(context.TODO(), instance)
	waitReconcilerProcessFinished(g, requests, 3)

	stsList := expectedStsCount(g, instance, 2)
	g.Expect(*stsList.Items[0].Spec.Replicas + *stsList.Items[1].Spec.Replicas).Should(gomega.BeEquivalentTo(1))

	var two int32 = 2
	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	instance.Spec.Replicas = &two
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	stsList = expectedStsCount(g, instance, 2)
	g.Expect(*stsList.Items[0].Spec.Replicas).Should(gomega.BeEquivalentTo(1))
	g.Expect(*stsList.Items[1].Spec.Replicas).Should(gomega.BeEquivalentTo(1))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	g.Expect(instance.Status.PoolReplicas).Should(gomega.BeEquivalentTo(map[string]int32{
		"pool-a": 1,
		"pool-b": 1,
	}))

	var five int32 = 6
	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	instance.Spec.Replicas = &five
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	stsList = expectedStsCount(g, instance, 2)
	g.Expect(*stsList.Items[0].Spec.Replicas).Should(gomega.BeEquivalentTo(3))
	g.Expect(*stsList.Items[1].Spec.Replicas).Should(gomega.BeEquivalentTo(3))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	g.Expect(instance.Status.PoolReplicas).Should(gomega.BeEquivalentTo(map[string]int32{
		"pool-a": 3,
		"pool-b": 3,
	}))

	var four int32 = 4
	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	instance.Spec.Replicas = &four
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	stsList = expectedStsCount(g, instance, 2)
	g.Expect(*stsList.Items[0].Spec.Replicas).Should(gomega.BeEquivalentTo(2))
	g.Expect(*stsList.Items[1].Spec.Replicas).Should(gomega.BeEquivalentTo(2))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	g.Expect(instance.Status.PoolReplicas).Should(gomega.BeEquivalentTo(map[string]int32{
		"pool-a": 2,
		"pool-b": 2,
	}))
}

func TestStsUpdate(t *testing.T) {
	g, requests, stopMgr, mgrStopped := setUp(t)
	defer func() {
		clean(g, c)
		close(stopMgr)
		mgrStopped.Wait()
	}()

	caseName := "test-sts-update"
	instance := &appsv1alpha1.YurtAppSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      caseName,
			Namespace: "default",
		},
		Spec: appsv1alpha1.YurtAppSetSpec{
			Replicas: &two,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name": caseName,
				},
			},
			WorkloadTemplate: appsv1alpha1.WorkloadTemplate{
				StatefulSetTemplate: &appsv1alpha1.StatefulSetTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"name": caseName,
						},
					},
					Spec: appsv1.StatefulSetSpec{
						WorkloadTemplate: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"name": caseName,
								},
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "container-a",
										Image: "nginx:1.0",
									},
								},
							},
						},
					},
				},
			},
			Topology: appsv1alpha1.Topology{
				Pools: []appsv1alpha1.Pool{
					{
						Name: "pool-a",
						NodeSelectorTerm: corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "node-name",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"nodeA"},
								},
							},
						},
					},
					{
						Name: "pool-b",
						NodeSelectorTerm: corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "node-name",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"nodeB"},
								},
							},
						},
					},
				},
			},
			RevisionHistoryLimit: &ten,
		},
	}

	// Create the YurtAppSet object and expect the Reconcile and Deployment to be created
	err := c.Create(context.TODO(), instance)
	// The instance object may not be a valid object because it might be missing some required fields.
	// Please modify the instance object by adding required fields and then remove the following if statement.
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer c.Delete(context.TODO(), instance)
	waitReconcilerProcessFinished(g, requests, 3)

	stsList := expectedStsCount(g, instance, 2)
	g.Expect(*stsList.Items[0].Spec.Replicas + *stsList.Items[1].Spec.Replicas).Should(gomega.BeEquivalentTo(2))
	revisionList := &appsv1.ControllerRevisionList{}
	g.Expect(c.List(context.TODO(), revisionList))
	g.Expect(len(revisionList.Items)).Should(gomega.BeEquivalentTo(1))
	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	v1 := revisionList.Items[0].Name
	g.Expect(instance.Status.CurrentRevision).Should(gomega.BeEquivalentTo(v1))

	instance.Spec.WorkloadTemplate.StatefulSetTemplate.Spec.WorkloadTemplate.Spec.Containers[0].Image = "nginx:2.0"
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	stsList = expectedStsCount(g, instance, 2)
	g.Expect(stsList.Items[0].Spec.WorkloadTemplate.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:2.0"))
	g.Expect(stsList.Items[1].Spec.WorkloadTemplate.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:2.0"))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	revisionList = &appsv1.ControllerRevisionList{}
	g.Expect(c.List(context.TODO(), revisionList))
	g.Expect(len(revisionList.Items)).Should(gomega.BeEquivalentTo(2))
	v2 := revisionList.Items[0].Name
	if v2 == v1 {
		v2 = revisionList.Items[1].Name
	}
	g.Expect(instance.Status.UpdateStatus.UpdatedRevision).Should(gomega.BeEquivalentTo(v2))
}

func TestStsRollingUpdatePartition(t *testing.T) {
	g, requests, stopMgr, mgrStopped := setUp(t)
	defer func() {
		clean(g, c)
		close(stopMgr)
		mgrStopped.Wait()
	}()

	caseName := "test-sts-rolling-update-partition"
	instance := &appsv1alpha1.YurtAppSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      caseName,
			Namespace: "default",
		},
		Spec: appsv1alpha1.YurtAppSetSpec{
			Replicas: &ten,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name": caseName,
				},
			},
			WorkloadTemplate: appsv1alpha1.WorkloadTemplate{
				StatefulSetTemplate: &appsv1alpha1.StatefulSetTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"name": caseName,
						},
					},
					Spec: appsv1.StatefulSetSpec{
						WorkloadTemplate: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"name": caseName,
								},
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "container-a",
										Image: "nginx:1.0",
									},
								},
							},
						},
					},
				},
			},
			UpdateStrategy: appsv1alpha1.YurtAppSetUpdateStrategy{},
			Topology: appsv1alpha1.Topology{
				Pools: []appsv1alpha1.Pool{
					{
						Name: "pool-a",
						NodeSelectorTerm: corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "node-name",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"nodeA"},
								},
							},
						},
					},
					{
						Name: "pool-b",
						NodeSelectorTerm: corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "node-name",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"nodeB"},
								},
							},
						},
					},
				},
			},
			RevisionHistoryLimit: &ten,
		},
	}

	// Create the YurtAppSet object and expect the Reconcile and Deployment to be created
	err := c.Create(context.TODO(), instance)
	// The instance object may not be a valid object because it might be missing some required fields.
	// Please modify the instance object by adding required fields and then remove the following if statement.
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer c.Delete(context.TODO(), instance)
	waitReconcilerProcessFinished(g, requests, 3)

	stsList := expectedStsCount(g, instance, 2)
	g.Expect(*stsList.Items[0].Spec.Replicas).Should(gomega.BeEquivalentTo(5))
	g.Expect(*stsList.Items[1].Spec.Replicas).Should(gomega.BeEquivalentTo(5))

	// update with partition
	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	instance.Spec.UpdateStrategy.StatefulSetUpdateStrategy = &appsv1alpha1.StatefulSetUpdateStrategy{
		Partitions: map[string]int32{
			"pool-a": 4,
			"pool-b": 3,
		},
	}
	instance.Spec.WorkloadTemplate.StatefulSetTemplate.Spec.WorkloadTemplate.Spec.Containers[0].Image = "nginx:2.0"
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	stsList = expectedStsCount(g, instance, 2)
	g.Expect(stsList.Items[0].Spec.WorkloadTemplate.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:2.0"))
	g.Expect(stsList.Items[1].Spec.WorkloadTemplate.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:2.0"))

	stsA := getPoolByName(stsList, "pool-a")
	g.Expect(stsA).ShouldNot(gomega.BeNil())
	g.Expect(*stsA.Spec.UpdateStrategy.RollingUpdate.Partition).Should(gomega.BeEquivalentTo(4))

	stsB := getPoolByName(stsList, "pool-b")
	g.Expect(stsB).ShouldNot(gomega.BeNil())
	g.Expect(*stsB.Spec.UpdateStrategy.RollingUpdate.Partition).Should(gomega.BeEquivalentTo(3))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	g.Expect(instance.Status.UpdateStatus.CurrentPartitions).Should(gomega.BeEquivalentTo(map[string]int32{
		"pool-a": 4,
		"pool-b": 3,
	}))

	// move on
	instance.Spec.UpdateStrategy.StatefulSetUpdateStrategy = &appsv1alpha1.StatefulSetUpdateStrategy{
		Partitions: map[string]int32{
			"pool-a": 0,
			"pool-b": 3,
		},
	}
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 4)

	stsList = expectedStsCount(g, instance, 2)
	g.Expect(stsList.Items[0].Spec.WorkloadTemplate.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:2.0"))
	g.Expect(stsList.Items[1].Spec.WorkloadTemplate.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:2.0"))

	stsA = getPoolByName(stsList, "pool-a")
	g.Expect(stsA).ShouldNot(gomega.BeNil())
	g.Expect(*stsA.Spec.UpdateStrategy.RollingUpdate.Partition).Should(gomega.BeEquivalentTo(0))

	stsB = getPoolByName(stsList, "pool-b")
	g.Expect(stsB).ShouldNot(gomega.BeNil())
	g.Expect(*stsB.Spec.UpdateStrategy.RollingUpdate.Partition).Should(gomega.BeEquivalentTo(3))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	g.Expect(instance.Status.UpdateStatus.CurrentPartitions).Should(gomega.BeEquivalentTo(map[string]int32{
		"pool-a": 0,
		"pool-b": 3,
	}))

	// move on
	instance.Spec.UpdateStrategy.StatefulSetUpdateStrategy = &appsv1alpha1.StatefulSetUpdateStrategy{
		Partitions: map[string]int32{},
	}
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	stsList = expectedStsCount(g, instance, 2)
	g.Expect(stsList.Items[0].Spec.WorkloadTemplate.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:2.0"))
	g.Expect(stsList.Items[1].Spec.WorkloadTemplate.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:2.0"))

	stsA = getPoolByName(stsList, "pool-a")
	g.Expect(stsA).ShouldNot(gomega.BeNil())
	g.Expect(*stsA.Spec.UpdateStrategy.RollingUpdate.Partition).Should(gomega.BeEquivalentTo(0))

	stsB = getPoolByName(stsList, "pool-b")
	g.Expect(stsB).ShouldNot(gomega.BeNil())
	g.Expect(*stsB.Spec.UpdateStrategy.RollingUpdate.Partition).Should(gomega.BeEquivalentTo(0))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	g.Expect(instance.Status.UpdateStatus.CurrentPartitions).Should(gomega.BeEquivalentTo(map[string]int32{
		"pool-a": 0,
		"pool-b": 0,
	}))
}

func TestStsRollingUpdateDeleteStuckPod(t *testing.T) {
	g, requests, stopMgr, mgrStopped := setUp(t)
	defer func() {
		clean(g, c)
		close(stopMgr)
		mgrStopped.Wait()
	}()

	caseName := "test-sts-rolling-update-delete-stuck-pod"
	instance := &appsv1alpha1.YurtAppSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      caseName,
			Namespace: "default",
		},
		Spec: appsv1alpha1.YurtAppSetSpec{
			Replicas: &ten,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name": caseName,
				},
			},
			WorkloadTemplate: appsv1alpha1.WorkloadTemplate{
				StatefulSetTemplate: &appsv1alpha1.StatefulSetTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"name": caseName,
						},
					},
					Spec: appsv1.StatefulSetSpec{
						WorkloadTemplate: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"name": caseName,
								},
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "container-a",
										Image: "nginx:1.0",
									},
								},
							},
						},
					},
				},
			},
			UpdateStrategy: appsv1alpha1.YurtAppSetUpdateStrategy{},
			Topology: appsv1alpha1.Topology{
				Pools: []appsv1alpha1.Pool{
					{
						Name: "pool-a",
						NodeSelectorTerm: corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "node-name",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"nodeA"},
								},
							},
						},
					},
					{
						Name: "pool-b",
						NodeSelectorTerm: corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "node-name",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"nodeB"},
								},
							},
						},
					},
				},
			},
			RevisionHistoryLimit: &ten,
		},
	}

	// Create the YurtAppSet object and expect the Reconcile and Deployment to be created
	err := c.Create(context.TODO(), instance)
	// The instance object may not be a valid object because it might be missing some required fields.
	// Please modify the instance object by adding required fields and then remove the following if statement.
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer c.Delete(context.TODO(), instance)
	waitReconcilerProcessFinished(g, requests, 3)

	stsList := expectedStsCount(g, instance, 2)
	g.Expect(*stsList.Items[0].Spec.Replicas).Should(gomega.BeEquivalentTo(5))
	g.Expect(*stsList.Items[1].Spec.Replicas).Should(gomega.BeEquivalentTo(5))

	g.Expect(provisionStatefulSetMockPod(c, &stsList.Items[0])).Should(gomega.BeNil())
	g.Expect(provisionStatefulSetMockPod(c, &stsList.Items[1])).Should(gomega.BeNil())

	g.Expect(retry(func() error { return collectPodOrdinal(c, &stsList.Items[0], "0,1,2,3,4") })).Should(gomega.BeNil())
	g.Expect(retry(func() error { return collectPodOrdinal(c, &stsList.Items[1], "0,1,2,3,4") })).Should(gomega.BeNil())

	// update with partition
	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	instance.Spec.UpdateStrategy.StatefulSetUpdateStrategy = &appsv1alpha1.StatefulSetUpdateStrategy{
		Partitions: map[string]int32{
			"pool-a": 4,
			"pool-b": 3,
		},
	}
	instance.Spec.WorkloadTemplate.StatefulSetTemplate.Spec.WorkloadTemplate.Spec.Containers[0].Image = "nginx:2.0"
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	stsList = expectedStsCount(g, instance, 2)
	g.Expect(stsList.Items[0].Spec.WorkloadTemplate.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:2.0"))
	g.Expect(stsList.Items[1].Spec.WorkloadTemplate.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:2.0"))

	stsA := getPoolByName(stsList, "pool-a")
	g.Expect(stsA).ShouldNot(gomega.BeNil())
	g.Expect(*stsA.Spec.UpdateStrategy.RollingUpdate.Partition).Should(gomega.BeEquivalentTo(4))
	g.Expect(collectPodOrdinal(c, stsA, "0,1,2,3")).Should(gomega.BeNil())

	stsB := getPoolByName(stsList, "pool-b")
	g.Expect(stsB).ShouldNot(gomega.BeNil())
	g.Expect(*stsB.Spec.UpdateStrategy.RollingUpdate.Partition).Should(gomega.BeEquivalentTo(3))
	g.Expect(collectPodOrdinal(c, stsB, "0,1,2")).Should(gomega.BeNil())
}

func TestStsOnDelete(t *testing.T) {
	g, requests, stopMgr, mgrStopped := setUp(t)
	defer func() {
		clean(g, c)
		close(stopMgr)
		mgrStopped.Wait()
	}()

	caseName := "test-sts-on-delete"
	instance := &appsv1alpha1.YurtAppSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      caseName,
			Namespace: "default",
		},
		Spec: appsv1alpha1.YurtAppSetSpec{
			Replicas: &ten,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name": caseName,
				},
			},
			WorkloadTemplate: appsv1alpha1.WorkloadTemplate{
				StatefulSetTemplate: &appsv1alpha1.StatefulSetTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"name": caseName,
						},
					},
					Spec: appsv1.StatefulSetSpec{
						WorkloadTemplate: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"name": caseName,
								},
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "container-a",
										Image: "nginx:1.0",
									},
								},
							},
						},
						UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
							Type: appsv1.OnDeleteStatefulSetStrategyType,
						},
					},
				},
			},
			UpdateStrategy: appsv1alpha1.YurtAppSetUpdateStrategy{},
			Topology: appsv1alpha1.Topology{
				Pools: []appsv1alpha1.Pool{
					{
						Name: "pool-a",
						NodeSelectorTerm: corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "node-name",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"nodeA"},
								},
							},
						},
					},
					{
						Name: "pool-b",
						NodeSelectorTerm: corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "node-name",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"nodeB"},
								},
							},
						},
					},
				},
			},
			RevisionHistoryLimit: &ten,
		},
	}

	// Create the YurtAppSet object and expect the Reconcile and Deployment to be created
	err := c.Create(context.TODO(), instance)
	// The instance object may not be a valid object because it might be missing some required fields.
	// Please modify the instance object by adding required fields and then remove the following if statement.
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer c.Delete(context.TODO(), instance)
	waitReconcilerProcessFinished(g, requests, 3)

	stsList := expectedStsCount(g, instance, 2)
	g.Expect(*stsList.Items[0].Spec.Replicas).Should(gomega.BeEquivalentTo(5))
	g.Expect(*stsList.Items[1].Spec.Replicas).Should(gomega.BeEquivalentTo(5))

	g.Expect(stsList.Items[0].Spec.UpdateStrategy.Type).Should(gomega.BeEquivalentTo(appsv1.OnDeleteStatefulSetStrategyType))
	g.Expect(stsList.Items[1].Spec.UpdateStrategy.Type).Should(gomega.BeEquivalentTo(appsv1.OnDeleteStatefulSetStrategyType))

	// update with partition
	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	instance.Spec.UpdateStrategy.StatefulSetUpdateStrategy = &appsv1alpha1.StatefulSetUpdateStrategy{
		Partitions: map[string]int32{
			"pool-a": 4,
			"pool-b": 3,
		},
	}
	instance.Spec.WorkloadTemplate.StatefulSetTemplate.Spec.WorkloadTemplate.Spec.Containers[0].Image = "nginx:2.0"
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	stsList = expectedStsCount(g, instance, 2)
	g.Expect(stsList.Items[0].Spec.WorkloadTemplate.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:2.0"))
	g.Expect(stsList.Items[1].Spec.WorkloadTemplate.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:2.0"))

	stsA := getPoolByName(stsList, "pool-a")
	g.Expect(stsA).ShouldNot(gomega.BeNil())
	g.Expect(stsA.Spec.UpdateStrategy.Type).Should(gomega.BeEquivalentTo(appsv1.OnDeleteStatefulSetStrategyType))
	g.Expect(stsA.Spec.UpdateStrategy.RollingUpdate).Should(gomega.BeNil())

	stsB := getPoolByName(stsList, "pool-b")
	g.Expect(stsB).ShouldNot(gomega.BeNil())
	g.Expect(stsB.Spec.UpdateStrategy.Type).Should(gomega.BeEquivalentTo(appsv1.OnDeleteStatefulSetStrategyType))
	g.Expect(stsB.Spec.UpdateStrategy.RollingUpdate).Should(gomega.BeNil())

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	g.Expect(instance.Status.UpdateStatus.CurrentPartitions).Should(gomega.BeEquivalentTo(map[string]int32{
		"pool-a": 4,
		"pool-b": 3,
	}))

	// move on
	instance.Spec.UpdateStrategy.StatefulSetUpdateStrategy = &appsv1alpha1.StatefulSetUpdateStrategy{
		Partitions: map[string]int32{
			"pool-a": 0,
			"pool-b": 3,
		},
	}
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	stsList = expectedStsCount(g, instance, 2)
	g.Expect(stsList.Items[0].Spec.WorkloadTemplate.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:2.0"))
	g.Expect(stsList.Items[1].Spec.WorkloadTemplate.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:2.0"))
}

func TestStsPoolCount(t *testing.T) {
	g, requests, stopMgr, mgrStopped := setUp(t)
	defer func() {
		clean(g, c)
		close(stopMgr)
		mgrStopped.Wait()
	}()

	caseName := "test-sts-pool-count"
	instance := &appsv1alpha1.YurtAppSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      caseName,
			Namespace: "default",
		},
		Spec: appsv1alpha1.YurtAppSetSpec{
			Replicas: &ten,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name": caseName,
				},
			},
			WorkloadTemplate: appsv1alpha1.WorkloadTemplate{
				StatefulSetTemplate: &appsv1alpha1.StatefulSetTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"name": caseName,
						},
					},
					Spec: appsv1.StatefulSetSpec{
						WorkloadTemplate: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"name": caseName,
								},
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "container-a",
										Image: "nginx:1.0",
									},
								},
							},
						},
					},
				},
			},
			Topology: appsv1alpha1.Topology{
				Pools: []appsv1alpha1.Pool{
					{
						Name: "pool-a",
						NodeSelectorTerm: corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "node-name",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"nodeA"},
								},
							},
						},
					},
					{
						Name: "pool-b",
						NodeSelectorTerm: corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "node-name",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"nodeB"},
								},
							},
						},
					},
				},
			},
			RevisionHistoryLimit: &ten,
		},
	}

	// Create the YurtAppSet object and expect the Reconcile and Deployment to be created
	err := c.Create(context.TODO(), instance)
	// The instance object may not be a valid object because it might be missing some required fields.
	// Please modify the instance object by adding required fields and then remove the following if statement.
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer c.Delete(context.TODO(), instance)
	waitReconcilerProcessFinished(g, requests, 3)

	stsList := expectedStsCount(g, instance, 2)
	g.Expect(*stsList.Items[0].Spec.Replicas).Should(gomega.BeEquivalentTo(5))
	g.Expect(*stsList.Items[1].Spec.Replicas).Should(gomega.BeEquivalentTo(5))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	nine := intstr.FromInt(9)
	instance.Spec.Topology.Pools[0].Replicas = &nine
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	stsList = expectedStsCount(g, instance, 2)
	setsubA := getPoolByName(stsList, "pool-a")
	g.Expect(*setsubA.Spec.Replicas).Should(gomega.BeEquivalentTo(9))
	setsubB := getPoolByName(stsList, "pool-b")
	g.Expect(*setsubB.Spec.Replicas).Should(gomega.BeEquivalentTo(1))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	percentage := intstr.FromString("40%")
	instance.Spec.Topology.Pools[0].Replicas = &percentage
	instance.Spec.WorkloadTemplate.StatefulSetTemplate.Spec.WorkloadTemplate.Spec.Containers[0].Image = "nginx:2.0"
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	stsList = expectedStsCount(g, instance, 2)
	setsubA = getPoolByName(stsList, "pool-a")
	g.Expect(*setsubA.Spec.Replicas).Should(gomega.BeEquivalentTo(4))
	g.Expect(setsubA.Spec.WorkloadTemplate.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:2.0"))
	setsubB = getPoolByName(stsList, "pool-b")
	g.Expect(*setsubB.Spec.Replicas).Should(gomega.BeEquivalentTo(6))
	g.Expect(setsubB.Spec.WorkloadTemplate.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:2.0"))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	percentage = intstr.FromString("30%")
	instance.Spec.Topology.Pools[0].Replicas = &percentage
	instance.Spec.WorkloadTemplate.StatefulSetTemplate.Spec.WorkloadTemplate.Spec.Containers[0].Image = "nginx:3.0"
	instance.Spec.UpdateStrategy.StatefulSetUpdateStrategy = &appsv1alpha1.StatefulSetUpdateStrategy{
		Partitions: map[string]int32{
			"pool-a": 1,
		},
	}
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	stsList = expectedStsCount(g, instance, 2)
	setsubA = getPoolByName(stsList, "pool-a")
	g.Expect(*setsubA.Spec.Replicas).Should(gomega.BeEquivalentTo(3))
	g.Expect(setsubA.Spec.WorkloadTemplate.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:3.0"))
	g.Expect(setsubA.Spec.UpdateStrategy.RollingUpdate).ShouldNot(gomega.BeNil())
	g.Expect(setsubA.Spec.UpdateStrategy.RollingUpdate.Partition).ShouldNot(gomega.BeNil())
	g.Expect(*setsubA.Spec.UpdateStrategy.RollingUpdate.Partition).Should(gomega.BeEquivalentTo(1))
	setsubB = getPoolByName(stsList, "pool-b")
	g.Expect(*setsubB.Spec.Replicas).Should(gomega.BeEquivalentTo(7))
	g.Expect(setsubB.Spec.WorkloadTemplate.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:3.0"))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	percentage = intstr.FromString("20%")
	instance.Spec.Topology.Pools[0].Replicas = &percentage
	instance.Spec.WorkloadTemplate.StatefulSetTemplate.Spec.WorkloadTemplate.Spec.Containers[0].Image = "nginx:4.0"
	instance.Spec.UpdateStrategy.StatefulSetUpdateStrategy.Partitions = map[string]int32{
		"pool-a": 2,
	}
	instance.Spec.Topology.Pools = append(instance.Spec.Topology.Pools, appsv1alpha1.Pool{
		Name: "pool-c",
		NodeSelectorTerm: corev1.NodeSelectorTerm{
			MatchExpressions: []corev1.NodeSelectorRequirement{
				{
					Key:      "node-name",
					Operator: corev1.NodeSelectorOpIn,
					Values:   []string{"nodeC"},
				},
			},
		},
	})
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	stsList = expectedStsCount(g, instance, 3)
	setsubA = getPoolByName(stsList, "pool-a")
	g.Expect(*setsubA.Spec.Replicas).Should(gomega.BeEquivalentTo(2))
	g.Expect(setsubA.Spec.WorkloadTemplate.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:4.0"))
	g.Expect(setsubA.Spec.UpdateStrategy.RollingUpdate).ShouldNot(gomega.BeNil())
	g.Expect(setsubA.Spec.UpdateStrategy.RollingUpdate.Partition).ShouldNot(gomega.BeNil())
	g.Expect(*setsubA.Spec.UpdateStrategy.RollingUpdate.Partition).Should(gomega.BeEquivalentTo(2))
	setsubB = getPoolByName(stsList, "pool-b")
	g.Expect(*setsubB.Spec.Replicas).Should(gomega.BeEquivalentTo(4))
	g.Expect(setsubB.Spec.WorkloadTemplate.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:4.0"))
	setsubB = getPoolByName(stsList, "pool-c")
	g.Expect(*setsubB.Spec.Replicas).Should(gomega.BeEquivalentTo(4))
	g.Expect(setsubB.Spec.WorkloadTemplate.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:4.0"))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	percentage = intstr.FromString("10%")
	instance.Spec.Topology.Pools[0].Replicas = &percentage
	instance.Spec.WorkloadTemplate.StatefulSetTemplate.Spec.WorkloadTemplate.Spec.Containers[0].Image = "nginx:5.0"
	instance.Spec.UpdateStrategy.StatefulSetUpdateStrategy.Partitions = map[string]int32{
		"pool-a": 2,
	}
	instance.Spec.Topology.Pools = instance.Spec.Topology.Pools[:2]
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 3)

	stsList = expectedStsCount(g, instance, 2)
	setsubA = getPoolByName(stsList, "pool-a")
	g.Expect(*setsubA.Spec.Replicas).Should(gomega.BeEquivalentTo(1))
	g.Expect(setsubA.Spec.WorkloadTemplate.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:5.0"))
	g.Expect(setsubA.Spec.UpdateStrategy.RollingUpdate).ShouldNot(gomega.BeNil())
	g.Expect(setsubA.Spec.UpdateStrategy.RollingUpdate.Partition).ShouldNot(gomega.BeNil())
	g.Expect(*setsubA.Spec.UpdateStrategy.RollingUpdate.Partition).Should(gomega.BeEquivalentTo(1))
	setsubB = getPoolByName(stsList, "pool-b")
	g.Expect(*setsubB.Spec.Replicas).Should(gomega.BeEquivalentTo(9))
	g.Expect(setsubB.Spec.WorkloadTemplate.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:5.0"))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	g.Expect(instance.Spec.UpdateStrategy.StatefulSetUpdateStrategy.Partitions["pool-a"]).Should(gomega.BeEquivalentTo(2))
	percentage = intstr.FromString("40%")
	instance.Spec.Topology.Pools[0].Replicas = &percentage
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 3)

	stsList = expectedStsCount(g, instance, 2)
	setsubA = getPoolByName(stsList, "pool-a")
	g.Expect(*setsubA.Spec.Replicas).Should(gomega.BeEquivalentTo(4))
	g.Expect(setsubA.Spec.WorkloadTemplate.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:5.0"))
	g.Expect(setsubA.Spec.UpdateStrategy.RollingUpdate).ShouldNot(gomega.BeNil())
	g.Expect(setsubA.Spec.UpdateStrategy.RollingUpdate.Partition).ShouldNot(gomega.BeNil())
	g.Expect(*setsubA.Spec.UpdateStrategy.RollingUpdate.Partition).Should(gomega.BeEquivalentTo(2))
	setsubB = getPoolByName(stsList, "pool-b")
	g.Expect(*setsubB.Spec.Replicas).Should(gomega.BeEquivalentTo(6))
	g.Expect(setsubB.Spec.WorkloadTemplate.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:5.0"))
}

func retry(assert func() error) error {
	var err error
	for i := 0; i < 3; i++ {
		err = assert()
		if err == nil {
			return nil
		}
		time.Sleep(1 * time.Second)
	}

	return err
}

func collectPodOrdinal(c client.Client, sts *appsv1.StatefulSet, expected string) error {
	selector, err := metav1.LabelSelectorAsSelector(sts.Spec.Selector)
	if err != nil {
		return err
	}

	podList := &corev1.PodList{}
	if err := c.List(context.TODO(), podList, &client.ListOptions{LabelSelector: selector}); err != nil {
		return err
	}

	marks := make([]bool, len(podList.Items))
	for _, pod := range podList.Items {
		ordinal := int(yurtctlutil.GetOrdinal(&pod))
		if ordinal >= len(marks) || ordinal < 0 {
			continue
		}

		marks[ordinal] = true
	}

	got := ""
	for idx, mark := range marks {
		if mark {
			got = fmt.Sprintf("%s,%d", got, idx)
		}
	}

	if len(got) > 0 {
		got = got[1:]
	}

	if got != expected {
		return fmt.Errorf("expected %s, got %s", expected, got)
	}

	return nil
}

func provisionStatefulSetMockPod(c client.Client, sts *appsv1.StatefulSet) error {
	if sts.Spec.Replicas == nil {
		return nil
	}

	replicas := *sts.Spec.Replicas
	for {
		replicas--
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: sts.Namespace,
				Name:      fmt.Sprintf("%s-%d", sts.Name, replicas),
				Labels:    sts.Spec.WorkloadTemplate.Labels,
			},
			Spec: sts.Spec.WorkloadTemplate.Spec,
		}

		if err := c.Create(context.TODO(), pod); err != nil {
			if !apierrors.IsAlreadyExists(err) {
				return err
			}
		}

		if replicas == 0 {
			return nil
		}
	}
}

func waitReconcilerProcessFinished(g *gomega.GomegaWithT, requests chan reconcile.Request, minCount int) {
	timeoutChan := time.After(timeout)
	maxTimeoutChan := time.After(timeout * 2)
	for {
		minCount--
		select {
		case <-requests:
			continue
		case <-timeoutChan:
			if minCount <= 0 {
				return
			}
		case <-maxTimeoutChan:
			return
		}
	}
}

func getPoolByName(stsList *appsv1.StatefulSetList, name string) *appsv1.StatefulSet {
	for _, sts := range stsList.Items {
		if sts.Labels[appsv1alpha1.PoolNameLabelKey] == name {
			return &sts
		}
	}

	return nil
}

func expectedStsCount(g *gomega.GomegaWithT, yas *appsv1alpha1.YurtAppSet, count int) *appsv1.StatefulSetList {
	stsList := &appsv1.StatefulSetList{}

	selector, err := metav1.LabelSelectorAsSelector(yas.Spec.Selector)
	g.Expect(err).Should(gomega.BeNil())

	g.Eventually(func() error {
		if err := c.List(context.TODO(), stsList, &client.ListOptions{LabelSelector: selector}); err != nil {
			return err
		}

		if len(stsList.Items) != count {
			return fmt.Errorf("expected %d sts, got %d", count, len(stsList.Items))
		}

		return nil
	}, timeout).Should(gomega.Succeed())

	return stsList
}

func setUp(t *testing.T) (*gomega.GomegaWithT, chan reconcile.Request, chan struct{}, *sync.WaitGroup) {
	g := gomega.NewGomegaWithT(t)
	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(cfg, manager.Options{MetricsBindAddress: "0"})
	g.Expect(err).NotTo(gomega.HaveOccurred())
	c = mgr.GetClient()
	recFn, requests := SetupTestReconcile(newReconciler(mgr))
	g.Expect(add(mgr, recFn)).NotTo(gomega.HaveOccurred())
	stopMgr, mgrStopped := StartTestManager(mgr, g)

	return g, requests, stopMgr, mgrStopped
}

func clean(g *gomega.GomegaWithT, c client.Client) {
	yasList := &appsv1alpha1.YurtAppSetList{}
	if err := c.List(context.TODO(), yasList); err == nil {
		for _, yas := range yasList.Items {
			c.Delete(context.TODO(), &yas)
		}
	}
	g.Eventually(func() error {
		if err := c.List(context.TODO(), yasList); err != nil {
			return err
		}

		if len(yasList.Items) != 0 {
			return fmt.Errorf("expected %d sts, got %d", 0, len(yasList.Items))
		}

		return nil
	}, timeout, time.Second).Should(gomega.Succeed())

	rList := &appsv1.ControllerRevisionList{}
	if err := c.List(context.TODO(), rList); err == nil {
		for _, yas := range rList.Items {
			c.Delete(context.TODO(), &yas)
		}
	}
	g.Eventually(func() error {
		if err := c.List(context.TODO(), rList); err != nil {
			return err
		}

		if len(rList.Items) != 0 {
			return fmt.Errorf("expected %d sts, got %d", 0, len(rList.Items))
		}

		return nil
	}, timeout, time.Second).Should(gomega.Succeed())

	stsList := &appsv1.StatefulSetList{}
	if err := c.List(context.TODO(), stsList); err == nil {
		for _, sts := range stsList.Items {
			c.Delete(context.TODO(), &sts)
		}
	}
	g.Eventually(func() error {
		if err := c.List(context.TODO(), stsList); err != nil {
			return err
		}

		if len(stsList.Items) != 0 {
			return fmt.Errorf("expected %d sts, got %d", 0, len(stsList.Items))
		}

		return nil
	}, timeout, time.Second).Should(gomega.Succeed())

	// TODO delete other resources list @kadisi

		astsList := &appsv1.StatefulSetList{}
		if err := c.List(context.TODO(), astsList); err == nil {
			for _, asts := range astsList.Items {
				c.Delete(context.TODO(), &asts)
			}
		}
		g.Eventually(func() error {
			if err := c.List(context.TODO(), astsList); err != nil {
				return err
			}

			if len(astsList.Items) != 0 {
				return fmt.Errorf("expected %d asts, got %d", 0, len(astsList.Items))
			}

			return nil
		}, timeout, time.Second).Should(gomega.Succeed())

	podList := &corev1.PodList{}
	if err := c.List(context.TODO(), podList); err == nil {
		for _, pod := range podList.Items {
			c.Delete(context.TODO(), &pod)
		}
	}
	g.Eventually(func() error {
		if err := c.List(context.TODO(), podList); err != nil {
			return err
		}

		if len(podList.Items) != 0 {
			return fmt.Errorf("expected %d sts, got %d", 0, len(podList.Items))
		}

		return nil
	}, timeout, time.Second).Should(gomega.Succeed())
}


*/
