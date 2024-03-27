/*
Copyright 2016 The Kubernetes Authors.

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

package apparmor

import (
	v1 "k8s.io/api/core/v1"
)

// GetProfileName returns the name of the profile to use with the container.
func GetProfileName(pod *v1.Pod, containerName string) string {
	return GetProfileNameFromPodAnnotations(pod.Annotations, containerName)
}

// GetProfileNameFromPodAnnotations gets the name of the profile to use with container from
// pod annotations
func GetProfileNameFromPodAnnotations(annotations map[string]string, containerName string) string {
	return annotations[v1.AppArmorBetaContainerAnnotationKeyPrefix+containerName]
}

// SetProfileName sets the name of the profile to use with the container.
func SetProfileName(pod *v1.Pod, containerName, profileName string) error {
	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}
	pod.Annotations[v1.AppArmorBetaContainerAnnotationKeyPrefix+containerName] = profileName
	return nil
}

// SetProfileNameFromPodAnnotations sets the name of the profile to use with the container.
func SetProfileNameFromPodAnnotations(annotations map[string]string, containerName, profileName string) error {
	if annotations == nil {
		return nil
	}
	annotations[v1.AppArmorBetaContainerAnnotationKeyPrefix+containerName] = profileName
	return nil
}
