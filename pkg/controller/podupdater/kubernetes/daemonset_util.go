/*
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

package kubernetes

import (
	"fmt"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetTargetNodeName get the target node name of DaemonSet pods. If `.spec.NodeName` is not empty (nil),
// return `.spec.NodeName`; otherwise, retrieve node name of pending pods from NodeAffinity. Return error
// if failed to retrieve node name from `.spec.NodeName` and NodeAffinity.
func GetTargetNodeName(pod *v1.Pod) (string, error) {
	if len(pod.Spec.NodeName) != 0 {
		return pod.Spec.NodeName, nil
	}

	// Retrieve node name of unscheduled pods from NodeAffinity
	if pod.Spec.Affinity == nil ||
		pod.Spec.Affinity.NodeAffinity == nil ||
		pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
		return "", fmt.Errorf("no spec.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution for pod %s/%s",
			pod.Namespace, pod.Name)
	}

	terms := pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
	if len(terms) < 1 {
		return "", fmt.Errorf("no nodeSelectorTerms in requiredDuringSchedulingIgnoredDuringExecution of pod %s/%s",
			pod.Namespace, pod.Name)
	}

	for _, term := range terms {
		for _, exp := range term.MatchFields {
			if exp.Key == metav1.ObjectNameField &&
				exp.Operator == v1.NodeSelectorOpIn {
				if len(exp.Values) != 1 {
					return "", fmt.Errorf("the matchFields value of '%s' is not unique for pod %s/%s",
						metav1.ObjectNameField, pod.Namespace, pod.Name)
				}

				return exp.Values[0], nil
			}
		}
	}

	return "", fmt.Errorf("no node name found for pod %s/%s", pod.Namespace, pod.Name)
}
