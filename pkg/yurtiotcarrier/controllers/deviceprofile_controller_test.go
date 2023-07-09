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

	Context("When updating DeviceProfile Status", func() {
		It("Should trigger DeviceProfile instance", func() {
			By("By creating a new DeviceProfile resource")
			ctx := context.Background()

			rop := iotv1alpha1.ResourceOperation{
				DeviceResource: "EnableRandomization_Bool",
				DefaultValue:   "false",
			}

			cmd := iotv1alpha1.DeviceCommand{
				Name:               "WriteBoolValue",
				IsHidden:           false,
				ReadWrite:          "W",
				ResourceOperations: []iotv1alpha1.ResourceOperation{rop},
			}

			resp := iotv1alpha1.ResourceProperties{
				ReadWrite:    "W",
				DefaultValue: "true",
				ValueType:    "Bool",
			}

			res := iotv1alpha1.DeviceResource{
				Description: "used to decide whether to re-generate a random value",
				Name:        "EnableRandomization_Bool",
				IsHidden:    true,
				Properties:  resp,
			}

			dp := &iotv1alpha1.DeviceProfile{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "device.openyurt.io/v1alpha1",
					Kind:       "DeviceProfile",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      ProfileName,
					Namespace: CommonNamespace,
				},
				Spec: iotv1alpha1.DeviceProfileSpec{
					Description:     "test device profile",
					NodePool:        PoolName,
					Manufacturer:    "OpenYurt",
					Model:           Model,
					DeviceCommands:  []iotv1alpha1.DeviceCommand{cmd},
					DeviceResources: []iotv1alpha1.DeviceResource{res},
				},
			}
			Expect(k8sClient.Create(ctx, dp)).Should(Succeed())

			lookupKey := types.NamespacedName{Name: ProfileName, Namespace: CommonNamespace}
			created := &iotv1alpha1.DeviceProfile{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, created)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			time.Sleep(5 * time.Second)
			Expect(created.Spec.Model).Should(Equal(Model))

			Expect(k8sClient.Delete(ctx, created)).Should(Succeed())
		})
	})

})
