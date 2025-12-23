/*
Copyright 2025 The OpenYurt Authors.

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

package imagepreheat

import (
	"context"
	"strings"

	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/daemonsetupgradestrategy"
	podutil "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/pod"
)

func OwnerReferenceExistKind(ownerReferences []metav1.OwnerReference, expectKind string) bool {
	for _, ref := range ownerReferences {
		if ref.Kind == expectKind {
			return true
		}
	}
	return false
}

func GetPodAndOwnedDaemonSet(client client.Client, req reconcile.Request) (*v1.Pod, *appsv1.DaemonSet, error) {
	pod, err := GetPod(client, req.NamespacedName)
	if err != nil {
		return nil, nil, err
	}

	ds, err := GetPodOwnerDaemonSet(client, pod)
	if err != nil {
		return nil, nil, err
	}

	return pod, ds, nil
}

func GetPod(c client.Client, namespacedName types.NamespacedName) (*corev1.Pod, error) {
	pod := &v1.Pod{}
	if err := c.Get(context.TODO(), namespacedName, pod); err != nil {
		return nil, err
	}

	return pod, nil
}

func GetPodOwnerDaemonSet(c client.Client, pod *corev1.Pod) (*appsv1.DaemonSet, error) {
	var dsName string
	for _, owner := range pod.OwnerReferences {
		if owner.Kind == "DaemonSet" {
			dsName = owner.Name
			break
		}
	}
	if dsName == "" {
		return nil, errors.Errorf("pod %s/%s has no daemon set owner", pod.Namespace, pod.Name)
	}

	ds := &appsv1.DaemonSet{}
	if err := c.Get(context.TODO(), types.NamespacedName{Namespace: pod.Namespace, Name: dsName}, ds); err != nil {
		return nil, err
	}

	return ds, nil
}

func (r *ReconcileImagePull) patchPodImageStatus(pod *corev1.Pod, status corev1.ConditionStatus, reason string, message string) error {

	cond := corev1.PodCondition{
		Type:    daemonsetupgradestrategy.PodImageReady,
		Status:  status,
		Reason:  reason,
		Message: message,
	}

	if !podutil.UpdatePodCondition(&pod.Status, &cond) {
		return nil
	}

	if err := r.c.Status().Update(context.TODO(), pod); err != nil {
		return errors.Errorf("update pod status failed: %v", err)
	}
	return nil
}

func GetPodNextHashVersion(pod *corev1.Pod) string {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == daemonsetupgradestrategy.PodNeedUpgrade {
			return strings.TrimPrefix(cond.Message, daemonsetupgradestrategy.VersionPrefix)
		}
	}
	return ""
}

func ExpectedPodImageReadyStatus(pod *corev1.Pod, expectCondition corev1.ConditionStatus) bool {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == daemonsetupgradestrategy.PodImageReady && cond.Status == expectCondition && strings.TrimPrefix(cond.Message, daemonsetupgradestrategy.VersionPrefix) == GetPodNextHashVersion(pod) {
			return true
		}
	}
	return false
}

func int64Ptr(i int64) *int64 { return &i }
func int32Ptr(i int32) *int32 { return &i }
func boolPtr(b bool) *bool    { return &b }
