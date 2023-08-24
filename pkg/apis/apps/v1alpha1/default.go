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
	"k8s.io/apimachinery/pkg/util/intstr"
	v1 "k8s.io/kubernetes/pkg/apis/core/v1"
	utilpointer "k8s.io/utils/pointer"
)

// SetDefaultsNodePool set default values for NodePool.
func SetDefaultsNodePool(obj *NodePool) {
	// example for set default value for NodePool
	if obj.Annotations == nil {
		obj.Annotations = make(map[string]string)
	}

}

// SetDefaultsYurtAppSet set default values for YurtAppSet.
func SetDefaultsYurtAppSet(obj *YurtAppSet) {

	if obj.Spec.RevisionHistoryLimit == nil {
		obj.Spec.RevisionHistoryLimit = utilpointer.Int32Ptr(10)
	}

	if obj.Spec.WorkloadTemplate.StatefulSetTemplate != nil {
		SetDefaultPodSpec(&obj.Spec.WorkloadTemplate.StatefulSetTemplate.Spec.Template.Spec)
		for i := range obj.Spec.WorkloadTemplate.StatefulSetTemplate.Spec.VolumeClaimTemplates {
			a := &obj.Spec.WorkloadTemplate.StatefulSetTemplate.Spec.VolumeClaimTemplates[i]
			v1.SetDefaults_PersistentVolumeClaim(a)
			v1.SetDefaults_ResourceList(&a.Spec.Resources.Limits)
			v1.SetDefaults_ResourceList(&a.Spec.Resources.Requests)
			v1.SetDefaults_ResourceList(&a.Status.Capacity)
		}
	}
	if obj.Spec.WorkloadTemplate.DeploymentTemplate != nil {
		SetDefaultPodSpec(&obj.Spec.WorkloadTemplate.DeploymentTemplate.Spec.Template.Spec)
	}
}

// SetDefaultPodSpec sets default pod spec
func SetDefaultPodSpec(in *corev1.PodSpec) {
	v1.SetDefaults_PodSpec(in)
	for i := range in.Volumes {
		a := &in.Volumes[i]
		v1.SetDefaults_Volume(a)
		if a.VolumeSource.HostPath != nil {
			v1.SetDefaults_HostPathVolumeSource(a.VolumeSource.HostPath)
		}
		if a.VolumeSource.Secret != nil {
			v1.SetDefaults_SecretVolumeSource(a.VolumeSource.Secret)
		}
		if a.VolumeSource.ISCSI != nil {
			v1.SetDefaults_ISCSIVolumeSource(a.VolumeSource.ISCSI)
		}
		if a.VolumeSource.RBD != nil {
			v1.SetDefaults_RBDVolumeSource(a.VolumeSource.RBD)
		}
		if a.VolumeSource.DownwardAPI != nil {
			v1.SetDefaults_DownwardAPIVolumeSource(a.VolumeSource.DownwardAPI)
			for j := range a.VolumeSource.DownwardAPI.Items {
				b := &a.VolumeSource.DownwardAPI.Items[j]
				if b.FieldRef != nil {
					v1.SetDefaults_ObjectFieldSelector(b.FieldRef)
				}
			}
		}
		if a.VolumeSource.ConfigMap != nil {
			v1.SetDefaults_ConfigMapVolumeSource(a.VolumeSource.ConfigMap)
		}
		if a.VolumeSource.AzureDisk != nil {
			v1.SetDefaults_AzureDiskVolumeSource(a.VolumeSource.AzureDisk)
		}
		if a.VolumeSource.Projected != nil {
			v1.SetDefaults_ProjectedVolumeSource(a.VolumeSource.Projected)
			for j := range a.VolumeSource.Projected.Sources {
				b := &a.VolumeSource.Projected.Sources[j]
				if b.DownwardAPI != nil {
					for k := range b.DownwardAPI.Items {
						c := &b.DownwardAPI.Items[k]
						if c.FieldRef != nil {
							v1.SetDefaults_ObjectFieldSelector(c.FieldRef)
						}
					}
				}
				if b.ServiceAccountToken != nil {
					v1.SetDefaults_ServiceAccountTokenProjection(b.ServiceAccountToken)
				}
			}
		}
		if a.VolumeSource.ScaleIO != nil {
			v1.SetDefaults_ScaleIOVolumeSource(a.VolumeSource.ScaleIO)
		}
	}
	for i := range in.InitContainers {
		a := &in.InitContainers[i]
		v1.SetDefaults_Container(a)
		for j := range a.Ports {
			b := &a.Ports[j]
			SetDefaults_ContainerPort(b)
		}
		for j := range a.Env {
			b := &a.Env[j]
			if b.ValueFrom != nil {
				if b.ValueFrom.FieldRef != nil {
					v1.SetDefaults_ObjectFieldSelector(b.ValueFrom.FieldRef)
				}
			}
		}
		v1.SetDefaults_ResourceList(&a.Resources.Limits)
		v1.SetDefaults_ResourceList(&a.Resources.Requests)
		if a.LivenessProbe != nil {
			v1.SetDefaults_Probe(a.LivenessProbe)
			if a.LivenessProbe.Handler.HTTPGet != nil {
				v1.SetDefaults_HTTPGetAction(a.LivenessProbe.Handler.HTTPGet)
			}
		}
		if a.ReadinessProbe != nil {
			v1.SetDefaults_Probe(a.ReadinessProbe)
			if a.ReadinessProbe.Handler.HTTPGet != nil {
				v1.SetDefaults_HTTPGetAction(a.ReadinessProbe.Handler.HTTPGet)
			}
		}
		if a.Lifecycle != nil {
			if a.Lifecycle.PostStart != nil {
				if a.Lifecycle.PostStart.HTTPGet != nil {
					v1.SetDefaults_HTTPGetAction(a.Lifecycle.PostStart.HTTPGet)
				}
			}
			if a.Lifecycle.PreStop != nil {
				if a.Lifecycle.PreStop.HTTPGet != nil {
					v1.SetDefaults_HTTPGetAction(a.Lifecycle.PreStop.HTTPGet)
				}
			}
		}
	}
	for i := range in.Containers {
		a := &in.Containers[i]
		// For in-place update, we set default imagePullPolicy to Always
		if a.ImagePullPolicy == "" {
			a.ImagePullPolicy = corev1.PullAlways
		}
		v1.SetDefaults_Container(a)
		for j := range a.Ports {
			b := &a.Ports[j]
			SetDefaults_ContainerPort(b)
		}
		for j := range a.Env {
			b := &a.Env[j]
			if b.ValueFrom != nil {
				if b.ValueFrom.FieldRef != nil {
					v1.SetDefaults_ObjectFieldSelector(b.ValueFrom.FieldRef)
				}
			}
		}
		v1.SetDefaults_ResourceList(&a.Resources.Limits)
		v1.SetDefaults_ResourceList(&a.Resources.Requests)
		if a.LivenessProbe != nil {
			v1.SetDefaults_Probe(a.LivenessProbe)
			if a.LivenessProbe.Handler.HTTPGet != nil {
				v1.SetDefaults_HTTPGetAction(a.LivenessProbe.Handler.HTTPGet)
			}
		}
		if a.ReadinessProbe != nil {
			v1.SetDefaults_Probe(a.ReadinessProbe)
			if a.ReadinessProbe.Handler.HTTPGet != nil {
				v1.SetDefaults_HTTPGetAction(a.ReadinessProbe.Handler.HTTPGet)
			}
		}
		if a.Lifecycle != nil {
			if a.Lifecycle.PostStart != nil {
				if a.Lifecycle.PostStart.HTTPGet != nil {
					v1.SetDefaults_HTTPGetAction(a.Lifecycle.PostStart.HTTPGet)
				}
			}
			if a.Lifecycle.PreStop != nil {
				if a.Lifecycle.PreStop.HTTPGet != nil {
					v1.SetDefaults_HTTPGetAction(a.Lifecycle.PreStop.HTTPGet)
				}
			}
		}
	}
}

// TODO fix copy from https://github.com/contiv/client-go/blob/v2.0.0-alpha.1/pkg/api/v1/defaults.go#L104
func SetDefaults_ContainerPort(obj *corev1.ContainerPort) {
	if obj.Protocol == "" {
		obj.Protocol = corev1.ProtocolTCP
	}
}

// SetDefaultsYurtStaticSet sets default values for YurtStaticSet.
func SetDefaultsYurtStaticSet(obj *YurtStaticSet) {
	// Set default upgrade strategy to "AdvancedRollingUpdate" with max-unavailable to "10%"
	strategy := &obj.Spec.UpgradeStrategy
	if strategy.Type == "" {
		strategy.Type = AdvancedRollingUpdateUpgradeStrategyType
	}
	if strategy.Type == AdvancedRollingUpdateUpgradeStrategyType && strategy.MaxUnavailable == nil {
		v := intstr.FromString("10%")
		strategy.MaxUnavailable = &v
	}

	// Set default RevisionHistoryLimit to 10
	if obj.Spec.RevisionHistoryLimit == nil {
		obj.Spec.RevisionHistoryLimit = new(int32)
		*obj.Spec.RevisionHistoryLimit = 10
	}

	podSpec := &obj.Spec.Template.Spec
	if podSpec != nil {
		SetDefaultPodSpec(podSpec)
	}

	// use YurtStaticSet name and namespace to replace name and namespace in template metadata
	obj.Spec.Template.Name = obj.Name
	obj.Spec.Template.Namespace = obj.Namespace
}

// SetDefaultsYurtAppDaemon set default values for YurtAppDaemon.
func SetDefaultsYurtAppDaemon(obj *YurtAppDaemon) {

	if obj.Spec.RevisionHistoryLimit == nil {
		obj.Spec.RevisionHistoryLimit = utilpointer.Int32Ptr(10)
	}

	if obj.Spec.WorkloadTemplate.StatefulSetTemplate != nil {
		SetDefaultPodSpec(&obj.Spec.WorkloadTemplate.StatefulSetTemplate.Spec.Template.Spec)
		for i := range obj.Spec.WorkloadTemplate.StatefulSetTemplate.Spec.VolumeClaimTemplates {
			a := &obj.Spec.WorkloadTemplate.StatefulSetTemplate.Spec.VolumeClaimTemplates[i]
			v1.SetDefaults_PersistentVolumeClaim(a)
			v1.SetDefaults_ResourceList(&a.Spec.Resources.Limits)
			v1.SetDefaults_ResourceList(&a.Spec.Resources.Requests)
			v1.SetDefaults_ResourceList(&a.Status.Capacity)
		}
	}
	if obj.Spec.WorkloadTemplate.DeploymentTemplate != nil {
		SetDefaultPodSpec(&obj.Spec.WorkloadTemplate.DeploymentTemplate.Spec.Template.Spec)
	}
}

// SetDefaultsYurtAppOverrider set default values for YurtAppOverrider.
func SetDefaultsYurtAppOverrider(obj *YurtAppOverrider) {
	// example for set default value for YurtAppOverrider

	//if len(obj.Subject.Default) == 0 {
	//	obj.Spec.Default = "set-default-value-0"
	//}
}
