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
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	yurtClient "github.com/openyurtio/openyurt/cmd/yurt-manager/app/client"
	"github.com/openyurtio/openyurt/cmd/yurt-manager/app/config"
	"github.com/openyurtio/openyurt/cmd/yurt-manager/names"
	appsv1beta1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
	poolconfig "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/nodepool/config"
)

var (
	controllerResource = appsv1beta1.SchemeGroupVersion.WithResource("nodepools")
)

func Format(format string, args ...interface{}) string {
	s := fmt.Sprintf(format, args...)
	return fmt.Sprintf("%s: %s", names.NodePoolController, s)
}

// ReconcileNodePool reconciles a NodePool object
type ReconcileNodePool struct {
	client.Client
	recorder record.EventRecorder
	cfg      poolconfig.NodePoolControllerConfiguration
}

var _ reconcile.Reconciler = &ReconcileNodePool{}

// Add creates a new NodePool Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(ctx context.Context, c *config.CompletedConfig, mgr manager.Manager) error {
	klog.Infof("nodepool-controller add controller %s", controllerResource.String())
	r := &ReconcileNodePool{
		cfg:      c.ComponentConfig.NodePoolController,
		recorder: mgr.GetEventRecorderFor(names.NodePoolController),
		Client:   yurtClient.GetClientByControllerNameOrDie(mgr, names.NodePoolController),
	}

	// Create a new controller
	ctrl, err := controller.New(names.NodePoolController, mgr, controller.Options{
		Reconciler: r, MaxConcurrentReconciles: int(c.ComponentConfig.NodePoolController.ConcurrentNodePoolWorkers),
	})
	if err != nil {
		return err
	}

	if _, err := mgr.GetRESTMapper().KindFor(controllerResource); err != nil {
		klog.Infof("resource %s doesn't exist", controllerResource.String())
		return err
	}

	// Watch for changes to NodePool
	err = ctrl.Watch(source.Kind[client.Object](mgr.GetCache(), &appsv1beta1.NodePool{}, &handler.EnqueueRequestForObject{}))
	if err != nil {
		return err
	}

	// Watch for changes to Node
	err = ctrl.Watch(source.Kind[client.Object](mgr.GetCache(), &corev1.Node{}, &EnqueueNodePoolForNode{
		EnableSyncNodePoolConfigurations: r.cfg.EnableSyncNodePoolConfigurations,
		Recorder:                         r.recorder,
	}))
	if err != nil {
		return err
	}

	return nil

}

type NodePoolRelatedAttributes struct {
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	Taints      []corev1.Taint    `json:"taints,omitempty"`
}

// +kubebuilder:rbac:groups=apps.openyurt.io,resources=nodepools,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps.openyurt.io,resources=nodepools/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=nodes,verbs=get;update;patch

// Reconcile reads that state of the cluster for a NodePool object and makes changes based on the state read
// and what is in the NodePool.Spec
func (r *ReconcileNodePool) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {

	// Note !!!!!!!!!!
	// We strongly recommend use Format() to  encapsulation because Format() can print logs by module
	// @kadisi
	klog.Info(Format("Reconcile NodePool %s", req.Name))

	var nodePool appsv1beta1.NodePool
	// try to reconcile the NodePool object
	if err := r.Get(ctx, req.NamespacedName, &nodePool); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	klog.V(5).Infof("NodePool %s: %#+v", nodePool.Name, nodePool)

	var currentNodeList corev1.NodeList
	if err := r.List(ctx, &currentNodeList, client.MatchingLabels(map[string]string{
		projectinfo.GetNodePoolLabel(): nodePool.GetName(),
	})); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	var (
		readyNode    int32
		notReadyNode int32
		nodes        []string
	)

	// sync nodepool configurations to nodes
	for _, node := range currentNodeList.Items {
		// prepare nodepool status
		nodes = append(nodes, node.GetName())
		if isNodeReady(node) {
			readyNode += 1
		} else {
			notReadyNode += 1
		}

		// sync nodepool configurations into node
		if r.cfg.EnableSyncNodePoolConfigurations {
			updated, err := conciliateNode(&node, &nodePool)
			if err != nil {
				return ctrl.Result{}, err
			}
			if updated {
				if err := r.Update(ctx, &node); err != nil {
					klog.Error(Format("Update Node %s error %v", node.Name, err))
					return ctrl.Result{}, err
				}
			}
		}
	}

	// always update the node pool status if necessary
	needUpdate := conciliateNodePoolStatus(readyNode, notReadyNode, nodes, &nodePool)
	if needUpdate {
		klog.V(5).Infof("nodepool(%s): (%#+v) will be updated", nodePool.Name, nodePool)
		return ctrl.Result{}, r.Status().Update(ctx, &nodePool)
	} else {
		klog.V(5).Infof("nodepool(%#+v) don't need to be updated, ready=%d, notReady=%d, nodes=%v", nodePool, readyNode, notReadyNode, nodes)
	}
	return ctrl.Result{}, nil
}
