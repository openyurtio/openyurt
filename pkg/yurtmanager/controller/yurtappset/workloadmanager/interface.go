/*
Copyright 2024 The OpenYurt Authors.

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

package workloadmanager

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
)

type TemplateType string

const (
	StatefulSetTemplateType TemplateType = "StatefulSet"
	DeploymentTemplateType  TemplateType = "Deployment"
)

type WorkloadManager interface {
	GetTemplateType() TemplateType

	List(yas *v1beta1.YurtAppSet) ([]metav1.Object, error)
	Create(yas *v1beta1.YurtAppSet, nodepoolName, revision string) error
	Update(yas *v1beta1.YurtAppSet, workload metav1.Object, nodepoolName, revision string) error
	Delete(yas *v1beta1.YurtAppSet, workload metav1.Object) error
}
