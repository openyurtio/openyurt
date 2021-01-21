/*
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

package nodepool

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
	nodeutil "k8s.io/kubernetes/pkg/controller/util/node"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	appsv1alpha1 "github.com/alibaba/openyurt/pkg/yurtappmanager/apis/apps/v1alpha1"
	"github.com/alibaba/openyurt/pkg/yurtappmanager/constant"
	"github.com/alibaba/openyurt/pkg/yurtappmanager/util/gate"
)

const controllerName = "nodepool-controller"

var concurrentReconciles = 3

// NodePoolReconciler reconciles a NodePool object
type NodePoolReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	recorder          record.EventRecorder
	createDefaultPool bool
}

type NodePoolRelatedAttributes struct {
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	Taints      []corev1.Taint    `json:"taints,omitempty"`
}

// Add creates a new NodePool Controller and adds it to the Manager with default RBAC.
// The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, ctx context.Context) error {
	if !gate.ResourceEnabled(&appsv1alpha1.NodePool{}) {
		return nil
	}
	inf := ctx.Value(constant.ContextKeyCreateDefaultPool)
	cdp, ok := inf.(bool)
	if !ok {
		return errors.New("fail to assert interface to bool for command line option createDefaultPool")
	}
	return add(mgr, newReconciler(mgr, cdp))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, createDefaultPool bool) reconcile.Reconciler {
	return &NodePoolReconciler{
		Client:            mgr.GetClient(),
		Scheme:            mgr.GetScheme(),
		recorder:          mgr.GetEventRecorderFor(controllerName),
		createDefaultPool: createDefaultPool,
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(controllerName,
		mgr, controller.Options{
			Reconciler:              r,
			MaxConcurrentReconciles: concurrentReconciles})
	if err != nil {
		return err
	}

	npr, ok := r.(*NodePoolReconciler)
	if !ok {
		return errors.New("fail to assert interface to NodePoolReconciler")
	}

	// Watch for changes to NodePool
	err = c.Watch(&source.Kind{
		Type: &appsv1alpha1.NodePool{}},
		&handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to Node
	err = c.Watch(&source.Kind{
		Type: &corev1.Node{}},
		&EnqueueNodePoolForNode{})
	if err != nil {
		return err
	}

	if npr.createDefaultPool {
		// register a node controller with the underlying informer of the manager
		go createDefaultNodePool(mgr.GetClient())
	}
	return nil
}

// createNodePool creates an nodepool, it will retry 5 times if it fails
func createNodePool(c client.Client, name string,
	poolType appsv1alpha1.NodePoolType) {
	for i := 0; i < 5; i++ {
		np := appsv1alpha1.NodePool{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Spec: appsv1alpha1.NodePoolSpec{
				Type: poolType,
			},
		}
		err := c.Create(context.TODO(), &np)
		if err == nil {
			klog.V(4).Infof("the default nodepool(%s) is created", name)
			break
		}
		if apierrors.IsAlreadyExists(err) {
			klog.V(4).Infof("the default nodepool(%s) already exist", name)
			break
		}
		klog.Errorf("fail to create the node pool(%s): %s", name, err)
		time.Sleep(2 * time.Second)
	}
	klog.V(4).Info("fail to create the defualt nodepool after trying for 5 times")
}

// createDefaultNodePool creates the default NodePool if not exist
func createDefaultNodePool(client client.Client) {
	createNodePool(client,
		appsv1alpha1.DefaultEdgeNodePoolName, appsv1alpha1.Edge)
	createNodePool(client,
		appsv1alpha1.DefaultCloudNodePoolName, appsv1alpha1.Cloud)
}

// +kubebuilder:rbac:groups=apps.openyurt.io,resources=nodepools,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.openyurt.io,resources=nodepools/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch;update;patch

func (r *NodePoolReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	var nodePool appsv1alpha1.NodePool
	// try to reconcile the NodePool object
	if err := r.Get(ctx, req.NamespacedName, &nodePool); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	var desiredNodeList corev1.NodeList
	if err := r.List(ctx, &desiredNodeList, client.MatchingLabels(map[string]string{
		appsv1alpha1.LabelDesiredNodePool: nodePool.GetName(),
	})); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	var currentNodeList corev1.NodeList
	if err := r.List(ctx, &currentNodeList, client.MatchingLabels(map[string]string{
		appsv1alpha1.LabelCurrentNodePool: nodePool.GetName(),
	})); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// 1. handle the event of removing node out of the pool
	// nodes in currentNodeList but not in the desiredNodeList, will be
	// removed from the pool
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

	for _, rNode := range removedNodes {
		if err := removePoolRelatedAttrs(&rNode); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.Update(ctx, &rNode); err != nil {
			return ctrl.Result{}, err
		}
	}

	var (
		readyNode    int32
		notReadyNode int32
		nodes        []string
	)

	// 2. handle the event of adding node to the pool and the event of
	// updating node pool attributes
	for _, node := range desiredNodeList.Items {
		nodes = append(nodes, node.GetName())
		if isNodeReady(node) {
			readyNode += 1
		} else {
			notReadyNode += 1
		}

		attrUpdated, err := conciliatePoolRelatedAttrs(&node,
			NodePoolRelatedAttributes{
				Labels:      nodePool.Spec.Labels,
				Annotations: nodePool.Spec.Annotations,
				Taints:      nodePool.Spec.Taints,
			})
		if err != nil {
			return ctrl.Result{}, err
		}
		var ownerLabelUpdated bool
		if node.Labels[appsv1alpha1.LabelCurrentNodePool] != nodePool.GetName() {
			ownerLabelUpdated = true
			if len(node.Labels) == 0 {
				node.Labels = make(map[string]string)
			}
			node.Labels[appsv1alpha1.LabelCurrentNodePool] = nodePool.GetName()
		}

		if attrUpdated || ownerLabelUpdated {
			if err := r.Update(ctx, &node); err != nil {
				klog.Errorf("Update Node %s error %v", node.Name, err)
				return ctrl.Result{}, err
			}
		}
	}

	// 3. always update the node pool status if necessary
	return conciliateNodePoolStatus(r, readyNode, notReadyNode, nodes, &nodePool)
}

// removePoolRelatedAttrs removes attributes(label/annotation/taint) that
// relate to nodepool
func removePoolRelatedAttrs(node *corev1.Node) error {
	var npra NodePoolRelatedAttributes

	if _, exist := node.Annotations[appsv1alpha1.AnnotationPrevAttrs]; !exist {
		return nil
	}

	if err := json.Unmarshal(
		[]byte(node.Annotations[appsv1alpha1.AnnotationPrevAttrs]),
		&npra); err != nil {
		return err
	}

	for lk, lv := range npra.Labels {
		if node.Labels[lk] == lv {
			delete(node.Labels, lk)
		}
	}

	for ak, av := range npra.Labels {
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
	delete(node.Annotations, appsv1alpha1.AnnotationPrevAttrs)
	delete(node.Labels, appsv1alpha1.LabelCurrentNodePool)

	return nil
}

// conciliatePoolRelatedAttrs will update the node's attributes that related to
// the nodepool
func conciliatePoolRelatedAttrs(node *corev1.Node,
	npra NodePoolRelatedAttributes) (bool, error) {
	var attrUpdated bool
	preAttrs, exist := node.Annotations[appsv1alpha1.AnnotationPrevAttrs]
	if !exist {
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
		return attrUpdated, nil
	}
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
	return attrUpdated, nil
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
		if _, exist := containTaint(oldTaint, newTaints); !exist {
			node.Spec.Taints = removeTaint(oldTaint, node.Spec.Taints)
		}
	}

	// 2. update the node taints based on the latest node pool taints
	for _, nt := range newTaints {
		if _, exist := containTaint(nt, oldTaints); !exist {
			node.Spec.Taints = append(node.Spec.Taints, nt)
		}
	}
}

// conciliateNodePoolStatus will update the nodepool status
func conciliateNodePoolStatus(cli client.Client,
	readyNode,
	notReadyNode int32,
	nodes []string,
	nodePool *appsv1alpha1.NodePool) (ctrl.Result, error) {
	var updateNodePool bool
	if readyNode != nodePool.Status.ReadyNodeNum {
		nodePool.Status.ReadyNodeNum = readyNode
		updateNodePool = true
	}

	if notReadyNode != nodePool.Status.UnreadyNodeNum {
		nodePool.Status.UnreadyNodeNum = notReadyNode
		updateNodePool = true
	}

	// update the node list on demand
	sort.Strings(nodes)
	sort.Strings(nodePool.Status.Nodes)
	if !reflect.DeepEqual(nodes, nodePool.Status.Nodes) {
		nodePool.Status.Nodes = nodes
		updateNodePool = true
	}
	// update the nodepool on demand
	if updateNodePool {
		return ctrl.Result{}, cli.Status().Update(context.Background(), nodePool)
	}
	return ctrl.Result{}, nil
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
	node.Annotations[appsv1alpha1.AnnotationPrevAttrs] = string(npraJson)
	return nil
}

// addNodePoolToWorkQueue adds the nodepool the reconciler's workqueue
func addNodePoolToWorkQueue(npName string,
	q workqueue.RateLimitingInterface) {
	q.Add(reconcile.Request{
		NamespacedName: types.NamespacedName{Name: npName},
	})
}
