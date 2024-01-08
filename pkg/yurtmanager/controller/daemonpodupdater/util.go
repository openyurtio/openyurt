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

package daemonpodupdater

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/daemonpodupdater/kubernetes"
	podutil "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/pod"
)

// GetDaemonsetPods get all pods belong to the given daemonset
func GetDaemonsetPods(c client.Client, ds *appsv1.DaemonSet) ([]*corev1.Pod, error) {
	dsPods := make([]*corev1.Pod, 0)
	dsPodsNames := make([]string, 0)
	podlist := &corev1.PodList{}
	if err := c.List(context.TODO(), podlist, &client.ListOptions{Namespace: ds.Namespace}); err != nil {
		return nil, err
	}

	for i, pod := range podlist.Items {
		owner := metav1.GetControllerOf(&pod)
		if owner == nil {
			continue
		}
		if owner.UID == ds.UID {
			dsPods = append(dsPods, &podlist.Items[i])
			dsPodsNames = append(dsPodsNames, pod.Name)
		}
	}

	if len(dsPods) > 0 {
		klog.V(4).Infof("Daemonset %v has pods %v", ds.Name, dsPodsNames)
	}
	return dsPods, nil
}

// IsDaemonsetPodLatest check whether pod is the latest by comparing its Spec with daemonset's
// If pod is latest, return true, otherwise return false
func IsDaemonsetPodLatest(ds *appsv1.DaemonSet, pod *corev1.Pod) bool {
	hash := kubernetes.ComputeHash(&ds.Spec.Template, ds.Status.CollisionCount)
	klog.V(4).Infof("compute hash: %v", hash)
	generation, err := GetTemplateGeneration(ds)
	if err != nil {
		generation = nil
	}

	klog.V(5).Infof("daemonset %v revision hash is %v", ds.Name, hash)
	klog.V(5).Infof("daemonset %v generation is %v", ds.Name, generation)

	templateMatches := generation != nil && pod.Labels[extensions.DaemonSetTemplateGenerationKey] == fmt.Sprint(*generation)
	hashMatches := len(hash) > 0 && pod.Labels[extensions.DefaultDaemonSetUniqueLabelKey] == hash
	return hashMatches || templateMatches
}

// GetTemplateGeneration get annotation "deprecated.daemonset.template.generation" of the given daemonset
func GetTemplateGeneration(ds *appsv1.DaemonSet) (*int64, error) {
	annotation, found := ds.Annotations[appsv1.DeprecatedTemplateGeneration]
	if !found {
		return nil, nil
	}
	generation, err := strconv.ParseInt(annotation, 10, 64)
	if err != nil {
		return nil, err
	}
	return &generation, nil
}

// NodeReadyByName check if the given node is ready
func NodeReadyByName(c client.Client, nodeName string) (bool, error) {
	node := &corev1.Node{}
	if err := c.Get(context.TODO(), types.NamespacedName{Name: nodeName}, node); err != nil {
		return false, err
	}

	return NodeReady(&node.Status), nil
}

// NodeReady check if the given node status is ready
func NodeReady(nodeStatus *corev1.NodeStatus) bool {
	for _, cond := range nodeStatus.Conditions {
		if cond.Type == corev1.NodeReady {
			return cond.Status == corev1.ConditionTrue
		}
	}
	return false
}

// SetPodUpgradeCondition calculate and set pod condition "PodNeedUpgrade"
func SetPodUpgradeCondition(c client.Client, ds *appsv1.DaemonSet, pod *corev1.Pod) error {
	isUpdatable := IsDaemonsetPodLatest(ds, pod)

	// Comply with K8s, use constant ConditionTrue and ConditionFalse
	var status corev1.ConditionStatus
	switch isUpdatable {
	case true:
		status = corev1.ConditionFalse
	case false:
		status = corev1.ConditionTrue
	}

	cond := &corev1.PodCondition{
		Type:   PodNeedUpgrade,
		Status: status,
	}
	if change := podutil.UpdatePodCondition(&pod.Status, cond); change {

		if err := c.Status().Update(context.TODO(), pod, &client.UpdateOptions{}); err != nil {
			return err
		}
		klog.Infof("set pod %q condition PodNeedUpgrade to %v", pod.Name, !isUpdatable)
	}

	return nil
}

// checkPrerequisites checks that daemonset meets two conditions
// 1. annotation "apps.openyurt.io/update-strategy"="AdvancedRollingUpdate" or "OTA"
// 2. update strategy is "OnDelete"
func checkPrerequisites(ds *appsv1.DaemonSet) bool {
	v, ok := ds.Annotations[UpdateAnnotation]
	if !ok || (!strings.EqualFold(v, AutoUpdate) && !strings.EqualFold(v, OTAUpdate) && !strings.EqualFold(v, AdvancedRollingUpdate)) {
		return false
	}
	return ds.Spec.UpdateStrategy.Type == appsv1.OnDeleteDaemonSetStrategyType
}

// CloneAndAddLabel clones the given map and returns a new map with the given key and value added.
// Returns the given map, if labelKey is empty.
func CloneAndAddLabel(labels map[string]string, labelKey, labelValue string) map[string]string {
	if labelKey == "" {
		// Don't need to add a label.
		return labels
	}
	// Clone.
	newLabels := map[string]string{}
	for key, value := range labels {
		newLabels[key] = value
	}
	newLabels[labelKey] = labelValue
	return newLabels
}

// findUpdatedPodsOnNode looks at non-deleted pods on a given node and returns true if there
// is at most one of each old and new pods, or false if there are multiples. We can skip
// processing the particular node in those scenarios and let the manage loop prune the
// excess pods for our next time around.
func findUpdatedPodsOnNode(ds *appsv1.DaemonSet, podsOnNode []*corev1.Pod) (newPod, oldPod *corev1.Pod, ok bool) {
	for _, pod := range podsOnNode {
		if pod.DeletionTimestamp != nil {
			continue
		}

		if IsDaemonsetPodLatest(ds, pod) {
			if newPod != nil {
				return nil, nil, false
			}
			newPod = pod
		} else {
			if oldPod != nil {
				return nil, nil, false
			}
			oldPod = pod
		}
	}
	return newPod, oldPod, true
}

// GetTargetNodeName get the target node name of DaemonSet pods. If `.spec.NodeName` is not empty (nil),
// return `.spec.NodeName`; otherwise, retrieve node name of pending pods from NodeAffinity. Return error
// if failed to retrieve node name from `.spec.NodeName` and NodeAffinity.
func GetTargetNodeName(pod *corev1.Pod) (string, error) {
	if len(pod.Spec.NodeName) != 0 {
		return pod.Spec.NodeName, nil
	}

	// Retrieve node name of unscheduled pods from NodeAffinity
	if pod.Spec.Affinity == nil ||
		pod.Spec.Affinity.NodeAffinity == nil ||
		pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
		return "", fmt.Errorf("no spec.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution for pod %s/%s",
			pod.Namespace, pod.Name)
	}

	terms := pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
	if len(terms) < 1 {
		return "", fmt.Errorf("no nodeSelectorTerms in requiredDuringSchedulingIgnoredDuringExecution of pod %s/%s",
			pod.Namespace, pod.Name)
	}

	for _, term := range terms {
		for _, exp := range term.MatchFields {
			if exp.Key == metav1.ObjectNameField &&
				exp.Operator == corev1.NodeSelectorOpIn {
				if len(exp.Values) != 1 {
					return "", fmt.Errorf("the matchFields value of '%s' is not unique for pod %s/%s",
						metav1.ObjectNameField, pod.Namespace, pod.Name)
				}

				return exp.Values[0], nil
			}
		}
	}

	return "", fmt.Errorf("no node name found for pod %s/%s", pod.Namespace, pod.Name)
}

// IsPodUpdatable returns true if a pod is updatable; false otherwise.
func IsPodUpdatable(pod *corev1.Pod) bool {
	return IsPodUpgradeConditionTrue(pod.Status)
}

// IsPodUpgradeConditionTrue returns true if a pod is updatable; false otherwise.
func IsPodUpgradeConditionTrue(status corev1.PodStatus) bool {
	condition := GetPodUpgradeCondition(status)
	return condition != nil && condition.Status == corev1.ConditionTrue
}

// GetPodUpgradeCondition extracts the pod upgrade condition from the given status and returns that.
// Returns nil if the condition is not present.
func GetPodUpgradeCondition(status corev1.PodStatus) *corev1.PodCondition {
	_, condition := podutil.GetPodCondition(&status, PodNeedUpgrade)
	return condition
}
