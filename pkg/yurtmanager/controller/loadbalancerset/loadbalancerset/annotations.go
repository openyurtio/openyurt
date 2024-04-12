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
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/openyurtio/openyurt/pkg/apis/network"
	netv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/network/v1alpha1"
)

func aggregatePoolServicesAnnotations(poolServices []netv1alpha1.PoolService) map[string]string {
	aggregatedAnnotations := make(map[string]string)
	for _, ps := range poolServices {
		aggregatedAnnotations = mergeAnnotations(aggregatedAnnotations, filterIgnoredKeys(ps.Annotations))
	}

	return aggregatedAnnotations
}

func filterIgnoredKeys(annotations map[string]string) map[string]string {
	newAnnotations := make(map[string]string)
	for key, value := range annotations {
		if key == network.AnnotationNodePoolSelector {
			continue
		}
		if !strings.HasPrefix(key, network.AggregateAnnotationsKeyPrefix) {
			continue
		}
		newAnnotations[key] = value
	}
	return newAnnotations
}

func mergeAnnotations(m map[string]string, elem map[string]string) map[string]string {
	if len(elem) == 0 {
		return m
	}

	if m == nil {
		m = make(map[string]string)
	}

	for k, v := range elem {
		m[k] = mergeAnnotationValue(m[k], v)
	}

	return m
}

func mergeAnnotationValue(originalValue, addValue string) string {
	if len(originalValue) == 0 {
		return addValue
	}

	if len(addValue) == 0 {
		return originalValue
	}

	splitOriginalValues := strings.Split(originalValue, ",")
	if valueIsExist(splitOriginalValues, addValue) {
		return originalValue
	}

	return joinNewValue(splitOriginalValues, addValue)
}

func valueIsExist(originalValueList []string, addValue string) bool {
	for _, oldValue := range originalValueList {
		if addValue == oldValue {
			return true
		}
	}
	return false
}

func joinNewValue(originalValueList []string, addValue string) string {
	originalValueList = append(originalValueList, addValue)
	sort.Strings(originalValueList)

	return strings.Join(originalValueList, ",")
}

func compareAndUpdateServiceAnnotations(svc *corev1.Service, aggregatedAnnotations map[string]string) bool {
	currentAggregatedServiceAnnotations := filterIgnoredKeys(svc.Annotations)

	if reflect.DeepEqual(currentAggregatedServiceAnnotations, aggregatedAnnotations) {
		return false
	}

	update, deletion := diffAnnotations(currentAggregatedServiceAnnotations, aggregatedAnnotations)
	updateAnnotations(svc.Annotations, update, deletion)

	return true
}

func diffAnnotations(currentAnnotations, desiredAnnotations map[string]string) (update map[string]string, deletion map[string]string) {
	if currentAnnotations == nil {
		return desiredAnnotations, nil
	}
	if desiredAnnotations == nil {
		return nil, currentAnnotations
	}

	update = make(map[string]string)
	for key, value := range desiredAnnotations {
		if currentAnnotations[key] != value {
			update[key] = value
		}
	}

	deletion = make(map[string]string)
	for key, value := range currentAnnotations {
		if _, exist := desiredAnnotations[key]; !exist {
			deletion[key] = value
		}
	}
	return
}

func updateAnnotations(annotations, update, deletion map[string]string) {
	if len(update) == 0 && len(deletion) == 0 {
		return
	}
	if annotations == nil {
		annotations = make(map[string]string)
	}
	for key, value := range update {
		annotations[key] = value
	}

	for key := range deletion {
		delete(annotations, key)
	}
}

func annotationValueIsEqual(oldAnnotations, newAnnotations map[string]string, key string) bool {
	var oldValue string
	if oldAnnotations != nil {
		oldValue = oldAnnotations[key]
	}

	var newValue string
	if newAnnotations != nil {
		newValue = newAnnotations[key]
	}

	return oldValue == newValue
}
