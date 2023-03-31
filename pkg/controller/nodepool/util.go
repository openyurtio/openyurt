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
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openyurtio/openyurt/pkg/apis/apps"
	appsv1beta1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
	nodeutil "github.com/openyurtio/openyurt/pkg/controller/util/node"
)

var timeSleep = time.Sleep

// createNodePool creates an nodepool, it will retry 5 times if it fails
func createNodePool(c client.Client, name string,
	poolType appsv1beta1.NodePoolType) bool {
	for i := 0; i < 5; i++ {
		np := appsv1beta1.NodePool{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Spec: appsv1beta1.NodePoolSpec{
				Type: poolType,
			},
		}
		err := c.Create(context.TODO(), &np)
		if err == nil {
			klog.V(4).Infof("the default nodepool(%s) is created", name)
			return true
		}
		if apierrors.IsAlreadyExists(err) {
			klog.V(4).Infof("the default nodepool(%s) already exist", name)
			return false
		}
		klog.Errorf("fail to create the node pool(%s): %s", name, err)
		timeSleep(2 * time.Second)
	}
	klog.V(4).Info("fail to create the default nodepool after trying for 5 times")
	return false
}

// createDefaultNodePool creates the default NodePool if not exist
func createDefaultNodePool(client client.Client) {
	createNodePool(client,
		apps.DefaultEdgeNodePoolName, appsv1beta1.Edge)
	createNodePool(client,
		apps.DefaultCloudNodePoolName, appsv1beta1.Cloud)
}

// conciliatePoolRelatedAttrs will update the node's attributes that related to
// the nodepool
func concilateNode(node *corev1.Node, nodePool appsv1beta1.NodePool) (attrUpdated bool, err error) {
	// update node attr
	npra := NodePoolRelatedAttributes{
		Labels:      nodePool.Spec.Labels,
		Annotations: nodePool.Spec.Annotations,
		Taints:      nodePool.Spec.Taints,
	}

	if preAttrs, exist := node.Annotations[apps.AnnotationPrevAttrs]; !exist {
		node.Labels = mergeMap(node.Labels, npra.Labels)
		node.Annotations = mergeMap(node.Annotations, npra.Annotations)
		for _, npt := range npra.Taints {
			for i, nt := range node.Spec.Taints {
				if npt.Effect == nt.Effect && npt.Key == nt.Key {
					node.Spec.Taints = append(node.Spec.Taints[:i], node.Spec.Taints[i+1:]...)
					break
				}
			}
			node.Spec.Taints = append(node.Spec.Taints, npt)
		}
		if err := cachePrevPoolAttrs(node, npra); err != nil {
			return attrUpdated, err
		}
		attrUpdated = true
	} else {
		var preNpra NodePoolRelatedAttributes
		if err := json.Unmarshal([]byte(preAttrs), &preNpra); err != nil {
			return attrUpdated, err
		}
		if !reflect.DeepEqual(preNpra, npra) {
			// pool related attributes will be updated
			conciliateLabels(node, preNpra.Labels, npra.Labels)
			conciliateAnnotations(node, preNpra.Annotations, npra.Annotations)
			conciliateTaints(node, preNpra.Taints, npra.Taints)
			if err := cachePrevPoolAttrs(node, npra); err != nil {
				return attrUpdated, err
			}
			attrUpdated = true
		}
	}

	// update ownerLabel
	if node.Labels[apps.LabelCurrentNodePool] != nodePool.GetName() {
		if len(node.Labels) == 0 {
			node.Labels = make(map[string]string)
		}
		node.Labels[apps.LabelCurrentNodePool] = nodePool.GetName()
		attrUpdated = true
	}
	return attrUpdated, nil
}

// getRemovedNodes calculates removed nodes from current nodes and desired nodes
func getRemovedNodes(currentNodeList *corev1.NodeList, desiredNodeList *corev1.NodeList) []corev1.Node {
	var removedNodes []corev1.Node
	for _, mNode := range currentNodeList.Items {
		var found bool
		for _, dNode := range desiredNodeList.Items {
			if mNode.GetName() == dNode.GetName() {
				found = true
				break
			}
		}
		if !found {
			removedNodes = append(removedNodes, mNode)
		}
	}
	return removedNodes
}

// removePoolRelatedAttrs removes attributes(label/annotation/taint) that
// relate to nodepool
func removePoolRelatedAttrs(node *corev1.Node) error {
	var npra NodePoolRelatedAttributes

	if _, exist := node.Annotations[apps.AnnotationPrevAttrs]; !exist {
		return nil
	}

	if err := json.Unmarshal(
		[]byte(node.Annotations[apps.AnnotationPrevAttrs]),
		&npra); err != nil {
		return err
	}

	for lk, lv := range npra.Labels {
		if node.Labels[lk] == lv {
			delete(node.Labels, lk)
		}
	}

	for ak, av := range npra.Annotations {
		if node.Annotations[ak] == av {
			delete(node.Annotations, ak)
		}
	}

	for _, t := range npra.Taints {
		if i, exist := containTaint(t, node.Spec.Taints); exist {
			node.Spec.Taints = append(
				node.Spec.Taints[:i],
				node.Spec.Taints[i+1:]...)
		}
	}
	delete(node.Annotations, apps.AnnotationPrevAttrs)
	delete(node.Labels, apps.LabelCurrentNodePool)

	return nil
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

	// 1. remove taints from the node if they have been removed from the
	// node pool
	for _, oldTaint := range oldTaints {
		if _, exist := containTaint(oldTaint, node.Spec.Taints); exist {
			node.Spec.Taints = removeTaint(oldTaint, node.Spec.Taints)
		}
	}

	// 2. update the node taints based on the latest node pool taints
	for _, nt := range newTaints {
		node.Spec.Taints = append(node.Spec.Taints, nt)
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
	if !reflect.DeepEqual(nodes, nodePool.Status.Nodes) {
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

// cachePrevPoolAttrs caches the nodepool-related attributes to the
// node's annotation
func cachePrevPoolAttrs(node *corev1.Node,
	npra NodePoolRelatedAttributes) error {
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

// addNodePoolToWorkQueue adds the nodepool the reconciler's workqueue
func addNodePoolToWorkQueue(npName string,
	q workqueue.RateLimitingInterface) {
	q.Add(reconcile.Request{
		NamespacedName: types.NamespacedName{Name: npName},
	})
}
