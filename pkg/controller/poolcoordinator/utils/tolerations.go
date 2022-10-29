/*
Copyright 2022 The OpenYurt Authors.
Copyright 2017 The Kubernetes Authors.

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

package utils

import (
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/klog/v2"
)

// VerifyAgainstWhitelist checks if the provided tolerations
// satisfy the provided whitelist and returns true, otherwise returns false
func VerifyAgainstWhitelist(tolerations, whitelist []corev1.Toleration) bool {
	if len(whitelist) == 0 || len(tolerations) == 0 {
		return true
	}

next:
	for _, t := range tolerations {
		for _, w := range whitelist {
			if isSuperset(w, t) {
				continue next
			}
		}
		return false
	}

	return true
}

// MergeTolerations merges two sets of tolerations into one. If one toleration is a superset of
// another, only the superset is kept.
func MergeTolerations(first, second []corev1.Toleration) ([]corev1.Toleration, bool) {
	all := append(first, second...)
	var merged []corev1.Toleration
	var changed bool

next:
	for i, t := range all {
		for _, t2 := range merged {
			if isSuperset(t2, t) {
				continue next // t is redundant; ignore it
			}
		}
		if i+1 < len(all) {
			for _, t2 := range all[i+1:] {
				// If the tolerations are equal, prefer the first.
				if !apiequality.Semantic.DeepEqual(&t, &t2) && isSuperset(t2, t) {
					continue next // t is redundant; ignore it
				}
			}
		}
		merged = append(merged, t)
		changed = true
	}

	return merged, changed
}

// isSuperset checks whether ss tolerates a superset of t.
func isSuperset(ss, t corev1.Toleration) bool {
	if apiequality.Semantic.DeepEqual(&t, &ss) {
		return true
	}

	if t.Key != ss.Key &&
		// An empty key with Exists operator means match all keys & values.
		(ss.Key != "" || ss.Operator != corev1.TolerationOpExists) {
		return false
	}

	// An empty effect means match all effects.
	if t.Effect != ss.Effect && ss.Effect != "" {
		return false
	}

	if ss.Effect == corev1.TaintEffectNoExecute {
		if ss.TolerationSeconds != nil {
			if t.TolerationSeconds == nil ||
				*t.TolerationSeconds > *ss.TolerationSeconds {
				return false
			}
		}
	}

	switch ss.Operator {
	case corev1.TolerationOpEqual, "": // empty operator means Equal
		return t.Operator == corev1.TolerationOpEqual && t.Value == ss.Value
	case corev1.TolerationOpExists:
		return true
	default:
		klog.Errorf("Unknown toleration operator: %s", ss.Operator)
		return false
	}
}
