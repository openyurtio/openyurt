/*
Copyright 2023 The OpenYurt Authors.

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

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
	"github.com/openyurtio/openyurt/test/e2e/util"
	ycfg "github.com/openyurtio/openyurt/test/e2e/yurtconfig"
)

var _ = Describe("YurtAppOverrider Test", func() {
	ctx := context.Background()
	k8sClient := ycfg.YurtE2eCfg.RuntimeClient
	var namespaceName string
	timeout := 60 * time.Second
	nodePoolName := "nodepool-test"
	yurtAppSetName := "yurtappset-test"
	yurtAppOverriderName := "yurtappoverrider-test"
	var testReplicasOld int32 = 3
	var testReplicasNew int32 = 5
	createNameSpace := func() {
		ns := corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespaceName,
			},
		}
		Eventually(func() error {
			return k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationBackground))
		}).WithTimeout(timeout).WithPolling(500 * time.Millisecond).Should(SatisfyAny(BeNil(), &util.NotFoundMatcher{}))
		By("make sure namespace are removed")

		res := &corev1.Namespace{}
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: namespaceName}, res)
		}).WithTimeout(timeout).WithPolling(500 * time.Millisecond).Should(&util.NotFoundMatcher{})
		Eventually(func() error {
			return k8sClient.Create(ctx, &ns)
		}).WithTimeout(timeout).WithPolling(500 * time.Millisecond).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	}
	createNodePool := func() {
		Eventually(func() error {
			return k8sClient.Delete(ctx, &v1alpha1.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name:      nodePoolName,
					Namespace: namespaceName,
				},
			})
		}).WithTimeout(timeout).WithPolling(500 * time.Millisecond).Should(SatisfyAny(BeNil(), &util.NotFoundMatcher{}))
		testNodePool := v1alpha1.NodePool{
			ObjectMeta: metav1.ObjectMeta{
				Name:      nodePoolName,
				Namespace: namespaceName,
			},
		}
		Eventually(func() error {
			return k8sClient.Create(ctx, &testNodePool)
		}).WithTimeout(timeout).WithPolling(500 * time.Millisecond).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	}
	createYurtAppSet := func() {
		Eventually(func() error {
			return k8sClient.Delete(ctx, &v1alpha1.YurtAppSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      yurtAppSetName,
					Namespace: namespaceName,
				},
			})
		}).WithTimeout(timeout).WithPolling(500 * time.Millisecond).Should(SatisfyAny(BeNil(), &util.NotFoundMatcher{}))
		testYurtAppSet := v1alpha1.YurtAppSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      yurtAppSetName,
				Namespace: namespaceName,
			},
			Spec: v1alpha1.YurtAppSetSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "test"},
				},
				WorkloadTemplate: v1alpha1.WorkloadTemplate{
					DeploymentTemplate: &v1alpha1.DeploymentTemplateSpec{
						Spec: v1.DeploymentSpec{
							Template: corev1.PodTemplateSpec{
								ObjectMeta: metav1.ObjectMeta{
									Labels: map[string]string{"app": "test"},
								},
								Spec: corev1.PodSpec{
									Containers: []corev1.Container{{
										Image: "nginx-old",
										Name:  "nginx",
									}},
								},
							},
						},
					},
				},
				Topology: v1alpha1.Topology{
					Pools: []v1alpha1.Pool{
						{
							Name:     nodePoolName,
							Replicas: &testReplicasOld,
						},
					},
				},
			},
		}
		Eventually(func() error {
			return k8sClient.Create(ctx, &testYurtAppSet)
		}).WithTimeout(timeout).WithPolling(500 * time.Millisecond).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	}
	createYurtAppOverrider := func() {
		Eventually(func() error {
			return k8sClient.Delete(ctx, &v1alpha1.YurtAppOverrider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      yurtAppOverriderName,
					Namespace: namespaceName,
				},
			})
		}).WithTimeout(timeout).WithPolling(500 * time.Millisecond).Should(SatisfyAny(BeNil(), &util.NotFoundMatcher{}))
		testYurtAppOverrider := v1alpha1.YurtAppOverrider{
			ObjectMeta: metav1.ObjectMeta{
				Name:      yurtAppOverriderName,
				Namespace: namespaceName,
			},
			Subject: v1alpha1.Subject{
				Name: yurtAppSetName,
				TypeMeta: metav1.TypeMeta{
					Kind:       "YurtAppSet",
					APIVersion: "apps.openyurt.io/v1alpha1",
				},
			},
			Entries: []v1alpha1.Entry{
				{
					Pools: []string{"nodepool-test"},
					Items: []v1alpha1.Item{
						{
							Image: &v1alpha1.ImageItem{
								ContainerName: "nginx",
								ImageClaim:    "nginx-item",
							},
						},
						{
							Replicas: &testReplicasNew,
						},
					},
					Patches: []v1alpha1.Patch{
						{
							Operation: v1alpha1.Default,
							Path:      "/spec/template/spec/containers/0/image",
							Value: apiextensionsv1.JSON{
								Raw: []byte("nginx-patch"),
							},
						},
					},
				},
			},
		}
		Eventually(func() error {
			return k8sClient.Create(ctx, &testYurtAppOverrider)
		}).WithTimeout(timeout).WithPolling(500 * time.Millisecond).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	}
	deleteNodePool := func() {
		Eventually(func() error {
			return k8sClient.Delete(ctx, &v1alpha1.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name:      nodePoolName,
					Namespace: namespaceName,
				},
			})
		}).WithTimeout(timeout).WithPolling(500 * time.Millisecond).Should(SatisfyAny(BeNil(), &util.NotFoundMatcher{}))
	}
	deleteYurtAppSet := func() {
		Eventually(func() error {
			return k8sClient.Delete(ctx, &v1alpha1.YurtAppSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      yurtAppSetName,
					Namespace: namespaceName,
				},
			})
		}).WithTimeout(timeout).WithPolling(500 * time.Millisecond).Should(SatisfyAny(BeNil(), &util.NotFoundMatcher{}))
	}
	deleteYurtAppOverrider := func() {
		Eventually(func() error {
			return k8sClient.Delete(ctx, &v1alpha1.YurtAppOverrider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      yurtAppOverriderName,
					Namespace: namespaceName,
				},
			})
		}).WithTimeout(timeout).WithPolling(500 * time.Millisecond).Should(SatisfyAny(BeNil(), &util.NotFoundMatcher{}))
	}

	BeforeEach(func() {
		By("Start to run YurtAppOverrider test, prepare resources")
		namespaceName = "yurtappoverrider-e2e-test" + "-" + rand.String(4)
		createNameSpace()

	})
	AfterEach(func() {
		By("Cleanup resources after test")
		Expect(k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespaceName}}, client.PropagationPolicy(metav1.DeletePropagationBackground))).Should(Succeed())
	})

	Describe("Test function of YurtAppOverrider", func() {
		It("YurtAppOverrider should work after it is created", func() {
			By("validate replicas and image of deployment")
			Eventually(func() error {
				deploymentList := &v1.DeploymentList{}
				if err := k8sClient.List(ctx, deploymentList, client.InNamespace(namespaceName)); err != nil {
					return err
				}
				for _, deployment := range deploymentList.Items {
					if deployment.Labels["apps.openyurt.io/pool-name"] == nodePoolName {
						if deployment.Spec.Template.Spec.Containers[0].Image != "nginx-patch" {
							return fmt.Errorf("the image of nginx is not nginx-patch but %s", deployment.Spec.Template.Spec.Containers[0].Image)
						}
						if *deployment.Spec.Replicas != 5 {
							return fmt.Errorf("the replicas of nginx is not 3 but %d", *deployment.Spec.Replicas)
						}
					}
				}
				return nil
			}).WithTimeout(timeout).WithPolling(500 * time.Millisecond).Should(Succeed())
		})
		It("YurtAppOverrider should refresh template after it is updated", func() {
			By("Deployment will be returned to former when the YurtAppOverrider is deleted")
			yurtAppOverrider := &v1alpha1.YurtAppOverrider{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: yurtAppOverriderName, Namespace: namespaceName}, yurtAppOverrider)
			}).WithTimeout(timeout).WithPolling(500 * time.Millisecond).Should(Succeed())
			for _, entry := range yurtAppOverrider.Entries {
				entry.Pools = []string{}
			}
			Expect(k8sClient.Update(ctx, yurtAppOverrider)).Should(Succeed())
			Eventually(func() error {
				deploymentList := &v1.DeploymentList{}
				if err := k8sClient.List(ctx, deploymentList, client.MatchingLabels{
					"apps.openyurt.io/pool-name": nodePoolName,
				}); err != nil {
					return err
				}
				for _, deployment := range deploymentList.Items {
					if deployment.Spec.Template.Spec.Containers[0].Image != "nginx-old" {
						return fmt.Errorf("the image of nginx is not nginx but %s", deployment.Spec.Template.Spec.Containers[0].Image)
					}
					if *deployment.Spec.Replicas != 3 {
						return fmt.Errorf("the replicas of nginx is not 3 but %d", *deployment.Spec.Replicas)
					}
				}
				return nil
			}).WithTimeout(timeout).WithPolling(500 * time.Millisecond).Should(Succeed())
		})
		BeforeEach(func() {
			createNodePool()
			createYurtAppSet()
			createYurtAppOverrider()
		})
		AfterEach(func() {
			deleteNodePool()
			deleteYurtAppSet()
			deleteYurtAppOverrider()
		})
	})
})
