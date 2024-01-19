/*
Copyright 2024 The OpenYurt Authors.

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

package workloadmanager

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openyurtio/openyurt/pkg/apis/apps"
	"github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
)

func getWorkloadPrefix(controllerName, nodepoolName string) string {
	prefix := fmt.Sprintf("%s-%s-", controllerName, nodepoolName)
	if len(validation.NameIsDNSSubdomain(prefix, true)) != 0 {
		prefix = fmt.Sprintf("%s-", controllerName)
	}
	return prefix
}

func CreateNodeSelectorByNodepoolName(nodepool string) map[string]string {
	return map[string]string{
		projectinfo.GetNodePoolLabel(): nodepool,
	}
}

// Get label selector from yurtappset generated from yas name
func NewLabelSelectorForYurtAppSet(yas *v1beta1.YurtAppSet) (*metav1.LabelSelector, error) {
	if yas == nil {
		return nil, fmt.Errorf("yurtappset is nil")
	} else if yas.Name == "" {
		return nil, fmt.Errorf("yurtappset name is empty")
	}

	selector := labels.Set{
		apps.YurtAppSetOwnerLabelKey: yas.Name,
	}
	return &metav1.LabelSelector{
		MatchLabels: selector,
	}, nil
}

// Get selecetd NodePools from YurtAppSet
// return sets for deduplication of NodePools
func GetNodePoolsFromYurtAppSet(cli client.Client, yas *v1beta1.YurtAppSet) (npNames sets.String, err error) {
	return getSelectedNodepools(cli, yas.Spec.Pools, yas.Spec.NodePoolSelector)
}

// Get NodePools selected by pools and npSelector
// If specified pool does not exist, it will skip
func getSelectedNodepools(cli client.Client, pools []string, npSelector *metav1.LabelSelector) (selectedNps sets.String, err error) {
	selectedNps = sets.NewString()

	// get all nodepools
	allNps := v1beta1.NodePoolList{}
	err = cli.List(context.TODO(), &allNps)
	if err != nil {
		return nil, err
	}

	// use selector to get selector matched nps
	var selector labels.Selector
	if selector, err = metav1.LabelSelectorAsSelector(npSelector); err != nil {
		return nil, err
	}

	expectedNps := sets.NewString(pools...)

	for _, np := range allNps.Items {
		if selector != nil && selector.Matches(labels.Set(np.GetLabels())) {
			selectedNps.Insert(np.Name)
		}
		if expectedNps.Has(np.Name) {
			selectedNps.Insert(np.Name)
		}
	}

	return selectedNps, nil
}

func IsNodePoolRelatedToYurtAppSet(nodePool client.Object, yas *v1beta1.YurtAppSet) bool {
	return isNodePoolRelated(nodePool, yas.Spec.Pools, yas.Spec.NodePoolSelector)
}

func isNodePoolRelated(nodePool client.Object, pools []string, npSelector *metav1.LabelSelector) bool {

	if pools != nil && StringsContain(pools, nodePool.GetName()) {
		return true
	}
	if npSelector != nil {
		npSelector, _ := metav1.LabelSelectorAsSelector(npSelector)
		if npSelector.Matches(labels.Set(nodePool.GetLabels())) {
			return true
		}
	}
	return false
}

func CombineMaps(maps ...map[string]string) map[string]string {
	result := map[string]string{}
	for _, m := range maps {
		for k, v := range m {
			result[k] = v
		}
	}
	return result
}

func StringsContain(strs []string, str string) bool {
	for _, s := range strs {
		if s == str {
			return true
		}
	}
	return false
}

func GetWorkloadRefNodePool(workload metav1.Object) string {
	if nodePool, ok := workload.GetLabels()[apps.PoolNameLabelKey]; ok {
		return nodePool
	}
	return ""
}

func GetWorkloadHash(workload metav1.Object) string {
	if hash, ok := workload.GetLabels()[apps.ControllerRevisionHashLabelKey]; ok {
		return hash
	}
	return ""
}
