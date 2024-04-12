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

package loadbalancerset

import (
	"context"
	"fmt"
	"reflect"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
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
	"github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
	"github.com/openyurtio/openyurt/pkg/apis/network"
	netv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/network/v1alpha1"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/loadbalancerset/loadbalancerset/config"
)

const (
	labelManageBy = "poolservice.openyurt.io/managed-by"

	poolServiceModifiedEventMsgFormat        = "PoolService %s/%s resource is manually modified, the controller will overwrite this modification"
	poolServiceManagedConflictEventMsgFormat = "PoolService %s/%s is not managed by pool-service-controller, but the nodepool-labelselector of service %s/%s include it"
)

var (
	poolServicesControllerResource = netv1alpha1.SchemeGroupVersion.WithResource("poolservices")
	nodepoolsControllerResource    = v1beta1.SchemeGroupVersion.WithResource("nodepools")
)

func Format(format string, args ...interface{}) string {
	s := fmt.Sprintf(format, args...)
	return fmt.Sprintf("%s: %s", names.LoadBalancerSetController, s)
}

// Add creates a new PoolService Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(ctx context.Context, c *appconfig.CompletedConfig, mgr manager.Manager) error {
	klog.Infof(Format("loadbalancerset-controller add controller %s", poolServicesControllerResource.String()))
	r := newReconciler(c, mgr)

	if _, err := r.mapper.KindFor(poolServicesControllerResource); err != nil {
		return errors.Errorf("resource %s isn't exist", poolServicesControllerResource.String())
	}
	if _, err := r.mapper.KindFor(nodepoolsControllerResource); err != nil {
		return errors.Errorf("resource %s isn't exist", nodepoolsControllerResource.String())
	}

	return add(mgr, c, r)
}

var _ reconcile.Reconciler = &ReconcileLoadBalancerSet{}

// ReconcileLoadBalancerSet reconciles a PoolService object
type ReconcileLoadBalancerSet struct {
	client.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder
	mapper   meta.RESTMapper

	configration config.LoadBalancerSetControllerConfiguration
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(c *appconfig.CompletedConfig, mgr manager.Manager) *ReconcileLoadBalancerSet {
	return &ReconcileLoadBalancerSet{
		Client:       mgr.GetClient(),
		scheme:       mgr.GetScheme(),
		mapper:       mgr.GetRESTMapper(),
		recorder:     mgr.GetEventRecorderFor(names.LoadBalancerSetController),
		configration: c.ComponentConfig.LoadBalancerSetController,
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, cfg *appconfig.CompletedConfig, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(names.LoadBalancerSetController, mgr, controller.Options{
		Reconciler: r, MaxConcurrentReconciles: int(cfg.ComponentConfig.LoadBalancerSetController.ConcurrentLoadBalancerSetWorkers),
	})
	if err != nil {
		return err
	}

	// Watch for changes to PoolService
	err = c.Watch(&source.Kind{Type: &corev1.Service{}}, &handler.EnqueueRequestForObject{}, NewServicePredicated())
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &netv1alpha1.PoolService{}}, NewPoolServiceEventHandler(), NewPoolServicePredicated())
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &v1beta1.NodePool{}}, NewNodePoolEventHandler(mgr.GetClient()), NewNodePoolPredicated())
	if err != nil {
		return err
	}

	return nil
}

// +kubebuilder:rbac:groups=network.openyurt.io,resources=poolservices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=network.openyurt.io,resources=poolservices/status,verbs=get;update;patch

// Reconcile reads that state of the cluster for a PoolService object and makes changes based on the state read
// and what is in the PoolService.Spec
func (r *ReconcileLoadBalancerSet) Reconcile(_ context.Context, request reconcile.Request) (reconcile.Result, error) {

	// Note !!!!!!!!!!
	// We strongly recommend use Format() to  encapsulation because Format() can print logs by module
	// @kadisi
	klog.Infof(Format("Reconcile PoolService %s/%s", request.Namespace, request.Name))

	service := &corev1.Service{}
	err := r.Get(context.TODO(), request.NamespacedName, service)
	if apierrors.IsNotFound(err) {
		return reconcile.Result{}, nil
	}
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "failed to get service %s", request.String())
	}

	copySvc := service.DeepCopy()

	if shouldDeleteAllPoolServices(copySvc) {
		if err := r.deleteAllPoolServices(copySvc); err != nil {
			return reconcile.Result{}, errors.Wrapf(err, "failed to clean pool services")
		}
		return reconcile.Result{}, nil
	}

	if err := r.reconcilePoolServices(copySvc); err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "failed to reconcile service")
	}

	if err := r.syncService(copySvc); err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "failed to sync service %s/%s", copySvc.Namespace, copySvc.Name)
	}

	return reconcile.Result{}, nil
}

func shouldDeleteAllPoolServices(svc *corev1.Service) bool {
	if !svc.DeletionTimestamp.IsZero() {
		return true
	}

	if !isLoadBalancerSetService(svc) {
		return true
	}

	return false
}

func (r *ReconcileLoadBalancerSet) deleteAllPoolServices(svc *corev1.Service) error {
	currentPoolServices, err := r.currentPoolServices(svc)
	if err != nil {
		return errors.Wrapf(err, "failed to get current pool services for service %s/%s", svc.Namespace, svc.Name)
	}

	if canRemoveFinalizer(currentPoolServices) {
		if err := r.removeFinalizer(svc); err != nil {
			return errors.Wrapf(err, "failed to remove finalizer")
		}
		return nil
	}

	if err := r.deletePoolServices(currentPoolServices); err != nil {
		return errors.Wrapf(err, "failed to delete all pool services for service %s/%s", svc.Namespace, svc.Name)
	}

	return nil
}

func (r *ReconcileLoadBalancerSet) currentPoolServices(svc *corev1.Service) ([]netv1alpha1.PoolService, error) {
	listSelector := &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{
			network.LabelServiceName: svc.Name,
			labelManageBy:            names.LoadBalancerSetController}),
		Namespace: svc.Namespace,
	}

	poolServiceList := &netv1alpha1.PoolServiceList{}
	if err := r.List(context.TODO(), poolServiceList, listSelector); err != nil {
		return nil, errors.Wrapf(err, "failed to list pool service with %s/%s", svc.Namespace, svc.Name)
	}

	return filterInvalidPoolService(poolServiceList.Items), nil
}

func filterInvalidPoolService(poolServices []netv1alpha1.PoolService) []netv1alpha1.PoolService {
	var filteredPoolServices []netv1alpha1.PoolService
	for _, item := range poolServices {
		if !isValidPoolService(&item) {
			klog.Warningf("Pool Service %s/%s is not valid, label %s or %s is modified",
				item.Namespace, item.Name, network.LabelServiceName, network.LabelNodePoolName)
			continue
		}
		filteredPoolServices = append(filteredPoolServices, item)
	}
	return filteredPoolServices
}

func isValidPoolService(poolService *netv1alpha1.PoolService) bool {
	if poolService.Labels == nil {
		return false
	}
	if poolService.Labels[network.LabelServiceName]+"-"+poolService.Labels[network.LabelNodePoolName] != poolService.Name {
		return false
	}
	return true
}

func canRemoveFinalizer(poolServices []netv1alpha1.PoolService) bool {
	return len(poolServices) == 0
}

func (r *ReconcileLoadBalancerSet) deletePoolServices(poolServices []netv1alpha1.PoolService) error {
	for _, ps := range poolServices {
		if !ps.DeletionTimestamp.IsZero() {
			continue
		}

		if err := r.Delete(context.Background(), &ps); err != nil {
			return errors.Wrapf(err, "failed to delete poolservice %s/%s", ps.Namespace, ps.Name)
		}
	}
	return nil
}

func (r *ReconcileLoadBalancerSet) reconcilePoolServices(svc *corev1.Service) error {
	if err := r.addFinalizer(svc); err != nil {
		return errors.Wrapf(err, "failed to add finalizer")
	}

	if err := r.syncPoolServices(svc); err != nil {
		return errors.Wrapf(err, "failed to sync pool services")
	}
	return nil
}

func (r *ReconcileLoadBalancerSet) syncPoolServices(svc *corev1.Service) error {
	currentPoolServices, err := r.currentPoolServices(svc)
	if err != nil {
		return errors.Wrapf(err, "failed to get current pool services for service %s/%s", svc.Namespace, svc.Name)
	}

	desiredPoolServices, err := r.desiredPoolServices(svc)
	if err != nil {
		return errors.Wrapf(err, "failed to calculate desire pool services for service %s/%s", svc.Namespace, svc.Name)
	}
	poolServicesToApply, poolServicesToDelete := r.diffPoolServices(desiredPoolServices, currentPoolServices)

	if err := r.deletePoolServices(poolServicesToDelete); err != nil {
		return errors.Wrapf(err, "failed to delete pool services %v", poolServicesToDelete)
	}

	if err := r.applyPoolServices(poolServicesToApply); err != nil {
		return errors.Wrapf(err, "failed to apply pool services %v", poolServicesToApply)
	}

	return nil
}

func (r *ReconcileLoadBalancerSet) desiredPoolServices(svc *corev1.Service) ([]netv1alpha1.PoolService, error) {
	if !isLoadBalancerSetService(svc) {
		klog.Warningf("service %s/%s is not multi regional service, set desire pool services is nil", svc.Namespace, svc.Name)
		return nil, nil
	}

	nps, err := r.listNodePoolsByLabelSelector(svc)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to list nodepool with service")
	}

	var pss []netv1alpha1.PoolService
	for _, np := range nps {
		pss = append(pss, buildPoolService(svc, &np))
	}
	return pss, nil
}

func (r *ReconcileLoadBalancerSet) listNodePoolsByLabelSelector(svc *corev1.Service) ([]v1beta1.NodePool, error) {
	labelStr := svc.Annotations[network.AnnotationNodePoolSelector]
	labelSelector, err := labels.Parse(labelStr)
	if err != nil {
		return nil, err
	}

	npList := &v1beta1.NodePoolList{}
	if err := r.List(context.Background(), npList, &client.ListOptions{LabelSelector: labelSelector}); err != nil {
		return nil, err
	}

	return filterDeletionNodePools(npList.Items), nil
}

func filterDeletionNodePools(allItems []v1beta1.NodePool) []v1beta1.NodePool {
	var filterItems []v1beta1.NodePool

	for _, item := range allItems {
		if !item.DeletionTimestamp.IsZero() {
			continue
		}
		filterItems = append(filterItems, item)
	}

	return filterItems
}

func buildPoolService(svc *corev1.Service, np *v1beta1.NodePool) netv1alpha1.PoolService {
	isController, isBlockOwnerDeletion := true, true
	return netv1alpha1.PoolService{
		TypeMeta: v1.TypeMeta{
			Kind:       "PoolService",
			APIVersion: netv1alpha1.GroupVersion.String(),
		},
		ObjectMeta: v1.ObjectMeta{
			Namespace: svc.Namespace,
			Name:      svc.Name + "-" + np.Name,
			Labels:    map[string]string{network.LabelServiceName: svc.Name, network.LabelNodePoolName: np.Name, labelManageBy: names.LoadBalancerSetController},
			OwnerReferences: []v1.OwnerReference{
				{
					APIVersion:         svc.APIVersion,
					Name:               svc.Name,
					Kind:               svc.Kind,
					UID:                svc.UID,
					Controller:         &isController,
					BlockOwnerDeletion: &isBlockOwnerDeletion,
				},
				{
					APIVersion:         np.APIVersion,
					Name:               np.Name,
					Kind:               np.Kind,
					UID:                np.UID,
					BlockOwnerDeletion: &isBlockOwnerDeletion,
				},
			},
		},
		Spec: netv1alpha1.PoolServiceSpec{
			LoadBalancerClass: svc.Spec.LoadBalancerClass,
		},
	}
}

func (r *ReconcileLoadBalancerSet) diffPoolServices(desirePoolServices, currentPoolServices []netv1alpha1.PoolService) (applications []netv1alpha1.PoolService, deletions []netv1alpha1.PoolService) {
	for _, dps := range desirePoolServices {
		if exist := r.isPoolServicePresent(currentPoolServices, dps); !exist {
			applications = append(applications, dps)
		}
	}

	for _, cps := range currentPoolServices {
		if exist := r.isPoolServicePresent(desirePoolServices, cps); !exist {
			deletions = append(deletions, cps)
		}
	}

	return
}

func (r *ReconcileLoadBalancerSet) isPoolServicePresent(poolServices []netv1alpha1.PoolService, ps netv1alpha1.PoolService) bool {
	for _, dps := range poolServices {
		if dps.Name == ps.Name {
			return true
		}
	}
	return false
}

func (r *ReconcileLoadBalancerSet) applyPoolServices(poolServices []netv1alpha1.PoolService) error {
	for _, ps := range poolServices {
		if err := r.applyPoolService(&ps); err != nil {
			return errors.Wrapf(err, "failed to apply pool service %s/%s", ps.Namespace, ps.Name)
		}
	}
	return nil
}

func (r *ReconcileLoadBalancerSet) applyPoolService(poolService *netv1alpha1.PoolService) error {
	currentPoolService, exist, err := r.tryGetPoolService(poolService.Namespace, poolService.Name)
	if err != nil {
		return errors.Wrapf(err, "failed to try get pool service %s/%s", poolService.Namespace, poolService.Name)
	}

	if exist {
		if err := r.compareAndUpdatePoolService(currentPoolService, poolService); err != nil {
			return errors.Wrapf(err, "failed to compare and update pool service %s/%s", poolService.Namespace, poolService.Name)
		}
		return nil
	}

	return r.Create(context.Background(), poolService, &client.CreateOptions{})
}

func (r *ReconcileLoadBalancerSet) tryGetPoolService(namespace, name string) (*netv1alpha1.PoolService, bool, error) {
	currentPs := &netv1alpha1.PoolService{}
	err := r.Get(context.Background(), types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}, currentPs)

	if apierrors.IsNotFound(err) {
		return nil, false, nil
	}
	return currentPs, true, err
}

func (r *ReconcileLoadBalancerSet) compareAndUpdatePoolService(currentPoolService, desirePoolService *netv1alpha1.PoolService) error {
	if currentPoolService.Labels[labelManageBy] != names.LoadBalancerSetController {
		r.recorder.Eventf(currentPoolService, corev1.EventTypeWarning, "ManagedConflict", poolServiceManagedConflictEventMsgFormat,
			currentPoolService.Namespace, currentPoolService.Name, currentPoolService.Namespace, desirePoolService.Labels[network.LabelServiceName])
		return nil
	}

	isLabelUpdated := compareAndUpdatePoolServiceLabel(currentPoolService, desirePoolService.Labels)
	isOwnerUpdated := compareAndUpdatePoolServiceOwners(currentPoolService, desirePoolService.OwnerReferences)

	if !isLabelUpdated && !isOwnerUpdated {
		return nil
	}

	r.recorder.Eventf(currentPoolService, corev1.EventTypeWarning, "Modified", poolServiceModifiedEventMsgFormat, currentPoolService.Namespace, currentPoolService.Name)
	if err := r.Update(context.Background(), currentPoolService); err != nil {
		return errors.Wrapf(err, "failed to update pool service")
	}

	return nil
}

func compareAndUpdatePoolServiceLabel(currentPoolService *netv1alpha1.PoolService, desireLabels map[string]string) bool {
	isUpdate := false
	if currentPoolService.Labels[network.LabelServiceName] != desireLabels[network.LabelServiceName] {
		currentPoolService.Labels[network.LabelServiceName] = desireLabels[network.LabelServiceName]
		isUpdate = true
	}

	if currentPoolService.Labels[network.LabelNodePoolName] != desireLabels[network.LabelNodePoolName] {
		currentPoolService.Labels[network.LabelNodePoolName] = desireLabels[network.LabelNodePoolName]
		isUpdate = true
	}

	return isUpdate
}

func compareAndUpdatePoolServiceOwners(currentPoolService *netv1alpha1.PoolService, desireOwners []v1.OwnerReference) bool {
	if !reflect.DeepEqual(currentPoolService.OwnerReferences, desireOwners) {
		currentPoolService.OwnerReferences = desireOwners
		return true
	}
	return false
}

func (r *ReconcileLoadBalancerSet) syncService(svc *corev1.Service) error {
	poolServices, err := r.currentPoolServices(svc)
	if err != nil {
		return errors.Wrapf(err, "failed to get current pool services for service %s/%s", svc.Namespace, svc.Name)
	}

	aggregatedAnnotations := aggregatePoolServicesAnnotations(poolServices)
	aggregatedLbStatus := aggregateLbStatus(poolServices)

	return r.compareAndUpdateService(svc, aggregatedAnnotations, aggregatedLbStatus)
}

func (r *ReconcileLoadBalancerSet) compareAndUpdateService(svc *corev1.Service, annotations map[string]string, lbStatus corev1.LoadBalancerStatus) error {
	isUpdatedAnnotations := compareAndUpdateServiceAnnotations(svc, annotations)
	isUpdatedLbStatus := compareAndUpdateServiceLbStatus(svc, lbStatus)

	if !isUpdatedLbStatus && !isUpdatedAnnotations {
		return nil
	}

	return r.Update(context.Background(), svc)
}
