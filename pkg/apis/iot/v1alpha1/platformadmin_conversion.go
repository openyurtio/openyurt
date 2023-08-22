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
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	"github.com/openyurtio/openyurt/pkg/apis/iot/v1alpha2"
)

func (src *PlatformAdmin) ConvertTo(dstRaw conversion.Hub) error {
	// Transform metadata
	dst := dstRaw.(*v1alpha2.PlatformAdmin)
	dst.ObjectMeta = src.ObjectMeta
	dst.TypeMeta = src.TypeMeta
	dst.TypeMeta.APIVersion = "iot.openyurt.io/v1alpha2"

	// Transform spec
	dst.Spec.Version = src.Spec.Version
	dst.Spec.Security = false
	dst.Spec.ImageRegistry = src.Spec.ImageRegistry
	dst.Spec.PoolName = src.Spec.PoolName
	dst.Spec.Platform = v1alpha2.PlatformAdminPlatformEdgeX

	// Transform status
	dst.Status.Ready = src.Status.Ready
	dst.Status.Initialized = src.Status.Initialized
	dst.Status.ReadyComponentNum = src.Status.DeploymentReadyReplicas
	dst.Status.UnreadyComponentNum = src.Status.DeploymentReplicas - src.Status.DeploymentReadyReplicas
	dst.Status.Conditions = transToV2Condition(src.Status.Conditions)

	// Transform additionaldeployment
	if len(src.Spec.AdditionalDeployment) > 0 {
		additionalDeployment, err := json.Marshal(src.Spec.AdditionalDeployment)
		if err != nil {
			return err
		}
		dst.ObjectMeta.Annotations["AdditionalDeployments"] = string(additionalDeployment)
	}

	// Transform additionalservice
	if len(src.Spec.AdditionalService) > 0 {
		additionalService, err := json.Marshal(src.Spec.AdditionalService)
		if err != nil {
			return err
		}
		dst.ObjectMeta.Annotations["AdditionalServices"] = string(additionalService)
	}

	//TODO: Components

	return nil
}

func (dst *PlatformAdmin) ConvertFrom(srcRaw conversion.Hub) error {
	// Transform metadata
	src := srcRaw.(*v1alpha2.PlatformAdmin)
	dst.ObjectMeta = src.ObjectMeta
	dst.TypeMeta = src.TypeMeta
	dst.TypeMeta.APIVersion = "iot.openyurt.io/v1alpha1"

	// Transform spec
	dst.Spec.Version = src.Spec.Version
	dst.Spec.ImageRegistry = src.Spec.ImageRegistry
	dst.Spec.PoolName = src.Spec.PoolName
	dst.Spec.ServiceType = corev1.ServiceTypeClusterIP

	// Transform status
	dst.Status.Ready = src.Status.Ready
	dst.Status.Initialized = src.Status.Initialized
	dst.Status.ServiceReadyReplicas = src.Status.ReadyComponentNum
	dst.Status.ServiceReplicas = src.Status.ReadyComponentNum + src.Status.UnreadyComponentNum
	dst.Status.DeploymentReadyReplicas = src.Status.ReadyComponentNum
	dst.Status.DeploymentReplicas = src.Status.ReadyComponentNum + src.Status.UnreadyComponentNum
	dst.Status.Conditions = transToV1Condition(src.Status.Conditions)

	// Transform additionaldeployment
	if _, ok := src.ObjectMeta.Annotations["AdditionalDeployments"]; ok {
		var additionalDeployments []DeploymentTemplateSpec = make([]DeploymentTemplateSpec, 0)
		err := json.Unmarshal([]byte(src.ObjectMeta.Annotations["AdditionalDeployments"]), &additionalDeployments)
		if err != nil {
			return err
		}
		dst.Spec.AdditionalDeployment = additionalDeployments
	}

	// Transform additionalservice
	if _, ok := src.ObjectMeta.Annotations["AdditionalServices"]; ok {
		var additionalServices []ServiceTemplateSpec = make([]ServiceTemplateSpec, 0)
		err := json.Unmarshal([]byte(src.ObjectMeta.Annotations["AdditionalServices"]), &additionalServices)
		if err != nil {
			return err
		}
		dst.Spec.AdditionalService = additionalServices
	}

	return nil
}

func transToV1Condition(c2 []v1alpha2.PlatformAdminCondition) (c1 []PlatformAdminCondition) {
	for _, ic := range c2 {
		c1 = append(c1, PlatformAdminCondition{
			Type:               PlatformAdminConditionType(ic.Type),
			Status:             ic.Status,
			LastTransitionTime: ic.LastTransitionTime,
			Reason:             ic.Reason,
			Message:            ic.Message,
		})
	}
	return
}

func transToV2Condition(c1 []PlatformAdminCondition) (c2 []v1alpha2.PlatformAdminCondition) {
	for _, ic := range c1 {
		c2 = append(c2, v1alpha2.PlatformAdminCondition{
			Type:               v1alpha2.PlatformAdminConditionType(ic.Type),
			Status:             ic.Status,
			LastTransitionTime: ic.LastTransitionTime,
			Reason:             ic.Reason,
			Message:            ic.Message,
		})
	}
	return
}
