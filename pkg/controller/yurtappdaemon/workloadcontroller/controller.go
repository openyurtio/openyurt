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
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
)

type WorkloadController interface {
	ObjectKey(load *Workload) client.ObjectKey
	GetAllWorkloads(daemon *v1alpha1.YurtAppDaemon) ([]*Workload, error)
	CreateWorkload(daemon *v1alpha1.YurtAppDaemon, nodepool v1alpha1.NodePool, revision string) error
	UpdateWorkload(load *Workload, daemon *v1alpha1.YurtAppDaemon, nodepool v1alpha1.NodePool, revision string) error
	DeleteWorkload(daemon *v1alpha1.YurtAppDaemon, load *Workload) error
	GetTemplateType() v1alpha1.TemplateType
}
