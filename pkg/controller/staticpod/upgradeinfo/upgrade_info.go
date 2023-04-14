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
	"context"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/kubectl/pkg/util/podutils"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
	"github.com/openyurtio/openyurt/pkg/controller/staticpod/util"
)

const (
	StaticPodHashAnnotation     = "openyurt.io/static-pod-hash"
	OTALatestManifestAnnotation = "openyurt.io/ota-latest-version"
)

// UpgradeInfo is a structure that stores some information used by static pods to upgrade.
type UpgradeInfo struct {
	// Static pod running on the node
	StaticPod *corev1.Pod

	// Upgrade worker pod running on the node
	WorkerPod *corev1.Pod

	// Indicate whether the static pod on the node needs to be upgraded.
	// If true, the static pod is not up-to-date and needs to be upgraded.
	UpgradeNeeded bool

	// Indicate whether the worker pod on the node is running.
	// If true, then the upgrade operation is in progress and does not
	// need to create a new worker pod.
	WorkerPodRunning bool

	// Indicate whether the node is ready. It's used in Auto mode.
	Ready bool
}

// New constructs the upgrade information for nodes which have the target static pod
func New(c client.Client, instance *appsv1alpha1.StaticPod, workerPodName string) (map[string]*UpgradeInfo, error) {
	infos := make(map[string]*UpgradeInfo)

	var podList, upgradeWorkerPodList corev1.PodList
	if err := c.List(context.TODO(), &podList, &client.ListOptions{Namespace: instance.Namespace}); err != nil {
		return nil, err
	}

	if err := c.List(context.TODO(), &upgradeWorkerPodList, &client.ListOptions{Namespace: instance.Namespace}); err != nil {
		return nil, err
	}

	for i, pod := range podList.Items {
		nodeName := pod.Spec.NodeName
		if nodeName == "" || pod.DeletionTimestamp != nil {
			continue
		}

		// The name format of mirror static pod is `StaticPodName-NodeName`
		if util.Hyphen(instance.Name, nodeName) == pod.Name && isStaticPod(&pod) {
			if info := infos[nodeName]; info == nil {
				infos[nodeName] = &UpgradeInfo{}
			}
			infos[nodeName].StaticPod = &podList.Items[i]
		}
	}

	for i, pod := range upgradeWorkerPodList.Items {
		nodeName := pod.Spec.NodeName
		if nodeName == "" || pod.DeletionTimestamp != nil {
			continue
		}
		// The name format of worker pods are `WorkerPodName-NodeName-Hash` Todo: may lead to mismatch
		if strings.Contains(pod.Name, workerPodName) {
			if info := infos[nodeName]; info == nil {
				infos[nodeName] = &UpgradeInfo{}
			}
			infos[nodeName].WorkerPod = &upgradeWorkerPodList.Items[i]
		}
	}

	return infos, nil
}

// isStaticPod judges whether a pod is static by its OwnerReference
func isStaticPod(pod *corev1.Pod) bool {
	for _, ownerRef := range pod.GetOwnerReferences() {
		if ownerRef.Kind == "Node" {
			return true
		}
	}
	return false
}

// ReadyUpgradeWaitingNodes gets those nodes that satisfied
// 1. node is ready
// 2. node needs to be upgraded
// 3. no latest worker pod running on the node
// On these nodes, new worker pods need to be created for auto mode
func ReadyUpgradeWaitingNodes(infos map[string]*UpgradeInfo) []string {
	var nodes []string
	for node, info := range infos {
		if info.UpgradeNeeded && !info.WorkerPodRunning && info.Ready {
			nodes = append(nodes, node)
		}
	}
	return nodes
}

// ReadyNodes gets nodes that are ready
func ReadyNodes(infos map[string]*UpgradeInfo) []string {
	var nodes []string
	for node, info := range infos {
		if info.Ready {
			nodes = append(nodes, node)
		}
	}
	return nodes
}

// UpgradeNeededNodes gets nodes that are not running the latest static pods
func UpgradeNeededNodes(infos map[string]*UpgradeInfo) []string {
	var nodes []string
	for node, info := range infos {
		if info.UpgradeNeeded {
			nodes = append(nodes, node)
		}
	}
	return nodes
}

// UpgradedNodes gets nodes that are running the latest static pods
func UpgradedNodes(infos map[string]*UpgradeInfo) []string {
	var nodes []string
	for node, info := range infos {
		if !info.UpgradeNeeded {
			nodes = append(nodes, node)
		}
	}
	return nodes
}

// SetUpgradeNeededInfos sets `UpgradeNeeded` flag and counts the number of upgraded nodes
func SetUpgradeNeededInfos(infos map[string]*UpgradeInfo, latestHash string) int32 {
	var upgradedNumber int32

	for _, info := range infos {
		if info.StaticPod != nil {
			if info.StaticPod.Annotations[StaticPodHashAnnotation] != latestHash {
				// Indicate the static pod in this node needs to be upgraded
				info.UpgradeNeeded = true
				continue
			}
			upgradedNumber++
		}
	}

	return upgradedNumber
}

// ReadyStaticPodsNumber counts the number of ready static pods
func ReadyStaticPodsNumber(infos map[string]*UpgradeInfo) int32 {
	var readyNumber int32

	for _, info := range infos {
		if info.StaticPod != nil {
			if podutils.IsPodReady(info.StaticPod) {
				readyNumber++
			}
		}
	}

	return readyNumber
}
