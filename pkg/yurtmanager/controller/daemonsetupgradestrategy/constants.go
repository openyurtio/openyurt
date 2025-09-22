/*
Copyright 2025 The OpenYurt Authors.

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

package daemonsetupgradestrategy

import corev1 "k8s.io/api/core/v1"

const (
	// UpdateAnnotation is the annotation key used in DaemonSet spec to indicate
	// which update strategy is selected. Currently, "OTA" and "AdvancedRollingUpdate" are supported.
	UpdateAnnotation = "apps.openyurt.io/update-strategy"

	// OTAUpdate set DaemonSet to over-the-air update mode.
	// In daemonPodUpdater controller, we add PodNeedUpgrade condition to pods.
	OTAUpdate = "OTA"
	// AutoUpdate set DaemonSet to Auto update mode.
	// In this mode, DaemonSet will keep updating even if there are not-ready nodes.
	// For more details, see https://github.com/openyurtio/openyurt/pull/921.
	AdvancedRollingUpdate = "AdvancedRollingUpdate"

	// Import corev1 if not already imported
	// import corev1 "k8s.io/api/core/v1"

	// PodNeedUpgrade indicates whether the pod is able to upgrade.
	PodNeedUpgrade = corev1.PodConditionType("PodNeedUpgrade")
	// PodImageReady indicates whether the pod image has been pulled
	PodImageReady = corev1.PodConditionType("PodImageReady")

	WaitPullImage    = "WaitPullImage"
	PullImageFail    = "PullImageFail"
	PullImageSuccess = "PullImageSuccess"

	// MaxUnavailableAnnotation is the annotation key added to DaemonSet to indicate
	// the max unavailable pods number. It's used with "apps.openyurt.io/update-strategy=AdvancedRollingUpdate".
	// If this annotation is not explicitly stated, it will be set to the default value 1.
	MaxUnavailableAnnotation = "apps.openyurt.io/max-unavailable"
	DefaultMaxUnavailable    = "10%"

	// BurstReplicas is a rate limiter for booting pods on a lot of pods.
	// The value of 250 is chosen b/c values that are too high can cause registry DoS issues.
	BurstReplicas = 250

	ImagePullJobNamePrefix = "image-pre-pull-"

	VersionPrefix = "controllerrevision: "
)
