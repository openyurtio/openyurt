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

package workloadcontroller

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/validation"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
)

func getWorkloadPrefix(controllerName, nodepoolName string) string {
	prefix := fmt.Sprintf("%s-%s-", controllerName, nodepoolName)
	if len(validation.NameIsDNSSubdomain(prefix, true)) != 0 {
		prefix = fmt.Sprintf("%s-", controllerName)
	}
	return prefix
}

func CreateNodeSelectorByNodepoolName(nodepool string) map[string]string {
	return map[string]string{
		projectinfo.GetNodePoolLabel(): nodepool,
	}
}

func TaintsToTolerations(taints []corev1.Taint) []corev1.Toleration {
	tolerations := []corev1.Toleration{}
	for _, taint := range taints {
		toleation := corev1.Toleration{
			Key:      taint.Key,
			Operator: corev1.TolerationOpExists,
			Effect:   taint.Effect,
		}
		tolerations = append(tolerations, toleation)
	}
	return tolerations
}
