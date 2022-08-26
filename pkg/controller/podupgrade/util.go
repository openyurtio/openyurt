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

package podupgrade

import (
	"context"
	"encoding/binary"
	"fmt"
	"hash"
	"hash/fnv"
	"strconv"

	"github.com/davecgh/go-spew/spew"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/rand"
	client "k8s.io/client-go/kubernetes"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/klog/v2"
)

const (
	UpgradeAnnotation       = "apps.openyurt.io/upgrade-strategy"
	PodUpgradableAnnotation = "apps.openyurt.io/pod-upgradable"

	OTAUpgrade  = "ota"
	AutoUpgrade = "auto"

	DaemonSet = "DaemonSet"
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

// GetDaemonsetPods get all pods belong to the given daemonset
func GetNodePods(podLister corelisters.PodLister, node *corev1.Node) ([]*corev1.Pod, error) {
	nodePods := make([]*corev1.Pod, 0)
	nodePodsNames := make([]string, 0)

	pods, err := podLister.List(labels.Everything())
	if err != nil {
		return nil, err
	}

	for i, pod := range pods {
		if pod.Spec.NodeName == node.Name {
			nodePods = append(nodePods, pods[i])
			nodePodsNames = append(nodePodsNames, pod.Name)
		}
	}

	if len(nodePodsNames) > 0 {
		klog.V(5).Infof("Daemonset %v has pods %v", node.Name, nodePodsNames)
	}
	return nodePods, nil
}

// IsDaemonsetPodLatest check whether pod is latest by comparing its Spec with daemonset's
// If pod is latest, return true, otherwise return false
func IsDaemonsetPodLatest(ds *appsv1.DaemonSet, pod *corev1.Pod) (bool, error) {
	hash := ComputeHash(&ds.Spec.Template, ds.Status.CollisionCount)
	klog.V(4).Infof("compute hash: %v", hash)
	generation, err := GetTemplateGeneration(ds)
	if err != nil {
		return false, err
	}

	klog.V(5).Infof("daemonset %v revision hash is %v", ds.Name, hash)
	klog.V(5).Infof("daemonset %v generation is %v", ds.Name, generation)

	templateMatches := generation != nil && pod.Labels[extensions.DaemonSetTemplateGenerationKey] == fmt.Sprint(generation)
	hashMatches := len(hash) > 0 && pod.Labels[extensions.DefaultDaemonSetUniqueLabelKey] == hash
	return hashMatches || templateMatches, nil
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

func NodeReadyByName(client client.Interface, nodeName string) (bool, error) {
	node, err := client.CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
	if err != nil {
		return false, err
	}

	return NodeReady(&node.Status), nil
}

// NodeReady check if the given node is ready
func NodeReady(nodeStatus *corev1.NodeStatus) bool {
	for _, cond := range nodeStatus.Conditions {
		if cond.Type == corev1.NodeReady {
			return cond.Status == corev1.ConditionTrue
		}
	}
	return false
}

func SetPodUpgradeAnnotation(clientset client.Interface, ds *appsv1.DaemonSet, pod *corev1.Pod) error {
	ok, err := IsDaemonsetPodLatest(ds, pod)
	if err != nil {
		return err
	}

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

// ComputeHash returns a hash value calculated from pod template and
// a collisionCount to avoid hash collision. The hash will be safe encoded to
// avoid bad words.
func ComputeHash(template *corev1.PodTemplateSpec, collisionCount *int32) string {
	podTemplateSpecHasher := fnv.New32a()
	DeepHashObject(podTemplateSpecHasher, *template)

	// Add collisionCount in the hash if it exists.
	if collisionCount != nil {
		collisionCountBytes := make([]byte, 8)
		binary.LittleEndian.PutUint32(collisionCountBytes, uint32(*collisionCount))
		podTemplateSpecHasher.Write(collisionCountBytes)
	}

	return rand.SafeEncodeString(fmt.Sprint(podTemplateSpecHasher.Sum32()))
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
