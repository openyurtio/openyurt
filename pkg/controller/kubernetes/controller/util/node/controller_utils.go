/*
Copyright 2016 The Kubernetes Authors.
Copyright 2021 The OpenYurt Authors.

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

package node

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	clientset "k8s.io/client-go/kubernetes"
	appsv1listers "k8s.io/client-go/listers/apps/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"

	nodepkg "github.com/openyurtio/openyurt/pkg/controller/kubernetes/util/node"
)

// DeletePods will delete all pods from master running on given node,
// and return true if any pods were deleted, or were found pending
// deletion.
func DeletePods(kubeClient clientset.Interface, pods []*v1.Pod, recorder record.EventRecorder, nodeName, nodeUID string, daemonStore appsv1listers.DaemonSetLister) (bool, error) {
	remaining := false
	var updateErrList []error

	if len(pods) > 0 {
		RecordNodeEvent(recorder, nodeName, nodeUID, v1.EventTypeNormal, "DeletingAllPods", fmt.Sprintf("Deleting all Pods from Node %v.", nodeName))
	}

	for i := range pods {
		// Defensive check, also needed for tests.
		if pods[i].Spec.NodeName != nodeName {
			continue
		}

		// Pod will be modified, so making copy is required.
		pod := pods[i].DeepCopy()
		// Set reason and message in the pod object.
		if _, err := SetPodTerminationReason(kubeClient, pod, nodeName); err != nil {
			if apierrors.IsConflict(err) {
				updateErrList = append(updateErrList,
					fmt.Errorf("update status failed for pod %q: %v", Pod(pod), err))
				continue
			}
		}
		// if the pod has already been marked for deletion, we still return true that there are remaining pods.
		if pod.DeletionGracePeriodSeconds != nil {
			remaining = true
			continue
		}
		// if the pod is managed by a daemonset, ignore it
		if _, err := daemonStore.GetPodDaemonSets(pod); err == nil {
			// No error means at least one daemonset was found
			continue
		}

		klog.V(2).Infof("Starting deletion of pod %v/%v", pod.Namespace, pod.Name)
		recorder.Eventf(pod, v1.EventTypeNormal, "NodeControllerEviction", "Marking for deletion Pod %s from Node %s", pod.Name, nodeName)
		if err := kubeClient.CoreV1().Pods(pod.Namespace).Delete(context.TODO(), pod.Name, metav1.DeleteOptions{}); err != nil {
			if apierrors.IsNotFound(err) {
				// NotFound error means that pod was already deleted.
				// There is nothing left to do with this pod.
				continue
			}
			return false, err
		}
		remaining = true
	}

	if len(updateErrList) > 0 {
		return false, utilerrors.NewAggregate(updateErrList)
	}
	return remaining, nil
}

// SetPodTerminationReason attempts to set a reason and message in the
// pod status, updates it in the apiserver, and returns an error if it
// encounters one.
func SetPodTerminationReason(kubeClient clientset.Interface, pod *v1.Pod, nodeName string) (*v1.Pod, error) {
	if pod.Status.Reason == nodepkg.NodeUnreachablePodReason {
		return pod, nil
	}

	pod.Status.Reason = nodepkg.NodeUnreachablePodReason
	pod.Status.Message = fmt.Sprintf(nodepkg.NodeUnreachablePodMessage, nodeName, pod.Name)

	var updatedPod *v1.Pod
	var err error
	if updatedPod, err = kubeClient.CoreV1().Pods(pod.Namespace).UpdateStatus(context.TODO(), pod, metav1.UpdateOptions{}); err != nil {
		return nil, err
	}
	return updatedPod, nil
}

// RecordNodeEvent records a event related to a node.
func RecordNodeEvent(recorder record.EventRecorder, nodeName, nodeUID, eventtype, reason, event string) {
	ref := &v1.ObjectReference{
		APIVersion: "v1",
		Kind:       "Node",
		Name:       nodeName,
		UID:        types.UID(nodeUID),
		Namespace:  "",
	}
	klog.V(2).Infof("Recording %s event message for node %s", event, nodeName)
	recorder.Eventf(ref, eventtype, reason, "Node %s event: %s", nodeName, event)
}

// GetNodeCondition extracts the provided condition from the given status and returns that.
// Returns nil and -1 if the condition is not present, and the index of the located condition.
func GetNodeCondition(status *v1.NodeStatus, conditionType v1.NodeConditionType) (int, *v1.NodeCondition) {
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

// Pod returns a string representing a pod in a consistent human readable format,
// with pod UID as part of the string.
func Pod(pod *v1.Pod) string {
	return PodDesc(pod.Name, pod.Namespace, pod.UID)
}

// PodDesc returns a string representing a pod in a consistent human readable format,
// with pod UID as part of the string.
func PodDesc(podName, podNamespace string, podUID types.UID) string {
	// Use underscore as the delimiter because it is not allowed in pod name
	// (DNS subdomain format), while allowed in the container name format.
	return fmt.Sprintf("%s_%s(%s)", podName, podNamespace, podUID)
}
