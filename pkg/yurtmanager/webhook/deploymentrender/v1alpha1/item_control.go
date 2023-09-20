/*
Copyright 2023 The OpenYurt Authors.

Licensed under the Apache License, Version 2.0 (the License);
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an AS IS BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	v1 "k8s.io/api/apps/v1"

	"github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
)

func replaceItems(deployment *v1.Deployment, items []v1alpha1.Item) {
	for _, item := range items {
		switch {
		case item.Replicas != nil:
			deployment.Spec.Replicas = item.Replicas
		case item.Image != nil:
			for i := range deployment.Spec.Template.Spec.Containers {
				if deployment.Spec.Template.Spec.Containers[i].Name == item.Image.ContainerName {
					deployment.Spec.Template.Spec.Containers[i].Image = item.Image.ImageClaim
				}
			}
			for i := range deployment.Spec.Template.Spec.InitContainers {
				if deployment.Spec.Template.Spec.InitContainers[i].Name == item.Image.ContainerName {
					deployment.Spec.Template.Spec.InitContainers[i].Image = item.Image.ImageClaim
				}
			}
		}
	}
}
