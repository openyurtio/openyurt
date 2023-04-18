/*
Copyright 2023 The OpenYurt Authors.

Licensed under the Apache License, Version 2.0 (the License);
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an AS IS BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package nodepool

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/openyurtio/openyurt/cmd/yurt-manager/app/config"
	"github.com/openyurtio/openyurt/pkg/apis/apps"
	appsv1beta1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
	nodepoolconfig "github.com/openyurtio/openyurt/pkg/controller/nodepool/config"
	utilclient "github.com/openyurtio/openyurt/pkg/util/client"
	utildiscovery "github.com/openyurtio/openyurt/pkg/util/discovery"
)

var (
	concurrentReconciles = 3
	controllerKind       = appsv1beta1.SchemeGroupVersion.WithKind("NodePool")
)

const (
	controllerName = "NodePool-controller"
)

func Format(format string, args ...interface{}) string {
	s := fmt.Sprintf(format, args...)
	return fmt.Sprintf("%s: %s", controllerName, s)
}

// ReconcileNodePool reconciles a NodePool object
type ReconcileNodePool struct {
	client.Client
	scheme       *runtime.Scheme
	recorder     record.EventRecorder
	Configration nodepoolconfig.NodePoolControllerConfiguration
}

var _ reconcile.Reconciler = &ReconcileNodePool{}

// Add creates a new NodePool Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(c *config.CompletedConfig, mgr manager.Manager) error {
	if !utildiscovery.DiscoverGVK(controllerKind) {
		klog.Errorf(Format("DiscoverGVK error"))
		return nil
	}
	return add(mgr, newReconciler(c, mgr))
}

type NodePoolRelatedAttributes struct {
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	Taints      []corev1.Taint    `json:"taints,omitempty"`
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(c *config.CompletedConfig, mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileNodePool{
		Client:       utilclient.NewClientFromManager(mgr, controllerName),
		scheme:       mgr.GetScheme(),
		recorder:     mgr.GetEventRecorderFor(controllerName),
		Configration: c.ComponentConfig.NodePoolController,
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(controllerName, mgr, controller.Options{
		Reconciler: r, MaxConcurrentReconciles: concurrentReconciles,
	})
	if err != nil {
		return err
	}

	// Watch for changes to NodePool
	err = c.Watch(&source.Kind{Type: &appsv1beta1.NodePool{}}, &handler.EnqueueRequestForObject{})
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

	npr, ok := r.(*ReconcileNodePool)
	if !ok {
		return errors.New(Format("fail to assert interface to NodePoolReconciler"))
	}

	if npr.Configration.CreateDefaultPool {
		// register a node controller with the underlying informer of the manager
		go createDefaultNodePool(mgr.GetClient())
	}
	return nil
}

// +kubebuilder:rbac:groups=apps.openyurt.io,resources=nodepools,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.openyurt.io,resources=nodepools/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;patch;delete

// Reconcile reads that state of the cluster for a NodePool object and makes changes based on the state read
// and what is in the NodePool.Spec
func (r *ReconcileNodePool) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {

	// Note !!!!!!!!!!
	// We strongly recommend use Format() to  encapsulation because Format() can print logs by module
	// @kadisi
	klog.Infof(Format("Reconcile NodePool %s/%s", req.Namespace, req.Name))

	var nodePool appsv1beta1.NodePool
	// try to reconcile the NodePool object
	if err := r.Get(ctx, req.NamespacedName, &nodePool); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	klog.Infof(Format("NodePool %++v", nodePool))

	var desiredNodeList corev1.NodeList
	if err := r.List(ctx, &desiredNodeList, client.MatchingLabels(map[string]string{
		apps.LabelDesiredNodePool: nodePool.GetName(),
	})); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	var currentNodeList corev1.NodeList
	if err := r.List(ctx, &currentNodeList, client.MatchingLabels(map[string]string{
		apps.LabelCurrentNodePool: nodePool.GetName(),
	})); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// 1. handle the event of removing node out of the pool
	// nodes in currentNodeList but not in the desiredNodeList, will be
	// removed from the pool
	removedNodes := getRemovedNodes(&currentNodeList, &desiredNodeList)
	for _, rNode := range removedNodes {
		if err := removePoolRelatedAttrs(&rNode); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.Update(ctx, &rNode); err != nil {
			return ctrl.Result{}, err
		}
	}

	// 2. handle the event of adding node to the pool and the event of
	// updating node pool attributes
	var (
		readyNode    int32
		notReadyNode int32
		nodes        []string
	)

	for _, node := range desiredNodeList.Items {
		// prepare nodepool status
		nodes = append(nodes, node.GetName())
		if isNodeReady(node) {
			readyNode += 1
		} else {
			notReadyNode += 1
		}

		// update node status according to nodepool
		updated, err := concilateNode(&node, nodePool)
		if err != nil {
			return ctrl.Result{}, err
		}
		if updated {
			if err := r.Update(ctx, &node); err != nil {
				klog.Errorf(Format("Update Node %s error %v", node.Name, err))
				return ctrl.Result{}, err
			}
		}
	}

	// 3. always update the node pool status if necessary
	needUpdate := conciliateNodePoolStatus(readyNode, notReadyNode, nodes, &nodePool)
	if needUpdate {
		return ctrl.Result{}, r.Status().Update(ctx, &nodePool)
	}
	return ctrl.Result{}, nil
}
