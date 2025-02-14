/*
Copyright 2025 The OpenYurt Authors.

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

package v1beta2

import (
	"context"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openyurtio/openyurt/pkg/apis/apps"
	"github.com/openyurtio/openyurt/pkg/apis/apps/v1beta2"
)

// Default satisfies the defaulting webhook interface.
func (webhook *NodePoolHandler) Default(ctx context.Context, obj runtime.Object) error {
	np, ok := obj.(*v1beta2.NodePool)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a NodePool but got a %T", obj))
	}

	// specify default type as Edge
	if len(np.Spec.Type) == 0 {
		np.Spec.Type = v1beta2.Edge
	}

	if np.Labels == nil {
		np.Labels = map[string]string{
			apps.NodePoolTypeLabel: strings.ToLower(string(np.Spec.Type)),
		}
	} else {
		np.Labels[apps.NodePoolTypeLabel] = strings.ToLower(string(np.Spec.Type))
	}

	// init node pool status
	np.Status = v1beta2.NodePoolStatus{
		ReadyNodeNum:   0,
		UnreadyNodeNum: 0,
		Nodes:          make([]string, 0),
	}

	// Set default election strategy
	if np.Spec.LeaderElectionStrategy == "" {
		np.Spec.LeaderElectionStrategy = string(v1beta2.ElectionStrategyRandom)
	}

	// Set default LeaderReplicas
	if np.Spec.LeaderReplicas <= 0 {
		np.Spec.LeaderReplicas = 1
	}

	// Set default PoolScopeMetadata
	defaultPoolScopeMetadata := []v1.GroupVersionResource{
		{
			Group:    "",
			Version:  "v1",
			Resource: "services",
		},
		{
			Group:    "discovery.k8s.io",
			Version:  "v1",
			Resource: "endpointslices",
		},
	}

	// Ensure defaultPoolScopeMetadata
	// Hash existing PoolScopeMetadata
	gvrMap := make(map[v1.GroupVersionResource]struct{})
	for _, m := range np.Spec.PoolScopeMetadata {
		gvrMap[m] = struct{}{}
	}

	// Add missing defaultPoolScopeMetadata
	for _, m := range defaultPoolScopeMetadata {
		if _, ok := gvrMap[m]; !ok {
			np.Spec.PoolScopeMetadata = append(np.Spec.PoolScopeMetadata, m)
		}
	}

	return nil
}
