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

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const PodNodeNameFieldIndex = "spec.nodeName"

func (r *ReconcileYurtNodeConversion) restartRecreatablePods(ctx context.Context, nodeName string, restartBefore *metav1.Time) error {
	podList := &corev1.PodList{}
	if err := r.List(ctx, podList, client.MatchingFields{PodNodeNameFieldIndex: nodeName}); err != nil {
		return err
	}

	var errs []error
	for i := range podList.Items {
		pod := &podList.Items[i]
		if !shouldRestartPod(pod, nodeName) {
			continue
		}
		if restartBefore != nil && pod.CreationTimestamp.After(restartBefore.Time) {
			continue
		}

		klog.Infof("delete pod %s/%s on node %s to refresh in-cluster env after node conversion", pod.Namespace, pod.Name, nodeName)
		if err := r.Delete(ctx, pod); err != nil && !apierrors.IsNotFound(err) {
			errs = append(errs, fmt.Errorf("failed to delete pod %s/%s on node %s: %w", pod.Namespace, pod.Name, nodeName, err))
		}
	}

	return utilerrors.NewAggregate(errs)
}

func shouldRestartPod(pod *corev1.Pod, nodeName string) bool {
	if pod == nil || pod.Spec.NodeName != nodeName {
		return false
	}
	if pod.DeletionTimestamp != nil {
		return false
	}
	if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
		return false
	}
	if len(pod.OwnerReferences) == 0 {
		return false
	}
	if pod.Annotations[corev1.MirrorPodAnnotationKey] != "" {
		return false
	}
	if ownedByConversionJob(pod, conversionJobName(nodeName)) {
		return false
	}

	return true
}

func ownedByConversionJob(pod *corev1.Pod, jobName string) bool {
	for _, ownerRef := range pod.OwnerReferences {
		if ownerRef.Kind == "Job" && ownerRef.Name == jobName {
			return true
		}
	}
	return false
}

func jobCompletionCutoff(job *batchv1.Job) *metav1.Time {
	if job == nil {
		return nil
	}
	if job.Status.CompletionTime != nil {
		return job.Status.CompletionTime
	}
	for _, cond := range job.Status.Conditions {
		if cond.Type == batchv1.JobComplete && cond.Status == corev1.ConditionTrue && !cond.LastTransitionTime.IsZero() {
			t := cond.LastTransitionTime
			return &t
		}
	}
	return nil
}
