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

package podupdater

import (
	"context"
	"fmt"
	"strconv"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	client "k8s.io/client-go/kubernetes"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/klog/v2"

	k8sutil "github.com/openyurtio/openyurt/pkg/controller/podupdater/kubernetes"
)

// GetDaemonsetPods get all pods belong to the given daemonset
func GetDaemonsetPods(podLister corelisters.PodLister, ds *appsv1.DaemonSet) ([]*corev1.Pod, error) {
	dsPods := make([]*corev1.Pod, 0)
	dsPodsNames := make([]string, 0)
	pods, err := podLister.Pods(ds.Namespace).List(labels.Everything())
	if err != nil {
		return nil, err
	}

	for i, pod := range pods {
		owner := metav1.GetControllerOf(pod)
		if owner == nil {
			continue
		}
		if owner.UID == ds.UID {
			dsPods = append(dsPods, pods[i])
			dsPodsNames = append(dsPodsNames, pod.Name)
		}
	}

	if len(dsPods) > 0 {
		klog.V(4).Infof("Daemonset %v has pods %v", ds.Name, dsPodsNames)
	}
	return dsPods, nil
}

// IsDaemonsetPodLatest check whether pod is latest by comparing its Spec with daemonset's
// If pod is latest, return true, otherwise return false
func IsDaemonsetPodLatest(ds *appsv1.DaemonSet, pod *corev1.Pod) bool {
	hash := k8sutil.ComputeHash(&ds.Spec.Template, ds.Status.CollisionCount)
	klog.V(4).Infof("compute hash: %v", hash)
	generation, err := GetTemplateGeneration(ds)
	if err != nil {
		generation = nil
	}

	klog.V(5).Infof("daemonset %v revision hash is %v", ds.Name, hash)
	klog.V(5).Infof("daemonset %v generation is %v", ds.Name, generation)

	templateMatches := generation != nil && pod.Labels[extensions.DaemonSetTemplateGenerationKey] == fmt.Sprint(generation)
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
func NodeReadyByName(nodeList corelisters.NodeLister, nodeName string) (bool, error) {
	node, err := nodeList.Get(nodeName)
	if err != nil {
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

// SetPodUpgradeAnnotation calculate and set annotation "apps.openyurt.io/pod-upgradable" to pod
func SetPodUpgradeAnnotation(clientset client.Interface, ds *appsv1.DaemonSet, pod *corev1.Pod) error {
	ok := IsDaemonsetPodLatest(ds, pod)

	var res bool
	if !ok {
		res = true
	}

	metav1.SetMetaDataAnnotation(&pod.ObjectMeta, PodUpgradableAnnotation, strconv.FormatBool(res))
	if _, err := clientset.CoreV1().Pods(pod.Namespace).Update(context.TODO(), pod, metav1.UpdateOptions{}); err != nil {
		return err
	}
	klog.Infof("set pod %q annotation apps.openyurt.io/pod-upgradable to %v", pod.Name, res)

	return nil
}

// checkPrerequisites checks that daemonset meets two conditions
// 1. annotation "apps.openyurt.io/upgrade-strategy"="auto" or "ota"
// 2. update strategy is "OnDelete"
func checkPrerequisites(ds *appsv1.DaemonSet) bool {
	v, ok := ds.Annotations[UpgradeAnnotation]
	if !ok || (v != "auto" && v != "ota") {
		return false
	}
	return ds.Spec.UpdateStrategy.Type == appsv1.OnDeleteDaemonSetStrategyType
}

// Clones the given map and returns a new map with the given key and value added.
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
