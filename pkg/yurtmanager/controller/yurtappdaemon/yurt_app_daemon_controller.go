/*
Copyright 2023 The OpenYurt Authors.

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

package yurtappdaemon

import (
	"context"
	"fmt"
	"reflect"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	yurtClient "github.com/openyurtio/openyurt/cmd/yurt-manager/app/client"
	"github.com/openyurtio/openyurt/cmd/yurt-manager/app/config"
	"github.com/openyurtio/openyurt/cmd/yurt-manager/names"
	unitv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/yurtappdaemon/workloadcontroller"
)

var (
	controllerResource = unitv1alpha1.SchemeGroupVersion.WithResource("yurtappdaemons")
)

const (
	slowStartInitialBatchSize = 1

	eventTypeRevisionProvision  = "RevisionProvision"
	eventTypeTemplateController = "TemplateController"

	eventTypeWorkloadsCreated = "CreateWorkload"
	eventTypeWorkloadsUpdated = "UpdateWorkload"
	eventTypeWorkloadsDeleted = "DeleteWorkload"
)

func Format(format string, args ...interface{}) string {
	s := fmt.Sprintf(format, args...)
	return fmt.Sprintf("%s: %s", names.YurtAppDaemonController, s)
}

// Add creates a new YurtAppDaemon Controller and adds it to the Manager with default RBAC.
// The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(ctx context.Context, c *config.CompletedConfig, mgr manager.Manager) error {
	if _, err := mgr.GetRESTMapper().KindFor(controllerResource); err != nil {
		klog.Infof("resource %s doesn't exist", controllerResource.String())
		return err
	}

	klog.Infof("yurtappdaemon-controller add controller %s", controllerResource.String())
	return add(mgr, c, newReconciler(mgr))
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, cfg *config.CompletedConfig, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(names.YurtAppDaemonController, mgr, controller.Options{Reconciler: r, MaxConcurrentReconciles: int(cfg.Config.ComponentConfig.YurtAppDaemonController.ConcurrentYurtAppDaemonWorkers)})
	if err != nil {
		return err
	}

	// Watch for changes to YurtAppDaemon
	err = c.Watch(source.Kind[client.Object](mgr.GetCache(), &unitv1alpha1.YurtAppDaemon{}, &handler.EnqueueRequestForObject{}))
	if err != nil {
		return err
	}

	// Watch for changes to NodePool
	err = c.Watch(source.Kind[client.Object](mgr.GetCache(), &unitv1alpha1.NodePool{}, &EnqueueYurtAppDaemonForNodePool{client: yurtClient.GetClientByControllerNameOrDie(mgr, names.YurtAppDaemonController)}))
	if err != nil {
		return err
	}
	return nil
}

var _ reconcile.Reconciler = &ReconcileYurtAppDaemon{}

// ReconcileYurtAppDaemon reconciles a YurtAppDaemon object
type ReconcileYurtAppDaemon struct {
	client.Client
	scheme *runtime.Scheme

	recorder record.EventRecorder
	controls map[unitv1alpha1.TemplateType]workloadcontroller.WorkloadController
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileYurtAppDaemon{
		Client:   yurtClient.GetClientByControllerNameOrDie(mgr, names.YurtAppDaemonController),
		scheme:   mgr.GetScheme(),
		recorder: mgr.GetEventRecorderFor(names.YurtAppDaemonController),
		controls: map[unitv1alpha1.TemplateType]workloadcontroller.WorkloadController{
			//			unitv1alpha1.StatefulSetTemplateType: &StatefulSetController{Client: yurtClient.GetClientByControllerNameOrDie(mgr, names.YurtAppDaemonController), scheme: mgr.GetScheme()},
			unitv1alpha1.DeploymentTemplateType: &workloadcontroller.DeploymentController{Client: yurtClient.GetClientByControllerNameOrDie(mgr, names.YurtAppDaemonController), Scheme: mgr.GetScheme()},
		},
	}
}

// +kubebuilder:rbac:groups=apps.openyurt.io,resources=yurtappdaemons,verbs=get;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.openyurt.io,resources=yurtappdaemons/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=controllerrevisions,verbs=get;create;update;patch;delete

// Reconcile reads that state of the cluster for a YurtAppDaemon object and makes changes based on the state read
// and what is in the YurtAppDaemon.Spec
func (r *ReconcileYurtAppDaemon) Reconcile(_ context.Context, request reconcile.Request) (reconcile.Result, error) {
	klog.V(4).Infof("Reconcile YurtAppDaemon %s/%s", request.Namespace, request.Name)
	// Fetch the YurtAppDaemon instance
	instance := &unitv1alpha1.YurtAppDaemon{}
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

	currentRevision, updatedRevision, collisionCount, err := r.constructYurtAppDaemonRevisions(instance)
	if err != nil {
		klog.Errorf("could not construct controller revision of YurtAppDaemon %s/%s: %s", instance.Namespace, instance.Name, err)
		r.recorder.Event(instance.DeepCopy(), corev1.EventTypeWarning, fmt.Sprintf("Failed%s", eventTypeRevisionProvision), err.Error())
		return reconcile.Result{}, err
	}

	expectedRevision := currentRevision
	if updatedRevision != nil {
		expectedRevision = updatedRevision
	}

	klog.Infof("YurtAppDaemon [%s/%s] get expectRevision %v collisionCount %v", instance.GetNamespace(), instance.GetName(),
		expectedRevision.Name, collisionCount)

	control, templateType, err := r.getTemplateControls(instance)
	if err != nil {
		r.recorder.Event(instance.DeepCopy(), corev1.EventTypeWarning, fmt.Sprintf("Failed%s", eventTypeTemplateController), err.Error())
		return reconcile.Result{}, err
	}

	if control == nil {
		r.recorder.Event(instance.DeepCopy(), corev1.EventTypeWarning, fmt.Sprintf("YurtAppDaemon[%s/%s] could not get control", instance.Namespace, instance.Name), fmt.Sprintf("could not find control"))
		return reconcile.Result{}, fmt.Errorf("could not find control")
	}

	currentNPToWorkload, err := r.getNodePoolToWorkLoad(instance, control)
	if err != nil {
		klog.Errorf("YurtAppDaemon[%s/%s] could not get nodePoolWorkload, error: %s", instance.Namespace, instance.Name, err)
		return reconcile.Result{}, err
	}

	allNameToNodePools, err := r.getNameToNodePools(instance)
	if err != nil {
		klog.Errorf("YurtAppDaemon[%s/%s] could not get nameToNodePools, error: %s", instance.Namespace, instance.Name, err)
		return reconcile.Result{}, err
	}

	newStatus, err := r.manageWorkloads(instance, currentNPToWorkload, allNameToNodePools, expectedRevision.Name, templateType)
	if err != nil {
		return reconcile.Result{}, err
	}

	return r.updateStatus(instance, newStatus, oldStatus, expectedRevision, collisionCount, templateType, currentNPToWorkload)
}

func (r *ReconcileYurtAppDaemon) updateStatus(instance *unitv1alpha1.YurtAppDaemon, newStatus, oldStatus *unitv1alpha1.YurtAppDaemonStatus,
	currentRevision *appsv1.ControllerRevision, collisionCount int32, templateType unitv1alpha1.TemplateType, currentNodepoolToWorkload map[string]*workloadcontroller.Workload) (reconcile.Result, error) {

	newStatus = r.calculateStatus(instance, newStatus, currentRevision, collisionCount, templateType, currentNodepoolToWorkload)
	_, err := r.updateYurtAppDaemon(instance, oldStatus, newStatus)

	return reconcile.Result{}, err
}

func (r *ReconcileYurtAppDaemon) updateYurtAppDaemon(yad *unitv1alpha1.YurtAppDaemon, oldStatus, newStatus *unitv1alpha1.YurtAppDaemonStatus) (*unitv1alpha1.YurtAppDaemon, error) {
	if oldStatus.CurrentRevision == newStatus.CurrentRevision &&
		*oldStatus.CollisionCount == *newStatus.CollisionCount &&
		oldStatus.TemplateType == newStatus.TemplateType &&
		yad.Generation == newStatus.ObservedGeneration &&
		reflect.DeepEqual(oldStatus.NodePools, newStatus.NodePools) &&
		reflect.DeepEqual(oldStatus.Conditions, newStatus.Conditions) {
		klog.Infof("YurtAppDaemon[%s/%s] oldStatus==newStatus, no need to update status", yad.GetNamespace(), yad.GetName())
		return yad, nil
	}

	newStatus.ObservedGeneration = yad.Generation

	var getErr, updateErr error
	for i, obj := 0, yad; ; i++ {
		klog.V(4).Info(fmt.Sprintf("YurtAppDaemon[%s/%s] The %d th time updating status for %v[%s/%s], ",
			yad.GetNamespace(), yad.GetName(), i, obj.Kind, obj.Namespace, obj.Name) +
			fmt.Sprintf("sequence No: %v->%v", obj.Status.ObservedGeneration, newStatus.ObservedGeneration))

		obj.Status = *newStatus

		updateErr = r.Client.Status().Update(context.TODO(), obj)
		if updateErr == nil {
			return obj, nil
		}
		if i >= updateRetries {
			break
		}
		tmpObj := &unitv1alpha1.YurtAppDaemon{}
		if getErr = r.Client.Get(context.TODO(), client.ObjectKey{Namespace: obj.Namespace, Name: obj.Name}, tmpObj); getErr != nil {
			return nil, getErr
		}
		obj = tmpObj
	}

	klog.Errorf("could not update YurtAppDaemon %s/%s status: %s", yad.Namespace, yad.Name, updateErr)
	return nil, updateErr
}

func (r *ReconcileYurtAppDaemon) calculateStatus(instance *unitv1alpha1.YurtAppDaemon, newStatus *unitv1alpha1.YurtAppDaemonStatus,
	currentRevision *appsv1.ControllerRevision, collisionCount int32, templateType unitv1alpha1.TemplateType, currentNodepoolToWorkload map[string]*workloadcontroller.Workload) *unitv1alpha1.YurtAppDaemonStatus {

	newStatus.CollisionCount = &collisionCount

	var workloadFailure string
	overriderList := unitv1alpha1.YurtAppOverriderList{}
	if err := r.List(context.TODO(), &overriderList); err != nil {
		workloadFailure = fmt.Sprintf("unable to list yurtappoverrider: %v", err)
	}
	for _, overrider := range overriderList.Items {
		if overrider.Subject.Kind == "YurtAppDaemon" && overrider.Subject.Name == instance.Name {
			newStatus.OverriderRef = overrider.Name
			break
		}
	}

	newStatus.WorkloadSummaries = make([]unitv1alpha1.WorkloadSummary, 0)
	for _, workload := range currentNodepoolToWorkload {
		newStatus.WorkloadSummaries = append(newStatus.WorkloadSummaries, unitv1alpha1.WorkloadSummary{
			AvailableCondition: workload.Status.AvailableCondition,
			Replicas:           workload.Status.Replicas,
			ReadyReplicas:      workload.Status.ReadyReplicas,
			WorkloadName:       workload.Name,
		})
	}
	if newStatus.CurrentRevision == "" {
		// init with current revision
		newStatus.CurrentRevision = currentRevision.Name
	}
	newStatus.TemplateType = templateType

	if workloadFailure == "" {
		RemoveYurtAppDaemonCondition(newStatus, unitv1alpha1.WorkLoadFailure)
	} else {
		SetYurtAppDaemonCondition(newStatus, NewYurtAppDaemonCondition(unitv1alpha1.WorkLoadFailure, corev1.ConditionFalse, "Error", workloadFailure))
	}
	return newStatus
}

func (r *ReconcileYurtAppDaemon) manageWorkloads(instance *unitv1alpha1.YurtAppDaemon, currentNodepoolToWorkload map[string]*workloadcontroller.Workload,
	allNameToNodePools map[string]unitv1alpha1.NodePool, expectedRevision string, templateType unitv1alpha1.TemplateType) (newStatus *unitv1alpha1.YurtAppDaemonStatus, updateErr error) {

	newStatus = instance.Status.DeepCopy()

	nps := make([]string, 0, len(allNameToNodePools))
	for np := range allNameToNodePools {
		nps = append(nps, np)
	}
	newStatus.NodePools = nps

	needDeleted, needUpdate, needCreate := r.classifyWorkloads(instance, currentNodepoolToWorkload, allNameToNodePools, expectedRevision)
	provision, err := r.manageWorkloadsProvision(instance, allNameToNodePools, expectedRevision, templateType, needDeleted, needCreate)
	if err != nil {
		SetYurtAppDaemonCondition(newStatus, NewYurtAppDaemonCondition(unitv1alpha1.WorkLoadProvisioned, corev1.ConditionFalse, "Error", err.Error()))
		return newStatus, fmt.Errorf("could not manage workload provision: %v", err)
	}

	if provision {
		SetYurtAppDaemonCondition(newStatus, NewYurtAppDaemonCondition(unitv1alpha1.WorkLoadProvisioned, corev1.ConditionTrue, "", ""))
	}

	if len(needUpdate) > 0 {
		_, updateErr = util.SlowStartBatch(len(needUpdate), slowStartInitialBatchSize, func(index int) error {
			u := needUpdate[index]
			updateWorkloadErr := r.controls[templateType].UpdateWorkload(u, instance, allNameToNodePools[u.GetNodePoolName()], expectedRevision)
			if updateWorkloadErr != nil {
				r.recorder.Event(instance.DeepCopy(), corev1.EventTypeWarning, fmt.Sprintf("Failed %s", eventTypeWorkloadsUpdated),
					fmt.Sprintf("Error updating workload type(%s) %s when updating: %s", templateType, u.Name, updateWorkloadErr))
				klog.Errorf("YurtAppDaemon[%s/%s] update workload[%s/%s/%s] error %v", instance.GetNamespace(), instance.GetName(),
					templateType, u.Namespace, u.Name, err)
			}
			return updateWorkloadErr
		})
	}

	if updateErr == nil {
		SetYurtAppDaemonCondition(newStatus, NewYurtAppDaemonCondition(unitv1alpha1.WorkLoadUpdated, corev1.ConditionTrue, "", ""))
	} else {
		SetYurtAppDaemonCondition(newStatus, NewYurtAppDaemonCondition(unitv1alpha1.WorkLoadUpdated, corev1.ConditionFalse, "Error", updateErr.Error()))
	}

	return newStatus, updateErr
}

func (r *ReconcileYurtAppDaemon) manageWorkloadsProvision(instance *unitv1alpha1.YurtAppDaemon,
	allNameToNodePools map[string]unitv1alpha1.NodePool, expectedRevision string, templateType unitv1alpha1.TemplateType,
	needDeleted []*workloadcontroller.Workload, needCreate []string) (bool, error) {
	// 针对于Create 的 需要创建

	var errs []error
	if len(needCreate) > 0 {
		// do not consider deletion
		var createdNum int
		var createdErr error
		createdNum, createdErr = util.SlowStartBatch(len(needCreate), slowStartInitialBatchSize, func(idx int) error {
			nodepoolName := needCreate[idx]
			err := r.controls[templateType].CreateWorkload(instance, allNameToNodePools[nodepoolName], expectedRevision)
			//err := r.poolControls[workloadType].CreatePool(ud, poolName, revision, replicas)
			if err != nil {
				klog.Errorf("YurtAppDaemon[%s/%s] templatetype %s create workload by nodepool %s error: %s",
					instance.GetNamespace(), instance.GetName(), templateType, nodepoolName, err.Error())
				if !errors.IsTimeout(err) {
					return fmt.Errorf("YurtAppDaemon[%s/%s] templatetype %s create workload by nodepool %s error: %s",
						instance.GetNamespace(), instance.GetName(), templateType, nodepoolName, err.Error())
				}
			}
			klog.Infof("YurtAppDaemon[%s/%s] templatetype %s create workload by nodepool %s success",
				instance.GetNamespace(), instance.GetName(), templateType, nodepoolName)
			return nil
		})
		if createdErr == nil {
			r.recorder.Eventf(instance.DeepCopy(), corev1.EventTypeNormal, fmt.Sprintf("Successful %s", eventTypeWorkloadsCreated), "Create %d Workload type(%s)", createdNum, templateType)
		} else {
			errs = append(errs, createdErr)
		}
	}

	// manage deleting
	if len(needDeleted) > 0 {
		var deleteErrs []error
		// need deleted
		for _, d := range needDeleted {
			if err := r.controls[templateType].DeleteWorkload(instance, d); err != nil {
				deleteErrs = append(deleteErrs, fmt.Errorf("YurtAppDaemon[%s/%s] delete workload[%s/%s/%s] error %v",
					instance.GetNamespace(), instance.GetName(), templateType, d.Namespace, d.Name, err))
			}
		}
		if len(deleteErrs) > 0 {
			errs = append(errs, deleteErrs...)
		} else {
			r.recorder.Eventf(instance.DeepCopy(), corev1.EventTypeNormal, fmt.Sprintf("Successful %s", eventTypeWorkloadsDeleted), "Delete %d Workload type(%s)", len(needDeleted), templateType)
		}
	}

	return len(needCreate) > 0 || len(needDeleted) > 0, utilerrors.NewAggregate(errs)
}

func (r *ReconcileYurtAppDaemon) classifyWorkloads(instance *unitv1alpha1.YurtAppDaemon, currentNodepoolToWorkload map[string]*workloadcontroller.Workload,
	allNameToNodePools map[string]unitv1alpha1.NodePool, expectedRevision string) (needDeleted, needUpdate []*workloadcontroller.Workload, needCreate []string) {

	for npName, load := range currentNodepoolToWorkload {
		if np, ok := allNameToNodePools[npName]; ok {
			match := true
			// judge workload NodeSelector
			if !reflect.DeepEqual(load.GetNodeSelector(), workloadcontroller.CreateNodeSelectorByNodepoolName(npName)) {
				match = false
			}
			// judge workload whether toleration all taints
			if !IsTolerationsAllTaints(load.GetToleration(), np.Spec.Taints) {
				match = false
			}

			// judge revision
			if load.GetRevision() != expectedRevision {
				match = false
			}

			if !match {
				klog.V(4).Infof("YurtAppDaemon[%s/%s] need update [%s/%s/%s]", instance.GetNamespace(),
					instance.GetName(), load.GetKind(), load.Namespace, load.Name)
				needUpdate = append(needUpdate, load)
			}
		} else {
			needDeleted = append(needDeleted, load)
			klog.V(4).Infof("YurtAppDaemon[%s/%s] need delete [%s/%s/%s]", instance.GetNamespace(),
				instance.GetName(), load.GetKind(), load.Namespace, load.Name)
		}
	}

	for vnp := range allNameToNodePools {
		if _, ok := currentNodepoolToWorkload[vnp]; !ok {
			needCreate = append(needCreate, vnp)
			klog.V(4).Infof("YurtAppDaemon[%s/%s] need create new workload by nodepool %s", instance.GetNamespace(),
				instance.GetName(), vnp)
		}
	}

	return
}

func (r *ReconcileYurtAppDaemon) getNameToNodePools(instance *unitv1alpha1.YurtAppDaemon) (map[string]unitv1alpha1.NodePool, error) {
	klog.V(4).Infof("YurtAppDaemon [%s/%s] prepare to get associated nodepools",
		instance.Namespace, instance.Name)

	nodepoolSelector, err := metav1.LabelSelectorAsSelector(instance.Spec.NodePoolSelector)
	if err != nil {
		return nil, err
	}

	nodepools := unitv1alpha1.NodePoolList{}
	if err := r.Client.List(context.TODO(), &nodepools, &client.ListOptions{LabelSelector: nodepoolSelector}); err != nil {
		klog.Errorf("YurtAppDaemon [%s/%s] could not get NodePoolList", instance.GetNamespace(),
			instance.GetName())
		return nil, nil
	}

	indices := make(map[string]unitv1alpha1.NodePool)
	for i, v := range nodepools.Items {
		indices[v.GetName()] = v
		klog.V(4).Infof("YurtAppDaemon [%s/%s] get %d's associated nodepools %s",
			instance.Namespace, instance.Name, i, v.Name)

	}

	return indices, nil
}

func (r *ReconcileYurtAppDaemon) getTemplateControls(instance *unitv1alpha1.YurtAppDaemon) (workloadcontroller.WorkloadController,
	unitv1alpha1.TemplateType, error) {
	switch {
	case instance.Spec.WorkloadTemplate.StatefulSetTemplate != nil:
		return r.controls[unitv1alpha1.StatefulSetTemplateType], unitv1alpha1.StatefulSetTemplateType, nil
	case instance.Spec.WorkloadTemplate.DeploymentTemplate != nil:
		return r.controls[unitv1alpha1.DeploymentTemplateType], unitv1alpha1.DeploymentTemplateType, nil
	default:
		klog.Errorf("The appropriate WorkloadTemplate was not found")
		return nil, "", fmt.Errorf("The appropriate WorkloadTemplate was not found, Now Support(%s/%s)",
			unitv1alpha1.StatefulSetTemplateType, unitv1alpha1.DeploymentTemplateType)
	}
}

func (r *ReconcileYurtAppDaemon) getNodePoolToWorkLoad(instance *unitv1alpha1.YurtAppDaemon, c workloadcontroller.WorkloadController) (map[string]*workloadcontroller.Workload, error) {
	klog.V(4).Infof("YurtAppDaemon [%s/%s/%s] prepare to get all workload", c.GetTemplateType(), instance.Namespace, instance.Name)

	nodePoolsToWorkloads := make(map[string]*workloadcontroller.Workload)
	workloads, err := c.GetAllWorkloads(instance)
	if err != nil {
		klog.Errorf("Get all workloads for YurtAppDaemon[%s/%s] error %v", instance.GetNamespace(),
			instance.GetName(), err)
		return nil, err
	}
	// 获得workload 里对应的NodePool
	for i, w := range workloads {
		if w.GetNodePoolName() != "" {
			nodePoolsToWorkloads[w.GetNodePoolName()] = workloads[i]
			klog.V(4).Infof("YurtAppDaemon [%s/%s] get %d's workload[%s/%s/%s]",
				instance.Namespace, instance.Name, i, c.GetTemplateType(), w.Namespace, w.Name)
		} else {
			klog.Warningf("YurtAppDaemon [%s/%s] %d's workload[%s/%s/%s] has no nodepool annotation",
				instance.Namespace, instance.Name, i, c.GetTemplateType(), w.Namespace, w.Name)
		}
	}
	klog.V(4).Infof("YurtAppDaemon [%s/%s] get %d %s workloads",
		instance.Namespace, instance.Name, len(nodePoolsToWorkloads), c.GetTemplateType())
	return nodePoolsToWorkloads, nil
}
