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

package yurthubinstaller

import (
	"context"
	"fmt"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	yurtClient "github.com/openyurtio/openyurt/cmd/yurt-manager/app/client"
	"github.com/openyurtio/openyurt/cmd/yurt-manager/app/config"
	"github.com/openyurtio/openyurt/cmd/yurt-manager/names"
	nodeservant "github.com/openyurtio/openyurt/pkg/node-servant"
	installerconfig "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/yurthubinstaller/config"
)

func Format(format string, args ...interface{}) string {
	s := fmt.Sprintf(format, args...)
	return fmt.Sprintf("%s: %s", names.YurtHubInstallerController, s)
}

// ReconcileYurtHubInstaller reconciles YurtHub installation on nodes based on labels
type ReconcileYurtHubInstaller struct {
	client.Client
	recorder record.EventRecorder
	cfg      installerconfig.YurtHubInstallerControllerConfiguration
}

var _ reconcile.Reconciler = &ReconcileYurtHubInstaller{}

// Add creates a new YurtHub Installer Controller and adds it to the Manager with default RBAC
func Add(ctx context.Context, c *config.CompletedConfig, mgr manager.Manager) error {
	if !c.ComponentConfig.YurtHubInstallerController.EnableYurtHubInstaller {
		klog.Info("yurthub-installer-controller is disabled")
		return nil
	}

	klog.Info("yurthub-installer-controller add controller")
	r := &ReconcileYurtHubInstaller{
		cfg:      c.ComponentConfig.YurtHubInstallerController,
		recorder: mgr.GetEventRecorderFor(names.YurtHubInstallerController),
		Client:   yurtClient.GetClientByControllerNameOrDie(mgr, names.YurtHubInstallerController),
	}

	// Create a new controller
	ctrl, err := controller.New(names.YurtHubInstallerController, mgr, controller.Options{
		Reconciler:              r,
		MaxConcurrentReconciles: int(c.ComponentConfig.YurtHubInstallerController.ConcurrentYurtHubInstallerWorkers),
	})
	if err != nil {
		return err
	}

	// Watch for changes to Node
	err = ctrl.Watch(source.Kind[client.Object](mgr.GetCache(), &corev1.Node{}, &handler.EnqueueRequestForObject{}))
	if err != nil {
		return err
	}

	// Watch for changes to Jobs (to update node annotations when installation completes)
	err = ctrl.Watch(source.Kind[client.Object](mgr.GetCache(), &batchv1.Job{}, &EnqueueNodeForJob{}))
	if err != nil {
		return err
	}

	return nil
}

// Reconcile reads the state of the cluster for a Node object and makes changes based on the edge worker label
func (r *ReconcileYurtHubInstaller) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	klog.V(4).Infof(Format("Reconcile Node %s", request.Name))

	// Fetch the Node instance
	node := &corev1.Node{}
	if err := r.Get(ctx, request.NamespacedName, node); err != nil {
		if apierrors.IsNotFound(err) {
			// Node has been deleted, nothing to do
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	// Check if the node has the edge worker label
	hasEdgeLabel := node.Labels != nil && node.Labels[EdgeWorkerLabel] == "true"
	isInstalled := node.Annotations != nil && node.Annotations[YurtHubInstalledAnnotation] == "true"
	isInProgress := node.Annotations != nil && node.Annotations[YurtHubInstallationInProgressAnnotation] == "true"

	klog.V(4).Infof(Format("Node %s: hasEdgeLabel=%v, isInstalled=%v, isInProgress=%v", 
		node.Name, hasEdgeLabel, isInstalled, isInProgress))

	if hasEdgeLabel && !isInstalled && !isInProgress {
		// Label is present, but YurtHub is not installed - trigger installation
		klog.Infof(Format("Triggering YurtHub installation for node %s", node.Name))
		return r.installYurtHub(ctx, node)
	} else if !hasEdgeLabel && isInstalled {
		// Label is removed, but YurtHub is still installed - trigger uninstallation
		klog.Infof(Format("Triggering YurtHub uninstallation for node %s", node.Name))
		return r.uninstallYurtHub(ctx, node)
	} else if isInProgress {
		// Installation is in progress, check job status
		return r.checkInstallationProgress(ctx, node)
	}

	return reconcile.Result{}, nil
}

// installYurtHub triggers YurtHub installation on the node using a Job
func (r *ReconcileYurtHubInstaller) installYurtHub(ctx context.Context, node *corev1.Node) (reconcile.Result, error) {
	// Get YurtHub version from annotation or use default
	yurtHubVersion := DefaultYurtHubVersion
	if node.Annotations != nil {
		if version, ok := node.Annotations[YurtHubVersionAnnotation]; ok && version != "" {
			yurtHubVersion = version
		}
	}
	if r.cfg.YurtHubVersion != "" {
		yurtHubVersion = r.cfg.YurtHubVersion
	}

	// Get node pool name from node labels
	nodePoolName := ""
	if node.Labels != nil {
		if pool, ok := node.Labels["apps.openyurt.io/nodepool"]; ok {
			nodePoolName = pool
		}
	}

	// Create a bootstrap token for YurtHub (simplified - in production, this should use token API)
	// For now, we'll use a configmap-based approach
	joinToken, err := r.getOrCreateBootstrapToken(ctx, node.Name)
	if err != nil {
		klog.Errorf(Format("Failed to get bootstrap token for node %s: %v", node.Name, err))
		r.recorder.Eventf(node, corev1.EventTypeWarning, "BootstrapTokenFailed", "Failed to get bootstrap token: %v", err)
		return reconcile.Result{RequeueAfter: 30 * time.Second}, err
	}

	// Create the installation Job
	job, err := r.createInstallationJob(ctx, node, joinToken, nodePoolName, yurtHubVersion)
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			klog.V(4).Infof(Format("Installation job for node %s already exists", node.Name))
			// Job already exists, mark as in progress
			return r.markInstallationInProgress(ctx, node)
		}
		klog.Errorf(Format("Failed to create installation job for node %s: %v", node.Name, err))
		r.recorder.Eventf(node, corev1.EventTypeWarning, "InstallJobFailed", "Failed to create installation job: %v", err)
		return reconcile.Result{RequeueAfter: 30 * time.Second}, err
	}

	klog.Infof(Format("Created installation job %s for node %s", job.Name, node.Name))
	r.recorder.Eventf(node, corev1.EventTypeNormal, "InstallJobCreated", "Created YurtHub installation job %s", job.Name)

	// Mark installation as in progress
	return r.markInstallationInProgress(ctx, node)
}

// uninstallYurtHub triggers YurtHub uninstallation on the node using a Job
func (r *ReconcileYurtHubInstaller) uninstallYurtHub(ctx context.Context, node *corev1.Node) (reconcile.Result, error) {
	// Create the uninstallation Job
	job, err := r.createUninstallationJob(ctx, node)
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			klog.V(4).Infof(Format("Uninstallation job for node %s already exists", node.Name))
			return r.markUninstallationInProgress(ctx, node)
		}
		klog.Errorf(Format("Failed to create uninstallation job for node %s: %v", node.Name, err))
		r.recorder.Eventf(node, corev1.EventTypeWarning, "UninstallJobFailed", "Failed to create uninstallation job: %v", err)
		return reconcile.Result{RequeueAfter: 30 * time.Second}, err
	}

	klog.Infof(Format("Created uninstallation job %s for node %s", job.Name, node.Name))
	r.recorder.Eventf(node, corev1.EventTypeNormal, "UninstallJobCreated", "Created YurtHub uninstallation job %s", job.Name)

	return r.markUninstallationInProgress(ctx, node)
}

// createInstallationJob creates a Kubernetes Job to install YurtHub on the node
func (r *ReconcileYurtHubInstaller) createInstallationJob(ctx context.Context, node *corev1.Node, joinToken, nodePoolName, yurtHubVersion string) (*batchv1.Job, error) {
	// Prepare the render context for the node-servant job template
	renderCtx := map[string]string{
		"node_servant_image":          r.cfg.NodeServantImage,
		"joinToken":                   joinToken,
		"nodePoolName":                nodePoolName,
		"yurthub_healthcheck_timeout": "2m",
	}

	// Use the node-servant job template to create the installation job
	job, err := nodeservant.RenderNodeServantJob("convert", renderCtx, node.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to render installation job: %w", err)
	}

	// Set TTL for automatic cleanup
	ttl := int32(JobTTLSecondsAfterFinished)
	job.Spec.TTLSecondsAfterFinished = &ttl

	// Create the job
	if err := r.Create(ctx, job); err != nil {
		return nil, err
	}

	return job, nil
}

// createUninstallationJob creates a Kubernetes Job to uninstall YurtHub from the node
func (r *ReconcileYurtHubInstaller) createUninstallationJob(ctx context.Context, node *corev1.Node) (*batchv1.Job, error) {
	// Prepare the render context for the node-servant job template
	renderCtx := map[string]string{
		"node_servant_image": r.cfg.NodeServantImage,
	}

	// Use the node-servant job template to create the uninstallation job
	job, err := nodeservant.RenderNodeServantJob("revert", renderCtx, node.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to render uninstallation job: %w", err)
	}

	// Set TTL for automatic cleanup
	ttl := int32(JobTTLSecondsAfterFinished)
	job.Spec.TTLSecondsAfterFinished = &ttl

	// Create the job
	if err := r.Create(ctx, job); err != nil {
		return nil, err
	}

	return job, nil
}

// markInstallationInProgress marks the node as having an installation in progress
func (r *ReconcileYurtHubInstaller) markInstallationInProgress(ctx context.Context, node *corev1.Node) (reconcile.Result, error) {
	nodeCopy := node.DeepCopy()
	if nodeCopy.Annotations == nil {
		nodeCopy.Annotations = make(map[string]string)
	}
	nodeCopy.Annotations[YurtHubInstallationInProgressAnnotation] = "true"

	if err := r.Patch(ctx, nodeCopy, client.MergeFrom(node)); err != nil {
		return reconcile.Result{}, err
	}

	// Requeue to check progress
	return reconcile.Result{RequeueAfter: 30 * time.Second}, nil
}

// markUninstallationInProgress marks the node as having an uninstallation in progress
func (r *ReconcileYurtHubInstaller) markUninstallationInProgress(ctx context.Context, node *corev1.Node) (reconcile.Result, error) {
	nodeCopy := node.DeepCopy()
	if nodeCopy.Annotations == nil {
		nodeCopy.Annotations = make(map[string]string)
	}
	nodeCopy.Annotations[YurtHubInstallationInProgressAnnotation] = "true"
	// Remove the installed annotation as we're uninstalling
	delete(nodeCopy.Annotations, YurtHubInstalledAnnotation)

	if err := r.Patch(ctx, nodeCopy, client.MergeFrom(node)); err != nil {
		return reconcile.Result{}, err
	}

	// Requeue to check progress
	return reconcile.Result{RequeueAfter: 30 * time.Second}, nil
}

// checkInstallationProgress checks the status of the installation/uninstallation job
func (r *ReconcileYurtHubInstaller) checkInstallationProgress(ctx context.Context, node *corev1.Node) (reconcile.Result, error) {
	// List jobs for this node
	jobList := &batchv1.JobList{}
	listOpts := []client.ListOption{
		client.InNamespace("kube-system"),
		client.MatchingLabels(map[string]string{
			"node": node.Name,
		}),
	}

	if err := r.List(ctx, jobList, listOpts...); err != nil {
		return reconcile.Result{}, err
	}

	// Find the most recent job for this node
	var latestJob *batchv1.Job
	var latestTime metav1.Time
	for i := range jobList.Items {
		job := &jobList.Items[i]
		if job.CreationTimestamp.After(latestTime.Time) {
			latestJob = job
			latestTime = job.CreationTimestamp
		}
	}

	if latestJob == nil {
		// No job found, clear in-progress annotation
		return r.clearInProgressAnnotation(ctx, node)
	}

	// Check job status
	if latestJob.Status.Succeeded > 0 {
		// Job completed successfully
		klog.Infof(Format("Job %s completed successfully for node %s", latestJob.Name, node.Name))
		return r.markInstallationComplete(ctx, node, latestJob)
	} else if latestJob.Status.Failed > 0 {
		// Job failed
		klog.Warningf(Format("Job %s failed for node %s", latestJob.Name, node.Name))
		r.recorder.Eventf(node, corev1.EventTypeWarning, "InstallationFailed", "YurtHub installation job %s failed", latestJob.Name)
		return r.clearInProgressAnnotation(ctx, node)
	}

	// Job still running, requeue
	return reconcile.Result{RequeueAfter: 30 * time.Second}, nil
}

// markInstallationComplete marks the installation as complete
func (r *ReconcileYurtHubInstaller) markInstallationComplete(ctx context.Context, node *corev1.Node, job *batchv1.Job) (reconcile.Result, error) {
	nodeCopy := node.DeepCopy()
	if nodeCopy.Annotations == nil {
		nodeCopy.Annotations = make(map[string]string)
	}

	// Determine if this was an install or uninstall job
	isUninstallJob := false
	for _, arg := range job.Spec.Template.Spec.Containers[0].Args {
		if arg == "/usr/local/bin/entry.sh revert" {
			isUninstallJob = true
			break
		}
	}

	if isUninstallJob {
		// Uninstallation completed
		delete(nodeCopy.Annotations, YurtHubInstalledAnnotation)
		delete(nodeCopy.Annotations, YurtHubInstallationInProgressAnnotation)
		r.recorder.Event(node, corev1.EventTypeNormal, "UninstallationComplete", "YurtHub uninstallation completed successfully")
	} else {
		// Installation completed
		nodeCopy.Annotations[YurtHubInstalledAnnotation] = "true"
		delete(nodeCopy.Annotations, YurtHubInstallationInProgressAnnotation)
		r.recorder.Event(node, corev1.EventTypeNormal, "InstallationComplete", "YurtHub installation completed successfully")
	}

	if err := r.Patch(ctx, nodeCopy, client.MergeFrom(node)); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

// clearInProgressAnnotation removes the in-progress annotation
func (r *ReconcileYurtHubInstaller) clearInProgressAnnotation(ctx context.Context, node *corev1.Node) (reconcile.Result, error) {
	if node.Annotations != nil && node.Annotations[YurtHubInstallationInProgressAnnotation] != "" {
		nodeCopy := node.DeepCopy()
		delete(nodeCopy.Annotations, YurtHubInstallationInProgressAnnotation)

		if err := r.Patch(ctx, nodeCopy, client.MergeFrom(node)); err != nil {
			return reconcile.Result{}, err
		}
	}
	return reconcile.Result{}, nil
}

// getOrCreateBootstrapToken gets or creates a bootstrap token for the node
// This is a simplified implementation - in production, this should use the Kubernetes Bootstrap Token API
func (r *ReconcileYurtHubInstaller) getOrCreateBootstrapToken(ctx context.Context, nodeName string) (string, error) {
	// Check if a secret with bootstrap token already exists for this node
	secretName := fmt.Sprintf("yurthub-bootstrap-%s", nodeName)
	secret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: "kube-system"}, secret)
	
	if err == nil {
		// Secret exists, return the token
		if token, ok := secret.Data["token"]; ok {
			return string(token), nil
		}
	}

	if !apierrors.IsNotFound(err) {
		return "", err
	}

	// Secret doesn't exist, create a placeholder
	// In a real implementation, this should call the token API to create a proper bootstrap token
	// For now, we'll create a configmap-based token placeholder
	secret = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: "kube-system",
		},
		StringData: map[string]string{
			"token": "placeholder-token-" + nodeName, // This should be a real bootstrap token
		},
	}

	if err := r.Create(ctx, secret); err != nil {
		return "", err
	}

	return secret.StringData["token"], nil
}
