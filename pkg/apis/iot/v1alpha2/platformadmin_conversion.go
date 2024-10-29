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

package v1alpha2

import (
	"encoding/json"

	"sigs.k8s.io/controller-runtime/pkg/conversion"

	"github.com/openyurtio/openyurt/pkg/apis/iot/v1beta1"
)

func (src *PlatformAdmin) ConvertTo(dstRaw conversion.Hub) error {
	// Transform metadata
	dst := dstRaw.(*v1beta1.PlatformAdmin)
	dst.ObjectMeta = src.ObjectMeta
	dst.TypeMeta = src.TypeMeta
	dst.TypeMeta.APIVersion = "iot.openyurt.io/v1beta1"
	// Transform spec
	dst.Spec.Version = src.Spec.Version
	dst.Spec.Security = false
	dst.Spec.ImageRegistry = src.Spec.ImageRegistry
	dst.Spec.NodePools = []string{src.Spec.PoolName}
	dst.Spec.Platform = v1beta1.PlatformAdminPlatformEdgeX
	dst.Spec.Components = make([]v1beta1.Component, len(src.Spec.Components))
	for i, component := range src.Spec.Components {
		dst.Spec.Components[i] = v1beta1.Component{
			Name: component.Name,
		}
	}
	// Transform status
	dst.Status.Ready = src.Status.Ready
	dst.Status.Initialized = src.Status.Initialized
	dst.Status.ReadyComponentNum = src.Status.ReadyComponentNum
	dst.Status.UnreadyComponentNum = src.Status.UnreadyComponentNum
	dst.Status.Conditions = transToV1Beta1Condition(src.Status.Conditions)
	// Transform AdditionalNodepools
	if _, ok := src.ObjectMeta.Annotations["AdditionalNodepools"]; ok {
		var additionalNodePools []string
		err := json.Unmarshal([]byte(src.ObjectMeta.Annotations["AdditionalNodepools"]), &additionalNodePools)
		if err != nil {
			return err
		}
		dst.Spec.NodePools = append(dst.Spec.NodePools, additionalNodePools...)
	}
	return nil
}

func (dst *PlatformAdmin) ConvertFrom(srcRaw conversion.Hub) error {
	// Transform metadata
	src := srcRaw.(*v1beta1.PlatformAdmin)
	dst.ObjectMeta = src.ObjectMeta
	dst.TypeMeta = src.TypeMeta
	dst.TypeMeta.APIVersion = "iot.openyurt.io/v1alpha2"
	// Transform spec
	dst.Spec.Version = src.Spec.Version
	dst.Spec.Security = false
	dst.Spec.ImageRegistry = src.Spec.ImageRegistry
	dst.Spec.PoolName = src.Spec.NodePools[0]
	if len(src.Spec.NodePools) > 1 {
		additionalNodePools := src.Spec.NodePools[1:]
		additionalNodePoolsJSON, err := json.Marshal(additionalNodePools)
		if err != nil {
			return err
		}
		dst.ObjectMeta.Annotations["AdditionalNodepools"] = string(additionalNodePoolsJSON)
	}
	dst.Spec.Platform = PlatformAdminPlatformEdgeX
	dst.Spec.Components = make([]Component, len(src.Spec.Components))
	for i, component := range src.Spec.Components {
		dst.Spec.Components[i] = Component{
			Name: component.Name,
		}
	}
	// Transform status
	dst.Status.Ready = src.Status.Ready
	dst.Status.Initialized = src.Status.Initialized
	dst.Status.ReadyComponentNum = src.Status.ReadyComponentNum
	dst.Status.UnreadyComponentNum = src.Status.UnreadyComponentNum
	dst.Status.Conditions = transToV1Alpha2Condition(src.Status.Conditions)
	return nil
}

func transToV1Alpha2Condition(srcConditions []v1beta1.PlatformAdminCondition) (dstConditions []PlatformAdminCondition) {
	for _, condition := range srcConditions {
		dstConditions = append(dstConditions, PlatformAdminCondition{
			Type:               PlatformAdminConditionType(condition.Type),
			Status:             condition.Status,
			LastTransitionTime: condition.LastTransitionTime,
			Reason:             condition.Reason,
			Message:            condition.Message,
		})
	}
	return
}

func transToV1Beta1Condition(srcConditions []PlatformAdminCondition) (dstConditions []v1beta1.PlatformAdminCondition) {
	for _, condition := range srcConditions {
		dstConditions = append(dstConditions, v1beta1.PlatformAdminCondition{
			Type:               v1beta1.PlatformAdminConditionType(condition.Type),
			Status:             condition.Status,
			LastTransitionTime: condition.LastTransitionTime,
			Reason:             condition.Reason,
			Message:            condition.Message,
		})
	}
	return
}
