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

package controllers

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	iotv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/iot/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// +kubebuilder:docs-gen:collapse=Imports

var _ = Describe("Device controller", func() {

	// Define utility constants for object names and testing timeouts/durations and intervals.
	const ()

	Context("When updating Device Status", func() {
		It("Should trigger Device instance", func() {
			By("By creating a new Device resource")
			ctx := context.Background()

			dev := &iotv1alpha1.Device{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "device.openyurt.io/v1alpha1",
					Kind:       "Device",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      DeviceName,
					Namespace: CommonNamespace,
				},
				Spec: iotv1alpha1.DeviceSpec{
					Description: "unit test device",
					AdminState:  "UNLOCKED",
					NodePool:    PoolName,
					Managed:     true,
					Notify:      true,
					Service:     ServiceName,
				},
			}
			Expect(k8sClient.Create(ctx, dev)).Should(Succeed())

			lookupKey := types.NamespacedName{Name: DeviceName, Namespace: CommonNamespace}
			created := &iotv1alpha1.Device{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, created)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			time.Sleep(5 * time.Second)
			Expect(created.Spec.NodePool).Should(Equal(PoolName))

			Expect(k8sClient.Delete(ctx, created)).Should(Succeed())
		})
	})

})
