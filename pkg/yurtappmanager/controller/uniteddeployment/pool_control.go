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
*/

package uniteddeployment

import (
	"context"
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"

	alpha1 "github.com/alibaba/openyurt/pkg/yurtappmanager/apis/apps/v1alpha1"
	"github.com/alibaba/openyurt/pkg/yurtappmanager/controller/uniteddeployment/adapter"
	"github.com/alibaba/openyurt/pkg/yurtappmanager/util/refmanager"
)

// PoolControl provides pool operations of MutableSet.
type PoolControl struct {
	client.Client

	scheme  *runtime.Scheme
	adapter adapter.Adapter
}

// GetAllPools returns all of pools owned by the UnitedDeployment.
func (m *PoolControl) GetAllPools(ud *alpha1.UnitedDeployment) (pools []*Pool, err error) {
	selector, err := metav1.LabelSelectorAsSelector(ud.Spec.Selector)
	if err != nil {
		return nil, err
	}

	setList := m.adapter.NewResourceListObject()
	err = m.Client.List(context.TODO(), setList, &client.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, err
	}

	manager, err := refmanager.New(m.Client, ud.Spec.Selector, ud, m.scheme)
	if err != nil {
		return nil, err
	}

	v := reflect.ValueOf(setList).Elem().FieldByName("Items")
	selected := make([]metav1.Object, v.Len())
	for i := 0; i < v.Len(); i++ {
		selected[i] = v.Index(i).Addr().Interface().(metav1.Object)
	}
	claimedSets, err := manager.ClaimOwnedObjects(selected)
	if err != nil {
		return nil, err
	}

	for _, claimedSet := range claimedSets {
		pool, err := m.convertToPool(claimedSet)
		if err != nil {
			return nil, err
		}
		pools = append(pools, pool)
	}
	return pools, nil
}

// CreatePool creates the Pool depending on the inputs.
func (m *PoolControl) CreatePool(ud *alpha1.UnitedDeployment, poolName string, revision string,
	replicas int32) error {

	set := m.adapter.NewResourceObject()
	m.adapter.ApplyPoolTemplate(ud, poolName, revision, replicas, set)

	klog.V(4).Infof("Have %d replicas when creating Pool for UnitedDeployment %s/%s", replicas, ud.Namespace, ud.Name)
	return m.Create(context.TODO(), set)
}

// UpdatePool is used to update the pool. The target Pool workload can be found with the input pool.
func (m *PoolControl) UpdatePool(pool *Pool, ud *alpha1.UnitedDeployment, revision string, replicas int32) error {
	set := m.adapter.NewResourceObject()
	var updateError error
	for i := 0; i < updateRetries; i++ {
		getError := m.Client.Get(context.TODO(), m.objectKey(pool), set)
		if getError != nil {
			return getError
		}

		if err := m.adapter.ApplyPoolTemplate(ud, pool.Name, revision, replicas, set); err != nil {
			return err
		}
		updateError = m.Client.Update(context.TODO(), set)
		if updateError == nil {
			break
		}
	}

	if updateError != nil {
		return updateError
	}

	return m.adapter.PostUpdate(ud, set, revision)
}

// DeletePool is called to delete the pool. The target Pool workload can be found with the input pool.
func (m *PoolControl) DeletePool(pool *Pool) error {
	set := pool.Spec.PoolRef.(runtime.Object)
	return m.Delete(context.TODO(), set, client.PropagationPolicy(metav1.DeletePropagationBackground))
}

// GetPoolFailure return the error message extracted form Pool workload status conditions.
func (m *PoolControl) GetPoolFailure(pool *Pool) *string {
	return m.adapter.GetPoolFailure()
}

// IsExpected checks the pool is expected revision or not.
func (m *PoolControl) IsExpected(pool *Pool, revision string) bool {
	return m.adapter.IsExpected(pool.Spec.PoolRef, revision)
}

func (m *PoolControl) convertToPool(set metav1.Object) (*Pool, error) {
	poolName, err := getPoolNameFrom(set)
	if err != nil {
		return nil, err
	}
	specReplicas, err := m.adapter.GetDetails(set)
	if err != nil {
		return nil, err
	}

	pool := &Pool{
		Name:      poolName,
		Namespace: set.GetNamespace(),
		Spec: PoolSpec{
			PoolRef: set,
		},
		Status: PoolStatus{
			ObservedGeneration: m.adapter.GetStatusObservedGeneration(set),
			ReplicasInfo:       specReplicas,
		},
	}
	return pool, nil
}

func (m *PoolControl) objectKey(pool *Pool) client.ObjectKey {
	return types.NamespacedName{
		Namespace: pool.Namespace,
		Name:      pool.Spec.PoolRef.GetName(),
	}
}
