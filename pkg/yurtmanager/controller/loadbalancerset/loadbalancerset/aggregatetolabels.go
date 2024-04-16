/*
Copyright 2024 The OpenYurt Authors.

Licensed under the Apache License, Version 2.0 (the License);
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an AS IS BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package loadbalancerset

import (
	"reflect"

	v1 "k8s.io/api/core/v1"

	"github.com/openyurtio/openyurt/pkg/apis/network/v1alpha1"
)

func aggregatePoolServicesLabels(poolServices []v1alpha1.PoolService) map[string]string {
	var labelList []map[string]string
	for _, ps := range poolServices {
		labelList = append(labelList, ps.Status.AggregateToLabels)
	}

	return aggregatedMaps(labelList...)
}

func compareAndUpdateServiceLabels(svc *v1.Service, aggregatedLabels map[string]string) bool {
	currentAggregatedLabels := filterIgnoredKeys(svc.Labels)

	if reflect.DeepEqual(currentAggregatedLabels, aggregatedLabels) {
		return false
	}

	svc.Labels = mergeAndPurgeMap(svc.Labels, aggregatedLabels)

	return true
}
