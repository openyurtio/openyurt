/*
Copyright 2023 The OpenYurt Authors.

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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	"github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
)

func (src *YurtAppSet) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta1.YurtAppSet)

	dst.ObjectMeta = src.ObjectMeta

	// convert spec
	if src.Spec.WorkloadTemplate.DeploymentTemplate != nil {
		if dst.Spec.Workload.WorkloadTemplate.DeploymentTemplate == nil {
			dst.Spec.Workload.WorkloadTemplate.DeploymentTemplate = &v1beta1.DeploymentTemplateSpec{}
		}
		src.Spec.WorkloadTemplate.DeploymentTemplate.ObjectMeta.DeepCopyInto(&dst.Spec.Workload.WorkloadTemplate.DeploymentTemplate.ObjectMeta)
		src.Spec.WorkloadTemplate.DeploymentTemplate.Spec.DeepCopyInto(&dst.Spec.Workload.WorkloadTemplate.DeploymentTemplate.Spec)
	}

	if src.Spec.WorkloadTemplate.StatefulSetTemplate != nil {
		if dst.Spec.Workload.WorkloadTemplate.StatefulSetTemplate == nil {
			dst.Spec.Workload.WorkloadTemplate.StatefulSetTemplate = &v1beta1.StatefulSetTemplateSpec{}
		}
		src.Spec.WorkloadTemplate.StatefulSetTemplate.ObjectMeta.DeepCopyInto(&dst.Spec.Workload.WorkloadTemplate.StatefulSetTemplate.ObjectMeta)
		src.Spec.WorkloadTemplate.StatefulSetTemplate.Spec.DeepCopyInto(&dst.Spec.Workload.WorkloadTemplate.StatefulSetTemplate.Spec)
	}

	pools := make([]string, 0)
	tweaks := make([]v1beta1.WorkloadTweak, 0)
	for _, pool := range src.Spec.Topology.Pools {
		if len(pool.NodeSelectorTerm.MatchExpressions) == 1 && pool.NodeSelectorTerm.MatchExpressions[0].Key == projectinfo.GetNodePoolLabel() ||
			pool.NodeSelectorTerm.MatchExpressions[0].Operator == corev1.NodeSelectorOpIn {
			pools = append(pools, pool.NodeSelectorTerm.MatchExpressions[0].Values...)
			if pool.Replicas != nil {
				tweaks = append(tweaks, v1beta1.WorkloadTweak{
					Pools: pool.NodeSelectorTerm.MatchExpressions[0].Values,
					Tweaks: v1beta1.Tweaks{
						Replicas: pool.Replicas,
					},
				})
			}
		}
	}
	dst.Spec.Pools = pools
	dst.Spec.Workload.WorkloadTweaks = tweaks
	dst.Spec.RevisionHistoryLimit = src.Spec.RevisionHistoryLimit

	// convert status
	dst.Status.ObservedGeneration = src.Status.ObservedGeneration
	dst.Status.CollisionCount = src.Status.CollisionCount
	dst.Status.TotalWorkloads = int32(len(src.Status.PoolReplicas))
	dst.Status.CurrentRevision = src.Status.CurrentRevision
	dst.Status.Conditions = make([]v1beta1.YurtAppSetCondition, 0)
	for _, condition := range src.Status.Conditions {
		dst.Status.Conditions = append(dst.Status.Conditions,
			v1beta1.YurtAppSetCondition{
				Type:               v1beta1.YurtAppSetConditionType(condition.Type),
				Status:             condition.Status,
				LastTransitionTime: condition.LastTransitionTime,
				Reason:             condition.Reason,
				Message:            condition.Message,
			})
	}
	readyWorkloads := 0
	for _, workload := range src.Status.WorkloadSummaries {
		if workload.Replicas != workload.ReadyReplicas {
			readyWorkloads++
		}
	}
	dst.Status.ReadyWorkloads = int32(readyWorkloads)

	klog.V(4).Infof("convert from v1alpha1 to v1beta1 for yurtappset %s", src.Name)
	return nil
}

func (dst *YurtAppSet) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta1.YurtAppSet)
	var defaultReplicas int32

	dst.ObjectMeta = src.ObjectMeta

	// convert spec
	dst.Spec.Selector = &metav1.LabelSelector{}
	if src.Spec.Workload.WorkloadTemplate.DeploymentTemplate != nil {
		if dst.Spec.WorkloadTemplate.DeploymentTemplate == nil {
			dst.Spec.WorkloadTemplate.DeploymentTemplate = &DeploymentTemplateSpec{}
		}
		src.Spec.Workload.WorkloadTemplate.DeploymentTemplate.ObjectMeta.DeepCopyInto(&dst.Spec.WorkloadTemplate.DeploymentTemplate.ObjectMeta)
		src.Spec.Workload.WorkloadTemplate.DeploymentTemplate.Spec.DeepCopyInto(&dst.Spec.WorkloadTemplate.DeploymentTemplate.Spec)
		if src.Spec.Workload.WorkloadTemplate.DeploymentTemplate.Spec.Replicas != nil {
			defaultReplicas = *src.Spec.Workload.WorkloadTemplate.DeploymentTemplate.Spec.Replicas
		} else {
			defaultReplicas = 0
		}

		if src.Spec.Workload.WorkloadTemplate.DeploymentTemplate.Spec.Selector != nil {
			dst.Spec.WorkloadTemplate.DeploymentTemplate.Spec.Selector.DeepCopyInto(dst.Spec.Selector)
		}
		dst.Status.TemplateType = DeploymentTemplateType
	}

	if src.Spec.Workload.WorkloadTemplate.StatefulSetTemplate != nil {
		if dst.Spec.WorkloadTemplate.StatefulSetTemplate == nil {
			dst.Spec.WorkloadTemplate.StatefulSetTemplate = &StatefulSetTemplateSpec{}
		}
		src.Spec.Workload.WorkloadTemplate.StatefulSetTemplate.ObjectMeta.DeepCopyInto(&dst.Spec.WorkloadTemplate.StatefulSetTemplate.ObjectMeta)
		src.Spec.Workload.WorkloadTemplate.StatefulSetTemplate.Spec.DeepCopyInto(&dst.Spec.WorkloadTemplate.StatefulSetTemplate.Spec)
		if src.Spec.Workload.WorkloadTemplate.StatefulSetTemplate.Spec.Replicas != nil {
			defaultReplicas = *src.Spec.Workload.WorkloadTemplate.StatefulSetTemplate.Spec.Replicas
		} else {
			defaultReplicas = 0
		}

		if src.Spec.Workload.WorkloadTemplate.StatefulSetTemplate.Spec.Selector != nil {
			dst.Spec.WorkloadTemplate.StatefulSetTemplate.Spec.Selector.DeepCopyInto(dst.Spec.Selector)
		}
		dst.Status.TemplateType = StatefulSetTemplateType
	}

	poolTweaks := make(map[string]*Pool, 0)
	for _, poolName := range src.Spec.Pools {
		poolTweaks[poolName] = &Pool{
			Name: poolName,
			NodeSelectorTerm: corev1.NodeSelectorTerm{
				MatchExpressions: []corev1.NodeSelectorRequirement{
					{
						Key:      projectinfo.GetNodePoolLabel(),
						Operator: corev1.NodeSelectorOpIn,
						Values:   []string{poolName},
					},
				},
			},
			Replicas: &defaultReplicas,
		}
	}

	for _, tweak := range src.Spec.Workload.WorkloadTweaks {
		for _, poolName := range tweak.Pools {
			if _, ok := poolTweaks[poolName]; ok && tweak.Tweaks.Replicas != nil {
				poolTweaks[poolName].Replicas = tweak.Tweaks.Replicas
			}
		}
	}

	for _, tweak := range poolTweaks {
		dst.Spec.Topology.Pools = append(dst.Spec.Topology.Pools, *tweak)
	}

	// convert status
	dst.Status.ObservedGeneration = src.Status.ObservedGeneration
	dst.Status.CollisionCount = src.Status.CollisionCount
	dst.Status.CurrentRevision = src.Status.CurrentRevision
	dst.Status.Conditions = make([]YurtAppSetCondition, 0)
	// this is just an estimate, because the real value can not be obtained from v1beta1 status
	dst.Status.ReadyReplicas = src.Status.ReadyWorkloads * defaultReplicas
	dst.Status.Replicas = src.Status.TotalWorkloads * defaultReplicas

	dst.Status.PoolReplicas = make(map[string]int32)
	for _, pool := range src.Spec.Pools {
		dst.Status.PoolReplicas[pool] = defaultReplicas
	}

	for _, condition := range src.Status.Conditions {
		dst.Status.Conditions = append(dst.Status.Conditions, YurtAppSetCondition{
			Type:               YurtAppSetConditionType(condition.Type),
			Status:             condition.Status,
			LastTransitionTime: condition.LastTransitionTime,
			Reason:             condition.Reason,
			Message:            condition.Message,
		})
	}

	klog.V(4).Infof("convert from v1beta1 to v1alpha1 for yurtappset %s", src.Name)
	return nil
}
