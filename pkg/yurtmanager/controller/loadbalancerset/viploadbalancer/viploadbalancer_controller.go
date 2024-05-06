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
	"fmt"
	"strconv"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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

var (
	poolServicesControllerResource = netv1alpha1.SchemeGroupVersion.WithResource("poolservices")
)

const (
	AnnotationVipLoadBalancerVRID = "service.openyurt.io/vrid"
	VipLoadBalancerClass          = "service.openyurt.io/viplb"

	AnnotationServiceTopologyKey           = "openyurt.io/topologyKeys"
	AnnotationServiceTopologyValueNodePool = "openyurt.io/nodepool"

	VipLoadBalancerFinalizer = "viploadbalancer.openyurt.io/resources"
)

func Format(format string, args ...interface{}) string {
	s := fmt.Sprintf(format, args...)
	return fmt.Sprintf("%s: %s", names.VipLoadBalancerController, s)
}

// Add creates a new EdgeLoadBalace Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(ctx context.Context, c *appconfig.CompletedConfig, mgr manager.Manager) error {
	klog.Infof(Format("viploadbalacer-controller add controller %s", poolServicesControllerResource.String()))
	r := newReconciler(c, mgr)

	if _, err := r.mapper.KindFor(poolServicesControllerResource); err != nil {
		return fmt.Errorf("resource %s isn't exist", poolServicesControllerResource.String())
	}

	return add(mgr, c, r)
}

var _ reconcile.Reconciler = &ReconcileVipLoadBalancer{}

// ReconcileVipLoadBalancer reconciles service, endpointslice and PoolService object
type ReconcileVipLoadBalancer struct {
	client.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder
	mapper   meta.RESTMapper

	Configration config.VipLoadBalancerControllerConfiguration
	VRIDManager  *VRIDManager
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(c *appconfig.CompletedConfig, mgr manager.Manager) *ReconcileVipLoadBalancer {
	return &ReconcileVipLoadBalancer{
		Client:       mgr.GetClient(),
		scheme:       mgr.GetScheme(),
		mapper:       mgr.GetRESTMapper(),
		recorder:     mgr.GetEventRecorderFor(names.VipLoadBalancerController),
		Configration: c.ComponentConfig.VipLoadBalancerController,
		VRIDManager:  NewVRIDManager(),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, cfg *appconfig.CompletedConfig, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(names.VipLoadBalancerController, mgr, controller.Options{
		Reconciler: r, MaxConcurrentReconciles: int(cfg.ComponentConfig.VipLoadBalancerController.ConcurrentVipLoadBalancerWorkers),
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

// +kubebuilder:rbac:groups=network.openyurt.io,resources=poolservices,verbs=get;list;watch;create;update;patch;delete

// Reconcile reads that state of the cluster for a PoolService object and makes changes based on the state read
func (r *ReconcileVipLoadBalancer) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	klog.Infof(Format("Reconcile VipLoadBalancer %s/%s", request.Namespace, request.Name))

	// Fetch the PoolService instance
	poolService := &netv1alpha1.PoolService{}
	err := r.Get(context.TODO(), request.NamespacedName, poolService)
	if err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	copyPoolService := poolService.DeepCopy()

	if poolService.DeletionTimestamp != nil {
		return r.reconcileDelete(ctx, copyPoolService)
	}

	if err := r.syncPoolService(ctx, copyPoolService); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileVipLoadBalancer) syncPoolService(ctx context.Context, poolService *netv1alpha1.PoolService) error {
	klog.V(4).Infof(Format("SyncPoolServices VipLoadBalancer %s/%s", poolService.Namespace, poolService.Name))
	// if not exist, create a new VRID
	if err := r.handleVRID(ctx, poolService); err != nil {
		return err
	}

	// sync VRID from the poolservice
	if err := r.syncVRID(ctx, poolService); err != nil {
		klog.Errorf(Format("Failed to sync VRID on Pool Service %s/%s: %v", poolService.Namespace, poolService.Name, err))
		return err
	}

	// add finalizer to the PoolService
	if err := r.addFinalizer(ctx, poolService); err != nil {
		klog.Errorf(Format("Failed to add finalizer to PoolService %s/%s: %v", poolService.Namespace, poolService.Name, err))
		return err
	}

	return nil
}

func (r *ReconcileVipLoadBalancer) syncVRID(ctx context.Context, poolService *netv1alpha1.PoolService) error {
	currentVRIDs, err := r.getCurrentVRID(ctx, poolService)
	if err != nil {
		return fmt.Errorf("failed to get current PoolServices: %v", err)
	}

	r.VRIDManager.SyncVRID(poolService.Labels[network.LabelNodePoolName], currentVRIDs)
	return nil
}

func (r *ReconcileVipLoadBalancer) getCurrentVRID(ctx context.Context, poolService *netv1alpha1.PoolService) ([]int, error) {
	// Get the poolservice list
	listSelector := &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{
			network.LabelNodePoolName: poolService.Labels[network.LabelNodePoolName],
		}),
	}
	poolServiceList := &netv1alpha1.PoolServiceList{}
	if err := r.List(ctx, poolServiceList, listSelector); err != nil {
		return nil, err
	}

	return filterInvalidPoolService(poolServiceList.Items), nil
}

func filterInvalidPoolService(poolServices []netv1alpha1.PoolService) []int {
	poolVrids := []int{}
	for _, poolService := range poolServices {
		if *poolService.Spec.LoadBalancerClass != VipLoadBalancerClass || poolService.Labels == nil {
			continue
		}

		if _, ok := poolService.Annotations[AnnotationVipLoadBalancerVRID]; !ok {
			continue
		}

		vrid, err := strconv.Atoi(poolService.Annotations[AnnotationVipLoadBalancerVRID])
		if err != nil {
			klog.Errorf(Format("Failed to convert VRID to int: %v", err))
			continue
		}

		if vrid < MINVRIDLIMIT || vrid > MAXVRIDLIMIT {
			klog.Errorf(Format("Invalid VRID %d", vrid))
			continue
		}

		poolVrids = append(poolVrids, vrid)
	}

	return poolVrids
}

func (r *ReconcileVipLoadBalancer) hasValidVRID(poolService netv1alpha1.PoolService) bool {
	if poolService.Annotations == nil {
		return false
	}

	if _, ok := poolService.Annotations[AnnotationVipLoadBalancerVRID]; !ok {
		return false
	}

	return true
}

func (r *ReconcileVipLoadBalancer) handleVRID(ctx context.Context, poolService *netv1alpha1.PoolService) error {
	if r.hasValidVRID(*poolService) {
		return nil
	}

	// Assign a new VRID to the PoolService
	if err := r.assignVRID(ctx, poolService); err != nil {
		return err
	}

	return nil
}

func (r *ReconcileVipLoadBalancer) assignVRID(ctx context.Context, poolService *netv1alpha1.PoolService) error {
	// Get the poolName from the PoolService
	poolName := poolService.Labels[network.LabelNodePoolName]
	// Get a new VRID
	vrid := r.VRIDManager.GetVRID(poolName)
	if vrid == EVICTED {
		return fmt.Errorf("VRID usage limit exceeded")
	}

	// Set the VRID to the PoolService
	if poolService.Annotations == nil {
		poolService.Annotations = make(map[string]string)
	}

	poolService.Annotations[AnnotationVipLoadBalancerVRID] = strconv.Itoa(vrid)

	// Update the PoolService
	if err := r.Update(ctx, poolService); err != nil {
		klog.Errorf(Format("Failed to create PoolService %s/%s: %v", poolService.Namespace, poolService.Name, err))
		return err
	}
	return nil
}

func (r *ReconcileVipLoadBalancer) reconcileDelete(ctx context.Context, poolService *netv1alpha1.PoolService) (reconcile.Result, error) {
	klog.V(4).Infof(Format("ReconcilDelete VipLoadBalancer %s/%s", poolService.Namespace, poolService.Name))
	poolName := poolService.Labels[network.LabelNodePoolName]

	vrid, err := strconv.Atoi(poolService.Annotations[AnnotationVipLoadBalancerVRID])
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("invalid VRID in poolservice: %v", err)
	}

	if !r.VRIDManager.IsValid(poolName, vrid) {
		return reconcile.Result{}, fmt.Errorf("VRID: %d is not in valid range", vrid)
	}
	r.VRIDManager.ReleaseVRID(poolName, vrid)
	// remove the finalizer in the PoolService

	if err := r.removeFinalizer(ctx, poolService); err != nil {
		klog.Errorf(Format("Failed to remove finalizer from PoolService %s/%s: %v", poolService.Namespace, poolService.Name, err))
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileVipLoadBalancer) addFinalizer(ctx context.Context, poolService *netv1alpha1.PoolService) error {
	if controllerutil.ContainsFinalizer(poolService, VipLoadBalancerFinalizer) {
		return nil
	}

	controllerutil.AddFinalizer(poolService, VipLoadBalancerFinalizer)
	return r.Update(ctx, poolService)
}

func (r *ReconcileVipLoadBalancer) removeFinalizer(ctx context.Context, poolService *netv1alpha1.PoolService) error {
	if !controllerutil.ContainsFinalizer(poolService, VipLoadBalancerFinalizer) {
		return nil
	}

	controllerutil.RemoveFinalizer(poolService, VipLoadBalancerFinalizer)
	return r.Update(ctx, poolService)
}
