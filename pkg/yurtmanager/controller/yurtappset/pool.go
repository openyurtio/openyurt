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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	unitv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/yurtappset/adapter"
)

// Pool stores the details of a pool resource owned by one YurtAppSet.
type Pool struct {
	Name      string
	Namespace string
	Spec      PoolSpec
	Status    PoolStatus
}

// PoolSpec stores the spec details of the Pool
type PoolSpec struct {
	PoolRef metav1.Object
}

// PoolStatus stores the observed state of the Pool.
type PoolStatus struct {
	ObservedGeneration int64
	adapter.ReplicasInfo
	PatchInfo string
}

// ResourceRef stores the Pool resource it represents.
type ResourceRef struct {
	Resources []metav1.Object
}

// ControlInterface defines the interface that YurtAppSet uses to list, create, update, and delete Pools.
type ControlInterface interface {
	// GetAllPools returns the pools which are managed by the YurtAppSet.
	GetAllPools(yas *unitv1alpha1.YurtAppSet) ([]*Pool, error)
	// CreatePool creates the pool depending on the inputs.
	CreatePool(yas *unitv1alpha1.YurtAppSet, unit string, revision string, replicas int32) error
	// UpdatePool updates the target pool with the input information.
	UpdatePool(pool *Pool, yas *unitv1alpha1.YurtAppSet, revision string, replicas int32) error
	// DeletePool is used to delete the input pool.
	DeletePool(*Pool) error
	// GetPoolFailure extracts the pool failure message to expose on YurtAppSet status.
	GetPoolFailure(*Pool) *string
	// IsExpected check the pool is the expected revision
	IsExpected(pool *Pool, revision string) bool
}
