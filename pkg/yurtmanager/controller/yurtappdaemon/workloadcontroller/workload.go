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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	unitv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/apps"
)

type Workload struct {
	Name      string
	Namespace string
	Kind      string
	Spec      WorkloadSpec
	Status    WorkloadStatus
}

// WorkloadSpec stores the spec details of the workload
type WorkloadSpec struct {
	Ref          metav1.Object
	Tolerations  []corev1.Toleration
	NodeSelector map[string]string
}

// WorkloadStatus stores the observed state of the Workload.
type WorkloadStatus struct {
	Replicas           int32
	ReadyReplicas      int32
	AvailableCondition corev1.ConditionStatus
}

func (w *Workload) GetRevision() string {
	return w.Spec.Ref.GetLabels()[unitv1alpha1.ControllerRevisionHashLabelKey]
}

func (w *Workload) GetNodePoolName() string {
	return w.Spec.Ref.GetAnnotations()[unitv1alpha1.AnnotationRefNodePool]
}

func (w *Workload) GetToleration() []corev1.Toleration {
	return w.Spec.Tolerations
}

func (w *Workload) GetNodeSelector() map[string]string {
	return w.Spec.NodeSelector
}

func (w *Workload) GetKind() string {
	return w.Kind
}
