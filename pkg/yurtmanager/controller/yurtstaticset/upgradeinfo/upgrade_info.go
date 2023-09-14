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

package upgradeinfo

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/kubectl/pkg/util/podutils"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
	podutil "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/pod"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/yurtstaticset/util"
)

const (
	StaticPodHashAnnotation = "openyurt.io/static-pod-hash"
)

// UpgradeInfo is a structure that stores some information used by YurtStaticSet to upgrade.
type UpgradeInfo struct {
	// Static pod running on the node
	StaticPod *corev1.Pod

	// Upgrade worker pod running on the node
	WorkerPod *corev1.Pod

	// Indicate whether the static pod is ready
	StaticPodReady bool

	// Indicate whether the static pod on the node needs to be upgraded.
	// If true, the static pod is not up-to-date and needs to be upgraded.
	UpgradeNeeded bool

	// Indicate whether the worker pod on the node is running.
	// If true, then the upgrade operation is in progress and does not
	// need to create a new worker pod.
	WorkerPodRunning bool

	// Indicate the worker pod status
	WorkerPodStatusPhase corev1.PodPhase

	// Indicate whether the worker pod need to be delete
	WorkerPodDeleteNeeded bool

	// Indicate whether the node is ready. It's used in AdvancedRollingUpdate mode.
	NodeReady bool
}

// New constructs the upgrade information for nodes which have the target static pod
func New(c client.Client, instance *appsv1alpha1.YurtStaticSet, workerPodName, hash string) (map[string]*UpgradeInfo, error) {
	infos := make(map[string]*UpgradeInfo)

	var podList corev1.PodList
	if err := c.List(context.TODO(), &podList, &client.ListOptions{Namespace: instance.Namespace}); err != nil {
		return nil, err
	}

	for i, pod := range podList.Items {
		nodeName := pod.Spec.NodeName
		if nodeName == "" || pod.DeletionTimestamp != nil {
			continue
		}

		// The name format of mirror static pod is `StaticPodName-NodeName`
		if util.Hyphen(instance.Name, nodeName) == pod.Name && podutil.IsStaticPod(&pod) {
			// initialize static pod info
			if err := initStaticPodInfo(instance, c, nodeName, hash, &podList.Items[i], infos); err != nil {
				return nil, err
			}
		}

		// The name format of worker pods are `WorkerPodName-YssName-NodeName-Hash`
		name := workerPodName + instance.Name
		if strings.Contains(pod.Name, name) {
			// initialize worker pod info
			if err := initWorkerPodInfo(nodeName, hash, &podList.Items[i], infos); err != nil {
				return nil, err
			}
		}
	}

	return infos, nil
}

func initStaticPodInfo(instance *appsv1alpha1.YurtStaticSet, c client.Client, nodeName, hash string,
	pod *corev1.Pod, infos map[string]*UpgradeInfo) error {

	if info := infos[nodeName]; info == nil {
		infos[nodeName] = &UpgradeInfo{}
	}
	infos[nodeName].StaticPod = pod

	hashAnnotation, ok := pod.Annotations[StaticPodHashAnnotation]
	if ok && hashAnnotation != hash {
		// Indicate the static pod in this node needs to be upgraded
		infos[nodeName].UpgradeNeeded = true
	} else if !ok && !match(instance, pod) {
		// Indicate the static pod which is already existing and has no hash
		infos[nodeName].UpgradeNeeded = true
	}

	// Sets the ready status static pod
	if podutils.IsPodReady(pod) {
		infos[nodeName].StaticPodReady = true
	}

	// Sets the ready status for every node which has the target static pod
	ready, err := util.NodeReadyByName(c, nodeName)
	if err != nil {
		return err
	}
	infos[nodeName].NodeReady = ready
	return nil
}

func initWorkerPodInfo(nodeName, hash string, pod *corev1.Pod, infos map[string]*UpgradeInfo) error {
	if info := infos[nodeName]; info == nil {
		infos[nodeName] = &UpgradeInfo{}
	}
	infos[nodeName].WorkerPod = pod

	infos[nodeName].WorkerPodStatusPhase = pod.Status.Phase
	switch pod.Status.Phase {
	case corev1.PodFailed:
		// The worker pod is failed, then some irreparable failure has occurred. Just stop reconcile and update status
		return fmt.Errorf("fail to init worker pod info, cause worker pod %s failed", pod.Name)
	case corev1.PodSucceeded:
		// The worker pod is succeeded, then this node must be up-to-date. Just delete this worker pod
		infos[nodeName].WorkerPodDeleteNeeded = true
	default:
		// In this node, the latest worker pod is still running, and we don't need to create new worker for it
		infos[nodeName].WorkerPodRunning = true
	}

	if pod.Annotations[StaticPodHashAnnotation] != hash {
		// If the worker pod is not up-to-date, then it can be recreated directly
		infos[nodeName].WorkerPodDeleteNeeded = true
	}
	return nil
}

// match check if the given YurtStaticSet's template matches the pod.
func match(instance *appsv1alpha1.YurtStaticSet, pod *corev1.Pod) bool {

	yssBytes, err := json.Marshal(instance.Spec.Template)
	if err != nil {
		return false
	}
	var yssRaw map[string]interface{}
	err = json.Unmarshal(yssBytes, &yssRaw)
	if err != nil {
		return false
	}
	yssSpec := yssRaw["spec"].(map[string]interface{})
	yssMetadata := yssRaw["metadata"].(map[string]interface{})
	delete(yssMetadata, "name")
	delete(yssMetadata, "creationTimestamp")

	podBytes, err := json.Marshal(pod)
	if err != nil {
		return false
	}
	var podRaw map[string]interface{}
	err = json.Unmarshal(podBytes, &podRaw)
	if err != nil {
		return false
	}
	podSpec := podRaw["spec"].(map[string]interface{})
	podMetadata := podRaw["metadata"].(map[string]interface{})

	for k, v := range yssSpec {
		if value, ok := podSpec[k]; ok {
			byte1, err := json.Marshal(value)
			if err != nil {
				return false
			}
			byte2, err := json.Marshal(v)
			if err != nil {
				return false
			}
			if !bytes.Equal(byte1, byte2) {
				return false
			}
		} else {
			return false
		}
	}

	for k, v := range yssMetadata {
		if value, ok := podMetadata[k]; ok {
			byte1, err := json.Marshal(value)
			if err != nil {
				return false
			}
			byte2, err := json.Marshal(v)
			if err != nil {
				return false
			}
			if !bytes.Equal(byte1, byte2) {
				return false
			}
		} else {
			return false
		}
	}

	return true
}

// ReadyUpgradeWaitingNodes gets those nodes that satisfied
// 1. node is ready
// 2. node needs to be upgraded
// 3. no latest worker pod running on the node
// On these nodes, new worker pods need to be created for AdvancedRollingUpdate mode
func ReadyUpgradeWaitingNodes(infos map[string]*UpgradeInfo) []string {
	var nodes []string
	for node, info := range infos {
		if info.UpgradeNeeded && !info.WorkerPodRunning && info.NodeReady {
			nodes = append(nodes, node)
		}
	}
	return nodes
}

// ListOutUpgradeNeededNodesAndUpgradedNodes gets nodes that are not running the latest static pods and running the latest static pods
func ListOutUpgradeNeededNodesAndUpgradedNodes(infos map[string]*UpgradeInfo) ([]string, []string) {
	var upgradeNeededNodes, upgradeNodes []string
	for node, info := range infos {
		if info.UpgradeNeeded {
			upgradeNeededNodes = append(upgradeNeededNodes, node)
		} else {
			upgradeNodes = append(upgradeNodes, node)
		}
	}
	return upgradeNeededNodes, upgradeNodes
}

// CalculateOperateInfoFromUpgradeInfoMap calculate the number of ready static pods, upgraded nodes,
// the delete pods and whether all worker is finished.
func CalculateOperateInfoFromUpgradeInfoMap(infos map[string]*UpgradeInfo) (int32, int32, bool, []*corev1.Pod) {
	var (
		upgradedNumber int32
		readyNumber    int32
		allSucceeded   = true
		deletePods     = make([]*corev1.Pod, 0)
	)

	for _, info := range infos {
		if info.StaticPod != nil {
			// counts the number of ready static pods and upgraded nodes
			if info.StaticPodReady {
				readyNumber++
			}
			if !info.UpgradeNeeded {
				upgradedNumber++
			}
		}

		if info.WorkerPod != nil {
			// sync worker pods info
			if info.WorkerPodDeleteNeeded {
				deletePods = append(deletePods, info.WorkerPod)
			}
			if info.WorkerPodStatusPhase != corev1.PodFailed && info.WorkerPodStatusPhase != corev1.PodSucceeded {
				allSucceeded = false
			}
		}
	}
	return upgradedNumber, readyNumber, allSucceeded, deletePods
}
