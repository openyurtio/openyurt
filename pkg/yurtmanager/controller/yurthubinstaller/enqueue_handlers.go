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
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// EnqueueNodeForJob enqueues nodes when their associated installation/uninstallation jobs complete
type EnqueueNodeForJob struct{}

// Create implements EventHandler
func (e *EnqueueNodeForJob) Create(ctx context.Context, evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	// Not interested in job creation events
}

// Update implements EventHandler
func (e *EnqueueNodeForJob) Update(ctx context.Context, evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	oldJob, ok := evt.ObjectOld.(*batchv1.Job)
	if !ok {
		return
	}

	newJob, ok := evt.ObjectNew.(*batchv1.Job)
	if !ok {
		return
	}

	// Check if this is a YurtHub installation/uninstallation job
	if !isYurtHubJob(newJob) {
		return
	}

	// Check if the job status changed to completed or failed
	if (oldJob.Status.Succeeded == 0 && newJob.Status.Succeeded > 0) ||
		(oldJob.Status.Failed == 0 && newJob.Status.Failed > 0) {
		
		nodeName := getNodeNameFromJob(newJob)
		if nodeName != "" {
			klog.V(4).Infof("Job %s for node %s completed, enqueuing node for reconciliation", newJob.Name, nodeName)
			q.Add(reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: nodeName,
				},
			})
		}
	}
}

// Delete implements EventHandler
func (e *EnqueueNodeForJob) Delete(ctx context.Context, evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	// Not interested in job deletion events
}

// Generic implements EventHandler
func (e *EnqueueNodeForJob) Generic(ctx context.Context, evt event.GenericEvent, q workqueue.RateLimitingInterface) {
	// Not interested in generic events
}

// isYurtHubJob checks if a job is a YurtHub installation or uninstallation job
func isYurtHubJob(job *batchv1.Job) bool {
	return strings.HasPrefix(job.Name, YurtHubInstallJobPrefix) ||
		strings.HasPrefix(job.Name, YurtHubUninstallJobPrefix) ||
		strings.HasPrefix(job.Name, "node-servant-convert-") ||
		strings.HasPrefix(job.Name, "node-servant-revert-")
}

// getNodeNameFromJob extracts the node name from a job
func getNodeNameFromJob(job *batchv1.Job) string {
	// The node-servant job template includes the node name in the job spec
	if job.Spec.Template.Spec.NodeName != "" {
		return job.Spec.Template.Spec.NodeName
	}

	// Fallback: try to extract from job name
	if strings.HasPrefix(job.Name, "node-servant-convert-") {
		return strings.TrimPrefix(job.Name, "node-servant-convert-")
	}
	if strings.HasPrefix(job.Name, "node-servant-revert-") {
		return strings.TrimPrefix(job.Name, "node-servant-revert-")
	}

	return ""
}
