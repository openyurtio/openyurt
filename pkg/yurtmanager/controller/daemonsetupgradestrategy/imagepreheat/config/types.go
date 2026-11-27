/*
Copyright 2026 The OpenYurt Authors.

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

package config

const (
	// BackendJob preheats images by creating one Job per pod on the pod's node.
	BackendJob = "job"
	// BackendDragonfly preheats images through Dragonfly P2P preheat jobs, batched per NodePool.
	BackendDragonfly = "dragonfly"
)

// ImagePreheatControllerConfiguration contains elements describing ImagePreheatController.
type ImagePreheatControllerConfiguration struct {
	// Backend selects how images are preheated: "job" (default) or "dragonfly".
	Backend string
	// DragonflyManagerAddress is the base address of the Dragonfly manager, e.g. http://dragonfly-manager.dragonfly-system:8080.
	DragonflyManagerAddress string
	// DragonflyTokenSecret is the "namespace/name" of a Secret holding a Dragonfly personal access token under key "token".
	DragonflyTokenSecret string
	// DragonflyInsecureRegistries lists registry hosts that are addressed over plain http when building manifest urls.
	DragonflyInsecureRegistries []string
}
