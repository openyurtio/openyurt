/*
Copyright 2022 The OpenYurt Authors.

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

package util

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openyurtio/openyurt/pkg/apis/apps/v1beta2"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
)

// GetNodepool will get the nodepool with the given name
func GetNodepool(ctx context.Context, k8sClient client.Client, name string) (*v1beta2.NodePool, error) {
	pool := &v1beta2.NodePool{}
	if err := k8sClient.Get(ctx, client.ObjectKey{Name: name}, pool); err != nil {
		return nil, err
	}
	return pool, nil
}

// DeleteNodePool will delete the nodepool with the given name
func DeleteNodePool(ctx context.Context, k8sClient client.Client, name string) error {
	pool, err := GetNodepool(ctx, k8sClient, name)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	if err := k8sClient.Delete(ctx, pool); err != nil {
		return err
	}

	return nil
}

func CleanupNodePoolLabel(ctx context.Context, k8sClient client.Client) error {
	nodes := &corev1.NodeList{}
	if err := k8sClient.List(ctx, nodes); err != nil {
		return err
	}

	for _, originNode := range nodes.Items {
		labelDeleted := false
		newNode := originNode.DeepCopy()
		if newNode.Labels != nil {
			for k := range newNode.Labels {
				if k == projectinfo.GetNodePoolLabel() {
					delete(newNode.Labels, projectinfo.GetNodePoolLabel())
					labelDeleted = true
				}
			}
		}
		if labelDeleted {
			if err := k8sClient.Patch(context.TODO(), newNode, client.MergeFrom(&originNode)); err != nil {
				return err
			}
		}
	}
	return nil
}

type TestNodePool struct {
	NodePool v1beta2.NodePool
	Nodes    sets.Set[string]
}

// InitTestNodePool will create nodepools and add labels to nodes according to the pools
func InitTestNodePool(
	ctx context.Context,
	k8sClient client.Client,
	pool TestNodePool,
) error {
	err := k8sClient.Create(ctx, &pool.NodePool)
	if err != nil && !errors.IsAlreadyExists(err) {
		return err
	}

	nodes := &corev1.NodeList{}
	if err := k8sClient.List(ctx, nodes); err != nil {
		return err
	}

	for _, originNode := range nodes.Items {
		newNode := originNode.DeepCopy()
		nodeLabels := newNode.Labels
		if nodeLabels == nil {
			nodeLabels = map[string]string{}
		}

		if !pool.Nodes.Has(originNode.Name) {
			continue
		}

		nodeLabels[projectinfo.GetNodePoolLabel()] = pool.NodePool.Name
		newNode.Labels = nodeLabels
		if err := k8sClient.Patch(ctx, newNode, client.MergeFrom(&originNode)); err != nil {
			return err
		}
	}
	return nil
}

const (
	NodePoolName = "yurt-pool2"
)

// PrepareNodePoolWithNode will create a edge nodepool named "nodepool-with-node" and add the "openyurt-e2e-test-worker" node to this nodepool.
// In order for Pods to be successfully deployed in e2e tests, a nodepool with nodes needs to be created
func PrepareNodePoolWithNode(ctx context.Context, k8sClient client.Client, nodeName string) error {
	if err := k8sClient.Get(ctx, client.ObjectKey{Name: NodePoolName}, &v1beta2.NodePool{}); err == nil {
		return nil
	} else if !errors.IsNotFound(err) {
		return err
	}

	if err := k8sClient.Create(ctx, &v1beta2.NodePool{
		ObjectMeta: metav1.ObjectMeta{
			Name: NodePoolName,
		},
		Spec: v1beta2.NodePoolSpec{
			Type: v1beta2.Edge,
		}}); err != nil {
		return err
	}

	node := &corev1.Node{}
	if err := k8sClient.Get(ctx, client.ObjectKey{Name: nodeName}, node); err != nil {
		return err
	}

	patchObj := client.MergeFrom(node.DeepCopy())
	node.Labels[projectinfo.GetNodePoolLabel()] = NodePoolName

	if err := k8sClient.Patch(ctx, node, patchObj); err != nil {
		return err
	}
	return nil
}
