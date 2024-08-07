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
	"sort"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
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
	"github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
	network "github.com/openyurtio/openyurt/pkg/apis/network"
	netv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/network/v1alpha1"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/loadbalancerset/viploadbalancer/config"
)

var (
	poolServicesControllerResource = netv1alpha1.SchemeGroupVersion.WithResource("poolservices")
)

const (
	AnnotationVipLoadBalancerVRID          = "service.openyurt.io/vrid"
	AnnotationVipLoadBalancerIPS           = "service.openyurt.io/desired-vips"
	VipLoadBalancerClass                   = "service.openyurt.io/viplb"
	AnnotationServiceTopologyKey           = "openyurt.io/topologyKeys"
	AnnotationServiceTopologyValueNodePool = "openyurt.io/nodepool"
	AnnotationNodePoolAddressPools         = "openyurt.io/address-pools"
	AnnotationServiceVIPAddress            = "service.openyurt.io/vip"
	AnnotationServiceVIPStatus             = "service.openyurt.io/vip-status"
	AnnotationServiceVIPStatusOnline       = "online"
	AnnotationServiceVIPStatusOffline      = "offline"

	VipLoadBalancerFinalizer               = "viploadbalancer.openyurt.io/resources"
	poolServiceVRIDExhaustedEventMsgFormat = "PoolService %s/%s in NodePool %s has exhausted all VRIDs"
)

const (
	ServiceVIPUnknown int = iota
	ServiceVIPOnline
	ServiceVIPOffline
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

	Configuration config.VipLoadBalancerControllerConfiguration
	IPManagers    map[string]*IPManager
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(c *appconfig.CompletedConfig, mgr manager.Manager) *ReconcileVipLoadBalancer {
	return &ReconcileVipLoadBalancer{
		Client:        mgr.GetClient(),
		scheme:        mgr.GetScheme(),
		mapper:        mgr.GetRESTMapper(),
		recorder:      mgr.GetEventRecorderFor(names.VipLoadBalancerController),
		Configuration: c.ComponentConfig.VipLoadBalancerController,
		IPManagers:    make(map[string]*IPManager),
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
	err = c.Watch(source.Kind(mgr.GetCache(), &netv1alpha1.PoolService{}), &handler.EnqueueRequestForObject{}, NewPoolServicePredicated())
	if err != nil {
		return err
	}

	return nil
}

// +kubebuilder:rbac:groups=network.openyurt.io,resources=poolservices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=network.openyurt.io,resources=poolservices/status,verbs=get;update;patch

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

	// Check if the PoolService instance is deleted
	if poolService.DeletionTimestamp != nil {
		return r.reconcileDelete(ctx, copyPoolService)
	}

	if err := r.reconcilePoolService(ctx, copyPoolService); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileVipLoadBalancer) reconcilePoolService(ctx context.Context, poolService *netv1alpha1.PoolService) error {
	klog.V(4).Infof(Format("ReconcilePoolService VipLoadBalancer %s/%s", poolService.Namespace, poolService.Name))
	// add finalizer to the PoolService
	if err := r.addFinalizer(ctx, poolService); err != nil {
		klog.Errorf(Format("Failed to add finalizer to PoolService %s/%s: %v", poolService.Namespace, poolService.Name, err))
		return err
	}

	// sync PoolService
	if err := r.syncPoolService(ctx, poolService); err != nil {
		klog.Errorf(Format("Failed to sync PoolService %s/%s: %v", poolService.Namespace, poolService.Name, err))
		return err
	}

	return nil
}

func (r *ReconcileVipLoadBalancer) syncPoolService(ctx context.Context, poolService *netv1alpha1.PoolService) error {
	klog.V(4).Infof(Format("SyncPoolServices VipLoadBalancer %s/%s", poolService.Namespace, poolService.Name))

	// sync VRRP from the recently poolservice
	if err := r.syncVRRPs(ctx, poolService); err != nil {
		klog.Errorf(Format("Failed to sync VRID on Pool Service %s/%s: %v", poolService.Namespace, poolService.Name, err))
		return err
	}

	// if not exist, create a new VRID
	if err := r.handleVRRPs(ctx, poolService); err != nil {
		return err
	}

	if err := r.syncPoolServiceStatus(ctx, poolService); err != nil {
		klog.Errorf(Format("Failed to sync PoolService %s/%s status: %v", poolService.Namespace, poolService.Name, err))
		return err
	}

	return nil
}

func (r *ReconcileVipLoadBalancer) syncVRRPs(ctx context.Context, poolService *netv1alpha1.PoolService) error {
	poolName := poolService.Labels[network.LabelNodePoolName]
	// Get the pool address from nodepool label
	if err := r.syncIPAddressPools(ctx, poolName); err != nil {
		return fmt.Errorf("failed to sync ip addresses from nodepool: %v", err)
	}

	currentVRRPs, err := r.getCurrentAssignedVRRPs(ctx, poolService)
	if err != nil {
		return fmt.Errorf("failed to get current PoolServices: %v", err)
	}

	// Sync the VRRPs
	r.IPManagers[poolName].Sync(currentVRRPs)
	return nil
}

func (r *ReconcileVipLoadBalancer) syncIPAddressPools(ctx context.Context, poolName string) error {
	poolAddress, err := r.getCurrentPoolAddress(ctx, poolName)
	if err != nil {
		return fmt.Errorf("failed to get available Pool address of nodepool: %v", err)
	}

	// check if nodepool has not address-pools
	if _, ok := r.IPManagers[poolName]; !ok {
		r.IPManagers[poolName], err = NewIPManager(poolAddress)
		if err != nil {
			return fmt.Errorf("failed to create IPManager for nodepool %s: %v", poolName, err)
		}
	}

	// TODO: if user update poolAddress label in the nodepool
	if poolAddress != nil && !r.IPManagers[poolName].IPRanges.HasAll(poolAddress...) {
		klog.Infof(Format("NodePool: %s 's IP address pool has been updated, please delete it and reconfigure it", poolName))
	}

	return nil
}

func (r *ReconcileVipLoadBalancer) getCurrentPoolAddress(ctx context.Context, poolName string) ([]string, error) {
	np := &v1beta1.NodePool{}

	// Get All NodePools from the cluster by nodepool name
	if err := r.Get(ctx, client.ObjectKey{Name: poolName}, np); err != nil {
		return nil, err
	}

	// Check if the NodePool has address pools
	if np.Annotations == nil {
		return nil, fmt.Errorf("NodePool %s doesn't have not available annotations", np.Name)
	}

	if poolAddress, ok := np.Annotations[AnnotationNodePoolAddressPools]; ok {
		return ParseIP(poolAddress), nil
	}

	return nil, fmt.Errorf("NodePool %s is not assigned address pools", np.Name)
}

func (r *ReconcileVipLoadBalancer) syncPoolServiceStatus(ctx context.Context, poolService *netv1alpha1.PoolService) error {
	klog.V(4).Infof(Format("SyncPoolServiceStatus VipLoadBalancer %s/%s", poolService.Namespace, poolService.Name))

	if !r.checkIfVipServiceOnline(ctx, poolService) {
		klog.Infof(Format("SyncPoolServiceStatus VipLoadBalancer %s/%s is not online in the nodepool agent", poolService.Namespace, poolService.Name))
		return nil
	}

	desiredLbStatus, err := r.desiredLbStatus(poolService)
	if err != nil {
		return fmt.Errorf("failed to calculate desire lb status for poolservice %s/%s: %v", poolService.Namespace, poolService.Name, err)
	}

	poolService.Status.LoadBalancer = desiredLbStatus
	if err := r.Update(ctx, poolService); err != nil {
		klog.Errorf(Format("Failed to update PoolService %s/%s status: %v", poolService.Namespace, poolService.Name, err))
		return err
	}

	return nil
}

func (r *ReconcileVipLoadBalancer) desiredLbStatus(poolService *netv1alpha1.PoolService) (corev1.LoadBalancerStatus, error) {
	ips := strings.Split(poolService.Annotations[AnnotationVipLoadBalancerIPS], ",")
	if len(ips) == 0 {
		// not ready in assign, wait to have next reconclie
		klog.Infof(Format("PoolService: %s/%s has no ips, please check vrid maybe out of limit", poolService.Namespace, poolService.Name))
		return corev1.LoadBalancerStatus{}, fmt.Errorf("PoolService: %s/%s has no ips, please check vrid maybe out of limit", poolService.Namespace, poolService.Name)
	}

	var lbIngress []corev1.LoadBalancerIngress
	for _, ip := range ips {
		lbIngress = append(lbIngress, corev1.LoadBalancerIngress{IP: ip})
	}

	sort.Slice(lbIngress, func(i, j int) bool {
		return lbIngress[i].IP < lbIngress[j].IP
	})

	return corev1.LoadBalancerStatus{
		Ingress: lbIngress,
	}, nil
}

func (r *ReconcileVipLoadBalancer) getCurrentAssignedVRRPs(ctx context.Context, poolService *netv1alpha1.PoolService) ([]VRRP, error) {
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

func filterInvalidPoolService(poolServices []netv1alpha1.PoolService) []VRRP {
	poolIPVrids := []VRRP{}
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

		ips := []string{}
		for _, ip := range poolService.Status.LoadBalancer.Ingress {
			ips = append(ips, ip.IP)
		}
		poolIPVrids = append(poolIPVrids, NewVRRP(ips, vrid))
	}

	return poolIPVrids
}

func (r *ReconcileVipLoadBalancer) checkVRRPs(poolService netv1alpha1.PoolService) (*VRRP, error) {
	// check if the poolservice has vrid-ips
	ipvrid, err := r.isValidVRRP(&poolService)
	if err != nil {
		// vrrp is invalid
		klog.Errorf(Format("Get invalid VRRP from PoolService %s/%s: %v", poolService.Namespace, poolService.Name, err))
		return nil, fmt.Errorf("get invalid VRRP from PoolService %s/%s: %v", poolService.Namespace, poolService.Name, err)
	}

	return ipvrid, nil
}

func (r *ReconcileVipLoadBalancer) containsVRID(poolService *netv1alpha1.PoolService) bool {
	if poolService.Annotations == nil {
		return false
	}

	if _, ok := poolService.Annotations[AnnotationVipLoadBalancerVRID]; !ok {
		return false
	}

	return true
}

func (r *ReconcileVipLoadBalancer) isValidVRRP(poolService *netv1alpha1.PoolService) (*VRRP, error) {
	// check if the poolservice has vrid
	if !r.containsVRID(poolService) {
		// not have VRID
		return nil, fmt.Errorf("PoolService %s/%s doesn't have VRID", poolService.Namespace, poolService.Name)
	}

	vrid, err := strconv.Atoi(poolService.Annotations[AnnotationVipLoadBalancerVRID])
	if err != nil {
		return nil, fmt.Errorf("PoolService %s/%s has invalid VRID: %v", poolService.Namespace, poolService.Name, err)
	}

	ips := []string{}
	if poolService.Status.LoadBalancer.Ingress != nil {
		for _, ip := range poolService.Status.LoadBalancer.Ingress {
			ips = append(ips, ip.IP)
		}
	}

	vrrp := NewVRRP(ips, vrid)
	if err := r.IPManagers[poolService.Labels[network.LabelNodePoolName]].IsValid(vrrp); err != nil {
		return nil, fmt.Errorf("VRID: %d is not valid: %v", vrid, err)
	}

	return &vrrp, nil
}

func (r *ReconcileVipLoadBalancer) handleVRRPs(ctx context.Context, poolService *netv1alpha1.PoolService) error {
	// Check if the PoolService has a available VRID
	if _, err := r.checkVRRPs(*poolService); err == nil {
		// If yes, use the user-specified vrid
		return nil
	}

	// If not, Assign a new VRID to the PoolService
	if err := r.assignVRID(ctx, poolService); err != nil {
		return err
	}

	return nil
}

func (r *ReconcileVipLoadBalancer) assignVRID(ctx context.Context, poolService *netv1alpha1.PoolService) error {
	// Get the poolName from the PoolService
	poolName := poolService.Labels[network.LabelNodePoolName]

	svc, err := r.getReferenceService(ctx, poolService)
	if err != nil {
		klog.Errorf(Format("Failed to get reference service from PoolService %s/%s: %v", poolService.Namespace, poolService.Name, err))
		return err
	}

	var vips []string
	// if specify ip for poolservice annotation, use it as vip
	if svc.Annotations != nil {
		if vipAddress, ok := svc.Annotations[AnnotationServiceVIPAddress]; ok {
			vips = ParseIP(vipAddress)
		}
	}

	vrrp, err := r.IPManagers[poolName].Assign(vips)
	if err != nil {
		// if no available ipvrid, return nil, and wait for next reconcile
		klog.Errorf(Format("Failed to get a new VRID: %v", err))
		r.recorder.Eventf(poolService, corev1.EventTypeWarning, "VRIDExhausted", poolServiceVRIDExhaustedEventMsgFormat,
			poolService.Namespace, poolService.Name, poolName)
		return nil
	}

	// Set the VRID to the PoolService
	if poolService.Annotations == nil {
		poolService.Annotations = make(map[string]string)
	}

	// add ips and vrid in annotions for status sync
	poolService.Annotations[AnnotationVipLoadBalancerIPS] = strings.Join(vrrp.IPs, ",")
	poolService.Annotations[AnnotationVipLoadBalancerVRID] = strconv.Itoa(vrrp.VRID)

	// Update the PoolService
	if err := r.Update(ctx, poolService); err != nil {
		klog.Errorf(Format("Failed to create PoolService %s/%s: %v", poolService.Namespace, poolService.Name, err))
		return err
	}
	return nil
}

func (r *ReconcileVipLoadBalancer) getReferenceService(ctx context.Context, ps *netv1alpha1.PoolService) (*corev1.Service, error) {
	// get the reference service from poolservice
	service := &corev1.Service{}
	svcName := ps.Labels[network.LabelServiceName]
	if err := r.Get(ctx, types.NamespacedName{Name: svcName, Namespace: ps.Namespace}, service); err != nil {
		return nil, err
	}

	return service, nil
}

func (r *ReconcileVipLoadBalancer) reconcileDelete(ctx context.Context, poolService *netv1alpha1.PoolService) (reconcile.Result, error) {
	klog.V(4).Infof(Format("ReconcilDelete VipLoadBalancer %s/%s", poolService.Namespace, poolService.Name))
	poolName := poolService.Labels[network.LabelNodePoolName]

	// Check if the PoolService has a valid VRRP
	ipvrid, err := r.checkVRRPs(*poolService)

	if err != nil {
		return reconcile.Result{}, nil
	}

	// Release the VRRP
	r.IPManagers[poolName].Release(*ipvrid)
	// update the PoolService
	if err := r.Update(ctx, poolService); err != nil {
		klog.Errorf(Format("Failed to update PoolService %s/%s: %v", poolService.Namespace, poolService.Name, err))
		return reconcile.Result{}, err
	}

	// check if the agent has remove the vip service
	if r.checkIfVipServiceOffline(ctx, poolService) {
		// remove the finalizer in the PoolService
		if err := r.removeFinalizer(ctx, poolService); err != nil {
			klog.Errorf(Format("Failed to remove finalizer from PoolService %s/%s: %v", poolService.Namespace, poolService.Name, err))
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileVipLoadBalancer) checkIfVipServiceOffline(ctx context.Context, poolService *netv1alpha1.PoolService) bool {
	klog.V(4).Infof(Format("checkIfVipServiceOffline VipLoadBalancer %s/%s", poolService.Namespace, poolService.Name))
	return r.getVipServiceStatus(ctx, poolService) == ServiceVIPOffline
}

func (r *ReconcileVipLoadBalancer) checkIfVipServiceOnline(ctx context.Context, poolService *netv1alpha1.PoolService) bool {
	klog.V(4).Infof(Format("checkIfVipServiceOnline VipLoadBalancer %s/%s", poolService.Namespace, poolService.Name))
	return r.getVipServiceStatus(ctx, poolService) == ServiceVIPOnline
}

func (r *ReconcileVipLoadBalancer) getVipServiceStatus(ctx context.Context, poolService *netv1alpha1.PoolService) int {
	klog.V(4).Infof(Format("getVipServiceStatus VipLoadBalancer %s/%s", poolService.Namespace, poolService.Name))
	// Get the reference endpoint
	listSelector := &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{
			network.LabelServiceName: poolService.Labels[network.LabelServiceName],
		}),
		Namespace: poolService.Namespace,
	}

	endpointSlice := &corev1.EndpointsList{}
	if err := r.List(ctx, endpointSlice, listSelector); err != nil {
		klog.Errorf(Format("Failed to get Endpoints from PoolService %s/%s: %v", poolService.Namespace, poolService.Name, err))
		return ServiceVIPUnknown
	}

	if len(endpointSlice.Items) == 0 {
		klog.Errorf(Format("get Endpoints from PoolService %s/%s is empty", poolService.Namespace, poolService.Name))
		return ServiceVIPUnknown
	}

	ready := 0
	target := len(endpointSlice.Items)/2 + 1
	for _, ep := range endpointSlice.Items {
		if ep.Annotations == nil {
			continue
		}

		if _, ok := ep.Annotations[AnnotationServiceVIPStatus]; !ok {
			continue
		}

		if ep.Annotations[AnnotationServiceVIPStatus] == AnnotationServiceVIPStatusOnline {
			ready++
		}
	}

	if ready < target {
		return ServiceVIPOffline
	}

	return ServiceVIPOnline
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
