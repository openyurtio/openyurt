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

package nodepool

import (
	"encoding/json"
	"reflect"
	"sort"

	corev1 "k8s.io/api/core/v1"

	"github.com/openyurtio/openyurt/pkg/apis/apps"
	appsv1beta1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
	nodeutil "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/node"
)

// conciliatePoolRelatedAttrs will update the node's attributes that related to
// the nodepool
func conciliateNode(node *corev1.Node, nodePool *appsv1beta1.NodePool) (bool, error) {
	// update node attr
	newNpra := &NodePoolRelatedAttributes{
		Labels:      nodePool.Spec.Labels,
		Annotations: nodePool.Spec.Annotations,
		Taints:      nodePool.Spec.Taints,
	}

	oldNpra, err := decodePoolAttrs(node)
	if err != nil {
		return false, err
	}

	if !areNodePoolRelatedAttributesEqual(oldNpra, newNpra) {
		//klog.Infof("oldNpra: %#+v, \n newNpra: %#+v", oldNpra, newNpra)
		conciliateLabels(node, oldNpra.Labels, newNpra.Labels)
		conciliateAnnotations(node, oldNpra.Annotations, newNpra.Annotations)
		conciliateTaints(node, oldNpra.Taints, newNpra.Taints)
		if err := encodePoolAttrs(node, newNpra); err != nil {
			return false, err
		}
		return true, nil
	}

	return false, nil
}

// conciliateLabels will update the node's label that related to the nodepool
func conciliateLabels(node *corev1.Node, oldLabels, newLabels map[string]string) {
	// 1. remove labels from the node if they have been removed from the
	// node pool
	for oldK := range oldLabels {
		if _, exist := newLabels[oldK]; !exist {
			// label has been removed from the nodepool
			delete(node.Labels, oldK)
		}
	}

	// 2. update the node labels based on the latest node pool labels
	node.Labels = mergeMap(node.Labels, newLabels)
}

// conciliateLabels will update the node's annotation that related to the nodepool
func conciliateAnnotations(node *corev1.Node, oldAnnos, newAnnos map[string]string) {
	// 1. remove annotations from the node if they have been removed from the
	// node pool
	for oldK := range oldAnnos {
		if _, exist := newAnnos[oldK]; !exist {
			delete(node.Annotations, oldK)
		}
	}

	// 2. update the node annotations based on the latest node pool labels
	node.Annotations = mergeMap(node.Annotations, newAnnos)
}

// conciliateLabels will update the node's taint that related to the nodepool
func conciliateTaints(node *corev1.Node, oldTaints, newTaints []corev1.Taint) {

	// 1. remove old taints from the node
	for _, oldTaint := range oldTaints {
		if _, exist := containTaint(oldTaint, node.Spec.Taints); exist {
			node.Spec.Taints = removeTaint(oldTaint, node.Spec.Taints)
		}
	}

	// 2. add new node taints based on the latest node pool taints
	for _, nt := range newTaints {
		if _, exist := containTaint(nt, node.Spec.Taints); !exist {
			node.Spec.Taints = append(node.Spec.Taints, nt)
		}
	}
}

// conciliateNodePoolStatus will update the nodepool status if necessary
func conciliateNodePoolStatus(
	readyNode,
	notReadyNode int32,
	nodes []string,
	nodePool *appsv1beta1.NodePool) (needUpdate bool) {

	if readyNode != nodePool.Status.ReadyNodeNum {
		nodePool.Status.ReadyNodeNum = readyNode
		needUpdate = true
	}

	if notReadyNode != nodePool.Status.UnreadyNodeNum {
		nodePool.Status.UnreadyNodeNum = notReadyNode
		needUpdate = true
	}

	// update the node list on demand
	sort.Strings(nodes)
	sort.Strings(nodePool.Status.Nodes)
	if !(len(nodes) == 0 && len(nodePool.Status.Nodes) == 0 || reflect.DeepEqual(nodes, nodePool.Status.Nodes)) {
		nodePool.Status.Nodes = nodes
		needUpdate = true
	}

	return needUpdate
}

// containTaint checks if `taint` is in `taints`, if yes it will return
// the index of the taint and true, otherwise, it will return 0 and false.
// N.B. the uniqueness of the taint is based on both key and effect pair
func containTaint(taint corev1.Taint, taints []corev1.Taint) (int, bool) {
	for i, t := range taints {
		if taint.Effect == t.Effect && taint.Key == t.Key {
			return i, true
		}
	}
	return 0, false
}

// isNodeReady checks if the `node` is `corev1.NodeReady`
func isNodeReady(node corev1.Node) bool {
	_, nc := nodeutil.GetNodeCondition(&node.Status, corev1.NodeReady)
	// GetNodeCondition will return nil and -1 if the condition is not present
	return nc != nil && nc.Status == corev1.ConditionTrue
}

func mergeMap(m1, m2 map[string]string) map[string]string {
	if m1 == nil {
		m1 = make(map[string]string)
	}
	for k, v := range m2 {
		m1[k] = v
	}
	return m1
}

// removeTaint removes `taint` from `taints` if exist
func removeTaint(taint corev1.Taint, taints []corev1.Taint) []corev1.Taint {
	for i := 0; i < len(taints); i++ {
		if taint.Key == taints[i].Key && taint.Effect == taints[i].Effect {
			taints = append(taints[:i], taints[i+1:]...)
			break
		}
	}
	return taints
}

// encodePoolAttrs caches the nodepool-related attributes to the
// node's annotation
func encodePoolAttrs(node *corev1.Node,
	npra *NodePoolRelatedAttributes) error {
	npraJson, err := json.Marshal(npra)
	if err != nil {
		return err
	}
	if node.Annotations == nil {
		node.Annotations = make(map[string]string)
	}
	node.Annotations[apps.AnnotationPrevAttrs] = string(npraJson)
	return nil
}

// decodePoolAttrs resolves nodepool attributes from node annotation
func decodePoolAttrs(node *corev1.Node) (*NodePoolRelatedAttributes, error) {
	var oldNpra NodePoolRelatedAttributes
	if preAttrs, exist := node.Annotations[apps.AnnotationPrevAttrs]; !exist {
		return &oldNpra, nil
	} else {
		if err := json.Unmarshal([]byte(preAttrs), &oldNpra); err != nil {
			return &oldNpra, err
		}

		return &oldNpra, nil
	}
}

// areNodePoolRelatedAttributesEqual is used for checking NodePoolRelatedAttributes is equal
func areNodePoolRelatedAttributesEqual(a, b *NodePoolRelatedAttributes) bool {
	if a == nil || b == nil {
		return a == b
	}

	isLabelsEqual := (len(a.Labels) == 0 && len(b.Labels) == 0) || reflect.DeepEqual(a.Labels, b.Labels)
	isAnnotationsEqual := (len(a.Annotations) == 0 && len(b.Annotations) == 0) || reflect.DeepEqual(a.Annotations, b.Annotations)
	isTaintsEqual := (len(a.Taints) == 0 && len(b.Taints) == 0) || reflect.DeepEqual(a.Taints, b.Taints)

	return isLabelsEqual && isAnnotationsEqual && isTaintsEqual
}
