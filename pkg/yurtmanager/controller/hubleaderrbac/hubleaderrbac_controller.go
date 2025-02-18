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

package hubleaderrbac

import (
	"context"
	"slices"

	v1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
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
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/hubleaderrbac/config"
	nodepoolutil "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/nodepool"
)

var (
	controllerKind = appsv1beta2.SchemeGroupVersion.WithKind("Nodepool")
)

const (
	leaderRoleName = "yurt-hub-multiplexer"
)

// Add creates a new HubLeaderRBAC Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(ctx context.Context, cfg *appconfig.CompletedConfig, mgr manager.Manager) error {
	klog.Infof("hubleaderrbac-controller add controller %s", controllerKind.String())

	reconciler := &ReconcileHubLeaderRBAC{
		Client:        yurtClient.GetClientByControllerNameOrDie(mgr, names.HubLeaderRBACController),
		Configuration: cfg.ComponentConfig.HubLeaderRBACController,
	}

	// Create a new controller
	c, err := controller.New(
		names.HubLeaderRBACController,
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

			// Only update if pool scope metadata has changed
			return nodepoolutil.HasSliceContentChanged(oldPool.Spec.PoolScopeMetadata, newNode.Spec.PoolScopeMetadata)
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

var _ reconcile.Reconciler = &ReconcileHubLeaderRBAC{}

// ReconcileHubLeaderRBAC reconciles a HubLeader RBAC object
type ReconcileHubLeaderRBAC struct {
	client.Client
	Configuration config.HubLeaderRBACControllerConfiguration
}

// +kubebuilder:rbac:groups=apps.openyurt.io,resources=nodepools,verbs=get;
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles,verbs=create;get;update;escalate

// Reconcile reads that state of the cluster for a HubLeader object and makes changes based on the state read
// and what is in the HubLeader.Spec
func (r *ReconcileHubLeaderRBAC) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	klog.Infof("Reconcile nodepool leader rbac %s/%s", request.Namespace, request.Name)

	// Fetch the nodepools instances
	nodepools := &appsv1beta2.NodePoolList{}
	if err := r.List(ctx, nodepools); err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	// Reconcile the NodePool
	if err := r.reconcileHubLeaderRBAC(ctx, nodepools); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileHubLeaderRBAC) reconcileHubLeaderRBAC(
	ctx context.Context,
	nodepools *appsv1beta2.NodePoolList,
) error {
	// Get pool scoped metadata from all nodepools
	// when groups are the same, merge resources
	// use a set to achieve this
	processedGVR := make(map[string]sets.Set[string])
	rules := make([]v1.PolicyRule, 0, len(nodepools.Items))
	for _, np := range nodepools.Items {
		for _, gvr := range np.Spec.PoolScopeMetadata {
			if _, ok := processedGVR[gvr.Group]; ok {
				processedGVR[gvr.Group].Insert(gvr.Resource)
				continue
			}
			processedGVR[gvr.Group] = sets.New(gvr.Resource)
		}
	}

	// Rebuild merged resources into policy rules
	for g, resources := range processedGVR {
		resourceList := resources.UnsortedList()
		slices.Sort(resourceList)

		rules = append(rules, v1.PolicyRule{
			APIGroups: []string{g},
			Resources: resourceList,
			Verbs:     []string{"list", "watch"},
		})
	}

	// Sort the rules to ensure the order is deterministic
	slices.SortFunc(rules, func(a, b v1.PolicyRule) int {
		if cmp := slices.Compare(a.APIGroups, b.APIGroups); cmp != 0 {
			return cmp
		}
		return slices.Compare(a.Resources, b.Resources)
	})

	clusterRole := &v1.ClusterRole{}
	err := r.Get(ctx, types.NamespacedName{
		Name: leaderRoleName,
	}, clusterRole)
	if err != nil && !errors.IsNotFound(err) {
		// Error retrieving the clusterrole
		return err
	}

	// Create the clusterrole if it doesn't exist
	if errors.IsNotFound(err) {
		clusterRole = &v1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{
				Name: leaderRoleName,
			},
			Rules: rules,
		}
		return r.Create(ctx, clusterRole)
	}

	// Update the clusterrole if it exists and changed
	if !hasPolicyChanged(clusterRole.Rules, rules) {
		return nil
	}

	clusterRole.Rules = rules
	return r.Update(ctx, clusterRole)
}

// hasPolicyChanged checks if the policy rules have changed
func hasPolicyChanged(old, new []v1.PolicyRule) bool {
	if len(old) != len(new) {
		return true
	}

	// Sort both old and new
	slices.SortFunc(old, func(a, b v1.PolicyRule) int {
		if cmp := slices.Compare(a.APIGroups, b.APIGroups); cmp != 0 {
			return cmp
		}
		return slices.Compare(a.Resources, b.Resources)
	})

	slices.SortFunc(new, func(a, b v1.PolicyRule) int {
		if cmp := slices.Compare(a.APIGroups, b.APIGroups); cmp != 0 {
			return cmp
		}
		return slices.Compare(a.Resources, b.Resources)
	})

	return !slices.EqualFunc(old, new, func(a, b v1.PolicyRule) bool {
		return !nodepoolutil.HasSliceContentChanged(a.APIGroups, b.APIGroups) &&
			!nodepoolutil.HasSliceContentChanged(a.Resources, b.Resources) &&
			!nodepoolutil.HasSliceContentChanged(a.Verbs, b.Verbs)
	})
}
