/*
Copyright 2025 The OpenYurt Authors.

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

package hubleader

import (
	"context"
	"fmt"
	"maps"
	"slices"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	yurtClient "github.com/openyurtio/openyurt/cmd/yurt-manager/app/client"
	appconfig "github.com/openyurtio/openyurt/cmd/yurt-manager/app/config"
	"github.com/openyurtio/openyurt/cmd/yurt-manager/names"
	appsv1beta2 "github.com/openyurtio/openyurt/pkg/apis/apps/v1beta2"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/hubleader/config"
	nodeutil "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/node"
)

var (
	controllerKind = appsv1beta2.SchemeGroupVersion.WithKind("Nodepool")
)

// Add creates a new HubLeader Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(ctx context.Context, cfg *appconfig.CompletedConfig, mgr manager.Manager) error {
	klog.Infof("hubleader-controller add controller %s", controllerKind.String())

	reconciler := &ReconcileHubLeader{
		Client:        yurtClient.GetClientByControllerNameOrDie(mgr, names.HubLeaderController),
		recorder:      mgr.GetEventRecorderFor(names.HubLeaderController),
		Configuration: cfg.ComponentConfig.HubLeaderController,
	}

	// Create a new controller
	c, err := controller.New(
		names.HubLeaderController,
		mgr,
		controller.Options{
			Reconciler:              reconciler,
			MaxConcurrentReconciles: int(cfg.ComponentConfig.HubLeaderController.ConcurrentHubLeaderWorkers),
		},
	)
	if err != nil {
		return err
	}

	poolPredicate := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return true
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldPool, ok := e.ObjectOld.(*appsv1beta2.NodePool)
			if !ok {
				return false
			}
			newNode, ok := e.ObjectNew.(*appsv1beta2.NodePool)
			if !ok {
				return false
			}

			// Only update if:
			// 1. Leader election strategy has changed
			// 2. Leader replicas has changed
			// 3. Node readiness count has changed
			// 4. Leader node label selector has changed (if mark strategy)
			if oldPool.Spec.LeaderElectionStrategy != newNode.Spec.LeaderElectionStrategy ||
				oldPool.Spec.LeaderReplicas != newNode.Spec.LeaderReplicas ||
				oldPool.Status.ReadyNodeNum != newNode.Status.ReadyNodeNum ||
				oldPool.Status.UnreadyNodeNum != newNode.Status.UnreadyNodeNum ||
				(oldPool.Spec.LeaderElectionStrategy == string(appsv1beta2.ElectionStrategyMark) &&
					!maps.Equal(oldPool.Spec.LeaderNodeLabelSelector, newNode.Spec.LeaderNodeLabelSelector)) {
				return true

			}
			return false
		},
	}

	// Watch for changes to NodePool
	err = c.Watch(
		source.Kind[client.Object](
			mgr.GetCache(),
			&appsv1beta2.NodePool{},
			&handler.EnqueueRequestForObject{},
			poolPredicate,
		),
	)
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileHubLeader{}

// ReconcileHubLeader reconciles a HubLeader object
type ReconcileHubLeader struct {
	client.Client
	recorder      record.EventRecorder
	Configuration config.HubLeaderControllerConfiguration
}

// +kubebuilder:rbac:groups=apps.openyurt.io,resources=nodepool,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps.openyurt.io,resources=nodepool/status,verbs=get;update;patch

// Reconcile reads that state of the cluster for a HubLeader object and makes changes based on the state read
// and what is in the HubLeader.Spec
func (r *ReconcileHubLeader) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	klog.Infof("Reconcile NodePool leader %s/%s", request.Namespace, request.Name)

	// Fetch the NodePool instance
	nodepool := &appsv1beta2.NodePool{}
	if err := r.Get(ctx, request.NamespacedName, nodepool); err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	if !nodepool.Spec.InterConnectivity {
		// If the NodePool is not interconnectivity, it should not reconcile
		return reconcile.Result{}, nil
	}

	// Reconcile the NodePool
	if err := r.reconcileHubLeader(ctx, nodepool); err != nil {
		r.recorder.Eventf(nodepool, corev1.EventTypeWarning, "ReconcileError", "Failed to reconcile NodePool: %v", err)
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileHubLeader) reconcileHubLeader(ctx context.Context, nodepool *appsv1beta2.NodePool) error {
	// Get all nodes that belong to the nodepool
	var currentNodeList corev1.NodeList

	// Set match labels
	matchLabels := make(map[string]string)
	if nodepool.Spec.LeaderElectionStrategy == string(appsv1beta2.ElectionStrategyMark) {
		// Add mark strategy match labels
		matchLabels = nodepool.Spec.LeaderNodeLabelSelector
	}
	matchLabels[projectinfo.GetNodePoolLabel()] = nodepool.GetName()

	err := r.List(ctx, &currentNodeList, client.MatchingLabels(matchLabels))
	if err != nil {
		return client.IgnoreNotFound(err)
	}

	// Copy the nodepool to update
	updatedNodePool := nodepool.DeepCopy()

	// Cache nodes in the list by internalIP -> Node
	// if they are ready and have internal IP
	endpointsMap := make(map[string]*corev1.Node)
	for _, n := range currentNodeList.Items {
		internalIP, ok := nodeutil.GetInternalIP(&n)
		if !ok {
			// Can't be leader
			klog.V(5).InfoS("Node is missing Internal IP, skip consideration for hub leader", "node", n.Name)
			continue
		}

		if !nodeutil.IsNodeReady(n) {
			klog.V(5).InfoS("Node is not ready, skip consideration for hub leader", "node", n.Name)
			// Can't be leader if not ready
			continue
		}

		endpointsMap[internalIP] = &n
	}

	// Delete leader endpoints that are not in endpoints map
	// They are either not ready or not longer the node list and need to be removed
	leaderDeleteFn := func(endpoint string) bool {
		_, ok := endpointsMap[endpoint]
		return !ok
	}
	updatedLeaders := slices.DeleteFunc(updatedNodePool.Status.LeaderEndpoints, leaderDeleteFn)

	// If the number of leaders is not equal to the desired number of leaders
	if len(updatedLeaders) < int(nodepool.Spec.LeaderReplicas) {
		// Remove current leaders from candidates
		for _, leader := range updatedLeaders {
			delete(endpointsMap, leader)
		}

		leaders, ok := electNLeaders(
			nodepool.Spec.LeaderElectionStrategy,
			int(nodepool.Spec.LeaderReplicas)-len(updatedLeaders),
			endpointsMap,
		)
		if !ok {
			klog.Errorf("Failed to elect a leader for NodePool %s", nodepool.Name)
			return fmt.Errorf("failed to elect a leader for NodePool %s", nodepool.Name)
		}

		updatedLeaders = append(updatedLeaders, leaders...)
	} else if len(updatedLeaders) > int(nodepool.Spec.LeaderReplicas) {
		// Remove extra leaders
		updatedLeaders = updatedLeaders[:nodepool.Spec.LeaderReplicas]
	}

	updatedNodePool.Status.LeaderEndpoints = updatedLeaders

	if !hasLeadersChanged(nodepool.Status.LeaderEndpoints, updatedNodePool.Status.LeaderEndpoints) {
		return nil
	}

	// Update Status since changed
	if err = r.Status().Update(ctx, updatedNodePool); err != nil {
		klog.ErrorS(err, "Update NodePool status error", "nodepool", updatedNodePool.Name)
		return err
	}

	return nil
}

// hasLeadersChanged checks if the leader endpoints have changed
func hasLeadersChanged(old, new []string) bool {
	if len(old) != len(new) {
		return true
	}

	oldSet := make(map[string]struct{}, len(old))

	for i := range old {
		oldSet[old[i]] = struct{}{}
	}

	for i := range new {
		if _, ok := oldSet[new[i]]; !ok {
			return true
		}
	}

	return false
}

// electNLeaders elects N leaders from the candidates based on the strategy
func electNLeaders(
	strategy string,
	numLeaders int,
	candidates map[string]*corev1.Node,
) ([]string, bool) {
	leaderEndpoints := make([]string, 0, len(candidates))

	switch strategy {
	case string(appsv1beta2.ElectionStrategyMark), string(appsv1beta2.ElectionStrategyRandom):
		// Iterate candidates and append endpoints until
		// desired number of leaders is reached
		// Note: Iterating a map in Go is non-deterministic enough to be considered random
		// for this purpose
		for k := range candidates {
			leaderEndpoints = append(leaderEndpoints, k)
			numLeaders--

			if numLeaders == 0 {
				break
			}
		}
	default:
		klog.Errorf("Unknown leader election strategy %s", strategy)
		return nil, false
	}

	return leaderEndpoints, true
}
