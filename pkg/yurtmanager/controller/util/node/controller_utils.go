/*
Copyright 2020 The OpenYurt Authors.
Copyright 2016 The Kubernetes Authors.

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
	"encoding/json"
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	clientretry "k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
	taintutils "github.com/openyurtio/openyurt/pkg/util/taints"
	utilpod "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/pod"
)

const (
	// NodeUnreachablePodReason is the reason on a pod when its state cannot be confirmed as kubelet is unresponsive
	// on the node it is (was) running.
	NodeUnreachablePodReason = "NodeLost"
	// NodeUnreachablePodMessage is the message on a pod when its state cannot be confirmed as kubelet is unresponsive
	// on the node it is (was) running.
	NodeUnreachablePodMessage = "Node %v which was running pod %v is unresponsive"

	// PodBindingAnnotation can be added into pod annotation, which indicates that this pod will be bound to the node that it is scheduled to.
	PodBindingAnnotation = "apps.openyurt.io/binding"
)

var UpdateTaintBackoff = wait.Backoff{
	Steps:    5,
	Duration: 100 * time.Millisecond,
	Jitter:   1.0,
}

var UpdateLabelBackoff = wait.Backoff{
	Steps:    5,
	Duration: 100 * time.Millisecond,
	Jitter:   1.0,
}

// DeletePods will delete all pods from master running on given node,
// and return true if any pods were deleted, or were found pending
// deletion.
func DeletePods(ctx context.Context, c client.Client, pods []*v1.Pod, recorder record.EventRecorder, nodeName, nodeUID string) (bool, error) {
	remaining := false
	var updateErrList []error

	if len(pods) > 0 {
		RecordNodeEvent(ctx, recorder, nodeName, nodeUID, v1.EventTypeNormal, "DeletingAllPods", fmt.Sprintf("Deleting all Pods from Node %v.", nodeName))
	}

	for i := range pods {
		// Defensive check, also needed for tests.
		if pods[i].Spec.NodeName != nodeName {
			continue
		}

		// Pod will be modified, so making copy is required.
		pod := pods[i].DeepCopy()
		// Set reason and message in the pod object.
		if _, err := SetPodTerminationReason(ctx, c, pod, nodeName); err != nil {
			if apierrors.IsConflict(err) {
				updateErrList = append(updateErrList,
					fmt.Errorf("update status failed for pod %s/%s: %v", pod.Namespace, pod.Name, err))
				continue
			}
		}
		// if the pod has already been marked for deletion, we still return true that there are remaining pods.
		if pod.DeletionGracePeriodSeconds != nil {
			remaining = true
			continue
		}

		klog.InfoS("Starting deletion of pod", "pod", klog.KObj(pod))
		recorder.Eventf(pod, v1.EventTypeNormal, "NodeControllerEviction", "Marking for deletion Pod %s from Node %s", pod.Name, nodeName)
		//if err := kubeClient.CoreV1().Pods(pod.Namespace).Delete(ctx, pod.Name, metav1.DeleteOptions{}); err != nil {
		if err := c.Delete(ctx, pod); err != nil {
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
func SetPodTerminationReason(ctx context.Context, c client.Client, pod *v1.Pod, nodeName string) (*v1.Pod, error) {
	if pod.Status.Reason == NodeUnreachablePodReason {
		return pod, nil
	}

	pod.Status.Reason = NodeUnreachablePodReason
	pod.Status.Message = fmt.Sprintf(NodeUnreachablePodMessage, nodeName, pod.Name)

	var err error
	if err = c.Status().Update(ctx, pod); err != nil {
		return nil, err
	}

	return pod, nil
}

// MarkPodsNotReady updates ready status of given pods running on
// given node from master return true if success
func MarkPodsNotReady(ctx context.Context, c client.Client, recorder record.EventRecorder, pods []*v1.Pod, nodeName string) error {
	klog.V(2).InfoS("Update ready status of pods on node", "node", klog.KRef("", nodeName))

	errs := []error{}
	for i := range pods {
		// Defensive check, also needed for tests.
		if pods[i].Spec.NodeName != nodeName {
			continue
		}

		// Pod will be modified, so making copy is required.
		pod := pods[i].DeepCopy()
		for _, cond := range pod.Status.Conditions {
			if cond.Type != v1.PodReady {
				continue
			}

			cond.Status = v1.ConditionFalse
			if !utilpod.UpdatePodCondition(&pod.Status, &cond) {
				break
			}

			klog.V(2).InfoS("Updating ready status of pod to false", "pod", klog.KObj(pod))
			//if _, err := kubeClient.CoreV1().Pods(pod.Namespace).UpdateStatus(ctx, pod, metav1.UpdateOptions{}); err != nil {
			if err := c.Status().Update(ctx, pod, &client.UpdateOptions{}); err != nil {
				if apierrors.IsNotFound(err) {
					// NotFound error means that pod was already deleted.
					// There is nothing left to do with this pod.
					continue
				}
				klog.InfoS("could not update status for pod", "pod", klog.KObj(pod), "err", err)
				errs = append(errs, err)
			}
			// record NodeNotReady event after updateStatus to make sure pod still exists
			recorder.Event(pod, v1.EventTypeWarning, "NodeNotReady", "Node is not ready")
			break
		}
	}

	return utilerrors.NewAggregate(errs)
}

// RecordNodeEvent records a event related to a node.
func RecordNodeEvent(ctx context.Context, recorder record.EventRecorder, nodeName, nodeUID, eventtype, reason, event string) {
	ref := &v1.ObjectReference{
		APIVersion: "v1",
		Kind:       "Node",
		Name:       nodeName,
		UID:        types.UID(nodeUID),
		Namespace:  "",
	}
	klog.V(2).InfoS("Recording event message for node", "event", event, "node", klog.KRef("", nodeName))
	recorder.Eventf(ref, eventtype, reason, "Node %s event: %s", nodeName, event)
}

// RecordNodeStatusChange records a event related to a node status change. (Common to lifecycle and ipam)
func RecordNodeStatusChange(recorder record.EventRecorder, node *v1.Node, newStatus string) {
	ref := &v1.ObjectReference{
		APIVersion: "v1",
		Kind:       "Node",
		Name:       node.Name,
		UID:        node.UID,
		Namespace:  "",
	}
	klog.V(2).InfoS("Recording status change event message for node", "status", newStatus, "node", node.Name)
	// TODO: This requires a transaction, either both node status is updated
	// and event is recorded or neither should happen, see issue #6055.
	recorder.Eventf(ref, v1.EventTypeNormal, newStatus, "Node %s status is now: %s", node.Name, newStatus)
}

// SwapNodeControllerTaint returns true in case of success and false
// otherwise.
func SwapNodeControllerTaint(ctx context.Context, kubeClient clientset.Interface, taintsToAdd, taintsToRemove []*v1.Taint, node *v1.Node) bool {
	for _, taintToAdd := range taintsToAdd {
		now := metav1.Now()
		taintToAdd.TimeAdded = &now
	}

	err := AddOrUpdateTaintOnNode(ctx, kubeClient, node.Name, taintsToAdd...)
	if err != nil {
		utilruntime.HandleError(
			fmt.Errorf(
				"unable to taint %+v unresponsive Node %q: %v",
				taintsToAdd,
				node.Name,
				err))
		return false
	}
	klog.V(4).InfoS("Added taint to node", "taint", taintsToAdd, "node", klog.KRef("", node.Name))

	err = RemoveTaintOffNode(ctx, kubeClient, node.Name, node, taintsToRemove...)
	if err != nil {
		utilruntime.HandleError(
			fmt.Errorf(
				"unable to remove %+v unneeded taint from unresponsive Node %q: %v",
				taintsToRemove,
				node.Name,
				err))
		return false
	}
	klog.V(4).InfoS("Made sure that node has no taint", "node", klog.KRef("", node.Name), "taint", taintsToRemove)

	return true
}

// AddOrUpdateLabelsOnNode updates the labels on the node and returns true on
// success and false on failure.
func AddOrUpdateLabelsOnNode(ctx context.Context, kubeClient clientset.Interface, labelsToUpdate map[string]string, node *v1.Node) bool {
	if err := addOrUpdateLabelsOnNode(kubeClient, node.Name, labelsToUpdate); err != nil {
		utilruntime.HandleError(
			fmt.Errorf(
				"unable to update labels %+v for Node %q: %v",
				labelsToUpdate,
				node.Name,
				err))
		return false
	}
	klog.V(4).InfoS("Updated labels to node", "label", labelsToUpdate, "node", klog.KRef("", node.Name))
	return true
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

// AddOrUpdateTaintOnNode add taints to the node. If taint was added into node, it'll issue API calls
// to update nodes; otherwise, no API calls. Return error if any.
func AddOrUpdateTaintOnNode(ctx context.Context, c clientset.Interface, nodeName string, taints ...*v1.Taint) error {
	if len(taints) == 0 {
		return nil
	}
	firstTry := true
	return clientretry.RetryOnConflict(UpdateTaintBackoff, func() error {
		var err error
		var oldNode *v1.Node
		// First we try getting node from the API server cache, as it's cheaper. If it fails
		// we get it from etcd to be sure to have fresh data.
		option := metav1.GetOptions{}
		if firstTry {
			option.ResourceVersion = "0"
			firstTry = false
		}
		oldNode, err = c.CoreV1().Nodes().Get(ctx, nodeName, option)
		if err != nil {
			return err
		}

		var newNode *v1.Node
		oldNodeCopy := oldNode
		updated := false
		for _, taint := range taints {
			curNewNode, ok, err := taintutils.AddOrUpdateTaint(oldNodeCopy, taint)
			if err != nil {
				return fmt.Errorf("could not update taint of node")
			}
			updated = updated || ok
			newNode = curNewNode
			oldNodeCopy = curNewNode
		}
		if !updated {
			return nil
		}
		return PatchNodeTaints(ctx, c, nodeName, oldNode, newNode)
	})
}

// RemoveTaintOffNode is for cleaning up taints temporarily added to node,
// won't fail if target taint doesn't exist or has been removed.
// If passed a node it'll check if there's anything to be done, if taint is not present it won't issue
// any API calls.
func RemoveTaintOffNode(ctx context.Context, c clientset.Interface, nodeName string, node *v1.Node, taints ...*v1.Taint) error {
	if len(taints) == 0 {
		return nil
	}
	// Short circuit for limiting amount of API calls.
	if node != nil {
		match := false
		for _, taint := range taints {
			if taintutils.TaintExists(node.Spec.Taints, taint) {
				match = true
				break
			}
		}
		if !match {
			return nil
		}
	}

	firstTry := true
	return clientretry.RetryOnConflict(UpdateTaintBackoff, func() error {
		var err error
		var oldNode *v1.Node
		// First we try getting node from the API server cache, as it's cheaper. If it fails
		// we get it from etcd to be sure to have fresh data.
		option := metav1.GetOptions{}
		if firstTry {
			option.ResourceVersion = "0"
			firstTry = false
		}
		oldNode, err = c.CoreV1().Nodes().Get(ctx, nodeName, option)
		if err != nil {
			return err
		}

		var newNode *v1.Node
		oldNodeCopy := oldNode
		updated := false
		for _, taint := range taints {
			curNewNode, ok, err := taintutils.RemoveTaint(oldNodeCopy, taint)
			if err != nil {
				return fmt.Errorf("could not remove taint of node")
			}
			updated = updated || ok
			newNode = curNewNode
			oldNodeCopy = curNewNode
		}
		if !updated {
			return nil
		}
		return PatchNodeTaints(ctx, c, nodeName, oldNode, newNode)
	})
}

// PatchNodeTaints patches node's taints.
func PatchNodeTaints(ctx context.Context, c clientset.Interface, nodeName string, oldNode *v1.Node, newNode *v1.Node) error {
	// Strip base diff node from RV to ensure that our Patch request will set RV to check for conflicts over .spec.taints.
	// This is needed because .spec.taints does not specify patchMergeKey and patchStrategy and adding them is no longer an option for compatibility reasons.
	// Using other Patch strategy works for adding new taints, however will not resolve problem with taint removal.
	oldNodeNoRV := oldNode.DeepCopy()
	oldNodeNoRV.ResourceVersion = ""
	oldDataNoRV, err := json.Marshal(&oldNodeNoRV)
	if err != nil {
		return fmt.Errorf("could not marshal old node %#v for node %q: %v", oldNodeNoRV, nodeName, err)
	}

	newTaints := newNode.Spec.Taints
	newNodeClone := oldNode.DeepCopy()
	newNodeClone.Spec.Taints = newTaints
	newData, err := json.Marshal(newNodeClone)
	if err != nil {
		return fmt.Errorf("could not marshal new node %#v for node %q: %v", newNodeClone, nodeName, err)
	}

	patchBytes, err := strategicpatch.CreateTwoWayMergePatch(oldDataNoRV, newData, v1.Node{})
	if err != nil {
		return fmt.Errorf("could not create patch for node %q: %v", nodeName, err)
	}

	_, err = c.CoreV1().Nodes().Patch(ctx, nodeName, types.StrategicMergePatchType, patchBytes, metav1.PatchOptions{})
	return err
}

func addOrUpdateLabelsOnNode(kubeClient clientset.Interface, nodeName string, labelsToUpdate map[string]string) error {
	firstTry := true
	return clientretry.RetryOnConflict(UpdateLabelBackoff, func() error {
		var err error
		var node *v1.Node
		// First we try getting node from the API server cache, as it's cheaper. If it fails
		// we get it from etcd to be sure to have fresh data.
		option := metav1.GetOptions{}
		if firstTry {
			option.ResourceVersion = "0"
			firstTry = false
		}
		node, err = kubeClient.CoreV1().Nodes().Get(context.TODO(), nodeName, option)
		if err != nil {
			return err
		}

		// Make a copy of the node and update the labels.
		newNode := node.DeepCopy()
		if newNode.Labels == nil {
			newNode.Labels = make(map[string]string)
		}
		for key, value := range labelsToUpdate {
			newNode.Labels[key] = value
		}

		oldData, err := json.Marshal(node)
		if err != nil {
			return fmt.Errorf("could not marshal the existing node %#v: %v", node, err)
		}
		newData, err := json.Marshal(newNode)
		if err != nil {
			return fmt.Errorf("could not marshal the new node %#v: %v", newNode, err)
		}
		patchBytes, err := strategicpatch.CreateTwoWayMergePatch(oldData, newData, &v1.Node{})
		if err != nil {
			return fmt.Errorf("could not create a two-way merge patch: %v", err)
		}
		if _, err := kubeClient.CoreV1().Nodes().Patch(context.TODO(), node.Name, types.StrategicMergePatchType, patchBytes, metav1.PatchOptions{}); err != nil {
			return fmt.Errorf("could not patch the node: %v", err)
		}
		return nil
	})
}

func IsPodBoundenToNode(node *v1.Node) bool {
	if node.Annotations != nil &&
		(node.Annotations[projectinfo.GetAutonomyAnnotation()] == "true" ||
			node.Annotations[PodBindingAnnotation] == "true") {
		return true
	}

	return false
}
