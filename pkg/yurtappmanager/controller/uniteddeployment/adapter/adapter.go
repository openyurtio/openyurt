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
change Adapter interface
*/

package adapter

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	alpha1 "github.com/alibaba/openyurt/pkg/yurtappmanager/apis/apps/v1alpha1"
)

type Adapter interface {
	// NewResourceObject creates a empty pool object.
	NewResourceObject() runtime.Object
	// NewResourceListObject creates a empty pool list object.
	NewResourceListObject() runtime.Object
	// GetStatusObservedGeneration returns the observed generation of the pool.
	GetStatusObservedGeneration(pool metav1.Object) int64
	// GetDetails returns the replicas information of the pool status.
	GetDetails(pool metav1.Object) (replicasInfo ReplicasInfo, err error)
	// GetPoolFailure returns failure information of the pool.
	GetPoolFailure() *string
	// ApplyPoolTemplate updates the pool to the latest revision.
	ApplyPoolTemplate(ud *alpha1.UnitedDeployment, poolName, revision string, replicas int32, pool runtime.Object) error
	// IsExpected checks the pool is the expected revision or not.
	// If not, UnitedDeployment will call ApplyPoolTemplate to update it.
	IsExpected(pool metav1.Object, revision string) bool
	// PostUpdate does some works after pool updated
	PostUpdate(ud *alpha1.UnitedDeployment, pool runtime.Object, revision string) error
}

type ReplicasInfo struct {
	Replicas      int32
	ReadyReplicas int32
}
