/*
Copyright 2022 The OpenYurt Authors.

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

package constants

const (

	// NodeType flag sets the type of worker node to edge or cloud.
	NodeType = "node-type"

	// Organizations flag sets the extra organizations of hub agent client certificate.
	Organizations = "organizations"

	// NodeLabels flag sets the labels for worker node.
	NodeLabels = "node-labels"

	// PauseImage flag sets the pause image for worker node.
	PauseImage = "pause-image"

	// YurtHubImage flag sets the yurthub image for worker node.
	YurtHubImage = "yurthub-image"

	// KubernetesResourceServer flag sets the address for download k8s node resources.
	KubernetesResourceServer = "kubernetes-resource-server"
)
