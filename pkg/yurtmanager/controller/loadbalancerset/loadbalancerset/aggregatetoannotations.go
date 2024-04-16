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

	corev1 "k8s.io/api/core/v1"

	netv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/network/v1alpha1"
)

func aggregatePoolServicesAnnotations(poolServices []netv1alpha1.PoolService) map[string]string {
	var annotationsList []map[string]string
	for _, ps := range poolServices {
		annotationsList = append(annotationsList, ps.Status.AggregateToAnnotations)
	}

	return aggregatedMaps(annotationsList...)
}

func compareAndUpdateServiceAnnotations(svc *corev1.Service, aggregatedAnnotations map[string]string) bool {
	currentAggregatedAnnotations := filterIgnoredKeys(svc.Annotations)

	if reflect.DeepEqual(currentAggregatedAnnotations, aggregatedAnnotations) {
		return false
	}

	svc.Annotations = mergeAndPurgeMap(svc.Annotations, aggregatedAnnotations)

	return true
}
