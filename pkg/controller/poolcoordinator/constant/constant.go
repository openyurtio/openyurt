/*
Copyright 2022 The OpenYurt Authors.
Copyright 2017 The Kubernetes Authors.

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

package constant

const (
	DelegateHeartBeat = "openyurt.io/delegate-heartbeat"

	// when node cannot reach api-server directly but can be delegated lease, we should taint the node as unschedulable
	NodeNotSchedulableTaint = "node.openyurt.io/unschedulable"

	// PodBindingAnnotation can be added into pod annotation, which indicates that this pod will be bound to the node that it is scheduled to.
	PodBindingAnnotation = "apps.openyurt.io/binding"

	// number of lease intervals passed before we taint/detaint node as unschedulable
	LeaseDelegationThreshold = 4
)
