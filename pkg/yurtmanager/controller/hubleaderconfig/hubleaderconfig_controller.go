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

package hubleaderconfig

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/hubleaderconfig/config"
	nodepoolutil "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/nodepool"
)

var (
	controllerKind = appsv1beta2.SchemeGroupVersion.WithKind("Nodepool")
)

// Add creates a new HubLeader config Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(ctx context.Context, cfg *appconfig.CompletedConfig, mgr manager.Manager) error {
	klog.Infof("hubleaderconfig-controller add controller %s", controllerKind.String())

	reconciler := &ReconcileHubLeaderConfig{
		Client:        yurtClient.GetClientByControllerNameOrDie(mgr, names.HubLeaderConfigController),
		recorder:      mgr.GetEventRecorderFor(names.HubLeaderConfigController),
		Configuration: cfg.ComponentConfig.HubLeaderConfigController,
	}

	// Create a new controller
	c, err := controller.New(
		names.HubLeaderConfigController,
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
			_, ok := e.Object.(*appsv1beta2.NodePool)
			return ok
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			_, ok := e.Object.(*appsv1beta2.NodePool)
			return ok
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldPool, ok := e.ObjectOld.(*appsv1beta2.NodePool)
			if !ok {
				return false
			}
			newPool, ok := e.ObjectNew.(*appsv1beta2.NodePool)
			if !ok {
				return false
			}

			// Only update if the leader has changed or the pool scope metadata has changed
			return nodepoolutil.HasSliceContentChanged(
				oldPool.Status.LeaderEndpoints,
				newPool.Status.LeaderEndpoints,
			) || nodepoolutil.HasSliceContentChanged(
				oldPool.Spec.PoolScopeMetadata,
				newPool.Spec.PoolScopeMetadata,
			)
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

var _ reconcile.Reconciler = &ReconcileHubLeaderConfig{}

// ReconcileHubLeaderConfig reconciles a HubLeader object
type ReconcileHubLeaderConfig struct {
	client.Client
	recorder      record.EventRecorder
	Configuration config.HubLeaderConfigControllerConfiguration
}

// +kubebuilder:rbac:groups=apps.openyurt.io,resources=nodepools,verbs=get
// +kubebuilder:rbac:groups=apps.openyurt.io,resources=nodepools/status,verbs=get
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;update;patch;create

// Reconcile reads that state of the cluster nodepool leader status and updates the leader configmap object
func (r *ReconcileHubLeaderConfig) Reconcile(
	ctx context.Context,
	request reconcile.Request,
) (reconcile.Result, error) {
	klog.Infof("Reconcile NodePool leader %s/%s", request.Namespace, request.Name)

	// Fetch the NodePool instance
	nodepool := &appsv1beta2.NodePool{}
	if err := r.Get(ctx, request.NamespacedName, nodepool); err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	if nodepool.ObjectMeta.DeletionTimestamp != nil {
		return reconcile.Result{}, nil
	}

	// Reconcile the hub leader config
	if err := r.reconcileHubLeaderConfig(ctx, nodepool); err != nil {
		r.recorder.Eventf(nodepool, v1.EventTypeWarning, "ReconcileError", "Failed to reconcile NodePool: %v", err)
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileHubLeaderConfig) reconcileHubLeaderConfig(
	ctx context.Context,
	nodepool *appsv1beta2.NodePool,
) error {
	configMapName := projectinfo.GetHubleaderConfigMapName(nodepool.Name)

	// Get the leader ConfigMap for the nodepool
	leaderConfigMap := &v1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      configMapName,
		Namespace: r.Configuration.HubLeaderNamespace,
	}, leaderConfigMap)
	if err != nil && !errors.IsNotFound(err) {
		// Error retrieving the ConfigMap
		return err
	}

	// Add leader endpoints
	leaders := make([]string, 0, len(nodepool.Status.LeaderEndpoints))
	for _, leader := range nodepool.Status.LeaderEndpoints {
		leaders = append(leaders, leader.NodeName+"/"+leader.Address)
	}

	// Add pool scope metadata
	poolScopedMetadata := make([]string, 0, len(nodepool.Spec.PoolScopeMetadata))
	for _, metadata := range nodepool.Spec.PoolScopeMetadata {
		poolScopedMetadata = append(poolScopedMetadata, getGVRString(metadata))
	}

	// sort leaders and poolScopedMetadata in order to exclude the effects of differences
	// in the order of the elements.
	slices.Sort(leaders)
	slices.Sort(poolScopedMetadata)

	// Prepare data
	data := map[string]string{
		"leaders":                strings.Join(leaders, ","),
		"pool-scoped-metadata":   strings.Join(poolScopedMetadata, ","),
		"interconnectivity":      strconv.FormatBool(nodepool.Spec.InterConnectivity),
		"enable-leader-election": strconv.FormatBool(nodepool.Spec.EnableLeaderElection),
	}

	// If the ConfigMap does not exist, create it
	if errors.IsNotFound(err) {
		leaderConfigMap = &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      configMapName,
				Namespace: r.Configuration.HubLeaderNamespace,
				Labels: map[string]string{
					projectinfo.GetHubLeaderConfigMapLabel(): configMapName,
				},
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: nodepool.APIVersion,
						Kind:       nodepool.Kind,
						Name:       nodepool.Name,
						UID:        nodepool.UID,
					},
				},
			},
			Data: data,
		}

		// Create the ConfigMap resource
		return r.Create(ctx, leaderConfigMap)
	}

	if !maps.Equal(leaderConfigMap.Data, data) {
		// Update the ConfigMap resource
		leaderConfigMap.Data = data
		return r.Update(ctx, leaderConfigMap)
	}

	return nil
}

// getGVRString	returns a string representation of the GroupVersionResource
func getGVRString(gvr metav1.GroupVersionResource) string {
	return fmt.Sprintf("%s/%s/%s", gvr.Group, gvr.Version, gvr.Resource)
}
