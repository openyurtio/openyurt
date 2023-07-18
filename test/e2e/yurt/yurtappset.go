/*
Copyright 2020 The OpenYurt Authors.

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

package yurt

//import (
//	"context"
//	"errors"
//	"fmt"
//	"time"
//
//	. "github.com/onsi/ginkgo/v2"
//	. "github.com/onsi/gomega"
//	appsv1 "k8s.io/api/apps/v1"
//	corev1 "k8s.io/api/core/v1"
//	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
//	"k8s.io/apimachinery/pkg/runtime"
//	"k8s.io/apimachinery/pkg/util/rand"
//	"k8s.io/apimachinery/pkg/util/sets"
//	"k8s.io/utils/pointer"
//	"sigs.k8s.io/controller-runtime/pkg/client"
//	"sigs.k8s.io/yaml"
//
//	"github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
//	"github.com/openyurtio/openyurt/test/e2e/util"
//	ycfg "github.com/openyurtio/openyurt/test/e2e/yurtconfig"
//)
//
//var _ = Describe("YurtAppSet Test", func() {
//	ctx := context.Background()
//	timeoutSeconds := 28 * time.Second
//	k8sClient := ycfg.YurtE2eCfg.RuntimeClient
//	var namespaceName string
//
//	bjNpName := "beijing"
//	hzNpName := "hangzhou"
//
//	createNamespace := func() {
//		ns := corev1.Namespace{
//			ObjectMeta: metav1.ObjectMeta{
//				Name: namespaceName,
//			},
//		}
//		Eventually(
//			func() error {
//				return k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationForeground))
//			},
//			timeoutSeconds, time.Millisecond*500).Should(SatisfyAny(BeNil(), &util.NotFoundMatcher{}))
//		By("make sure all the resources are removed")
//
//		res := &corev1.Namespace{}
//		Eventually(
//			func() error {
//				return k8sClient.Get(ctx, client.ObjectKey{
//					Name: namespaceName,
//				}, res)
//			},
//			timeoutSeconds, time.Millisecond*500).Should(&util.NotFoundMatcher{})
//		Eventually(
//			func() error {
//				return k8sClient.Create(ctx, &ns)
//			},
//			timeoutSeconds, time.Millisecond*300).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
//	}
//
//	topologyTest := func() {
//		appName := "test-appset"
//		Eventually(
//			func() error {
//				return k8sClient.Delete(ctx, &v1alpha1.YurtAppSet{ObjectMeta: metav1.ObjectMeta{Name: appName, Namespace: namespaceName}})
//			},
//			timeoutSeconds, time.Millisecond*300).Should(SatisfyAny(BeNil(), &util.NotFoundMatcher{}))
//
//		bizContainerName := "biz"
//		testLabel := map[string]string{"app": appName}
//
//		hzBizImg := "busybox:1.36.0"
//		hzPatchStr := fmt.Sprintf(`
//spec:
//  template:
//    spec:
//      containers:
//      - name: %s
//        image: %s
//`, bizContainerName, hzBizImg)
//		hzPatchJson, _ := yaml.YAMLToJSON([]byte(hzPatchStr))
//		hzPatch := &runtime.RawExtension{Raw: hzPatchJson}
//
//		testYas := &v1alpha1.YurtAppSet{
//			ObjectMeta: metav1.ObjectMeta{
//				Namespace: namespaceName,
//				Name:      appName,
//			},
//			Spec: v1alpha1.YurtAppSetSpec{
//				Selector: &metav1.LabelSelector{MatchLabels: testLabel},
//				WorkloadTemplate: v1alpha1.WorkloadTemplate{
//					DeploymentTemplate: &v1alpha1.DeploymentTemplateSpec{
//						ObjectMeta: metav1.ObjectMeta{Labels: testLabel},
//						Spec: appsv1.DeploymentSpec{
//							Template: corev1.PodTemplateSpec{
//								ObjectMeta: metav1.ObjectMeta{
//									Labels: testLabel,
//								},
//								Spec: corev1.PodSpec{
//									Containers: []corev1.Container{{
//										Name:    bizContainerName,
//										Image:   "busybox",
//										Command: []string{"/bin/sh"},
//										Args:    []string{"-c", "while true; do echo hello; sleep 10;done"},
//									}},
//									Tolerations: []corev1.Toleration{{Key: "node-role.kubernetes.io/master", Effect: "NoSchedule"}},
//								},
//							},
//						},
//					},
//				},
//				Topology: v1alpha1.Topology{
//					Pools: []v1alpha1.Pool{
//						{Name: bjNpName,
//							NodeSelectorTerm: corev1.NodeSelectorTerm{
//								MatchExpressions: []corev1.NodeSelectorRequirement{
//									{
//										Key:      v1alpha1.LabelCurrentNodePool,
//										Operator: "In",
//										Values:   []string{bjNpName},
//									},
//								},
//							},
//							Replicas: pointer.Int32Ptr(1),
//						},
//						{Name: hzNpName, NodeSelectorTerm: corev1.NodeSelectorTerm{
//							MatchExpressions: []corev1.NodeSelectorRequirement{
//								{
//									Key:      v1alpha1.LabelCurrentNodePool,
//									Operator: "In",
//									Values:   []string{hzNpName},
//								},
//							},
//						},
//							Replicas: pointer.Int32Ptr(2),
//							Patch:    hzPatch,
//						},
//					},
//				},
//			},
//		}
//
//		Eventually(func() error {
//			return k8sClient.Create(ctx, testYas)
//		}, timeoutSeconds, time.Millisecond*300).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
//
//		Eventually(func() error {
//			testPods := &corev1.PodList{}
//			if err := k8sClient.List(ctx, testPods, client.InNamespace(namespaceName), client.MatchingLabels{"apps.openyurt.io/pool-name": bjNpName}); err != nil {
//				return err
//			}
//			if len(testPods.Items) != 1 {
//				return fmt.Errorf("yurtappset pods not reconcile")
//			}
//			for _, tmp := range testPods.Items {
//				if tmp.Status.Phase != corev1.PodRunning {
//					return errors.New("yurtappset pods not running")
//				}
//			}
//			return nil
//		}, timeoutSeconds, time.Millisecond*300).Should(SatisfyAny(BeNil()))
//		Eventually(func() error {
//			testPods := &corev1.PodList{}
//			if err := k8sClient.List(ctx, testPods, client.InNamespace(namespaceName), client.MatchingLabels{"apps.openyurt.io/pool-name": hzNpName}); err != nil {
//				return err
//			}
//			if len(testPods.Items) != 2 {
//				return fmt.Errorf("not reconcile")
//			}
//			for _, tmp := range testPods.Items {
//				for _, tmpContainer := range tmp.Spec.Containers {
//					if tmpContainer.Name == bizContainerName {
//						if tmpContainer.Image != hzBizImg {
//							return errors.New("yurtappset topology patch not work")
//						}
//					}
//				}
//				if tmp.Status.Phase != corev1.PodRunning {
//					return errors.New("pod not running")
//				}
//			}
//			return nil
//		}, timeoutSeconds, time.Millisecond*300).Should(SatisfyAny(BeNil()))
//	}
//
//	BeforeEach(func() {
//		By("Start to run yurtappset test, clean up previous resources")
//		namespaceName = "yurtappset-e2e-test" + "-" + rand.String(4)
//		k8sClient = ycfg.YurtE2eCfg.RuntimeClient
//		util.CleanupNodePoolLabel(ctx, k8sClient)
//		util.CleanupNodePool(ctx, k8sClient)
//		createNamespace()
//	})
//
//	AfterEach(func() {
//		By("Cleanup resources after test")
//		By(fmt.Sprintf("Delete the entire namespaceName %s", namespaceName))
//		Expect(k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespaceName}}, client.PropagationPolicy(metav1.DeletePropagationBackground))).Should(BeNil())
//		util.CleanupNodePoolLabel(ctx, k8sClient)
//		util.CleanupNodePool(ctx, k8sClient)
//	})
//
//	It("Test YurtAppSet Controller", func() {
//		By("Run YurtAppSet Controller Test")
//
//		poolToNodesMap := make(map[string]sets.String)
//		poolToNodesMap[bjNpName] = sets.NewString("openyurt-e2e-test-worker")
//		poolToNodesMap[hzNpName] = sets.NewString("openyurt-e2e-test-worker2")
//
//		util.InitNodeAndNodePool(ctx, k8sClient, poolToNodesMap)
//		topologyTest()
//	})
//
//})
