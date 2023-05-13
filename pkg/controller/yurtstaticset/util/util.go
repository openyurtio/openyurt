/*
Copyright 2023 The OpenYurt Authors.

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
	"bytes"
	"context"
	"fmt"
	"hash"
	"hash/fnv"

	"github.com/davecgh/go-spew/spew"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/cli-runtime/pkg/printers"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
)

const (
	ConfigMapPrefix = "yurt-static-set-"

	// PodNeedUpgrade indicates whether the pod is able to upgrade.
	PodNeedUpgrade corev1.PodConditionType = "PodNeedUpgrade"

	StaticPodHashAnnotation = "openyurt.io/static-pod-hash"
)

var (
	PodGVK = corev1.SchemeGroupVersion.WithKind("Pod")
)

func Hyphen(str1, str2 string) string {
	return str1 + "-" + str2
}

// WithConfigMapPrefix add prefix `yurt-static-set-` to the given string
func WithConfigMapPrefix(str string) string {
	return ConfigMapPrefix + str
}

// UnavailableCount returns 0 if unavailability is not requested, the expected
// unavailability number to allow out of numberToUpgrade if requested, or an error if
// the unavailability percentage requested is invalid.
func UnavailableCount(us *appsv1alpha1.YurtStaticSetUpgradeStrategy, numberToUpgrade int) (int, error) {
	if us == nil || us.Type != appsv1alpha1.AdvancedRollingUpdateUpgradeStrategyType {
		return 0, nil
	}
	return intstr.GetScaledValueFromIntOrPercent(us.MaxUnavailable, numberToUpgrade, true)
}

// ComputeHash returns a hash value calculated from pod template
func ComputeHash(template *corev1.PodTemplateSpec) string {
	podSpecHasher := fnv.New32a()
	DeepHashObject(podSpecHasher, *template)

	return rand.SafeEncodeString(fmt.Sprint(podSpecHasher.Sum32()))
}

// DeepHashObject writes specified object to hash using the spew library
// which follows pointers and prints actual values of the nested objects
// ensuring the hash does not change when a pointer changes.
func DeepHashObject(hasher hash.Hash, objectToWrite interface{}) {
	hasher.Reset()
	printer := spew.ConfigState{
		Indent:         " ",
		SortKeys:       true,
		DisableMethods: true,
		SpewKeys:       true,
	}
	printer.Fprintf(hasher, "%#v", objectToWrite)
}

// GenStaticPodManifest generates manifest from use-specified template
func GenStaticPodManifest(tmplSpec *corev1.PodTemplateSpec, hash string) (string, error) {
	pod := &corev1.Pod{ObjectMeta: *tmplSpec.ObjectMeta.DeepCopy(), Spec: *tmplSpec.Spec.DeepCopy()}
	// latest hash value will be added to the annotation to facilitate checking if the running static pods are latest
	metav1.SetMetaDataAnnotation(&pod.ObjectMeta, StaticPodHashAnnotation, hash)

	pod.GetObjectKind().SetGroupVersionKind(PodGVK)

	var buf bytes.Buffer
	y := printers.YAMLPrinter{}
	if err := y.PrintObj(pod, &buf); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// NodeReadyByName check if the given node is ready
func NodeReadyByName(c client.Client, nodeName string) (bool, error) {
	node := &corev1.Node{}
	if err := c.Get(context.TODO(), types.NamespacedName{Name: nodeName}, node); err != nil {
		return false, err
	}

	_, nc := GetNodeCondition(&node.Status, corev1.NodeReady)

	return nc != nil && nc.Status == corev1.ConditionTrue, nil
}

// GetNodeCondition extracts the provided condition from the given status and returns that.
// Returns nil and -1 if the condition is not present, and the index of the located condition.
func GetNodeCondition(status *corev1.NodeStatus, conditionType corev1.NodeConditionType) (int, *corev1.NodeCondition) {
	if status == nil {
		return -1, nil
	}
	for i := range status.Conditions {
		if status.Conditions[i].Type == conditionType {
			return i, &status.Conditions[i]
		}
	}
	return -1, nil
}

// SetPodUpgradeCondition set pod condition `PodNeedUpgrade` to the specified value
func SetPodUpgradeCondition(c client.Client, status corev1.ConditionStatus, pod *corev1.Pod) error {
	cond := &corev1.PodCondition{
		Type:   PodNeedUpgrade,
		Status: status,
	}
	if change := UpdatePodCondition(&pod.Status, cond); change {
		if err := c.Status().Update(context.TODO(), pod, &client.UpdateOptions{}); err != nil {
			return err
		}
	}

	return nil
}

// UpdatePodCondition updates existing pod condition or creates a new one. Sets LastTransitionTime to now if the
// status has changed.
// Returns true if pod condition has changed or has been added.
func UpdatePodCondition(status *corev1.PodStatus, condition *corev1.PodCondition) bool {
	condition.LastTransitionTime = metav1.Now()
	// Try to find this pod condition.
	conditionIndex, oldCondition := GetPodCondition(status, condition.Type)

	if oldCondition == nil {
		// We are adding new pod condition.
		status.Conditions = append(status.Conditions, *condition)
		return true
	}
	// We are updating an existing condition, so we need to check if it has changed.
	if condition.Status == oldCondition.Status {
		condition.LastTransitionTime = oldCondition.LastTransitionTime
	}

	isEqual := condition.Status == oldCondition.Status &&
		condition.Reason == oldCondition.Reason &&
		condition.Message == oldCondition.Message &&
		condition.LastProbeTime.Equal(&oldCondition.LastProbeTime) &&
		condition.LastTransitionTime.Equal(&oldCondition.LastTransitionTime)

	status.Conditions[conditionIndex] = *condition
	// Return true if one of the fields have changed.
	return !isEqual
}

// GetPodCondition extracts the provided condition from the given status and returns that.
// Returns nil and -1 if the condition is not present, and the index of the located condition.
func GetPodCondition(status *corev1.PodStatus, conditionType corev1.PodConditionType) (int, *corev1.PodCondition) {
	if status == nil {
		return -1, nil
	}
	return GetPodConditionFromList(status.Conditions, conditionType)
}

// GetPodConditionFromList extracts the provided condition from the given list of condition and
// returns the index of the condition and the condition. Returns -1 and nil if the condition is not present.
func GetPodConditionFromList(conditions []corev1.PodCondition, conditionType corev1.PodConditionType) (int, *corev1.PodCondition) {
	if conditions == nil {
		return -1, nil
	}
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return i, &conditions[i]
		}
	}
	return -1, nil
}
