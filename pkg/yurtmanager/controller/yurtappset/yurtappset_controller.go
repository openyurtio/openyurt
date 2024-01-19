/*
Copyright 2024 The OpenYurt Authors.
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
	"time"

	apps "k8s.io/api/apps/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
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

	"github.com/openyurtio/openyurt/cmd/yurt-manager/app/config"
	"github.com/openyurtio/openyurt/cmd/yurt-manager/names"
	unitv1beta1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/yurtappset/workloadmanager"
)

func init() {
	flag.IntVar(&concurrentReconciles, "yurtappset-workers", concurrentReconciles, "Max concurrent workers for YurtAppSet controller.")
}

var (
	concurrentReconciles = 3
	controllerResource   = unitv1beta1.SchemeGroupVersion.WithResource("yurtappsets")
)

const (
	eventTypeRevisionProvision = "RevisionProvision"
	eventTypeFindPools         = "FindPools"

	eventTypeWorkloadsCreated = "CreateWorkload"
	eventTypeWorkloadsUpdated = "UpdateWorkload"
	eventTypeWorkloadsDeleted = "DeleteWorkload"

	slowStartInitialBatchSize = 1
)

// Add creates a new YurtAppSet Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(ctx context.Context, c *config.CompletedConfig, mgr manager.Manager) error {
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
		workloadManagers: map[workloadmanager.TemplateType]workloadmanager.WorkloadManager{
			workloadmanager.DeploymentTemplateType: &workloadmanager.DeploymentManager{
				Client: mgr.GetClient(),
				Scheme: mgr.GetScheme(),
			},
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

	nodePoolPredicate := predicate.Funcs{
		CreateFunc: func(evt event.CreateEvent) bool {
			return true
		},
		DeleteFunc: func(evt event.DeleteEvent) bool {
			return true
		},
		UpdateFunc: func(evt event.UpdateEvent) bool {
			oldNodePool, ok := evt.ObjectOld.(*unitv1beta1.NodePool)
			if !ok {
				return false
			}
			newNodePool, ok := evt.ObjectNew.(*unitv1beta1.NodePool)
			if !ok {
				return false
			}
			// only enqueue if nodepool labels changed
			if !reflect.DeepEqual(oldNodePool.Labels, newNodePool.Labels) {
				return true
			}
			return false
		},
		GenericFunc: func(evt event.GenericEvent) bool {
			return false
		},
	}

	nodePoolToYurtAppSet := func(nodePool client.Object) (res []reconcile.Request) {
		res = make([]reconcile.Request, 0)
		yasList := &unitv1beta1.YurtAppSetList{}
		if err := mgr.GetClient().List(context.TODO(), yasList); err != nil {
			return
		}

		for _, yas := range yasList.Items {
			if workloadmanager.IsNodePoolRelatedToYurtAppSet(nodePool, &yas) {
				res = append(res, reconcile.Request{
					NamespacedName: types.NamespacedName{Name: yas.GetName(), Namespace: yas.GetNamespace()},
				})
			}
		}
		return
	}

	err = c.Watch(&source.Kind{Type: &unitv1beta1.NodePool{}}, handler.EnqueueRequestsFromMapFunc(nodePoolToYurtAppSet), nodePoolPredicate)
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &unitv1beta1.YurtAppSet{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &unitv1beta1.YurtAppSet{},
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

	recorder         record.EventRecorder
	workloadManagers map[workloadmanager.TemplateType]workloadmanager.WorkloadManager
}

// +kubebuilder:rbac:groups=apps.openyurt.io,resources=yurtappsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.openyurt.io,resources=nodepools,verbs=get;list;watch;
// +kubebuilder:rbac:groups=apps.openyurt.io,resources=yurtappsets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=statefulsets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=controllerrevisions,verbs=get;list;watch;create;update;patch;delete

// Reconcile reads that state of the cluster for a YurtAppSet object and makes changes based on the state read
// and what is in the YurtAppSet.Spec
func (r *ReconcileYurtAppSet) Reconcile(_ context.Context, request reconcile.Request) (res reconcile.Result, err error) {
	klog.V(2).Infof("Reconcile YurtAppSet %s/%s Start.", request.Namespace, request.Name)
	res = reconcile.Result{}

	// Get yas instance
	yas := &unitv1beta1.YurtAppSet{}
	err = r.Get(context.TODO(), client.ObjectKey{Namespace: request.Namespace, Name: request.Name}, yas)
	if err != nil {
		klog.Warningf("YurtAppSet %s/%s get fail, %v", request.Namespace, request.Name, err)
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	if yas.DeletionTimestamp != nil {
		return
	}

	// Get yas original status
	yasStatus := yas.Status.DeepCopy()

	// Get yas histories, create a new revision based on current spec
	allRevisions, expectedRevision, collisionCount, err := r.constructYurtAppSetRevisions(yas)
	yasStatus.CollisionCount = &collisionCount
	if err != nil {
		klog.Errorf("could not construct controller revision of YurtAppSet %s/%s: %s", yas.Namespace, yas.Name, err)
		r.recorder.Event(yas.DeepCopy(), corev1.EventTypeWarning, fmt.Sprintf("Failed%s", eventTypeRevisionProvision), err.Error())
		return
	} else if isRevisionInvalid(expectedRevision) {
		// if expectedRevision is invalid (current yas workload template is invalid), we should report error and not retry
		klog.Warningf("YurtAppSet[%s/%s] expectedRevision is invalid", yas.Namespace, yas.Name)
		return
	}

	// Conciliate workloads, udpate yas related workloads (deploy/sts)
	// this may infect yas appdispatched/appupdated/appdeleted condition
	expectedNps, curWorkloads, nErr := r.conciliateWorkloads(yas, expectedRevision, yasStatus)
	if nErr != nil {
		// if err, retry after 1s; err caused by invalid revision, no need to retry
		if !isRevisionInvalid(expectedRevision) {
			res.RequeueAfter = 1 * time.Second
		}
		klog.Warningf("YurtAppSet[%s/%s] conciliate workloads error: %v", yas.Namespace, yas.Name, nErr)
	}

	// Concilaiate yas, update yas status and clean yas related revisions
	if nErr := r.conciliateYurtAppSet(yas, curWorkloads, allRevisions, expectedRevision, expectedNps, yasStatus); nErr != nil {
		// if err, retry after 1s to wait for latest updates synced
		res.RequeueAfter = 1 * time.Second
		klog.Warningf("YurtAppSet[%s/%s] conciliate yurtappset error: %v", yas.GetNamespace(), yas.GetName(), nErr)
	}

	return
}

func (r *ReconcileYurtAppSet) getNodePoolsFromYurtAppSet(yas *unitv1beta1.YurtAppSet, newStatus *unitv1beta1.YurtAppSetStatus) (npNames sets.String, err error) {
	expectedNps, err := workloadmanager.GetNodePoolsFromYurtAppSet(r.Client, yas)
	if err != nil {
		return nil, err
	}
	if expectedNps.Len() == 0 {
		klog.V(4).Infof("No NodePools found for YurtAppSet %s/%s", yas.Namespace, yas.Name)
		r.recorder.Event(yas.DeepCopy(), corev1.EventTypeWarning, fmt.Sprintf("No%s", eventTypeFindPools), fmt.Sprintf("There are no matched nodepools for YurtAppSet %s/%s", yas.Namespace, yas.Name))
		SetYurtAppSetCondition(newStatus, NewYurtAppSetCondition(unitv1beta1.AppSetPoolFound, corev1.ConditionFalse, fmt.Sprintf("No%s", eventTypeFindPools), "There are no matched nodepools for YurtAppSet"))
	} else {
		klog.V(4).Infof("NodePools matched for YurtAppSet %s/%s: %v", yas.Namespace, yas.Name, expectedNps.List())
		SetYurtAppSetCondition(newStatus, NewYurtAppSetCondition(unitv1beta1.AppSetPoolFound, corev1.ConditionTrue, eventTypeFindPools, fmt.Sprintf("There are %d matched nodepools: %v", expectedNps.Len(), expectedNps.List())))
	}
	return expectedNps, nil
}

func (r *ReconcileYurtAppSet) getWorkloadManagerFromYurtAppSet(yas *unitv1beta1.YurtAppSet) (workloadmanager.WorkloadManager, error) {
	switch {
	case yas.Spec.Workload.WorkloadTemplate.StatefulSetTemplate != nil:
		return r.workloadManagers[workloadmanager.StatefulSetTemplateType], nil
	case yas.Spec.Workload.WorkloadTemplate.DeploymentTemplate != nil:
		return r.workloadManagers[workloadmanager.DeploymentTemplateType], nil
	default:
		klog.Errorf("Invalid WorkloadTemplate")
		return nil, fmt.Errorf("The appropriate WorkloadTemplate was not found, Now Support(%s/%s)",
			workloadmanager.StatefulSetTemplateType, workloadmanager.DeploymentTemplateType)
	}
}

func classifyWorkloads(yas *unitv1beta1.YurtAppSet, currentWorkloads []metav1.Object,
	expectedNodePools sets.String, expectedRevision string) (needDeleted, needUpdate []metav1.Object, needCreate []string) {

	// classify workloads by nodepool name
	nodePoolsToWorkloads := make(map[string]metav1.Object)
	for i, w := range currentWorkloads {
		if nodePool := workloadmanager.GetWorkloadRefNodePool(w); nodePool != "" {
			nodePoolsToWorkloads[nodePool] = currentWorkloads[i]
		} else {
			klog.Warningf("YurtAppSet [%s/%s] %d's workload[%s/%s] has no nodepool annotation",
				yas.GetNamespace(), yas.GetName(), i, w.GetNamespace(), w.GetName())
		}
	}
	klog.V(4).Infof("YurtAppSet [%s/%s] get %d workloads",
		yas.GetNamespace(), yas.GetName(), len(nodePoolsToWorkloads))

	for npName, load := range nodePoolsToWorkloads {
		if _, ok := expectedNodePools[npName]; ok {
			// workload already exist in expectedNp, check its revision is latest
			// if not, add workload to needUpdate list
			if curRevision := workloadmanager.GetWorkloadHash(load); curRevision != "" {
				if curRevision != expectedRevision {
					klog.V(4).Infof("YurtAppSet[%s/%s] need update [%s/%s]", yas.GetNamespace(),
						yas.GetName(), load.GetNamespace(), load.GetName())
					needUpdate = append(needUpdate, load)
				}
			} else {
				klog.Warningf("YurtAppSet[%s/%s] workload[%s/%s] has no revision", yas.GetNamespace(),
					yas.GetName(), load.GetNamespace(), load.GetName())
				needUpdate = append(needUpdate, load)
			}

		} else {
			// workload not exist in expectedNp, add workload to needDelete list
			needDeleted = append(needDeleted, load)
			klog.V(4).Infof("YurtAppSet[%s/%s] need delete [%s/%s]", yas.GetNamespace(),
				yas.GetName(), load.GetNamespace(), load.GetName())
		}
	}

	for np := range expectedNodePools {
		// expected np not exist in current np, add workload to needCreate list
		if _, ok := nodePoolsToWorkloads[np]; !ok && np != "" {
			needCreate = append(needCreate, np)
			klog.V(4).Infof("YurtAppSet[%s/%s] need create new workload by nodepool %s", yas.GetNamespace(),
				yas.GetName(), np)
		}
	}

	return
}

// Conciliate workloads as yas spec expect
func (r *ReconcileYurtAppSet) conciliateWorkloads(yas *unitv1beta1.YurtAppSet, expectedRevision *appsv1.ControllerRevision, newStatus *unitv1beta1.YurtAppSetStatus) (expectedNps sets.String, curWorkloads []metav1.Object, err error) {

	// Get yas selected NodePools
	// this may infect yas poolfound condition
	expectedNps, err = r.getNodePoolsFromYurtAppSet(yas, newStatus)
	if err != nil {
		klog.Errorf("could not get expected nodepools from YurtAppSet %s/%s: %s", yas.Namespace, yas.Name, err)
		return
	}

	// Get yas workloadManager
	workloadManager, err := r.getWorkloadManagerFromYurtAppSet(yas)
	if err != nil {
		return
	}

	// Get yas managed workloads
	curWorkloads, err = workloadManager.List(yas)
	if err != nil {
		klog.Errorf("could not get all workloads of YurtAppSet %s/%s: %s", yas.Namespace, yas.Name, err)
		return
	}

	var errs []error

	templateType := workloadManager.GetTemplateType()
	// Classify workloads into del/create/update 3 categories
	needDelWorkloads, needUpdateWorkloads, needCreateNodePools := classifyWorkloads(yas, curWorkloads, expectedNps, expectedRevision.GetName())

	// Manipulate resources
	// 1. create workloads
	if len(needCreateNodePools) > 0 {
		createdNum, createdErr := util.SlowStartBatch(len(needCreateNodePools), slowStartInitialBatchSize, func(idx int) error {
			nodepoolName := needCreateNodePools[idx]
			err := workloadManager.Create(yas, nodepoolName, expectedRevision.GetName())
			if err != nil {
				klog.Errorf("YurtAppSet[%s/%s] templatetype %s create workload by nodepool %s error: %s",
					yas.GetNamespace(), yas.GetName(), templateType, nodepoolName, err.Error())
				if !errors.IsTimeout(err) {
					if errors.IsInvalid(err) {
						// notify users provided workload template+tweak is not valid
						r.recorder.Event(yas.DeepCopy(), corev1.EventTypeWarning, fmt.Sprintf("Failed %s: invalid workload template in YurtAppSet", eventTypeWorkloadsCreated), err.Error())
						setRevisionInvalid(r.Client, expectedRevision)
					}
					return fmt.Errorf("YurtAppSet[%s/%s] templatetype %s create workload by nodepool %s error: %s",
						yas.GetNamespace(), yas.GetName(), templateType, nodepoolName, err.Error())
				}
			}
			klog.Infof("YurtAppSet[%s/%s] create workload %s[%s/%s] success",
				yas.GetNamespace(), yas.GetName(), templateType, nodepoolName)
			return nil
		})

		if createdErr == nil {
			r.recorder.Eventf(yas.DeepCopy(), corev1.EventTypeNormal, fmt.Sprintf("Successful %s", eventTypeWorkloadsCreated), "Create %d %s", createdNum, templateType)
			SetYurtAppSetCondition(newStatus, NewYurtAppSetCondition(unitv1beta1.AppSetAppDispatchced, corev1.ConditionTrue, "", "All expected workloads are created successfully"))
		} else {
			errs = append(errs, createdErr)
			SetYurtAppSetCondition(newStatus, NewYurtAppSetCondition(unitv1beta1.AppSetAppDispatchced, corev1.ConditionFalse, "CreateWorkloadError", createdErr.Error()))
		}
	}

	// 2. delete workloads
	if len(needDelWorkloads) > 0 {
		delNum, delErr := util.SlowStartBatch(len(needDelWorkloads), slowStartInitialBatchSize, func(idx int) error {
			workloadTobeDeleted := needDelWorkloads[idx]
			err := workloadManager.Delete(yas, workloadTobeDeleted)
			if err != nil {
				klog.Errorf("YurtAppSet[%s/%s] delete %s[%s/%s] error: %s",
					yas.GetNamespace(), yas.GetName(), templateType, workloadTobeDeleted.GetNamespace(), workloadTobeDeleted.GetName(), err.Error())
				if !errors.IsTimeout(err) {
					return fmt.Errorf("YurtAppSet[%s/%s] delete %s[%s/%s] error: %s",
						yas.GetNamespace(), yas.GetName(), templateType, workloadTobeDeleted.GetNamespace(), workloadTobeDeleted.GetName(), err.Error())
				}
			}
			klog.Infof("YurtAppSet[%s/%s] templatetype delete %s[%s/%s] success",
				yas.GetNamespace(), yas.GetName(), templateType, workloadTobeDeleted.GetNamespace(), workloadTobeDeleted.GetName())
			return nil
		})

		if delErr == nil {
			r.recorder.Eventf(yas.DeepCopy(), corev1.EventTypeNormal, fmt.Sprintf("Successful %s", eventTypeWorkloadsDeleted), "Delete %d %s", delNum, templateType)
			SetYurtAppSetCondition(newStatus, NewYurtAppSetCondition(unitv1beta1.AppSetAppDeleted, corev1.ConditionTrue, "", "Unexpected workloads are deleted successfully"))
		} else {
			errs = append(errs, delErr)
			SetYurtAppSetCondition(newStatus, NewYurtAppSetCondition(unitv1beta1.AppSetAppDeleted, corev1.ConditionFalse, "DeleteWorkloadError", delErr.Error()))
		}
	}

	// 3. update workloads
	if len(needUpdateWorkloads) > 0 {
		updatedNum, updateErr := util.SlowStartBatch(len(needUpdateWorkloads), slowStartInitialBatchSize, func(index int) error {
			workloadTobeUpdated := needUpdateWorkloads[index]
			err := workloadManager.Update(yas, workloadTobeUpdated, workloadmanager.GetWorkloadRefNodePool(workloadTobeUpdated), expectedRevision.GetName())
			if err != nil {
				if errors.IsInvalid(err) {
					// notify users provided workload template+tweak is not valid
					r.recorder.Event(yas.DeepCopy(), corev1.EventTypeWarning, fmt.Sprintf("Failed %s: invalid workload template in YurtAppSet", eventTypeWorkloadsUpdated), err.Error())
					setRevisionInvalid(r.Client, expectedRevision)
				}
				r.recorder.Event(yas.DeepCopy(), corev1.EventTypeWarning, fmt.Sprintf("Failed %s", eventTypeWorkloadsUpdated),
					fmt.Sprintf("Error updating %s %s when updating: %s", templateType, workloadTobeUpdated.GetName(), err))
				klog.Errorf("YurtAppSet[%s/%s] update workload[%s/%s/%s] error %v", yas.GetNamespace(), yas.GetName(),
					templateType, workloadTobeUpdated.GetNamespace(), workloadTobeUpdated.GetName(), err)
			}
			klog.Infof("YurtAppSet[%s/%s] templatetype %s update workload by nodepool %s success",
				yas.GetNamespace(), yas.GetName(), templateType, workloadmanager.GetWorkloadRefNodePool(workloadTobeUpdated))
			return err
		})

		if updateErr == nil {
			r.recorder.Eventf(yas.DeepCopy(), corev1.EventTypeNormal, fmt.Sprintf("Successful %s", eventTypeWorkloadsUpdated), "Update %d %s", updatedNum, templateType)
			SetYurtAppSetCondition(newStatus, NewYurtAppSetCondition(unitv1beta1.AppSetAppUpdated, corev1.ConditionTrue, "", "All expected workloads are updated successfully"))
		} else {
			errs = append(errs, updateErr)
			SetYurtAppSetCondition(newStatus, NewYurtAppSetCondition(unitv1beta1.AppSetAppUpdated, corev1.ConditionFalse, "UpdateWorkloadError", updateErr.Error()))
		}
	}

	err = utilerrors.NewAggregate(errs)
	return
}

func (r *ReconcileYurtAppSet) conciliateYurtAppSet(yas *unitv1beta1.YurtAppSet, curWorkloads []metav1.Object, allRevisions []*apps.ControllerRevision, expecteddRevison *appsv1.ControllerRevision, expectedNps sets.String, newStatus *unitv1beta1.YurtAppSetStatus) error {
	if err := r.conciliateYurtAppSetStatus(yas, curWorkloads, expecteddRevison, expectedNps, newStatus); err != nil {
		return err
	}
	if isRevisionInvalid(expecteddRevison) {
		// donot clean revisions, when it is invalid, because it will trigger a new reconcile which will lead to endless loop
		return nil
	}
	return cleanRevisions(r.Client, yas, allRevisions)
}

// update yas status and clean unused revisions
func (r *ReconcileYurtAppSet) conciliateYurtAppSetStatus(yas *unitv1beta1.YurtAppSet, curWorkloads []metav1.Object, expecteddRevison *appsv1.ControllerRevision, expectedNps sets.String, newStatus *unitv1beta1.YurtAppSetStatus) error {

	// calculate yas current status
	readyWorkloads, updatedWorkloads := 0, 0
	for _, workload := range curWorkloads {
		workloadObj := workload.(*appsv1.Deployment)
		if workloadObj.Status.ReadyReplicas == workloadObj.Status.Replicas {
			readyWorkloads++
		}
		if workloadmanager.GetWorkloadHash(workloadObj) == expecteddRevison.GetName() && workloadObj.Status.UpdatedReplicas == workloadObj.Status.Replicas {
			updatedWorkloads++
		}
	}

	newStatus.ReadyWorkloads = int32(readyWorkloads)
	newStatus.TotalWorkloads = int32(len(curWorkloads))
	newStatus.UpdatedWorkloads = int32(updatedWorkloads)

	if isRevisionInvalid(expecteddRevison) {
		klog.Infof("YurtAppSet[%s/%s] expected revision is invalid, no need to update revision", yas.GetNamespace(), yas.GetName())
	} else {
		newStatus.CurrentRevision = expecteddRevison.GetName()
	}

	if newStatus.TotalWorkloads == 0 {
		SetYurtAppSetCondition(newStatus, NewYurtAppSetCondition(unitv1beta1.AppSetAppReady, corev1.ConditionFalse, "NoWorkloadFound", ""))
	} else if newStatus.TotalWorkloads == newStatus.ReadyWorkloads {
		SetYurtAppSetCondition(newStatus, NewYurtAppSetCondition(unitv1beta1.AppSetAppReady, corev1.ConditionTrue, "AllWorkloadsReady", ""))
	} else {
		SetYurtAppSetCondition(newStatus, NewYurtAppSetCondition(unitv1beta1.AppSetAppReady, corev1.ConditionFalse, "NotAllWorkloadsReady", ""))
	}

	// update yas status
	oldStatus := yas.Status
	if oldStatus.CurrentRevision == newStatus.CurrentRevision &&
		oldStatus.CollisionCount != nil && newStatus.CollisionCount != nil && *oldStatus.CollisionCount == *newStatus.CollisionCount &&
		oldStatus.TotalWorkloads == newStatus.TotalWorkloads &&
		oldStatus.ReadyWorkloads == newStatus.ReadyWorkloads &&
		oldStatus.UpdatedWorkloads == newStatus.UpdatedWorkloads &&
		yas.Generation == newStatus.ObservedGeneration &&
		reflect.DeepEqual(oldStatus.Conditions, newStatus.Conditions) {
		klog.Infof("YurtAppSet[%s/%s] oldStatus==newStatus, no need to update status", yas.GetNamespace(), yas.GetName())
		return nil
	} else {
		klog.V(5).Infof("YurtAppSet[%s/%s] oldStatus=%+v, newStatus=%+v, need to update status", yas.GetNamespace(), yas.GetName(), oldStatus, newStatus)
	}

	newStatus.ObservedGeneration = yas.Generation

	yas.Status = *newStatus
	if err := r.Client.Status().Update(context.TODO(), yas); err != nil {
		return err
	}
	klog.Infof("YurtAppSet[%s/%s] update status success.", yas.Namespace, yas.Name)

	return nil
}
