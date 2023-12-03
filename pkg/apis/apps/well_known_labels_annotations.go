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

@CHANGELOG
OpenYurt Authors:
change some const value
*/

package apps

// YurtAppSet & YurtAppDaemon related labels and annotations
const (
	// ControllerRevisionHashLabelKey is used to record the controller revision of current resource.
	ControllerRevisionHashLabelKey = "apps.openyurt.io/controller-revision-hash"

	// PoolNameLabelKey is used to record the name of current pool.
	PoolNameLabelKey = "apps.openyurt.io/pool-name"

	// AnnotationPatchKey indicates the patch for every sub pool
	AnnotationPatchKey = "apps.openyurt.io/patch"

	AnnotationRefNodePool = "apps.openyurt.io/ref-nodepool"
)

// NodePool related labels and annotations
const (
	AnnotationPrevAttrs      = "nodepool.openyurt.io/previous-attributes"
	DesiredNodePoolLabel     = "apps.openyurt.io/desired-nodepool"
	NodePoolHostNetworkLabel = "nodepool.openyurt.io/hostnetwork"
	NodePoolChangedEvent     = "NodePoolChanged"
)
