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
	unitv1alpha1 "github.com/alibaba/openyurt/pkg/yurtappmanager/apis/apps/v1alpha1"
)

func GetNextReplicas(ud *unitv1alpha1.UnitedDeployment) map[string]int32 {
	next := make(map[string]int32)
	for _, pool := range ud.Spec.Topology.Pools {
		next[pool.Name] = 0
		if pool.Replicas != nil {
			next[pool.Name] = *pool.Replicas
		}
	}
	return next
}
