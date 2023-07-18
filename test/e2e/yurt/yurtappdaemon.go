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

//var _ = Describe("YurtAppDaemon Test", func() {
//	ctx := context.Background()
//	timeoutSeconds := 60 * time.Second
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
//		appName := "test-appdaemon"
//		Eventually(
//			func() error {
//				return k8sClient.Delete(ctx, &v1alpha1.YurtAppDaemon{ObjectMeta: metav1.ObjectMeta{Name: appName, Namespace: namespaceName}})
//			},
//			timeoutSeconds, time.Millisecond*300).Should(SatisfyAny(BeNil(), &util.NotFoundMatcher{}))
//
//		testLabel := map[string]string{"app": appName}
//
//		testYad := &v1alpha1.YurtAppDaemon{
//			ObjectMeta: metav1.ObjectMeta{
//				Namespace: namespaceName,
//				Name:      appName,
//			},
//			Spec: v1alpha1.YurtAppDaemonSpec{
//				Selector:         &metav1.LabelSelector{MatchLabels: testLabel},
//				NodePoolSelector: &metav1.LabelSelector{MatchLabels: map[string]string{apps.NodePoolTypeLabelKey: "edge"}},
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
//										Name:    "bb",
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
//			},
//		}
//
//		Eventually(func() error {
//			return k8sClient.Create(ctx, testYad)
//		}, timeoutSeconds, time.Millisecond*300).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
//
//		Eventually(func() error {
//			testPods := &corev1.PodList{}
//			if err := k8sClient.List(ctx, testPods, client.InNamespace(namespaceName), client.MatchingLabels{"apps.openyurt.io/pool-name": bjNpName}); err != nil {
//				return err
//			}
//			if len(testPods.Items) != 1 {
//				return fmt.Errorf("yurtappdaemon pods not reconcile")
//			}
//			for _, tmp := range testPods.Items {
//				if tmp.Status.Phase != corev1.PodRunning {
//					return errors.New("yurtappdaemon pods not running")
//				}
//			}
//			return nil
//		}, timeoutSeconds, time.Millisecond*300).Should(SatisfyAny(BeNil()))
//		Eventually(func() error {
//			testPods := &corev1.PodList{}
//			if err := k8sClient.List(ctx, testPods, client.InNamespace(namespaceName), client.MatchingLabels{"apps.openyurt.io/pool-name": hzNpName}); err != nil {
//				return err
//			}
//			if len(testPods.Items) != 1 {
//				return fmt.Errorf("not reconcile")
//			}
//			for _, tmp := range testPods.Items {
//				if tmp.Status.Phase != corev1.PodRunning {
//					return errors.New("pod not running")
//				}
//			}
//			return nil
//		}, timeoutSeconds, time.Millisecond*300).Should(SatisfyAny(BeNil()))
//
//		Eventually(func() error {
//			if err := util.CleanupNodePoolLabel(ctx, k8sClient); err != nil {
//				return err
//			}
//			return util.CleanupNodePool(ctx, k8sClient)
//		}, timeoutSeconds, time.Millisecond*300).Should(SatisfyAny(BeNil()))
//
//		Eventually(func() error {
//			testPods := &corev1.PodList{}
//			if err := k8sClient.List(ctx, testPods, client.InNamespace(namespaceName), client.MatchingLabels{"apps.openyurt.io/pool-name": bjNpName}); err != nil {
//				return err
//			}
//			if len(testPods.Items) != 0 {
//				return fmt.Errorf("yurtappdaemon pods not reconcile after nodepool removed")
//			}
//			return nil
//		}, timeoutSeconds, time.Millisecond*300).Should(SatisfyAny(BeNil()))
//		Eventually(func() error {
//			testPods := &corev1.PodList{}
//			if err := k8sClient.List(ctx, testPods, client.InNamespace(namespaceName), client.MatchingLabels{"apps.openyurt.io/pool-name": hzNpName}); err != nil {
//				return err
//			}
//			if len(testPods.Items) != 0 {
//				return fmt.Errorf("not reconcile after nodepool removed")
//			}
//			return nil
//		}, timeoutSeconds, time.Millisecond*300).Should(SatisfyAny(BeNil()))
//	}
//
//	BeforeEach(func() {
//		By("Start to run yurtappdaemon test, clean up previous resources")
//		namespaceName = "yurtappdaemon-e2e-test" + "-" + rand.String(4)
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
//	It("Test YurtAppDaemon Controller", func() {
//		By("Run YurtAppDaemon Controller Test")
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
