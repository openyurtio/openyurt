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

package v1beta1

import (
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openyurtio/openyurt/pkg/apis/apps"
)

// SetDefaultsNodePool set default values for NodePool.
func SetDefaultsNodePool(obj *NodePool) {
	// example for set default value for NodePool
	if obj.Annotations == nil {
		obj.Annotations = make(map[string]string)
	}

	obj.Spec.Selector = &metav1.LabelSelector{
		MatchLabels: map[string]string{apps.LabelCurrentNodePool: obj.Name},
	}

	// add NodePool.Spec.Type to NodePool labels
	if obj.Labels == nil {
		obj.Labels = make(map[string]string)
	}
	obj.Labels[apps.NodePoolTypeLabelKey] = strings.ToLower(string(obj.Spec.Type))

}
