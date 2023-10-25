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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	iotv1alpha2 "github.com/openyurtio/openyurt/pkg/apis/iot/v1alpha2"
	"github.com/openyurtio/openyurt/test/e2e/util"
	ycfg "github.com/openyurtio/openyurt/test/e2e/yurtconfig"
)

func generateTestVersions() []string {
	return []string{"levski", "jakarta", "kamakura", "ireland", "minnesota"}
}

var _ = Describe("OpenYurt IoT Test", func() {
	var platformAdminName string

	ctx := context.Background()
	k8sClient := ycfg.YurtE2eCfg.RuntimeClient
	timeout := 60 * time.Second
	platformadminTimeout := 5 * time.Minute
	testVersions := generateTestVersions()
	namespaceName := "iot-test-namespace"
	nodePoolName := util.NodePoolName

	createNamespace := func() {
		ns := corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespaceName,
			},
		}
		Eventually(
			func() error {
				return k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationForeground))
			}).WithTimeout(timeout).WithPolling(time.Millisecond * 500).Should(SatisfyAny(BeNil(), &util.NotFoundMatcher{}))
		By("make sure the needed namespace are removed")

		res := &corev1.Namespace{}
		Eventually(
			func() error {
				return k8sClient.Get(ctx, client.ObjectKey{
					Name: namespaceName,
				}, res)
			}).WithTimeout(timeout).WithPolling(time.Millisecond * 500).Should(&util.NotFoundMatcher{})
		Eventually(
			func() error {
				return k8sClient.Create(ctx, &ns)
			}).WithTimeout(timeout).WithPolling(time.Millisecond * 300).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	}

	createPlatformAdmin := func(version string) {
		Eventually(func() error {
			return k8sClient.Delete(ctx, &iotv1alpha2.PlatformAdmin{
				ObjectMeta: metav1.ObjectMeta{
					Name:      platformAdminName,
					Namespace: namespaceName,
				},
			})
		}).WithTimeout(timeout).WithPolling(500 * time.Millisecond).Should(SatisfyAny(BeNil(), &util.NotFoundMatcher{}))

		testPlatformAdmin := iotv1alpha2.PlatformAdmin{
			ObjectMeta: metav1.ObjectMeta{
				Name:      platformAdminName,
				Namespace: namespaceName,
			},
			Spec: iotv1alpha2.PlatformAdminSpec{
				Version:  version,
				PoolName: nodePoolName,
			},
		}
		Eventually(func() error {
			return k8sClient.Create(ctx, &testPlatformAdmin)
		}).WithTimeout(timeout).WithPolling(500 * time.Millisecond).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	}

	BeforeEach(func() {
		By("Start to run iot test, clean up previous resources")
		k8sClient = ycfg.YurtE2eCfg.RuntimeClient
		createNamespace()
	})

	AfterEach(func() {
		By("Cleanup resources after test")
		By(fmt.Sprintf("Delete the entire namespaceName %s", namespaceName))
		Expect(k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespaceName}}, client.PropagationPolicy(metav1.DeletePropagationBackground))).Should(BeNil())
	})

	for _, testVersion := range testVersions {
		version := testVersion
		Describe(fmt.Sprintf("Test the %s version of PlatformAdmin", version), func() {
			BeforeEach(func() {
				platformAdminName = "test-platform-admin-" + version
				createPlatformAdmin(version)
			})

			AfterEach(func() {
				By(fmt.Sprintf("Delete the platformAdmin %s", platformAdminName))
				Expect(k8sClient.Delete(ctx, &iotv1alpha2.PlatformAdmin{ObjectMeta: metav1.ObjectMeta{Name: platformAdminName, Namespace: namespaceName}}, client.PropagationPolicy(metav1.DeletePropagationBackground))).Should(BeNil())
			})

			It(fmt.Sprintf("The %s version of PlatformAdmin should be stable in ready state after it is created", version), func() {
				By("verify the status of platformadmin")
				Eventually(func() error {
					testPlatfromAdmin := &iotv1alpha2.PlatformAdmin{}
					if err := k8sClient.Get(ctx, types.NamespacedName{Name: platformAdminName, Namespace: namespaceName}, testPlatfromAdmin); err != nil {
						return err
					}
					if testPlatfromAdmin.Status.Ready == true {
						return nil
					} else {
						return fmt.Errorf("The %s version of PlatformAdmin is not ready", version)
					}
				}, platformadminTimeout, 5*time.Second).Should(Succeed())
			})
		})
	}
})
