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

func (c *PlatformAdmin) ConvertTo(dstRaw conversion.Hub) error {
	// Transform metadata
	dst := dstRaw.(*v1beta1.PlatformAdmin)
	dst.ObjectMeta = c.ObjectMeta
	dst.TypeMeta = c.TypeMeta
	dst.APIVersion = "iot.openyurt.io/v1beta1"
	// Transform spec
	dst.Spec.Version = c.Spec.Version
	dst.Spec.Security = false
	dst.Spec.ImageRegistry = c.Spec.ImageRegistry
	dst.Spec.NodePools = []string{c.Spec.PoolName}
	dst.Spec.Platform = v1beta1.PlatformAdminPlatformEdgeX
	dst.Spec.Components = make([]v1beta1.Component, len(c.Spec.Components))
	for i, component := range c.Spec.Components {
		dst.Spec.Components[i] = v1beta1.Component{
			Name: component.Name,
		}
	}
	// Transform status
	dst.Status.Ready = c.Status.Ready
	dst.Status.Initialized = c.Status.Initialized
	dst.Status.ReadyComponentNum = c.Status.ReadyComponentNum
	dst.Status.UnreadyComponentNum = c.Status.UnreadyComponentNum
	dst.Status.Conditions = transToV1Beta1Condition(c.Status.Conditions)
	// Transform AdditionalNodepools
	if _, ok := c.Annotations["AdditionalNodepools"]; ok {
		var additionalNodePools []string
		err := json.Unmarshal([]byte(c.Annotations["AdditionalNodepools"]), &additionalNodePools)
		if err != nil {
			return err
		}
		dst.Spec.NodePools = append(dst.Spec.NodePools, additionalNodePools...)
	}
	return nil
}

func (c *PlatformAdmin) ConvertFrom(srcRaw conversion.Hub) error {
	// Transform metadata
	srcRawV1beta1 := srcRaw.(*v1beta1.PlatformAdmin)
	c.ObjectMeta = srcRawV1beta1.ObjectMeta
	c.TypeMeta = srcRawV1beta1.TypeMeta
	c.APIVersion = "iot.openyurt.io/v1alpha2"
	// Transform spec
	c.Spec.Version = srcRawV1beta1.Spec.Version
	c.Spec.Security = false
	c.Spec.ImageRegistry = srcRawV1beta1.Spec.ImageRegistry
	c.Spec.PoolName = srcRawV1beta1.Spec.NodePools[0]
	if len(srcRawV1beta1.Spec.NodePools) > 1 {
		additionalNodePools := srcRawV1beta1.Spec.NodePools[1:]
		additionalNodePoolsJSON, err := json.Marshal(additionalNodePools)
		if err != nil {
			return err
		}
		c.Annotations["AdditionalNodepools"] = string(additionalNodePoolsJSON)
	}
	c.Spec.Platform = PlatformAdminPlatformEdgeX
	c.Spec.Components = make([]Component, len(srcRawV1beta1.Spec.Components))
	for i, component := range srcRawV1beta1.Spec.Components {
		c.Spec.Components[i] = Component{
			Name: component.Name,
		}
	}
	// Transform status
	c.Status.Ready = srcRawV1beta1.Status.Ready
	c.Status.Initialized = srcRawV1beta1.Status.Initialized
	c.Status.ReadyComponentNum = srcRawV1beta1.Status.ReadyComponentNum
	c.Status.UnreadyComponentNum = srcRawV1beta1.Status.UnreadyComponentNum
	c.Status.Conditions = transToV1Alpha2Condition(srcRawV1beta1.Status.Conditions)
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
