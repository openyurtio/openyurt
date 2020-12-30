/*
Copyright 2020 The OpenYurt Authors.
Copyright 2019 The Kruise Authors.

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

package adapter

import (
	"fmt"

	unitv1alpha1 "github.com/alibaba/openyurt/pkg/yurtappmanager/apis/apps/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func getPoolPrefix(controllerName, poolName string) string {
	prefix := fmt.Sprintf("%s-%s-", controllerName, poolName)
	if len(validation.NameIsDNSSubdomain(prefix, true)) != 0 {
		prefix = fmt.Sprintf("%s-", controllerName)
	}
	return prefix
}

func attachNodeAffinityAndTolerations(podSpec *corev1.PodSpec, pool *unitv1alpha1.Pool) {
	attachNodeAffinity(podSpec, pool)
	attachTolerations(podSpec, pool)
}

func attachNodeAffinity(podSpec *corev1.PodSpec, pool *unitv1alpha1.Pool) {
	if podSpec.Affinity == nil {
		podSpec.Affinity = &corev1.Affinity{}
	}

	if podSpec.Affinity.NodeAffinity == nil {
		podSpec.Affinity.NodeAffinity = &corev1.NodeAffinity{}
	}

	if podSpec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
		podSpec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = &corev1.NodeSelector{}
	}

	if podSpec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms == nil {
		podSpec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms = []corev1.NodeSelectorTerm{}
	}

	if len(podSpec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms) == 0 {
		podSpec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms = append(podSpec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms, corev1.NodeSelectorTerm{})
	}

	for _, matchExpression := range pool.NodeSelectorTerm.MatchExpressions {
		for i, term := range podSpec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms {
			term.MatchExpressions = append(term.MatchExpressions, matchExpression)
			podSpec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[i] = term
		}
	}
}

func attachTolerations(podSpec *corev1.PodSpec, poolConfig *unitv1alpha1.Pool) {

	if poolConfig.Tolerations == nil {
		return
	}

	if podSpec.Tolerations == nil {
		podSpec.Tolerations = []corev1.Toleration{}
	}

	for _, toleration := range poolConfig.Tolerations {
		podSpec.Tolerations = append(podSpec.Tolerations, toleration)
	}

	return
}

func getRevision(objMeta metav1.Object) string {
	if objMeta.GetLabels() == nil {
		return ""
	}
	return objMeta.GetLabels()[unitv1alpha1.ControllerRevisionHashLabelKey]
}

// getCurrentPartition calculates current partition by counting the pods not having the updated revision
func getCurrentPartition(pods []*corev1.Pod, revision string) *int32 {
	var partition int32
	for _, pod := range pods {
		if getRevision(&pod.ObjectMeta) != revision {
			partition++
		}
	}

	return &partition
}
