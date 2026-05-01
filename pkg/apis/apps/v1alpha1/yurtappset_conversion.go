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
		if dst.Spec.DeploymentTemplate == nil {
			dst.Spec.DeploymentTemplate = &v1beta1.DeploymentTemplateSpec{}
		}
		src.Spec.WorkloadTemplate.DeploymentTemplate.ObjectMeta.DeepCopyInto(&dst.Spec.DeploymentTemplate.ObjectMeta)
		src.Spec.WorkloadTemplate.DeploymentTemplate.Spec.DeepCopyInto(&dst.Spec.DeploymentTemplate.Spec)
	}

	if src.Spec.WorkloadTemplate.StatefulSetTemplate != nil {
		if dst.Spec.StatefulSetTemplate == nil {
			dst.Spec.StatefulSetTemplate = &v1beta1.StatefulSetTemplateSpec{}
		}
		src.Spec.WorkloadTemplate.StatefulSetTemplate.ObjectMeta.DeepCopyInto(&dst.Spec.StatefulSetTemplate.ObjectMeta)
		src.Spec.WorkloadTemplate.StatefulSetTemplate.Spec.DeepCopyInto(&dst.Spec.StatefulSetTemplate.Spec)
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
	dst.Spec.WorkloadTweaks = tweaks
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

func (src *YurtAppSet) ConvertFrom(srcRaw conversion.Hub) error {
	srcRawV1beta1 := srcRaw.(*v1beta1.YurtAppSet)
	var defaultReplicas int32

	src.ObjectMeta = srcRawV1beta1.ObjectMeta

	// convert spec
	src.Spec.Selector = &metav1.LabelSelector{}
	if srcRawV1beta1.Spec.DeploymentTemplate != nil {
		if src.Spec.WorkloadTemplate.DeploymentTemplate == nil {
			src.Spec.WorkloadTemplate.DeploymentTemplate = &DeploymentTemplateSpec{}
		}
		srcRawV1beta1.Spec.DeploymentTemplate.ObjectMeta.DeepCopyInto(&src.Spec.WorkloadTemplate.DeploymentTemplate.ObjectMeta)
		srcRawV1beta1.Spec.DeploymentTemplate.Spec.DeepCopyInto(&src.Spec.WorkloadTemplate.DeploymentTemplate.Spec)
		if srcRawV1beta1.Spec.DeploymentTemplate.Spec.Replicas != nil {
			defaultReplicas = *srcRawV1beta1.Spec.DeploymentTemplate.Spec.Replicas
		} else {
			defaultReplicas = 0
		}

		if srcRawV1beta1.Spec.DeploymentTemplate.Spec.Selector != nil {
			src.Spec.WorkloadTemplate.DeploymentTemplate.Spec.Selector.DeepCopyInto(src.Spec.Selector)
		}
		src.Status.TemplateType = DeploymentTemplateType
	}

	if srcRawV1beta1.Spec.StatefulSetTemplate != nil {
		if src.Spec.WorkloadTemplate.StatefulSetTemplate == nil {
			src.Spec.WorkloadTemplate.StatefulSetTemplate = &StatefulSetTemplateSpec{}
		}
		srcRawV1beta1.Spec.StatefulSetTemplate.ObjectMeta.DeepCopyInto(&src.Spec.WorkloadTemplate.StatefulSetTemplate.ObjectMeta)
		srcRawV1beta1.Spec.StatefulSetTemplate.Spec.DeepCopyInto(&src.Spec.WorkloadTemplate.StatefulSetTemplate.Spec)
		if srcRawV1beta1.Spec.StatefulSetTemplate.Spec.Replicas != nil {
			defaultReplicas = *srcRawV1beta1.Spec.StatefulSetTemplate.Spec.Replicas
		} else {
			defaultReplicas = 0
		}

		if srcRawV1beta1.Spec.StatefulSetTemplate.Spec.Selector != nil {
			src.Spec.WorkloadTemplate.StatefulSetTemplate.Spec.Selector.DeepCopyInto(src.Spec.Selector)
		}
		src.Status.TemplateType = StatefulSetTemplateType
	}

	poolTweaks := make(map[string]*Pool, 0)
	for _, poolName := range srcRawV1beta1.Spec.Pools {
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

	for _, tweak := range srcRawV1beta1.Spec.WorkloadTweaks {
		for _, poolName := range tweak.Pools {
			if _, ok := poolTweaks[poolName]; ok && tweak.Replicas != nil {
				poolTweaks[poolName].Replicas = tweak.Replicas
			}
		}
	}

	for _, tweak := range poolTweaks {
		src.Spec.Topology.Pools = append(src.Spec.Topology.Pools, *tweak)
	}

	// convert status
	src.Status.ObservedGeneration = srcRawV1beta1.Status.ObservedGeneration
	src.Status.CollisionCount = srcRawV1beta1.Status.CollisionCount
	src.Status.CurrentRevision = srcRawV1beta1.Status.CurrentRevision
	src.Status.Conditions = make([]YurtAppSetCondition, 0)
	// this is just an estimate, because the real value can not be obtained from v1beta1 status
	src.Status.ReadyReplicas = srcRawV1beta1.Status.ReadyWorkloads * defaultReplicas
	src.Status.Replicas = srcRawV1beta1.Status.TotalWorkloads * defaultReplicas

	src.Status.PoolReplicas = make(map[string]int32)
	for _, pool := range srcRawV1beta1.Spec.Pools {
		src.Status.PoolReplicas[pool] = defaultReplicas
	}

	for _, condition := range srcRawV1beta1.Status.Conditions {
		src.Status.Conditions = append(src.Status.Conditions, YurtAppSetCondition{
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
