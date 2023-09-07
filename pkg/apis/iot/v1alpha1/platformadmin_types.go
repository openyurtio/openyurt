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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// name of finalizer
	EdgexFinalizer = "edgex.edgexfoundry.org"

	LabelEdgeXGenerate = "www.edgexfoundry.org/generate"
)

// PlatformAdminConditionType indicates valid conditions type of a iot platform.
type PlatformAdminConditionType string
type PlatformAdminConditionSeverity string

// DeploymentTemplateSpec defines the pool template of Deployment.
type DeploymentTemplateSpec struct {
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              appsv1.DeploymentSpec `json:"spec"`
}

// DeploymentTemplateSpec defines the pool template of Deployment.
type ServiceTemplateSpec struct {
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              corev1.ServiceSpec `json:"spec"`
}

// PlatformAdminSpec defines the desired state of PlatformAdmin
type PlatformAdminSpec struct {
	Version string `json:"version,omitempty"`

	ImageRegistry string `json:"imageRegistry,omitempty"`

	PoolName string `json:"poolName,omitempty"`

	ServiceType corev1.ServiceType `json:"serviceType,omitempty"`
	// +optional
	AdditionalService []ServiceTemplateSpec `json:"additionalServices,omitempty"`

	// +optional
	AdditionalDeployment []DeploymentTemplateSpec `json:"additionalDeployments,omitempty"`
}

// PlatformAdminStatus defines the observed state of PlatformAdmin
type PlatformAdminStatus struct {
	// +optional
	Ready bool `json:"ready,omitempty"`
	// +optional
	Initialized bool `json:"initialized,omitempty"`
	// +optional
	ServiceReplicas int32 `json:"serviceReplicas,omitempty"`
	// +optional
	ServiceReadyReplicas int32 `json:"serviceReadyReplicas,omitempty"`
	// +optional
	DeploymentReplicas int32 `json:"deploymentReplicas,omitempty"`
	// +optional
	DeploymentReadyReplicas int32 `json:"deploymentReadyReplicas,omitempty"`

	// Current PlatformAdmin state
	// +optional
	Conditions []PlatformAdminCondition `json:"conditions,omitempty"`
}

// PlatformAdminCondition describes current state of a PlatformAdmin.
type PlatformAdminCondition struct {
	// Type of in place set condition.
	Type PlatformAdminConditionType `json:"type,omitempty"`

	// Status of the condition, one of True, False, Unknown.
	Status corev1.ConditionStatus `json:"status,omitempty"`

	// Last time the condition transitioned from one status to another.
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`

	// The reason for the condition's last transition.
	Reason string `json:"reason,omitempty"`

	// A human readable message indicating details about the transition.
	Message string `json:"message,omitempty"`
}

// +genclient
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,path=platformadmins,shortName=pa,categories=all
// +kubebuilder:printcolumn:name="READY",type="boolean",JSONPath=".status.ready",description="The platform ready status"
// +kubebuilder:printcolumn:name="Service",type="integer",JSONPath=".status.serviceReplicas",description="The Service Replica."
// +kubebuilder:printcolumn:name="ReadyService",type="integer",JSONPath=".status.serviceReadyReplicas",description="The Ready Service Replica."
// +kubebuilder:printcolumn:name="Deployment",type="integer",JSONPath=".status.deploymentReplicas",description="The Deployment Replica."
// +kubebuilder:printcolumn:name="ReadyDeployment",type="integer",JSONPath=".status.deploymentReadyReplicas",description="The Ready Deployment Replica."
// +kubebuilder:deprecatedversion:warning="iot.openyurt.io/v1alpha1 PlatformAdmin will be deprecated in future; use iot.openyurt.io/v1alpha2 PlatformAdmin; v1alpha1 PlatformAdmin.Spec.ServiceType only support ClusterIP"
// +kubebuilder:unservedversion

// PlatformAdmin is the Schema for the samples API
type PlatformAdmin struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PlatformAdminSpec   `json:"spec,omitempty"`
	Status PlatformAdminStatus `json:"status,omitempty"`
}

func (c *PlatformAdmin) GetConditions() []PlatformAdminCondition {
	return c.Status.Conditions
}

func (c *PlatformAdmin) SetConditions(conditions []PlatformAdminCondition) {
	c.Status.Conditions = conditions
}

//+kubebuilder:object:root=true

// PlatformAdminList contains a list of PlatformAdmin
type PlatformAdminList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PlatformAdmin `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PlatformAdmin{}, &PlatformAdminList{})
}
