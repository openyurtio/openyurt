/*
Copyright 2024 The OpenYurt Authors.

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

package viploadbalancer

import (
	"context"
	"flag"
	"fmt"
	"strconv"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	appconfig "github.com/openyurtio/openyurt/cmd/yurt-manager/app/config"
	"github.com/openyurtio/openyurt/cmd/yurt-manager/names"
	network "github.com/openyurtio/openyurt/pkg/apis/network"
	netv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/network/v1alpha1"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/loadbalancerset/viploadbalancer/config"
)

func init() {
	flag.IntVar(&concurrentReconciles, "edgeloadbalace-workers", concurrentReconciles, "Max concurrent workers for edgeloadbalace controller.")
}

var (
	concurrentReconciles     = 3
	controllerKind           = netv1alpha1.SchemeGroupVersion.WithKind("VipLoadBalancer")
	VipLoadBalancerVRIDLabel = "service.openyurt.io/vrid"
	VipLoadBalancerClass     = "service.openyurt.io/viplb"
)

func Format(format string, args ...interface{}) string {
	s := fmt.Sprintf(format, args...)
	return fmt.Sprintf("%s: %s", names.VipLoadBalancerController, s)
}

// Add creates a new EdgeLoadBalace Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(ctx context.Context, c *appconfig.CompletedConfig, mgr manager.Manager) error {
	klog.Infof(Format("viploadbalacer-controller add controller %s", controllerKind.String()))
	return add(mgr, newReconciler(c, mgr))
}

var _ reconcile.Reconciler = &ReconcileVipLoadBalancer{}

// ReconcileVipLoadBalancer reconciles service, endpointslice and PoolService object
type ReconcileVipLoadBalancer struct {
	client.Client
	scheme       *runtime.Scheme
	recorder     record.EventRecorder
	Configration config.VipLoadBalancerControllerConfiguration
	VRIDManager  *VRIDManager
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(c *appconfig.CompletedConfig, mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileVipLoadBalancer{
		Client:       mgr.GetClient(),
		scheme:       mgr.GetScheme(),
		recorder:     mgr.GetEventRecorderFor(names.VipLoadBalancerController),
		Configration: c.ComponentConfig.VipLoadBalancerController,
		VRIDManager:  NewVRIDManager(),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(names.VipLoadBalancerController, mgr, controller.Options{
		Reconciler: r, MaxConcurrentReconciles: concurrentReconciles,
	})
	if err != nil {
		return err
	}

	// Watch for changes to PoolService
	err = c.Watch(&source.Kind{Type: &netv1alpha1.PoolService{}}, &handler.EnqueueRequestForObject{}, NewPoolServicePredicated())
	if err != nil {
		return err
	}

	return nil
}

// +kubebuilder:rbac:groups=net.openyurt.io,resources=poolservices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch
// +kubebuilder:rbac:groups=discovery.k8s.io,resources=endpointslices,verbs=get;list;watch;patch

// Reconcile reads that state of the cluster for a PoolService object and makes changes based on the state read
func (r *ReconcileVipLoadBalancer) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	klog.Infof(Format("Reconcile VipLoadBalancer %s/%s", request.Namespace, request.Name))

	// Fetch the PoolService instance
	poolService := &netv1alpha1.PoolService{}
	err := r.Get(context.TODO(), request.NamespacedName, poolService)
	if err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, err
	}

	copyPoolService := poolService.DeepCopy()

	if poolService.DeletionTimestamp != nil {
		return r.reconcileDelete(ctx, copyPoolService)
	}

	if err := r.syncPoolServices(ctx, copyPoolService); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileVipLoadBalancer) syncPoolServices(ctx context.Context, poolService *netv1alpha1.PoolService) error {
	// Create a new PoolService if not exist
	klog.V(4).Infof(Format("ReconcilCreate VipLoadBalancer %s/%s", poolService.Namespace, poolService.Name))

	// if not exist, create a new VRID
	if err := r.handleVRID(ctx, poolService); err != nil {
		return err
	}

	// Create a new PoolService
	if err := r.Update(ctx, poolService); err != nil {
		klog.Errorf(Format("Failed to create PoolService %s/%s: %v", poolService.Namespace, poolService.Name, err))
		return err
	}

	return nil
}

func (r *ReconcileVipLoadBalancer) hasValidVRID(poolService netv1alpha1.PoolService) bool {
	if poolService.Labels == nil {
		return false
	}

	if _, ok := poolService.Labels[VipLoadBalancerVRIDLabel]; !ok {
		return false
	}

	return true
}

func (r *ReconcileVipLoadBalancer) handleVRID(_ context.Context, poolService *netv1alpha1.PoolService) error {
	if r.hasValidVRID(*poolService) {
		return nil
	}

	// TODO: sync VRID from the nodepool

	// Get the poolName from the PoolService
	poolName := poolService.Labels[network.LabelNodePoolName]
	// Get a new VRID
	vrid := r.VRIDManager.GetVRID(poolName)
	if vrid == EVICTED {
		return fmt.Errorf("VRID usage limit exceeded")
	}

	// Set the VRID to the PoolService
	poolService.Labels[VipLoadBalancerVRIDLabel] = strconv.Itoa(vrid)
	return nil
}

func (r *ReconcileVipLoadBalancer) reconcileDelete(ctx context.Context, poolService *netv1alpha1.PoolService) (reconcile.Result, error) {
	klog.V(4).Infof(Format("ReconcilDelete VipLoadBalancer %s/%s", poolService.Namespace, poolService.Name))
	poolName := poolService.Labels[network.LabelNodePoolName]

	vrid, err := strconv.Atoi(poolService.Labels[VipLoadBalancerVRIDLabel])
	if err != nil || !r.VRIDManager.isValid(poolName, vrid) {
		return reconcile.Result{}, fmt.Errorf("invalid VRID %d", vrid)
	}
	r.VRIDManager.ReleaseVRID(poolName, vrid)
	// Delete the PoolService
	if err := r.Delete(ctx, poolService, &client.DeleteOptions{}); err != nil {
		klog.Errorf(Format("Failed to delete PoolService %s/%s: %v", poolService.Namespace, poolService.Name, err))
		return reconcile.Result{}, err

	}

	return reconcile.Result{}, nil
}
