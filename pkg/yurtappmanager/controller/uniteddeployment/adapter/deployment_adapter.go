/*
Copyright 2019 The OpenYurt Authors.

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	alpha1 "github.com/alibaba/openyurt/pkg/yurtappmanager/apis/apps/v1alpha1"
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

// GetPoolFailure returns the failure information of the pool.
// Deployment has no condition.
func (a *DeploymentAdapter) GetPoolFailure() *string {
	return nil
}

// ApplyPoolTemplate updates the pool to the latest revision, depending on the DeploymentTemplate.
func (a *DeploymentAdapter) ApplyPoolTemplate(ud *alpha1.UnitedDeployment, poolName, revision string,
	replicas int32, obj runtime.Object) error {
	set := obj.(*appsv1.Deployment)

	var poolConfig *alpha1.Pool
	for _, pool := range ud.Spec.Topology.Pools {
		if pool.Name == poolName {
			poolConfig = &pool
			break
		}
	}
	if poolConfig == nil {
		return fmt.Errorf("fail to find pool config %s", poolName)
	}

	set.Namespace = ud.Namespace

	if set.Labels == nil {
		set.Labels = map[string]string{}
	}
	for k, v := range ud.Spec.WorkloadTemplate.DeploymentTemplate.Labels {
		set.Labels[k] = v
	}
	for k, v := range ud.Spec.Selector.MatchLabels {
		set.Labels[k] = v
	}
	set.Labels[alpha1.ControllerRevisionHashLabelKey] = revision
	// record the pool name as a label
	set.Labels[alpha1.PoolNameLabelKey] = poolName

	if set.Annotations == nil {
		set.Annotations = map[string]string{}
	}
	for k, v := range ud.Spec.WorkloadTemplate.DeploymentTemplate.Annotations {
		set.Annotations[k] = v
	}

	set.GenerateName = getPoolPrefix(ud.Name, poolName)

	selectors := ud.Spec.Selector.DeepCopy()
	selectors.MatchLabels[alpha1.PoolNameLabelKey] = poolName

	if err := controllerutil.SetControllerReference(ud, set, a.Scheme); err != nil {
		return err
	}

	set.Spec.Selector = selectors
	set.Spec.Replicas = &replicas

	set.Spec.Strategy = *ud.Spec.WorkloadTemplate.DeploymentTemplate.Spec.Strategy.DeepCopy()
	set.Spec.Template = *ud.Spec.WorkloadTemplate.DeploymentTemplate.Spec.Template.DeepCopy()
	if set.Spec.Template.Labels == nil {
		set.Spec.Template.Labels = map[string]string{}
	}
	set.Spec.Template.Labels[alpha1.PoolNameLabelKey] = poolName
	set.Spec.Template.Labels[alpha1.ControllerRevisionHashLabelKey] = revision

	set.Spec.RevisionHistoryLimit = ud.Spec.RevisionHistoryLimit
	set.Spec.MinReadySeconds = ud.Spec.WorkloadTemplate.DeploymentTemplate.Spec.MinReadySeconds
	set.Spec.Paused = ud.Spec.WorkloadTemplate.DeploymentTemplate.Spec.Paused
	set.Spec.ProgressDeadlineSeconds = ud.Spec.WorkloadTemplate.DeploymentTemplate.Spec.ProgressDeadlineSeconds

	attachNodeAffinityAndTolerations(&set.Spec.Template.Spec, poolConfig)
	return nil
}

// PostUpdate does some works after pool updated. Deployment will implement this method to clean stuck pods.
func (a *DeploymentAdapter) PostUpdate(ud *alpha1.UnitedDeployment, obj runtime.Object, revision string) error {
	// Do nothing,
	return nil
}

// IsExpected checks the pool is the expected revision or not.
// The revision label can tell the current pool revision.
func (a *DeploymentAdapter) IsExpected(obj metav1.Object, revision string) bool {
	return obj.GetLabels()[alpha1.ControllerRevisionHashLabelKey] != revision
}
