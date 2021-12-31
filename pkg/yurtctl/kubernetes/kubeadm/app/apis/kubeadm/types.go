/*
Copyright 2016 The Kubernetes Authors.
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

package kubeadm

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// BootstrapToken describes one bootstrap token, stored as a Secret in the cluster
// TODO: The BootstrapToken object should move out to either k8s.io/client-go or k8s.io/api in the future
// (probably as part of Bootstrap Tokens going GA). It should not be staged under the kubeadm API as it is now.
type BootstrapToken struct {
	// Token is used for establishing bidirectional trust between nodes and control-planes.
	// Used for joining nodes in the cluster.
	Token *BootstrapTokenString
	// Description sets a human-friendly message why this token exists and what it's used
	// for, so other administrators can know its purpose.
	Description string
	// TTL defines the time to live for this token. Defaults to 24h.
	// Expires and TTL are mutually exclusive.
	TTL *metav1.Duration
	// Expires specifies the timestamp when this token expires. Defaults to being set
	// dynamically at runtime based on the TTL. Expires and TTL are mutually exclusive.
	Expires *metav1.Time
	// Usages describes the ways in which this token can be used. Can by default be used
	// for establishing bidirectional trust, but that can be changed here.
	Usages []string
	// Groups specifies the extra groups that this token will authenticate as when/if
	// used for authentication
	Groups []string
}
