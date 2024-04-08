/*
Copyright 2024 The OpenYurt Authors.

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
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// PoolServiceSpec defines the desired state of PoolService
type PoolServiceSpec struct {
	// Inherited from service spec.LoadBalancerClass
	LoadBalancerClass *string `json:"loadBalancerClass,omitempty"`
}

// PoolServiceStatus defines the observed state of PoolService
type PoolServiceStatus struct {
	// LoadBalancer contains the current status of the load-balancer in the current nodepool
	LoadBalancer *v1.LoadBalancerStatus `json:"loadBalancer,omitempty"`

	// Current poolService state
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +genclient
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,path=poolservices,shortName=ps,categories=all
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp",description="CreationTimestamp is a timestamp representing the server time when this object was created. It is not guaranteed to be set in happens-before order across separate operations. Clients may not set this value. It is represented in RFC3339 form and is in UTC."

// PoolService is the Schema for the samples API
type PoolService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PoolServiceSpec   `json:"spec,omitempty"`
	Status PoolServiceStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// PoolServiceList contains a list of PoolService
type PoolServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PoolService `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PoolService{}, &PoolServiceList{})
}
