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

package initializer

import (
	"errors"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
	"github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
)

const (
	LabelNodePoolName = "openyurt.io/pool-name"
)

// WantsNodesGetterAndSynced is an interface for setting nodes getter and synced
type WantsNodesGetterAndSynced interface {
	SetNodesGetterAndSynced(filter.NodesInPoolGetter, cache.InformerSynced, bool) error
}

// imageCustomizationInitializer is responsible for initializing extra filters(except discardcloudservice, masterservice, servicetopology)
type nodesInitializer struct {
	enablePoolTopology bool
	nodesGetter        filter.NodesInPoolGetter
	nodesSynced        cache.InformerSynced
}

// NewNodesInitializer creates an filterInitializer object
func NewNodesInitializer(enableNodePool, enablePoolServiceTopology bool, dynamicInformerFactory dynamicinformer.DynamicSharedInformerFactory) filter.Initializer {
	var nodesGetter filter.NodesInPoolGetter
	var nodesSynced cache.InformerSynced
	var enablePoolTopology bool
	if enablePoolServiceTopology {
		enablePoolTopology = true
		nodesGetter, nodesSynced = createNodeGetterAndSyncedByNodeBucket(dynamicInformerFactory)
	} else if enableNodePool {
		enablePoolTopology = true
		nodesGetter, nodesSynced = createNodeGetterAndSyncedByNodePool(dynamicInformerFactory)
	} else {
		enablePoolTopology = false
		nodesSynced = func() bool {
			return true
		}
	}

	return &nodesInitializer{
		enablePoolTopology: enablePoolTopology,
		nodesGetter:        nodesGetter,
		nodesSynced:        nodesSynced,
	}
}

func createNodeGetterAndSyncedByNodeBucket(dynamicInformerFactory dynamicinformer.DynamicSharedInformerFactory) (filter.NodesInPoolGetter, cache.InformerSynced) {
	gvr := v1alpha1.GroupVersion.WithResource("nodebuckets")
	nodesSynced := dynamicInformerFactory.ForResource(gvr).Informer().HasSynced
	lister := dynamicInformerFactory.ForResource(gvr).Lister()
	nodesGetter := func(poolName string) ([]string, error) {
		nodes := make([]string, 0)
		if buckets, err := lister.List(labels.Set{LabelNodePoolName: poolName}.AsSelector()); err != nil {
			klog.Warningf("could not get node buckets for pool(%s), %v", poolName, err)
			return nodes, err
		} else {
			for i := range buckets {
				var nodeBucket *v1alpha1.NodeBucket
				switch bucket := buckets[i].(type) {
				case *v1alpha1.NodeBucket:
					nodeBucket = bucket
				case *unstructured.Unstructured:
					nodeBucket = new(v1alpha1.NodeBucket)
					if err := runtime.DefaultUnstructuredConverter.FromUnstructured(bucket.UnstructuredContent(), nodeBucket); err != nil {
						klog.Warningf("object(%s) is not a v1alpha1.NodeBucket, %v", nodeBucket.GetName(), err)
						return nodes, err
					}
				default:
					klog.Warningf("object(%s) is an unknown type", bucket.GetObjectKind().GroupVersionKind().String())
					return nodes, errors.New("object is an unknown type")
				}
				for _, node := range nodeBucket.Nodes {
					nodes = append(nodes, node.Name)
				}
			}
		}
		return nodes, nil
	}
	return nodesGetter, nodesSynced
}

func createNodeGetterAndSyncedByNodePool(dynamicInformerFactory dynamicinformer.DynamicSharedInformerFactory) (filter.NodesInPoolGetter, cache.InformerSynced) {
	gvr := v1beta1.GroupVersion.WithResource("nodepools")
	nodesSynced := dynamicInformerFactory.ForResource(gvr).Informer().HasSynced
	lister := dynamicInformerFactory.ForResource(gvr).Lister()
	nodesGetter := func(poolName string) ([]string, error) {
		nodes := make([]string, 0)
		runtimeObj, err := lister.Get(poolName)
		if err != nil {
			klog.Warningf("could not get nodepool %s, err: %v", poolName, err)
			return nodes, err
		}
		var nodePool *v1beta1.NodePool
		switch poolObj := runtimeObj.(type) {
		case *v1beta1.NodePool:
			nodePool = poolObj
		case *unstructured.Unstructured:
			nodePool = new(v1beta1.NodePool)
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(poolObj.UnstructuredContent(), nodePool); err != nil {
				klog.Warningf("object(%s) is not a v1beta1.NodePool, %v", poolObj.GetName(), err)
				return nodes, err
			}
		default:
			klog.Warningf("object(%s) is an unknown type", poolObj.GetObjectKind().GroupVersionKind().String())
			return nodes, errors.New("object is an unknown type")
		}

		nodes = append(nodes, nodePool.Status.Nodes...)
		return nodes, nil
	}
	return nodesGetter, nodesSynced
}

func (ni *nodesInitializer) Initialize(ins filter.ObjectFilter) error {
	if wants, ok := ins.(WantsNodesGetterAndSynced); ok {
		if err := wants.SetNodesGetterAndSynced(ni.nodesGetter, ni.nodesSynced, ni.enablePoolTopology); err != nil {
			return err
		}
	}
	return nil
}
