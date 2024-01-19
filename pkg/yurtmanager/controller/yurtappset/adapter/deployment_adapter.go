/*
Copyright 2021 The OpenYurt Authors.
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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/openyurtio/openyurt/pkg/apis/apps"
	alpha1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
)

type DeploymentAdapter struct {
	client.Client
	Scheme *runtime.Scheme
}

var _ Adapter = &DeploymentAdapter{}

// NewResourceObject creates a empty Deployment object.
func (a *DeploymentAdapter) NewResourceObject() runtime.Object {
	return &appsv1.Deployment{}
}

// NewResourceListObject creates a empty DeploymentList object.
func (a *DeploymentAdapter) NewResourceListObject() runtime.Object {
	return &appsv1.DeploymentList{}
}

// GetStatusObservedGeneration returns the observed generation of the pool.
func (a *DeploymentAdapter) GetStatusObservedGeneration(obj metav1.Object) int64 {
	return obj.(*appsv1.Deployment).Status.ObservedGeneration
}

// GetDetails returns the replicas detail the pool needs.
func (a *DeploymentAdapter) GetDetails(obj metav1.Object) (ReplicasInfo, error) {
	set := obj.(*appsv1.Deployment)
	var specReplicas int32
	if set.Spec.Replicas != nil {
		specReplicas = *set.Spec.Replicas
	}
	replicasInfo := ReplicasInfo{
		Replicas:      specReplicas,
		ReadyReplicas: set.Status.ReadyReplicas,
	}
	return replicasInfo, nil
}

// GetAvailableStatus returns the available condition status of the workload
func (a *DeploymentAdapter) GetAvailableStatus(obj metav1.Object) (conditionStatus corev1.ConditionStatus, err error) {
	dp := obj.(*appsv1.Deployment)
	for _, condition := range dp.Status.Conditions {
		if condition.Type == appsv1.DeploymentAvailable {
			return condition.Status, nil
		}
	}
	return corev1.ConditionUnknown, nil
}

// GetPoolFailure returns the failure information of the pool.
// Deployment has no condition.
func (a *DeploymentAdapter) GetPoolFailure() *string {
	return nil
}

// ApplyPoolTemplate updates the pool to the latest revision, depending on the DeploymentTemplate.
func (a *DeploymentAdapter) ApplyPoolTemplate(yas *alpha1.YurtAppSet, poolName, revision string,
	replicas int32, obj runtime.Object) error {
	set := obj.(*appsv1.Deployment)
	var poolConfig *alpha1.Pool
	for i, pool := range yas.Spec.Topology.Pools {
		if pool.Name == poolName {
			poolConfig = &(yas.Spec.Topology.Pools[i])
			break
		}
	}
	if poolConfig == nil {
		return fmt.Errorf("could not find pool config %s", poolName)
	}
	set.Namespace = yas.Namespace
	if set.Labels == nil {
		set.Labels = map[string]string{}
	}
	for k, v := range yas.Spec.WorkloadTemplate.DeploymentTemplate.Labels {
		set.Labels[k] = v
	}
	for k, v := range yas.Spec.Selector.MatchLabels {
		set.Labels[k] = v
	}
	set.Labels[apps.ControllerRevisionHashLabelKey] = revision
	// record the pool name as a label
	set.Labels[apps.PoolNameLabelKey] = poolName
	if set.Annotations == nil {
		set.Annotations = map[string]string{}
	}
	for k, v := range yas.Spec.WorkloadTemplate.DeploymentTemplate.Annotations {
		set.Annotations[k] = v
	}
	set.GenerateName = getPoolPrefix(yas.Name, poolName)
	selectors := yas.Spec.Selector.DeepCopy()
	selectors.MatchLabels[apps.PoolNameLabelKey] = poolName
	if err := controllerutil.SetControllerReference(yas, set, a.Scheme); err != nil {
		return err
	}
	set.Spec.Selector = selectors
	set.Spec.Replicas = &replicas
	set.Spec.Strategy = *yas.Spec.WorkloadTemplate.DeploymentTemplate.Spec.Strategy.DeepCopy()
	set.Spec.Template = *yas.Spec.WorkloadTemplate.DeploymentTemplate.Spec.Template.DeepCopy()
	if set.Spec.Template.Labels == nil {
		set.Spec.Template.Labels = map[string]string{}
	}
	set.Spec.Template.Labels[apps.PoolNameLabelKey] = poolName
	set.Spec.Template.Labels[apps.ControllerRevisionHashLabelKey] = revision
	set.Spec.RevisionHistoryLimit = yas.Spec.RevisionHistoryLimit
	set.Spec.MinReadySeconds = yas.Spec.WorkloadTemplate.DeploymentTemplate.Spec.MinReadySeconds
	set.Spec.Paused = yas.Spec.WorkloadTemplate.DeploymentTemplate.Spec.Paused
	set.Spec.ProgressDeadlineSeconds = yas.Spec.WorkloadTemplate.DeploymentTemplate.Spec.ProgressDeadlineSeconds
	attachNodeAffinityAndTolerations(&set.Spec.Template.Spec, poolConfig)
	if !PoolHasPatch(poolConfig, set) {
		klog.Infof("Deployment[%s/%s-] has no patches, do not need strategicmerge", set.Namespace,
			set.GenerateName)
		return nil
	}
	patched := &appsv1.Deployment{}
	if err := CreateNewPatchedObject(poolConfig.Patch, set, patched); err != nil {
		klog.Errorf("Deployment[%s/%s-] strategic merge by patch %s error %v", set.Namespace,
			set.GenerateName, string(poolConfig.Patch.Raw), err)
		return err
	}
	patched.DeepCopyInto(set)
	klog.Infof("Deployment [%s/%s-] has patches configure successfully:%v", set.Namespace,
		set.GenerateName, string(poolConfig.Patch.Raw))
	return nil
}

// PostUpdate does some works after pool updated. Deployment will implement this method to clean stuck pods.
func (a *DeploymentAdapter) PostUpdate(yas *alpha1.YurtAppSet, obj runtime.Object, revision string) error {
	// Do nothing,
	return nil
}

// IsExpected checks the pool is the expected revision or not.
// The revision label can tell the current pool revision.
func (a *DeploymentAdapter) IsExpected(obj metav1.Object, revision string) bool {
	return obj.GetLabels()[apps.ControllerRevisionHashLabelKey] != revision
}
