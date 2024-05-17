/*
Copyright 2024 The OpenYurt Authors.

Licensed under the Apache License, Version 2.0 (the License);
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an AS IS BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openyurtio/openyurt/pkg/apis/apps"
)

const (
	NodePoolHostNetworkLabelKey       = "nodepool.openyurt.io/hostnetwork"
	NodePoolHostNetworkLabelForbidden = "true"
)

// Default implements builder.CustomDefaulter.
func (webhook *PodHandler) Default(ctx context.Context, obj runtime.Object) error {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a Pod but got a %T", obj))
	}

	// Add NodeAffinity to pods in order to avoid pods to be scheduled on the nodes in the hostNetwork mode NodePool
	if pod.Annotations[apps.AnnotationExcludeHostNetworkPool] != "true" {
		return nil
	}

	excludeHostNetworkRequirement := corev1.NodeSelectorRequirement{
		Key:      NodePoolHostNetworkLabelKey,
		Operator: corev1.NodeSelectorOpNotIn,
		Values:   []string{NodePoolHostNetworkLabelForbidden},
	}

	if pod.Spec.Affinity == nil {
		pod.Spec.Affinity = &corev1.Affinity{}
	}

	if pod.Spec.Affinity.NodeAffinity == nil {
		pod.Spec.Affinity.NodeAffinity = &corev1.NodeAffinity{}
	}

	if pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
		pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = &corev1.NodeSelector{
			NodeSelectorTerms: []corev1.NodeSelectorTerm{
				{
					MatchExpressions: []corev1.NodeSelectorRequirement{},
				},
			},
		}
	} else if len(pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms) == 0 {
		pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms = append(
			pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms,
			corev1.NodeSelectorTerm{
				MatchExpressions: []corev1.NodeSelectorRequirement{},
			})
	}

	for i, term := range pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms {
		needToAddAffinity := true
		for _, expr := range term.MatchExpressions {
			if expr.Key == NodePoolHostNetworkLabelKey && expr.Operator == corev1.NodeSelectorOpNotIn {
				if len(expr.Values) == 1 && expr.Values[0] == NodePoolHostNetworkLabelForbidden {
					needToAddAffinity = false
					break
				}
			}
		}

		if needToAddAffinity {
			pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[i].MatchExpressions = append(
				pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[i].MatchExpressions,
				excludeHostNetworkRequirement,
			)
		}
	}

	return nil
}
