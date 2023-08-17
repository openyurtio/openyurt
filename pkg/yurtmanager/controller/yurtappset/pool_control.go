/*
Copyright 2021 The OpenYurt Authors.
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

package yurtappset

import (
	"context"
	"errors"
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	alpha1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/refmanager"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/yurtappset/adapter"
)

// PoolControl provides pool operations of MutableSet.
type PoolControl struct {
	client.Client

	scheme  *runtime.Scheme
	adapter adapter.Adapter
}

// GetAllPools returns all of pools owned by the YurtAppSet.
func (m *PoolControl) GetAllPools(yas *alpha1.YurtAppSet) (pools []*Pool, err error) {
	selector, err := metav1.LabelSelectorAsSelector(yas.Spec.Selector)
	if err != nil {
		return nil, err
	}

	setList := m.adapter.NewResourceListObject()
	cliSetList, ok := setList.(client.ObjectList)
	if !ok {
		return nil, errors.New("fail to convert runtime object to client.ObjectList")
	}
	err = m.Client.List(context.TODO(), cliSetList, &client.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, err
	}

	manager, err := refmanager.New(m.Client, yas.Spec.Selector, yas, m.scheme)
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
func (m *PoolControl) CreatePool(yas *alpha1.YurtAppSet, poolName string, revision string,
	replicas int32) error {

	set := m.adapter.NewResourceObject()
	m.adapter.ApplyPoolTemplate(yas, poolName, revision, replicas, set)

	klog.V(4).Infof("Have %d replicas when creating Pool for YurtAppSet %s/%s", replicas, yas.Namespace, yas.Name)
	cliSet, ok := set.(client.Object)
	if !ok {
		return errors.New("fail to convert runtime.Object to client.Object")
	}
	return m.Create(context.TODO(), cliSet)
}

// UpdatePool is used to update the pool. The target Pool workload can be found with the input pool.
func (m *PoolControl) UpdatePool(pool *Pool, yas *alpha1.YurtAppSet, revision string, replicas int32) error {
	set := m.adapter.NewResourceObject()
	cliSet, ok := set.(client.Object)
	if !ok {
		return errors.New("fail to convert runtime.Object to client.Object")
	}
	var updateError error
	for i := 0; i < updateRetries; i++ {
		getError := m.Client.Get(context.TODO(), m.objectKey(pool), cliSet)
		if getError != nil {
			return getError
		}

		if err := m.adapter.ApplyPoolTemplate(yas, pool.Name, revision, replicas, set); err != nil {
			return err
		}
		updateError = m.Client.Update(context.TODO(), cliSet)
		if updateError == nil {
			break
		}
	}

	if updateError != nil {
		return updateError
	}

	return m.adapter.PostUpdate(yas, set, revision)
}

// DeletePool is called to delete the pool. The target Pool workload can be found with the input pool.
func (m *PoolControl) DeletePool(pool *Pool) error {
	set := pool.Spec.PoolRef.(runtime.Object)
	cliSet, ok := set.(client.Object)
	if !ok {
		return errors.New("fail to convert runtime.Object to client.Object")
	}
	return m.Delete(context.TODO(), cliSet, client.PropagationPolicy(metav1.DeletePropagationBackground))
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
	if data, ok := set.GetAnnotations()[alpha1.AnnotationPatchKey]; ok {
		pool.Status.PatchInfo = data
	}
	return pool, nil
}

func (m *PoolControl) objectKey(pool *Pool) client.ObjectKey {
	return types.NamespacedName{
		Namespace: pool.Namespace,
		Name:      pool.Spec.PoolRef.GetName(),
	}
}
