/*
Copyright 2020 The OpenYurt Authors.
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

package uniteddeployment

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
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/alibaba/openyurt/pkg/yurtappmanager/controller/uniteddeployment/adapter"
	"github.com/alibaba/openyurt/pkg/yurtappmanager/util/gate"

	unitv1alpha1 "github.com/alibaba/openyurt/pkg/yurtappmanager/apis/apps/v1alpha1"
)

func init() {
	flag.IntVar(&concurrentReconciles, "uniteddeployment-workers", concurrentReconciles, "Max concurrent workers for UnitedDeployment controller.")
}

var (
	concurrentReconciles = 3
)

const (
	controllerName = "uniteddeployment-controller"

	eventTypeRevisionProvision  = "RevisionProvision"
	eventTypeFindPools          = "FindPools"
	eventTypeDupPoolsDelete     = "DeleteDuplicatedPools"
	eventTypePoolsUpdate        = "UpdatePool"
	eventTypeTemplateController = "TemplateController"

	slowStartInitialBatchSize = 1
)

// Add creates a new UnitedDeployment Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, _ context.Context) error {
	if !gate.ResourceEnabled(&unitv1alpha1.UnitedDeployment{}) {
		return nil
	}
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileUnitedDeployment{
		Client: mgr.GetClient(),
		scheme: mgr.GetScheme(),

		recorder: mgr.GetEventRecorderFor(controllerName),
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
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r, MaxConcurrentReconciles: concurrentReconciles})
	if err != nil {
		return err
	}

	// Watch for changes to UnitedDeployment
	err = c.Watch(&source.Kind{Type: &unitv1alpha1.UnitedDeployment{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &appsv1.StatefulSet{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &unitv1alpha1.UnitedDeployment{},
	})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &unitv1alpha1.UnitedDeployment{},
	})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileUnitedDeployment{}

// ReconcileUnitedDeployment reconciles a UnitedDeployment object
type ReconcileUnitedDeployment struct {
	client.Client
	scheme *runtime.Scheme

	recorder     record.EventRecorder
	poolControls map[unitv1alpha1.TemplateType]ControlInterface
}

// +kubebuilder:rbac:groups=apps.openyurt.io,resources=uniteddeployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.openyurt.io,resources=uniteddeployments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=statefulsets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=controllerrevisions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete

// Reconcile reads that state of the cluster for a UnitedDeployment object and makes changes based on the state read
// and what is in the UnitedDeployment.Spec
func (r *ReconcileUnitedDeployment) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	klog.V(4).Infof("Reconcile UnitedDeployment %s/%s", request.Namespace, request.Name)
	// Fetch the UnitedDeployment instance
	instance := &unitv1alpha1.UnitedDeployment{}
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

	currentRevision, updatedRevision, collisionCount, err := r.constructUnitedDeploymentRevisions(instance)
	if err != nil {
		klog.Errorf("Fail to construct controller revision of UnitedDeployment %s/%s: %s", instance.Namespace, instance.Name, err)
		r.recorder.Event(instance.DeepCopy(), corev1.EventTypeWarning, fmt.Sprintf("Failed%s", eventTypeRevisionProvision), err.Error())
		return reconcile.Result{}, err
	}

	control, poolType, err := r.getPoolControls(instance)
	if err != nil {
		r.recorder.Event(instance.DeepCopy(), corev1.EventTypeWarning, fmt.Sprintf("Failed%s", eventTypeTemplateController), err.Error())
		return reconcile.Result{}, err
	}

	klog.V(4).Infof("Get UnitedDeployment %s/%s all pools", request.Namespace, request.Name)

	nameToPool, err := r.getNameToPool(instance, control)
	if err != nil {
		klog.Errorf("Fail to get Pools of UnitedDeployment %s/%s: %s", instance.Namespace, instance.Name, err)
		r.recorder.Event(instance.DeepCopy(), corev1.EventTypeWarning, fmt.Sprintf("Failed %s",
			eventTypeFindPools), err.Error())
		return reconcile.Result{}, nil
	}

	nextReplicas := GetNextReplicas(instance)
	klog.V(4).Infof("Get UnitedDeployment %s/%s next Replicas %v", instance.Namespace, instance.Name, nextReplicas)

	expectedRevision := currentRevision
	if updatedRevision != nil {
		expectedRevision = updatedRevision
	}
	newStatus, err := r.managePools(instance, nameToPool, nextReplicas, expectedRevision, poolType)
	if err != nil {
		klog.Errorf("Fail to update UnitedDeployment %s/%s: %s", instance.Namespace, instance.Name, err)
		r.recorder.Event(instance.DeepCopy(), corev1.EventTypeWarning, fmt.Sprintf("Failed%s", eventTypePoolsUpdate), err.Error())
	}

	return r.updateStatus(instance, newStatus, oldStatus, nameToPool, currentRevision, collisionCount, control)
}

func (r *ReconcileUnitedDeployment) getNameToPool(instance *unitv1alpha1.UnitedDeployment, control ControlInterface) (map[string]*Pool, error) {
	pools, err := control.GetAllPools(instance)
	if err != nil {
		r.recorder.Event(instance.DeepCopy(), corev1.EventTypeWarning, fmt.Sprintf("Failed%s", eventTypeFindPools), err.Error())
		return nil, fmt.Errorf("fail to get all Pools for UnitedDeployment %s/%s: %s", instance.Namespace, instance.Name, err)
	}

	klog.V(4).Infof("Classify UnitedDeployment %s/%s by pool name", instance.Namespace, instance.Name)
	nameToPools := r.classifyPoolByPoolName(pools)

	nameToPool, err := r.deleteDupPool(nameToPools, control)
	if err != nil {
		r.recorder.Event(instance.DeepCopy(), corev1.EventTypeWarning, fmt.Sprintf("Failed%s", eventTypeDupPoolsDelete), err.Error())
		return nil, fmt.Errorf("fail to manage duplicate Pool of UnitedDeployment %s/%s: %s", instance.Namespace, instance.Name, err)
	}

	return nameToPool, nil
}

func (r *ReconcileUnitedDeployment) deleteDupPool(nameToPools map[string][]*Pool, control ControlInterface) (map[string]*Pool, error) {
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

func (r *ReconcileUnitedDeployment) getPoolControls(instance *unitv1alpha1.UnitedDeployment) (ControlInterface,
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

func (r *ReconcileUnitedDeployment) classifyPoolByPoolName(pools []*Pool) map[string][]*Pool {
	mapping := map[string][]*Pool{}

	for _, ss := range pools {
		poolName := ss.Name
		mapping[poolName] = append(mapping[poolName], ss)
	}
	return mapping
}

func (r *ReconcileUnitedDeployment) updateStatus(instance *unitv1alpha1.UnitedDeployment, newStatus, oldStatus *unitv1alpha1.UnitedDeploymentStatus,
	nameToPool map[string]*Pool, currentRevision *appsv1.ControllerRevision,
	collisionCount int32, control ControlInterface) (reconcile.Result, error) {

	newStatus = r.calculateStatus(instance, newStatus, nameToPool, currentRevision, collisionCount, control)
	_, err := r.updateUnitedDeployment(instance, oldStatus, newStatus)

	return reconcile.Result{}, err
}

func (r *ReconcileUnitedDeployment) calculateStatus(instance *unitv1alpha1.UnitedDeployment, newStatus *unitv1alpha1.UnitedDeploymentStatus,
	nameToPool map[string]*Pool, currentRevision *appsv1.ControllerRevision,
	collisionCount int32, control ControlInterface) *unitv1alpha1.UnitedDeploymentStatus {

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
		RemoveUnitedDeploymentCondition(newStatus, unitv1alpha1.PoolFailure)
	} else {
		SetUnitedDeploymentCondition(newStatus, NewUnitedDeploymentCondition(unitv1alpha1.PoolFailure, corev1.ConditionTrue, "Error", *poolFailure))
	}

	return newStatus
}

func getPoolTemplateType(obj *unitv1alpha1.UnitedDeployment) (templateType unitv1alpha1.TemplateType) {
	template := obj.Spec.WorkloadTemplate
	switch {
	case template.StatefulSetTemplate != nil:
		templateType = unitv1alpha1.StatefulSetTemplateType
	case template.DeploymentTemplate != nil:
		templateType = unitv1alpha1.DeploymentTemplateType
	default:
		klog.Warning("UnitedDeployment.Spec.WorkloadTemplate exist wrong template")
	}
	return
}

func (r *ReconcileUnitedDeployment) updateUnitedDeployment(ud *unitv1alpha1.UnitedDeployment, oldStatus, newStatus *unitv1alpha1.UnitedDeploymentStatus) (*unitv1alpha1.UnitedDeployment, error) {
	if oldStatus.CurrentRevision == newStatus.CurrentRevision &&
		oldStatus.CollisionCount == newStatus.CollisionCount &&
		oldStatus.Replicas == newStatus.Replicas &&
		oldStatus.ReadyReplicas == newStatus.ReadyReplicas &&
		ud.Generation == newStatus.ObservedGeneration &&
		reflect.DeepEqual(oldStatus.PoolReplicas, newStatus.PoolReplicas) &&
		reflect.DeepEqual(oldStatus.Conditions, newStatus.Conditions) {
		return ud, nil
	}

	newStatus.ObservedGeneration = ud.Generation

	var getErr, updateErr error
	for i, obj := 0, ud; ; i++ {
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
		tmpObj := &unitv1alpha1.UnitedDeployment{}
		if getErr = r.Client.Get(context.TODO(), client.ObjectKey{Namespace: obj.Namespace, Name: obj.Name}, tmpObj); getErr != nil {
			return nil, getErr
		}
		obj = tmpObj
	}

	klog.Errorf("fail to update UnitedDeployment %s/%s status: %s", ud.Namespace, ud.Name, updateErr)
	return nil, updateErr
}
