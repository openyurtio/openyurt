/*
Copyright 2020 The OpenYurt Authors.
Copyright 2019 The Kruise Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

@CHANGELOG
OpenYurt Authors:
Subset to pool
*/

package uniteddeployment

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog"

	unitv1alpha1 "github.com/alibaba/openyurt/pkg/yurtappmanager/apis/apps/v1alpha1"
	"github.com/alibaba/openyurt/pkg/yurtappmanager/util"
)

func (r *ReconcileUnitedDeployment) managePools(ud *unitv1alpha1.UnitedDeployment,
	nameToPool map[string]*Pool, nextReplicas map[string]int32,
	expectedRevision *appsv1.ControllerRevision,
	poolType unitv1alpha1.TemplateType) (newStatus *unitv1alpha1.UnitedDeploymentStatus, updateErr error) {

	newStatus = ud.Status.DeepCopy()
	exists, provisioned, err := r.managePoolProvision(ud, nameToPool, nextReplicas, expectedRevision, poolType)
	if err != nil {
		SetUnitedDeploymentCondition(newStatus, NewUnitedDeploymentCondition(unitv1alpha1.PoolProvisioned, corev1.ConditionFalse, "Error", err.Error()))
		return newStatus, fmt.Errorf("fail to manage Pool provision: %s", err)
	}

	if provisioned {
		SetUnitedDeploymentCondition(newStatus, NewUnitedDeploymentCondition(unitv1alpha1.PoolProvisioned, corev1.ConditionTrue, "", ""))
	}

	var needUpdate []string
	for _, name := range exists.List() {
		pool := nameToPool[name]
		if r.poolControls[poolType].IsExpected(pool, expectedRevision.Name) ||
			pool.Status.ReplicasInfo.Replicas != nextReplicas[name] {
			needUpdate = append(needUpdate, name)
		}
	}

	if len(needUpdate) > 0 {
		_, updateErr = util.SlowStartBatch(len(needUpdate), slowStartInitialBatchSize, func(index int) error {
			cell := needUpdate[index]
			pool := nameToPool[cell]
			replicas := nextReplicas[cell]

			klog.V(0).Infof("UnitedDeployment %s/%s needs to update Pool (%s) %s/%s with revision %s, replicas %d ",
				ud.Namespace, ud.Name, poolType, pool.Namespace, pool.Name, expectedRevision.Name, replicas)

			updatePoolErr := r.poolControls[poolType].UpdatePool(pool, ud, expectedRevision.Name, replicas)
			if updatePoolErr != nil {
				r.recorder.Event(ud.DeepCopy(), corev1.EventTypeWarning, fmt.Sprintf("Failed%s", eventTypePoolsUpdate), fmt.Sprintf("Error updating PodSet (%s) %s when updating: %s", poolType, pool.Name, updatePoolErr))
			}
			return updatePoolErr
		})
	}

	if updateErr == nil {
		SetUnitedDeploymentCondition(newStatus, NewUnitedDeploymentCondition(unitv1alpha1.PoolUpdated, corev1.ConditionTrue, "", ""))
	} else {
		SetUnitedDeploymentCondition(newStatus, NewUnitedDeploymentCondition(unitv1alpha1.PoolUpdated, corev1.ConditionFalse, "Error", updateErr.Error()))
	}
	return
}

func (r *ReconcileUnitedDeployment) managePoolProvision(ud *unitv1alpha1.UnitedDeployment,
	nameToPool map[string]*Pool, nextReplicas map[string]int32,
	expectedRevision *appsv1.ControllerRevision, workloadType unitv1alpha1.TemplateType) (sets.String, bool, error) {
	expectedPools := sets.String{}
	gotPools := sets.String{}

	for _, pool := range ud.Spec.Topology.Pools {
		expectedPools.Insert(pool.Name)
	}

	for poolName := range nameToPool {
		gotPools.Insert(poolName)
	}
	klog.V(4).Infof("UnitedDeployment %s/%s has pools %v, expects pools %v", ud.Namespace, ud.Name, gotPools.List(), expectedPools.List())

	var creates []string
	for _, expectPool := range expectedPools.List() {
		if gotPools.Has(expectPool) {
			continue
		}

		creates = append(creates, expectPool)
	}

	var deletes []string
	for _, gotPool := range gotPools.List() {
		if expectedPools.Has(gotPool) {
			continue
		}

		deletes = append(deletes, gotPool)
	}

	revision := expectedRevision.Name

	var errs []error
	// manage creating
	if len(creates) > 0 {
		// do not consider deletion
		klog.V(0).Infof("UnitedDeployment %s/%s needs creating pool (%s) with name: %v", ud.Namespace, ud.Name, workloadType, creates)
		createdPools := make([]string, len(creates))
		for i, pool := range creates {
			createdPools[i] = pool
		}

		var createdNum int
		var createdErr error
		createdNum, createdErr = util.SlowStartBatch(len(creates), slowStartInitialBatchSize, func(idx int) error {
			poolName := createdPools[idx]

			replicas := nextReplicas[poolName]
			err := r.poolControls[workloadType].CreatePool(ud, poolName, revision, replicas)
			if err != nil {
				if !errors.IsTimeout(err) {
					return fmt.Errorf("fail to create Pool (%s) %s: %s", workloadType, poolName, err.Error())
				}
			}

			return nil
		})
		if createdErr == nil {
			r.recorder.Eventf(ud.DeepCopy(), corev1.EventTypeNormal, fmt.Sprintf("Successful%s", eventTypePoolsUpdate), "Create %d Pool (%s)", createdNum, workloadType)
		} else {
			errs = append(errs, createdErr)
		}
	}

	// manage deleting
	if len(deletes) > 0 {
		klog.V(0).Infof("UnitedDeployment %s/%s needs deleting pool (%s) with name: [%v]", ud.Namespace, ud.Name, workloadType, deletes)
		var deleteErrs []error
		for _, poolName := range deletes {
			pool := nameToPool[poolName]
			if err := r.poolControls[workloadType].DeletePool(pool); err != nil {
				deleteErrs = append(deleteErrs, fmt.Errorf("fail to delete Pool (%s) %s/%s for %s: %s", workloadType, pool.Namespace, pool.Name, poolName, err))
			}
		}

		if len(deleteErrs) > 0 {
			errs = append(errs, deleteErrs...)
		} else {
			r.recorder.Eventf(ud.DeepCopy(), corev1.EventTypeNormal, fmt.Sprintf("Successful%s", eventTypePoolsUpdate), "Delete %d Pool (%s)", len(deletes), workloadType)
		}
	}

	// clean the other kind of pools
	// maybe user can chagne ud.Spec.WorkloadTemplate
	cleaned := false
	for t, control := range r.poolControls {
		if t == workloadType {
			continue
		}

		pools, err := control.GetAllPools(ud)
		if err != nil {
			errs = append(errs, fmt.Errorf("fail to list Pool of other type %s for UnitedDeployment %s/%s: %s", t, ud.Namespace, ud.Name, err))
			continue
		}

		for _, pool := range pools {
			cleaned = true
			if err := control.DeletePool(pool); err != nil {
				errs = append(errs, fmt.Errorf("fail to delete Pool %s of other type %s for UnitedDeployment %s/%s: %s", pool.Name, t, ud.Namespace, ud.Name, err))
				continue
			}
		}
	}

	return expectedPools.Intersection(gotPools), len(creates) > 0 || len(deletes) > 0 || cleaned, utilerrors.NewAggregate(errs)
}
