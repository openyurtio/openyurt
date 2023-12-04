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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Node represents a specified node in the nodepool
type Node struct {
	// Name is the name of node
	Name string `json:"name,omitempty"`
}

// +genclient
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,path=nodebuckets,shortName=nb,categories=all
// +kubebuilder:printcolumn:name="NUM-NODES",type="integer",JSONPath=".numNodes",description="NumNodes represents the number of nodes in the NodeBucket."
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp",description="CreationTimestamp is a timestamp representing the server time when this object was created. It is not guaranteed to be set in happens-before order across separate operations. Clients may not set this value. It is represented in RFC3339 form and is in UTC."

// NodeBucket is the Schema for the samples API
type NodeBucket struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// NumNodes represents the number of nodes in the nodebucket
	NumNodes int32 `json:"numNodes"`

	// Nodes represents a subset nodes in the nodepool
	Nodes []Node `json:"nodes"`
}

//+kubebuilder:object:root=true

// NodeBucketList contains a list of NodeBucket
type NodeBucketList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodeBucket `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NodeBucket{}, &NodeBucketList{})
}
