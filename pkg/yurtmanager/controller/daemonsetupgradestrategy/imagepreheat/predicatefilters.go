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

package imagepreheat

import (
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/daemonsetupgradestrategy"
)

func JobFilter(obj client.Object) bool {
	job, ok := obj.(*batchv1.Job)
	if !ok {
		return false
	}
	if !strings.HasPrefix(job.Name, daemonsetupgradestrategy.ImagePullJobNamePrefix) {
		return false
	}

	if OwnerReferenceExistKind(job.OwnerReferences, "Pod") {
		return true
	}

	return false
}

func PodFilter(obj client.Object) bool {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return false
	}

	if !podOwnerIsDaemonSet(pod) {
		return false
	}

	if ExpectedPodImageReadyStatus(pod, corev1.ConditionFalse) {
		return true
	}

	return false
}

func podOwnerIsDaemonSet(pod *corev1.Pod) bool {
	if len(pod.OwnerReferences) == 0 {
		return false
	}

	for _, owner := range pod.OwnerReferences {
		if owner.APIVersion == appsv1.SchemeGroupVersion.String() &&
			owner.Kind == "DaemonSet" {
			return true
		}
	}

	return false
}
