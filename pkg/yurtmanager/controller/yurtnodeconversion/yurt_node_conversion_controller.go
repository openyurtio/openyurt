/*
Copyright 2026 The OpenYurt Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package yurtnodeconversion

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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
	nodeservant "github.com/openyurtio/openyurt/pkg/node-servant"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
	nodeutil "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/node"
	conversionconfig "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/yurtnodeconversion/config"
)

const (
	conversionConditionType corev1.NodeConditionType = "YurtNodeConversionFailed"

	actionConvert = "convert"
	actionRevert  = "revert"
	actionNone    = ""

	reasonConverting    = "Converting"
	reasonConverted     = "Converted"
	reasonReverting     = "Reverting"
	reasonReverted      = "Reverted"
	reasonConvertFailed = "ConvertFailed"
	reasonRevertFailed  = "RevertFailed"
)

type ReconcileYurtNodeConversion struct {
	client.Client
	cfg              conversionconfig.YurtNodeConversionControllerConfiguration
	nodeServantImage string
	jobNamespace     string
}

var _ reconcile.Reconciler = &ReconcileYurtNodeConversion{}

func Format(format string, args ...interface{}) string {
	s := fmt.Sprintf(format, args...)
	return fmt.Sprintf("%s: %s", names.YurtNodeConversionController, s)
}

// Add wires the node conversion controller into yurt-manager and watches
// both Node label transitions and node-servant Job status updates
func Add(ctx context.Context, c *appconfig.CompletedConfig, mgr manager.Manager) error {
	if len(c.ComponentConfig.YurtStaticSetController.UpgradeWorkerImage) == 0 {
		return fmt.Errorf("node-servant-image is empty")
	}

	r := &ReconcileYurtNodeConversion{
		Client:           yurtClient.GetClientByControllerNameOrDie(mgr, names.YurtNodeConversionController),
		cfg:              c.ComponentConfig.YurtNodeConversionController,
		nodeServantImage: c.ComponentConfig.YurtStaticSetController.UpgradeWorkerImage,
		jobNamespace:     c.ComponentConfig.Generic.WorkingNamespace,
	}

	ctrl, err := controller.New(names.YurtNodeConversionController, mgr, controller.Options{
		Reconciler:              r,
		MaxConcurrentReconciles: int(r.cfg.ConcurrentYurtNodeConversionWorkers),
	})
	if err != nil {
		return err
	}

	if err := ctrl.Watch(
		source.Kind[client.Object](mgr.GetCache(), &corev1.Node{}, &handler.EnqueueRequestForObject{}, nodePoolLabelPredicate()),
	); err != nil {
		return err
	}

	if err := ctrl.Watch(
		source.Kind[client.Object](mgr.GetCache(), &batchv1.Job{}, handler.EnqueueRequestsFromMapFunc(r.mapJobToNode), conversionJobPredicate()),
	); err != nil {
		return err
	}

	return nil
}

// +kubebuilder:rbac:groups=core,resources=nodes,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=nodes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;create;update;patch;delete

func (r *ReconcileYurtNodeConversion) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	klog.Info(Format("reconcile node %s", req.Name))

	// Stage 1: load the latest Node snapshot and stop early for deleting nodes.
	node := &corev1.Node{}
	if err := r.Get(ctx, req.NamespacedName, node); err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}
	if node.DeletionTimestamp != nil {
		klog.V(4).Info(Format("skip deleting node %s", node.Name))
		return reconcile.Result{}, nil
	}

	job, err := r.getConversionJob(ctx, node.Name)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("get conversion job for node %s: %w", node.Name, err)
	}

	// The controller keeps three pieces of state in sync:
	// 1. desiredAction from labels
	// 2. current round from the fixed-name Job
	// 3. observed progress from the conversion condition
	desiredAction, nodePoolName := desiredActionFromNode(node)
	cond := getConversionCondition(node)
	currentJobAction, err := jobAction(job)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("determine action from conversion job for node %s: %w", node.Name, err)
	}
	logReconcileSnapshot(node, desiredAction, nodePoolName, cond, job, currentJobAction)

	// Stage 2: if a Job already exists, it owns the current round.
	// Reconcile only updates control-plane state around that Job instead of
	// starting a new round in parallel
	if job != nil {
		return r.reconcileExistingJob(ctx, node, job, desiredAction, currentJobAction)
	}

	// Stage 3: no Job exists. fallback: handles cases where host-level work succeeded
	// but the controller's finalization was interrupted after the Job's disappearance
	if action := interruptedFinalizationAction(node, cond); action != actionNone {
		klog.V(4).Info(Format("node(%s) resumes interrupted %s finalization without an active job", node.Name, action))
		return reconcile.Result{}, r.handleSuccessfulAction(ctx, node.Name, action)
	}

	if desiredAction == actionNone {
		klog.V(5).Info(Format("node(%s) already stable, nothing to do", node.Name))
		return reconcile.Result{}, nil
	}

	// Stage 4: start a new convert/revert round from labels
	return r.startNewConversionRound(ctx, node.Name, nodePoolName, desiredAction)
}

func (r *ReconcileYurtNodeConversion) reconcileExistingJob(
	ctx context.Context, node *corev1.Node, job *batchv1.Job,
	desiredAction, currentJobAction string,
) (reconcile.Result, error) {
	if isStaleJobForAction(currentJobAction, desiredAction) {
		return r.handleStaleJob(ctx, node, job, desiredAction, currentJobAction)
	}

	// Job failure terminates the current round and keeps the node cordoned
	if isJobFailed(job) {
		klog.Info(Format("node(%s) observed failed %s job %s", node.Name, currentJobAction, job.Name))
		if err := r.ensureNodeUnschedulable(ctx, node.Name, true); err != nil {
			return reconcile.Result{}, fmt.Errorf("keep node %s cordoned after failed %s job: %w", node.Name, currentJobAction, err)
		}
		if err := r.ensureNodeConversionCondition(ctx, node.Name,
			newConversionCondition(currentJobAction, failedReasonForAction(currentJobAction), failedMessage(job, currentJobAction))); err != nil {
			return reconcile.Result{}, fmt.Errorf("set failed conversion condition for node %s: %w", node.Name, err)
		}
		return reconcile.Result{}, nil
	}

	if isJobSucceeded(job) {
		klog.Info(Format("node(%s) observed succeeded %s job %s", node.Name, currentJobAction, job.Name))
		return reconcile.Result{}, r.handleSuccessfulAction(ctx, node.Name, currentJobAction)
	}

	// Job is still running — keep the node cordoned and update the in-progress condition
	klog.V(4).Info(Format("node(%s) keeps running %s job %s in progress", node.Name, currentJobAction, job.Name))
	if err := r.ensureNodeUnschedulable(ctx, node.Name, true); err != nil {
		return reconcile.Result{}, fmt.Errorf("cordon node %s while %s job is running: %w", node.Name, currentJobAction, err)
	}
	if err := r.ensureNodeConversionCondition(ctx, node.Name,
		newConversionCondition(currentJobAction, conditionReasonForInProgress(currentJobAction), inProgressMessage(node.Name, currentJobAction))); err != nil {
		return reconcile.Result{}, fmt.Errorf("set in-progress conversion condition for node %s: %w", node.Name, err)
	}
	return reconcile.Result{}, nil
}

// handleStaleJob handles a Job that belongs to the previous conversion direction
func (r *ReconcileYurtNodeConversion) handleStaleJob(
	ctx context.Context, node *corev1.Node, job *batchv1.Job,
	desiredAction, currentJobAction string,
) (reconcile.Result, error) {
	if isJobFinished(job) {
		klog.V(4).Info(Format("node(%s) deleting stale job %s, desiredAction=%s currentJobAction=%s",
			node.Name, job.Name, desiredAction, currentJobAction))
		if err := r.Delete(ctx, job); err != nil && !apierrors.IsNotFound(err) {
			return reconcile.Result{}, fmt.Errorf("delete stale conversion job %s for node %s: %w", job.Name, node.Name, err)
		}
		return reconcile.Result{Requeue: true}, nil
	}
	klog.V(4).Info(Format("node(%s) keeps stale in-flight job %s until it finishes, desiredAction=%s currentJobAction=%s",
		node.Name, job.Name, desiredAction, currentJobAction))
	return reconcile.Result{}, nil
}

// startNewConversionRound cordons the node, sets the in-progress condition and creates a new conversion Job
func (r *ReconcileYurtNodeConversion) startNewConversionRound(
	ctx context.Context, nodeName, nodePoolName, desiredAction string,
) (reconcile.Result, error) {
	if err := r.ensureNodeUnschedulable(ctx, nodeName, true); err != nil {
		return reconcile.Result{}, fmt.Errorf("cordon node %s before starting %s: %w", nodeName, desiredAction, err)
	}
	if err := r.ensureNodeConversionCondition(ctx, nodeName,
		newConversionCondition(desiredAction, conditionReasonForInProgress(desiredAction), inProgressMessage(nodeName, desiredAction))); err != nil {
		return reconcile.Result{}, fmt.Errorf("set in-progress conversion condition for node %s before starting %s: %w", nodeName, desiredAction, err)
	}
	klog.Info(Format("node(%s) starts %s round", nodeName, desiredAction))
	if err := r.createConversionJob(ctx, nodeName, nodePoolName, desiredAction); err != nil {
		if apierrors.IsAlreadyExists(err) {
			klog.V(4).Info(Format("node(%s) conversion job already exists while starting %s", nodeName, desiredAction))
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("create %s job for node %s: %w", desiredAction, nodeName, err)
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileYurtNodeConversion) mapJobToNode(_ context.Context, obj client.Object) []reconcile.Request {
	job, ok := obj.(*batchv1.Job)
	if !ok {
		return []reconcile.Request{}
	}
	nodeName := job.Labels[nodeservant.ConversionNodeLabelKey]
	if len(nodeName) == 0 {
		return []reconcile.Request{}
	}

	return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: nodeName}}}
}

func (r *ReconcileYurtNodeConversion) getConversionJob(ctx context.Context, nodeName string) (*batchv1.Job, error) {
	job := &batchv1.Job{}
	err := r.Get(ctx, types.NamespacedName{
		Namespace: r.conversionJobNamespace(),
		Name:      conversionJobName(nodeName),
	}, job)
	if apierrors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return job, nil
}

// createConversionJob renders the node-servant Job for one conversion round
func (r *ReconcileYurtNodeConversion) createConversionJob(ctx context.Context, nodeName, nodePoolName, action string) error {
	if err := r.validateConversionJobRequest(nodeName, nodePoolName, action); err != nil {
		return err
	}

	renderCtx := map[string]string{
		"jobNamespace":     r.conversionJobNamespace(),
		"nodeServantImage": r.nodeServantImage,
	}
	if action == actionConvert {
		renderCtx["nodePoolName"] = nodePoolName
	}

	job, err := nodeservant.RenderNodeServantJob(action, renderCtx, nodeName)
	if err != nil {
		return err
	}
	klog.Info(Format("create %s job %s for node %s", action, job.Name, nodeName))
	return r.Create(ctx, job)
}

// handleSuccessfulAction completes the control-plane side of a successful round
// after node-servant already finished the host-level operations on the node
func (r *ReconcileYurtNodeConversion) handleSuccessfulAction(ctx context.Context, nodeName, action string) error {
	klog.V(4).Info(Format("finalize successful %s round for node(%s)", action, nodeName))
	node := &corev1.Node{}
	if err := r.Get(ctx, types.NamespacedName{Name: nodeName}, node); err != nil {
		return err
	}

	if err := r.ensureEdgeWorkerLabel(ctx, nodeName, action == actionConvert); err != nil {
		return err
	}

	if !hasSucceededConditionForAction(getConversionCondition(node), action) {
		if err := r.ensureNodeConversionCondition(ctx, nodeName,
			newConversionCondition(action, succeededReasonForAction(action), succeededMessage(action))); err != nil {
			return err
		}
	}

	return r.ensureNodeUnschedulable(ctx, nodeName, false)
}

// ensureNodeUnschedulable keeps the node cordon state aligned with the current
// conversion phase and is intentionally idempotent
func (r *ReconcileYurtNodeConversion) ensureNodeUnschedulable(ctx context.Context, nodeName string, unschedulable bool) error {
	node := &corev1.Node{}
	if err := r.Get(ctx, types.NamespacedName{Name: nodeName}, node); err != nil {
		return err
	}
	if node.Spec.Unschedulable == unschedulable {
		return nil
	}

	updated := node.DeepCopy()
	updated.Spec.Unschedulable = unschedulable
	klog.V(4).Info(Format("patch node(%s) unschedulable=%t", nodeName, unschedulable))
	return r.Patch(ctx, updated, client.MergeFrom(node))
}

// ensureEdgeWorkerLabel applies the controller-managed source-of-truth label
// for the terminal edge/non-edge state and leaves unrelated labels untouched
func (r *ReconcileYurtNodeConversion) ensureEdgeWorkerLabel(ctx context.Context, nodeName string, enabled bool) error {
	node := &corev1.Node{}
	if err := r.Get(ctx, types.NamespacedName{Name: nodeName}, node); err != nil {
		return err
	}

	updated := node.DeepCopy()
	if updated.Labels == nil {
		updated.Labels = map[string]string{}
	}

	if enabled {
		updated.Labels[projectinfo.GetEdgeWorkerLabelKey()] = "true"
	} else {
		delete(updated.Labels, projectinfo.GetEdgeWorkerLabelKey())
	}

	if reflect.DeepEqual(node.Labels, updated.Labels) {
		return nil
	}
	klog.V(4).Info(Format("patch node(%s) %s=%t", nodeName, projectinfo.GetEdgeWorkerLabelKey(), enabled))
	return r.Patch(ctx, updated, client.MergeFrom(node))
}

// ensureNodeConversionCondition upserts the single conversion condition that
// reports round progress and terminal outcome back to users and controllers
func (r *ReconcileYurtNodeConversion) ensureNodeConversionCondition(ctx context.Context, nodeName string, cond corev1.NodeCondition) error {
	node := &corev1.Node{}
	if err := r.Get(ctx, types.NamespacedName{Name: nodeName}, node); err != nil {
		return err
	}
	if !setConversionCondition(node, cond) {
		return nil
	}
	klog.V(4).Info(Format("update node(%s) conversion condition to status=%s reason=%s", nodeName, cond.Status, cond.Reason))
	return r.Status().Update(ctx, node)
}

// desiredActionFromNode derives the next conversion direction from Node labels
func desiredActionFromNode(node *corev1.Node) (string, string) {
	nodePoolName := node.Labels[projectinfo.GetNodePoolLabel()]
	isEdgeWorker := node.Labels[projectinfo.GetEdgeWorkerLabelKey()] == "true"

	switch {
	case len(nodePoolName) != 0 && !isEdgeWorker:
		return actionConvert, nodePoolName
	case len(nodePoolName) == 0 && isEdgeWorker:
		return actionRevert, ""
	default:
		return actionNone, nodePoolName
	}
}

// interruptedFinalizationAction decides whether a no-Job node still belongs to
// an interrupted conversion/revert round that only needs controller-side finalization
func interruptedFinalizationAction(node *corev1.Node, cond *corev1.NodeCondition) string {
	action := conditionAction(cond)
	if !isInProgressConditionForAction(cond, action) {
		return actionNone
	}
	if !isHostLevelActionApplied(node, action) {
		return actionNone
	}
	if isStableForAction(node, action) {
		return actionNone
	}

	return action
}

func isHostLevelActionApplied(node *corev1.Node, action string) bool {
	switch action {
	case actionConvert:
		return hasNodePoolLabel(node) && isEdgeWorkerNode(node)
	case actionRevert:
		return !hasNodePoolLabel(node) && !isEdgeWorkerNode(node)
	default:
		return false
	}
}

func isStableForAction(node *corev1.Node, action string) bool {
	switch action {
	case actionConvert:
		return isConvertedStable(node)
	case actionRevert:
		return isRevertedStable(node)
	default:
		return false
	}
}

func validateConversionJobRequest(nodeName, nodePoolName, action string, nodeServantImage string) error {
	if len(strings.TrimSpace(nodeName)) == 0 {
		return fmt.Errorf("node name is empty")
	}
	if len(strings.TrimSpace(nodeServantImage)) == 0 {
		return fmt.Errorf("node-servant image is empty")
	}

	switch action {
	case actionConvert:
		if len(strings.TrimSpace(nodePoolName)) == 0 {
			return fmt.Errorf("nodepool name is empty for convert job")
		}
		return nil
	case actionRevert:
		return nil
	default:
		return fmt.Errorf("unsupported conversion action %q", action)
	}
}

func (r *ReconcileYurtNodeConversion) validateConversionJobRequest(nodeName, nodePoolName, action string) error {
	return validateConversionJobRequest(nodeName, nodePoolName, action, r.nodeServantImage)
}

func (r *ReconcileYurtNodeConversion) conversionJobNamespace() string {
	if ns := strings.TrimSpace(r.jobNamespace); ns != "" {
		return ns
	}
	return nodeservant.DefaultConversionJobNamespace
}

func isConvertedStable(node *corev1.Node) bool {
	cond := getConversionCondition(node)
	return len(node.Labels[projectinfo.GetNodePoolLabel()]) != 0 &&
		node.Labels[projectinfo.GetEdgeWorkerLabelKey()] == "true" &&
		!node.Spec.Unschedulable &&
		cond != nil &&
		cond.Status == corev1.ConditionFalse &&
		cond.Reason == reasonConverted
}

func isRevertedStable(node *corev1.Node) bool {
	cond := getConversionCondition(node)
	return len(node.Labels[projectinfo.GetNodePoolLabel()]) == 0 &&
		node.Labels[projectinfo.GetEdgeWorkerLabelKey()] != "true" &&
		!node.Spec.Unschedulable &&
		cond != nil &&
		cond.Status == corev1.ConditionFalse &&
		cond.Reason == reasonReverted
}

func getConversionCondition(node *corev1.Node) *corev1.NodeCondition {
	_, cond := nodeutil.GetNodeCondition(&node.Status, conversionConditionType)
	return cond
}

// setConversionCondition upserts the single conversion condition and preserves
// LastTransitionTime when status stays the same but reason/message changes
func setConversionCondition(node *corev1.Node, cond corev1.NodeCondition) bool {
	idx, current := nodeutil.GetNodeCondition(&node.Status, cond.Type)
	if current == nil {
		node.Status.Conditions = append(node.Status.Conditions, cond)
		return true
	}
	if current.Status == cond.Status && current.Reason == cond.Reason && current.Message == cond.Message {
		return false
	}

	if current.Status == cond.Status {
		cond.LastTransitionTime = current.LastTransitionTime
	}
	node.Status.Conditions[idx] = cond
	return true
}

// newConversionCondition builds the canonical condition payload for each phase
func newConversionCondition(action, reason, message string) corev1.NodeCondition {
	now := metav1.Now()
	status := corev1.ConditionFalse
	if reason == reasonConvertFailed || reason == reasonRevertFailed {
		status = corev1.ConditionTrue
	}

	return corev1.NodeCondition{
		Type:               conversionConditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastHeartbeatTime:  now,
		LastTransitionTime: now,
	}
}

// conditionAction maps a condition reason back to the convert/revert direction
func conditionAction(cond *corev1.NodeCondition) string {
	if cond == nil {
		return actionNone
	}

	switch cond.Reason {
	case reasonConverting, reasonConverted, reasonConvertFailed:
		return actionConvert
	case reasonReverting, reasonReverted, reasonRevertFailed:
		return actionRevert
	default:
		return actionNone
	}
}

func isInProgressConditionForAction(cond *corev1.NodeCondition, action string) bool {
	if cond == nil || cond.Status != corev1.ConditionFalse {
		return false
	}

	switch action {
	case actionConvert:
		return cond.Reason == reasonConverting
	case actionRevert:
		return cond.Reason == reasonReverting
	default:
		return false
	}
}

func hasSucceededConditionForAction(cond *corev1.NodeCondition, action string) bool {
	if cond == nil || cond.Status != corev1.ConditionFalse {
		return false
	}

	switch action {
	case actionConvert:
		return cond.Reason == reasonConverted
	case actionRevert:
		return cond.Reason == reasonReverted
	default:
		return false
	}
}

// jobAction extracts the direction from the unified node-servant Job command
func jobAction(job *batchv1.Job) (string, error) {
	if job == nil {
		return actionNone, nil
	}
	containers := job.Spec.Template.Spec.Containers
	if len(containers) == 0 || len(containers[0].Args) == 0 {
		return actionNone, fmt.Errorf("conversion job %s has empty command args", conversionJobNameFromJob(job))
	}

	command := strings.TrimSpace(containers[0].Args[0])
	switch {
	case strings.HasPrefix(command, "/usr/local/bin/entry.sh "+actionConvert):
		return actionConvert, nil
	case strings.HasPrefix(command, "/usr/local/bin/entry.sh "+actionRevert):
		return actionRevert, nil
	default:
		return actionNone, fmt.Errorf("conversion job %s has unsupported command %q", conversionJobNameFromJob(job), command)
	}
}

// isStaleJobForAction detects that the reused Job still belongs to the previous
// conversion direction while labels now require the opposite round
func isStaleJobForAction(jobAction, desiredAction string) bool {
	return jobAction != actionNone && desiredAction != actionNone && jobAction != desiredAction
}

func isJobFinished(job *batchv1.Job) bool {
	return isJobSucceeded(job) || isJobFailed(job)
}

func isJobSucceeded(job *batchv1.Job) bool {
	if job == nil {
		return false
	}
	for _, cond := range job.Status.Conditions {
		if cond.Type == batchv1.JobComplete && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return job.Status.Succeeded > 0
}

func isJobFailed(job *batchv1.Job) bool {
	if job == nil {
		return false
	}
	for _, cond := range job.Status.Conditions {
		if cond.Type == batchv1.JobFailed && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func conversionJobName(nodeName string) string {
	return nodeservant.ConversionJobNameBase + "-" + nodeName
}

func nodePoolLabelPredicate() predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(evt event.CreateEvent) bool {
			node, ok := evt.Object.(*corev1.Node)
			if !ok {
				return false
			}
			return hasNodePoolLabel(node) || isEdgeWorkerNode(node)
		},
		UpdateFunc: func(evt event.UpdateEvent) bool {
			oldNode, ok := evt.ObjectOld.(*corev1.Node)
			if !ok {
				return false
			}
			newNode, ok := evt.ObjectNew.(*corev1.Node)
			if !ok {
				return false
			}
			return nodePoolLabelAddedOrRemoved(oldNode, newNode) || edgeWorkerLabelChanged(oldNode, newNode)
		},
		DeleteFunc: func(evt event.DeleteEvent) bool {
			return false
		},
		GenericFunc: func(evt event.GenericEvent) bool {
			return false
		},
	}
}

func conversionJobPredicate() predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(evt event.CreateEvent) bool {
			return hasConversionNodeLabel(evt.Object)
		},
		UpdateFunc: func(evt event.UpdateEvent) bool {
			return hasConversionNodeLabel(evt.ObjectOld) || hasConversionNodeLabel(evt.ObjectNew)
		},
		DeleteFunc: func(evt event.DeleteEvent) bool {
			return hasConversionNodeLabel(evt.Object)
		},
		GenericFunc: func(evt event.GenericEvent) bool {
			return hasConversionNodeLabel(evt.Object)
		},
	}
}

func logReconcileSnapshot(node *corev1.Node, desiredAction, nodePoolName string, cond *corev1.NodeCondition, job *batchv1.Job, jobAction string) {
	klog.V(4).Info(Format("node(%s) snapshot desiredAction=%s nodePool=%q edgeWorker=%t unschedulable=%t condition=%s job=%s jobAction=%s",
		node.Name,
		desiredAction,
		nodePoolName,
		isEdgeWorkerNode(node),
		node.Spec.Unschedulable,
		conversionConditionSummary(cond),
		conversionJobSummary(job),
		jobAction))
}

func conversionConditionSummary(cond *corev1.NodeCondition) string {
	if cond == nil {
		return "nil"
	}
	return fmt.Sprintf("%s/%s", cond.Status, cond.Reason)
}

func conversionJobSummary(job *batchv1.Job) string {
	if job == nil {
		return "nil"
	}
	switch {
	case isJobSucceeded(job):
		return fmt.Sprintf("%s:succeeded", job.Name)
	case isJobFailed(job):
		return fmt.Sprintf("%s:failed", job.Name)
	default:
		return fmt.Sprintf("%s:active=%d succeeded=%d failed=%d", job.Name, job.Status.Active, job.Status.Succeeded, job.Status.Failed)
	}
}

func hasNodePoolLabel(node *corev1.Node) bool {
	return node != nil && len(node.Labels[projectinfo.GetNodePoolLabel()]) != 0
}

func isEdgeWorkerNode(node *corev1.Node) bool {
	return node != nil && node.Labels[projectinfo.GetEdgeWorkerLabelKey()] == "true"
}

func hasConversionNodeLabel(obj client.Object) bool {
	return obj != nil && len(obj.GetLabels()[nodeservant.ConversionNodeLabelKey]) != 0
}

func nodePoolLabelAddedOrRemoved(oldNode, newNode *corev1.Node) bool {
	return hasNodePoolLabel(oldNode) != hasNodePoolLabel(newNode)
}

func edgeWorkerLabelChanged(oldNode, newNode *corev1.Node) bool {
	return isEdgeWorkerNode(oldNode) != isEdgeWorkerNode(newNode)
}

func conditionReasonForInProgress(action string) string {
	if action == actionConvert {
		return reasonConverting
	}
	return reasonReverting
}

func succeededReasonForAction(action string) string {
	if action == actionConvert {
		return reasonConverted
	}
	return reasonReverted
}

func failedReasonForAction(action string) string {
	if action == actionConvert {
		return reasonConvertFailed
	}
	return reasonRevertFailed
}

func inProgressMessage(nodeName, action string) string {
	jobName := conversionJobName(nodeName)
	if action == actionConvert {
		return fmt.Sprintf("conversion Job %s is running", jobName)
	}
	return fmt.Sprintf("revert Job %s is running", jobName)
}

func succeededMessage(action string) string {
	if action == actionConvert {
		return "YurtHub installed and node converted successfully"
	}
	return "YurtHub uninstalled and node reverted successfully"
}

func failedMessage(job *batchv1.Job, action string) string {
	if job != nil {
		for _, cond := range job.Status.Conditions {
			if cond.Type == batchv1.JobFailed && cond.Status == corev1.ConditionTrue && len(cond.Message) != 0 {
				return cond.Message
			}
		}
	}
	if action == actionConvert {
		return fmt.Sprintf("conversion Job %s failed", conversionJobNameFromJob(job))
	}
	return fmt.Sprintf("revert Job %s failed", conversionJobNameFromJob(job))
}

func conversionJobNameFromJob(job *batchv1.Job) string {
	if job == nil || len(job.Name) == 0 {
		return "unknown"
	}
	return job.Name
}
