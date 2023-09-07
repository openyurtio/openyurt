/*
Copyright 2021 The OpenYurt Authors.
Copyright 2019 The Kruise Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

@CHANGELOG
OpenYurt Authors:
change uniteddeployment reconcile
*/

package yurtappset

import (
	"context"
	"flag"
	"fmt"
	"reflect"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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

	"github.com/openyurtio/openyurt/cmd/yurt-manager/app/config"
	"github.com/openyurtio/openyurt/cmd/yurt-manager/names"
	unitv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/yurtappset/adapter"
)

func init() {
	flag.IntVar(&concurrentReconciles, "yurtappset-workers", concurrentReconciles, "Max concurrent workers for YurtAppSet controller.")
}

var (
	concurrentReconciles = 3
	controllerResource   = unitv1alpha1.SchemeGroupVersion.WithResource("yurtappsets")
)

const (
	eventTypeRevisionProvision  = "RevisionProvision"
	eventTypeFindPools          = "FindPools"
	eventTypeDupPoolsDelete     = "DeleteDuplicatedPools"
	eventTypePoolsUpdate        = "UpdatePool"
	eventTypeTemplateController = "TemplateController"

	slowStartInitialBatchSize = 1
)

func Format(format string, args ...interface{}) string {
	s := fmt.Sprintf(format, args...)
	return fmt.Sprintf("%s: %s", names.YurtAppSetController, s)
}

// Add creates a new YurtAppSet Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(c *config.CompletedConfig, mgr manager.Manager) error {
	if _, err := mgr.GetRESTMapper().KindFor(controllerResource); err != nil {
		klog.Infof("resource %s doesn't exist", controllerResource.String())
		return err
	}

	klog.Infof("yurtappset-controller add controller %s", controllerResource.String())
	return add(mgr, newReconciler(c, mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(c *config.CompletedConfig, mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileYurtAppSet{
		Client: mgr.GetClient(),
		scheme: mgr.GetScheme(),

		recorder: mgr.GetEventRecorderFor(names.YurtAppSetController),
		poolControls: map[unitv1alpha1.TemplateType]ControlInterface{
			unitv1alpha1.StatefulSetTemplateType: &PoolControl{Client: mgr.GetClient(), scheme: mgr.GetScheme(),
				adapter: &adapter.StatefulSetAdapter{Client: mgr.GetClient(), Scheme: mgr.GetScheme()}},
			unitv1alpha1.DeploymentTemplateType: &PoolControl{Client: mgr.GetClient(), scheme: mgr.GetScheme(),
				adapter: &adapter.DeploymentAdapter{Client: mgr.GetClient(), Scheme: mgr.GetScheme()}},
		},
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(names.YurtAppSetController, mgr, controller.Options{Reconciler: r, MaxConcurrentReconciles: concurrentReconciles})
	if err != nil {
		return err
	}

	// Watch for changes to YurtAppSet
	err = c.Watch(&source.Kind{Type: &unitv1alpha1.YurtAppSet{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &appsv1.StatefulSet{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &unitv1alpha1.YurtAppSet{},
	})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &unitv1alpha1.YurtAppSet{},
	})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileYurtAppSet{}

// ReconcileYurtAppSet reconciles a YurtAppSet object
type ReconcileYurtAppSet struct {
	client.Client
	scheme *runtime.Scheme

	recorder     record.EventRecorder
	poolControls map[unitv1alpha1.TemplateType]ControlInterface
}

// +kubebuilder:rbac:groups=apps.openyurt.io,resources=yurtappsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.openyurt.io,resources=yurtappsets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=statefulsets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=controllerrevisions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch;create;update;patch;delete

// Reconcile reads that state of the cluster for a YurtAppSet object and makes changes based on the state read
// and what is in the YurtAppSet.Spec
func (r *ReconcileYurtAppSet) Reconcile(_ context.Context, request reconcile.Request) (reconcile.Result, error) {
	klog.V(4).Infof("Reconcile YurtAppSet %s/%s", request.Namespace, request.Name)
	// Fetch the YurtAppSet instance
	instance := &unitv1alpha1.YurtAppSet{}
	err := r.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	if instance.DeletionTimestamp != nil {
		return reconcile.Result{}, nil
	}
	oldStatus := instance.Status.DeepCopy()

	currentRevision, updatedRevision, collisionCount, err := r.constructYurtAppSetRevisions(instance)
	if err != nil {
		klog.Errorf("Fail to construct controller revision of YurtAppSet %s/%s: %s", instance.Namespace, instance.Name, err)
		r.recorder.Event(instance.DeepCopy(), corev1.EventTypeWarning, fmt.Sprintf("Failed%s", eventTypeRevisionProvision), err.Error())
		return reconcile.Result{}, err
	}

	control, poolType, err := r.getPoolControls(instance)
	if err != nil {
		r.recorder.Event(instance.DeepCopy(), corev1.EventTypeWarning, fmt.Sprintf("Failed%s", eventTypeTemplateController), err.Error())
		return reconcile.Result{}, err
	}

	klog.V(4).Infof("Get YurtAppSet %s/%s all pools", request.Namespace, request.Name)

	nameToPool, err := r.getNameToPool(instance, control)
	if err != nil {
		klog.Errorf("Fail to get Pools of YurtAppSet %s/%s: %s", instance.Namespace, instance.Name, err)
		r.recorder.Event(instance.DeepCopy(), corev1.EventTypeWarning, fmt.Sprintf("Failed %s",
			eventTypeFindPools), err.Error())
		return reconcile.Result{}, nil
	}

	nextPatches := GetNextPatches(instance)
	klog.V(4).Infof("Get YurtAppSet %s/%s next Patches %v", instance.Namespace, instance.Name, nextPatches)

	expectedRevision := currentRevision
	if updatedRevision != nil {
		expectedRevision = updatedRevision
	}
	newStatus, err := r.managePools(instance, nameToPool, nextPatches, expectedRevision, poolType)
	if err != nil {
		klog.Errorf("Fail to update YurtAppSet %s/%s: %s", instance.Namespace, instance.Name, err)
		r.recorder.Event(instance.DeepCopy(), corev1.EventTypeWarning, fmt.Sprintf("Failed%s", eventTypePoolsUpdate), err.Error())
	}

	return r.updateStatus(instance, newStatus, oldStatus, nameToPool, currentRevision, collisionCount, control)
}

func (r *ReconcileYurtAppSet) getNameToPool(instance *unitv1alpha1.YurtAppSet, control ControlInterface) (map[string]*Pool, error) {
	pools, err := control.GetAllPools(instance)
	if err != nil {
		r.recorder.Event(instance.DeepCopy(), corev1.EventTypeWarning, fmt.Sprintf("Failed%s", eventTypeFindPools), err.Error())
		return nil, fmt.Errorf("fail to get all Pools for YurtAppSet %s/%s: %s", instance.Namespace, instance.Name, err)
	}

	klog.V(4).Infof("Classify YurtAppSet %s/%s by pool name", instance.Namespace, instance.Name)
	nameToPools := r.classifyPoolByPoolName(pools)

	nameToPool, err := r.deleteDupPool(nameToPools, control)
	if err != nil {
		r.recorder.Event(instance.DeepCopy(), corev1.EventTypeWarning, fmt.Sprintf("Failed%s", eventTypeDupPoolsDelete), err.Error())
		return nil, fmt.Errorf("fail to manage duplicate Pool of YurtAppSet %s/%s: %s", instance.Namespace, instance.Name, err)
	}

	return nameToPool, nil
}

func (r *ReconcileYurtAppSet) deleteDupPool(nameToPools map[string][]*Pool, control ControlInterface) (map[string]*Pool, error) {
	nameToPool := map[string]*Pool{}
	for name, pools := range nameToPools {
		if len(pools) > 1 {
			for _, pool := range pools[1:] {
				klog.V(0).Infof("Delete duplicated Pool %s/%s for pool name %s", pool.Namespace, pool.Name, name)
				if err := control.DeletePool(pool); err != nil {
					if errors.IsNotFound(err) {
						continue
					}
					return nameToPool, err
				}
			}
		}

		if len(pools) > 0 {
			nameToPool[name] = pools[0]
		}
	}

	return nameToPool, nil
}

func (r *ReconcileYurtAppSet) getPoolControls(instance *unitv1alpha1.YurtAppSet) (ControlInterface,
	unitv1alpha1.TemplateType, error) {
	switch {

	case instance.Spec.WorkloadTemplate.StatefulSetTemplate != nil:
		return r.poolControls[unitv1alpha1.StatefulSetTemplateType], unitv1alpha1.StatefulSetTemplateType, nil
	case instance.Spec.WorkloadTemplate.DeploymentTemplate != nil:
		return r.poolControls[unitv1alpha1.DeploymentTemplateType], unitv1alpha1.DeploymentTemplateType, nil
	default:
		klog.Errorf("The appropriate WorkloadTemplate was not found")
		return nil, "", fmt.Errorf("The appropriate WorkloadTemplate was not found, Now Support(%s/%s)",
			unitv1alpha1.StatefulSetTemplateType, unitv1alpha1.DeploymentTemplateType)
	}
}

func (r *ReconcileYurtAppSet) classifyPoolByPoolName(pools []*Pool) map[string][]*Pool {
	mapping := map[string][]*Pool{}

	for _, ss := range pools {
		poolName := ss.Name
		mapping[poolName] = append(mapping[poolName], ss)
	}
	return mapping
}

func (r *ReconcileYurtAppSet) updateStatus(instance *unitv1alpha1.YurtAppSet, newStatus, oldStatus *unitv1alpha1.YurtAppSetStatus,
	nameToPool map[string]*Pool, currentRevision *appsv1.ControllerRevision,
	collisionCount int32, control ControlInterface) (reconcile.Result, error) {

	newStatus = r.calculateStatus(instance, newStatus, nameToPool, currentRevision, collisionCount, control)
	_, err := r.updateYurtAppSet(instance, oldStatus, newStatus)

	return reconcile.Result{}, err
}

func (r *ReconcileYurtAppSet) calculateStatus(instance *unitv1alpha1.YurtAppSet, newStatus *unitv1alpha1.YurtAppSetStatus,
	nameToPool map[string]*Pool, currentRevision *appsv1.ControllerRevision,
	collisionCount int32, control ControlInterface) *unitv1alpha1.YurtAppSetStatus {

	newStatus.CollisionCount = &collisionCount

	if newStatus.CurrentRevision == "" {
		// init with current revision
		newStatus.CurrentRevision = currentRevision.Name
	}

	// sync from status
	newStatus.PoolReplicas = make(map[string]int32)
	newStatus.ReadyReplicas = 0
	newStatus.Replicas = 0
	for _, pool := range nameToPool {
		newStatus.PoolReplicas[pool.Name] = pool.Status.Replicas
		newStatus.Replicas += pool.Status.Replicas
		newStatus.ReadyReplicas += pool.Status.ReadyReplicas
	}

	newStatus.TemplateType = getPoolTemplateType(instance)

	var poolFailure *string
	for _, pool := range nameToPool {
		failureMessage := control.GetPoolFailure(pool)
		if failureMessage != nil {
			poolFailure = failureMessage
			break
		}
	}

	if poolFailure == nil {
		RemoveYurtAppSetCondition(newStatus, unitv1alpha1.PoolFailure)
	} else {
		SetYurtAppSetCondition(newStatus, NewYurtAppSetCondition(unitv1alpha1.PoolFailure, corev1.ConditionTrue, "Error", *poolFailure))
	}

	return newStatus
}

func getPoolTemplateType(obj *unitv1alpha1.YurtAppSet) (templateType unitv1alpha1.TemplateType) {
	template := obj.Spec.WorkloadTemplate
	switch {
	case template.StatefulSetTemplate != nil:
		templateType = unitv1alpha1.StatefulSetTemplateType
	case template.DeploymentTemplate != nil:
		templateType = unitv1alpha1.DeploymentTemplateType
	default:
		klog.Warning("YurtAppSet.Spec.WorkloadTemplate exist wrong template")
	}
	return
}

func (r *ReconcileYurtAppSet) updateYurtAppSet(yas *unitv1alpha1.YurtAppSet, oldStatus, newStatus *unitv1alpha1.YurtAppSetStatus) (*unitv1alpha1.YurtAppSet, error) {
	if oldStatus.CurrentRevision == newStatus.CurrentRevision &&
		oldStatus.CollisionCount == newStatus.CollisionCount &&
		oldStatus.Replicas == newStatus.Replicas &&
		oldStatus.ReadyReplicas == newStatus.ReadyReplicas &&
		yas.Generation == newStatus.ObservedGeneration &&
		reflect.DeepEqual(oldStatus.PoolReplicas, newStatus.PoolReplicas) &&
		reflect.DeepEqual(oldStatus.Conditions, newStatus.Conditions) {
		return yas, nil
	}

	newStatus.ObservedGeneration = yas.Generation

	var getErr, updateErr error
	for i, obj := 0, yas; ; i++ {
		klog.V(4).Infof(fmt.Sprintf("The %d th time updating status for %v: %s/%s, ", i, obj.Kind, obj.Namespace, obj.Name) +
			fmt.Sprintf("sequence No: %v->%v", obj.Status.ObservedGeneration, newStatus.ObservedGeneration))

		obj.Status = *newStatus

		updateErr = r.Client.Status().Update(context.TODO(), obj)
		if updateErr == nil {
			return obj, nil
		}
		if i >= updateRetries {
			break
		}
		tmpObj := &unitv1alpha1.YurtAppSet{}
		if getErr = r.Client.Get(context.TODO(), client.ObjectKey{Namespace: obj.Namespace, Name: obj.Name}, tmpObj); getErr != nil {
			return nil, getErr
		}
		obj = tmpObj
	}

	klog.Errorf("fail to update YurtAppSet %s/%s status: %s", yas.Namespace, yas.Name, updateErr)
	return nil, updateErr
}
