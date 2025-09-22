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
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appconfig "github.com/openyurtio/openyurt/cmd/yurt-manager/app/config"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/daemonsetupgradestrategy"
)

func newTestPod(name string, node string, conds []corev1.PodCondition) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "apps/v1",
				Kind:       "DaemonSet",
			}},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name:  "c1",
				Image: "busybox:latest",
			}},
			InitContainers: []corev1.Container{{
				Name:  "init1",
				Image: "alpine:latest",
			}},
			NodeName: node,
		},
		Status: corev1.PodStatus{
			Conditions: conds,
		},
	}
}

func newTestJob(name string, succeeded, failed int32) *batchv1.Job {
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       "default",
			Labels:          map[string]string{"app": "image-prepull-pod"},
			OwnerReferences: []metav1.OwnerReference{{Kind: "Pod"}},
		},
		Status: batchv1.JobStatus{
			Succeeded: succeeded,
			Failed:    failed,
		},
	}
}

func newTestDaemonSet(name string) *appsv1.DaemonSet {
	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: appsv1.DaemonSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "test-container",
						Image: "test-image:latest",
					}},
					InitContainers: []corev1.Container{{
						Name:  "test-init",
						Image: "test-init:latest",
					}},
				},
			},
		},
	}
}

// TestReconcile_PodNotFound 测试 Pod 不存在的情况
func TestReconcile_PodNotFound(t *testing.T) {
	c := fake.NewClientBuilder().Build()
	r := &ReconcileImagePull{c: c}
	ctx := context.TODO()
	req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "nonexistent-pod"}}

	result, err := r.Reconcile(ctx, req)

	assert.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, result)
}

// TestReconcile_DaemonSetNotFound 测试 DaemonSet 不存在的情况
func TestReconcile_DaemonSetNotFound(t *testing.T) {
	pod := newTestPod("testpod", "testnode", []corev1.PodCondition{{
		Type:   daemonsetupgradestrategy.PodNeedUpgrade,
		Status: corev1.ConditionTrue,
	}})
	// Pod 没有正确的 OwnerReference
	pod.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: "apps/v1",
		Kind:       "DaemonSet",
		Name:       "nonexistent-ds",
	}}

	c := fake.NewClientBuilder().WithObjects(pod).Build()
	r := &ReconcileImagePull{c: c}
	ctx := context.TODO()
	req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "testpod"}}

	result, err := r.Reconcile(ctx, req)

	// 由于 client.IgnoreNotFound(err) 会忽略 NotFound 错误，所以这里应该没有错误
	assert.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, result)
}

// TestReconcile_NoNeedReconcile_PodDeleted 测试 Pod 被删除的情况
func TestReconcile_NoNeedReconcile_PodDeleted(t *testing.T) {
	now := metav1.Now()
	pod := newTestPod("testpod", "testnode", []corev1.PodCondition{{
		Type:   daemonsetupgradestrategy.PodNeedUpgrade,
		Status: corev1.ConditionTrue,
	}})
	pod.DeletionTimestamp = &now
	pod.Finalizers = []string{"test-finalizer"} // 添加 finalizer 以避免 fake client 错误
	pod.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: "apps/v1",
		Kind:       "DaemonSet",
		Name:       "test-ds",
	}}

	ds := newTestDaemonSet("test-ds")

	c := fake.NewClientBuilder().WithObjects(pod, ds).Build()
	r := &ReconcileImagePull{c: c}
	ctx := context.TODO()
	req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "testpod"}}

	result, err := r.Reconcile(ctx, req)

	assert.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, result)
}

// TestReconcile_NoNeedReconcile_DaemonSetDeleted 测试 DaemonSet 被删除的情况
func TestReconcile_NoNeedReconcile_DaemonSetDeleted(t *testing.T) {
	now := metav1.Now()
	pod := newTestPod("testpod", "testnode", []corev1.PodCondition{{
		Type:   daemonsetupgradestrategy.PodNeedUpgrade,
		Status: corev1.ConditionTrue,
	}})
	pod.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: "apps/v1",
		Kind:       "DaemonSet",
		Name:       "test-ds",
	}}

	ds := newTestDaemonSet("test-ds")
	ds.DeletionTimestamp = &now
	ds.Finalizers = []string{"test-finalizer"} // 添加 finalizer 以避免 fake client 错误

	c := fake.NewClientBuilder().WithObjects(pod, ds).Build()
	r := &ReconcileImagePull{c: c}
	ctx := context.TODO()
	req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "testpod"}}

	result, err := r.Reconcile(ctx, req)

	assert.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, result)
}

// TestReconcile_NoNeedReconcile_NotUpgradeStatus 测试不在升级状态的情况
func TestReconcile_NoNeedReconcile_NotUpgradeStatus(t *testing.T) {
	pod := newTestPod("testpod", "testnode", []corev1.PodCondition{{
		Type:   daemonsetupgradestrategy.PodImageReady,
		Status: corev1.ConditionTrue,
	}})
	pod.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: "apps/v1",
		Kind:       "DaemonSet",
		Name:       "test-ds",
	}}

	ds := newTestDaemonSet("test-ds")

	c := fake.NewClientBuilder().WithObjects(pod, ds).Build()
	r := &ReconcileImagePull{c: c}
	ctx := context.TODO()
	req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "testpod"}}

	result, err := r.Reconcile(ctx, req)

	assert.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, result)
}

// TestReconcile_CreateJobSuccess 测试成功创建 Job 的情况
func TestReconcile_CreateJobSuccess(t *testing.T) {
	pod := newTestPod("testpod", "testnode", []corev1.PodCondition{{
		Type:    daemonsetupgradestrategy.PodNeedUpgrade,
		Status:  corev1.ConditionTrue,
		Message: daemonsetupgradestrategy.VersionPrefix + "123",
	}, {
		Type:    daemonsetupgradestrategy.PodImageReady,
		Status:  corev1.ConditionFalse,
		Message: daemonsetupgradestrategy.VersionPrefix + "123",
	}})
	pod.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: "apps/v1",
		Kind:       "DaemonSet",
		Name:       "test-ds",
	}}

	ds := newTestDaemonSet("test-ds")

	c := fake.NewClientBuilder().WithObjects(pod, ds).Build()
	r := &ReconcileImagePull{c: c}
	ctx := context.TODO()
	req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "testpod"}}

	result, err := r.Reconcile(ctx, req)

	assert.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, result)

	// 检查 Job 是否创建
	var job batchv1.Job
	err = c.Get(ctx, types.NamespacedName{Namespace: "default", Name: daemonsetupgradestrategy.ImagePullJobNamePrefix + "testpod" + "-123"}, &job)
	assert.NoError(t, err)
	assert.Equal(t, daemonsetupgradestrategy.ImagePullJobNamePrefix+"testpod"+"-123", job.Name)
	assert.Equal(t, 2, len(job.Spec.Template.Spec.Containers)) // container + initContainer
}

// TestReconcile_JobAlreadyExists 测试 Job 已存在的情况
func TestReconcile_JobAlreadyExists(t *testing.T) {
	pod := newTestPod("testpod", "testnode", []corev1.PodCondition{{
		Type:    daemonsetupgradestrategy.PodNeedUpgrade,
		Status:  corev1.ConditionTrue,
		Message: daemonsetupgradestrategy.VersionPrefix + "123",
	}, {
		Type:    daemonsetupgradestrategy.PodImageReady,
		Status:  corev1.ConditionFalse,
		Message: daemonsetupgradestrategy.VersionPrefix + "123",
	}})
	pod.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: "apps/v1",
		Kind:       "DaemonSet",
		Name:       "test-ds",
	}}

	ds := newTestDaemonSet("test-ds")
	job := newTestJob(daemonsetupgradestrategy.ImagePullJobNamePrefix+"testpod"+"-123", 0, 0)

	c := fake.NewClientBuilder().WithObjects(pod, ds, job).Build()
	r := &ReconcileImagePull{c: c}
	ctx := context.TODO()
	req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "testpod"}}

	result, err := r.Reconcile(ctx, req)

	assert.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, result)
}

// TestReconcile_JobSucceeded_PatchPodTrue 测试 Job 成功完成，更新 Pod 状态为 True
func TestReconcile_JobSucceeded_PatchPodTrue(t *testing.T) {
	pod := newTestPod("testpod", "testnode", []corev1.PodCondition{{
		Type:    daemonsetupgradestrategy.PodNeedUpgrade,
		Status:  corev1.ConditionTrue,
		Message: daemonsetupgradestrategy.VersionPrefix + "123",
	}, {
		Type:    daemonsetupgradestrategy.PodImageReady,
		Status:  corev1.ConditionFalse,
		Message: daemonsetupgradestrategy.VersionPrefix + "123",
	}})
	pod.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: "apps/v1",
		Kind:       "DaemonSet",
		Name:       "test-ds",
	}}

	ds := newTestDaemonSet("test-ds")
	job := newTestJob(daemonsetupgradestrategy.ImagePullJobNamePrefix+"testpod"+"-123", 1, 0)

	c := fake.NewClientBuilder().WithObjects(pod, ds, job).Build()
	r := &ReconcileImagePull{c: c}
	ctx := context.TODO()
	req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "testpod"}}

	result, err := r.Reconcile(ctx, req)

	assert.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, result)

	// 检查 Pod 状态是否更新
	var patchedPod corev1.Pod
	err = c.Get(ctx, types.NamespacedName{Namespace: "default", Name: "testpod"}, &patchedPod)
	assert.NoError(t, err)

	found := false
	for _, cond := range patchedPod.Status.Conditions {
		if cond.Type == daemonsetupgradestrategy.PodImageReady && cond.Status == corev1.ConditionTrue {
			found = true
			break
		}
	}
	assert.True(t, found, "PodImageReady should be True after job succeeded")
}

// TestReconcile_JobFailed_PatchPodFalse 测试 Job 失败，更新 Pod 状态为 False
func TestReconcile_JobFailed_PatchPodFalse(t *testing.T) {
	pod := newTestPod("testpod", "testnode", []corev1.PodCondition{{
		Type:    daemonsetupgradestrategy.PodNeedUpgrade,
		Status:  corev1.ConditionTrue,
		Message: daemonsetupgradestrategy.VersionPrefix + "123",
	}, {
		Type:    daemonsetupgradestrategy.PodImageReady,
		Status:  corev1.ConditionFalse,
		Message: daemonsetupgradestrategy.VersionPrefix + "123",
	}})
	pod.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: "apps/v1",
		Kind:       "DaemonSet",
		Name:       "test-ds",
	}}

	ds := newTestDaemonSet("test-ds")
	job := newTestJob(daemonsetupgradestrategy.ImagePullJobNamePrefix+"testpod"+"-123", 0, 1)

	c := fake.NewClientBuilder().WithObjects(pod, ds, job).Build()
	r := &ReconcileImagePull{c: c}
	ctx := context.TODO()
	req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "testpod"}}

	result, err := r.Reconcile(ctx, req)

	assert.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, result)

	// 检查 Pod 状态是否更新
	var patchedPod corev1.Pod
	err = c.Get(ctx, types.NamespacedName{Namespace: "default", Name: "testpod"}, &patchedPod)
	assert.NoError(t, err)

	found := false
	for _, cond := range patchedPod.Status.Conditions {
		if cond.Type == daemonsetupgradestrategy.PodImageReady && cond.Status == corev1.ConditionFalse {
			found = true
			break
		}
	}
	assert.True(t, found, "PodImageReady should be False after job failed")
}

// TestPatchPodImageStatus_StatusUpdateError 测试 Status().Update 失败的情况
func TestPatchPodImageStatus_StatusUpdateError(t *testing.T) {
	pod := newTestPod("testpod", "testnode", []corev1.PodCondition{{
		Type:   daemonsetupgradestrategy.PodImageReady,
		Status: corev1.ConditionFalse,
	}})

	c := fake.NewClientBuilder().WithObjects(pod).Build()
	r := &ReconcileImagePull{c: c}

	// 测试更新状态为 True，这会触发 UpdatePodCondition 返回 true
	err := r.patchPodImageStatus(pod, corev1.ConditionTrue, daemonsetupgradestrategy.PullImageSuccess, "test message")
	assert.NoError(t, err) // 实际上 fake client 不会返回错误
}

// TestReconcile_UpdatePodImageReady_JobSucceeded 测试 Job 成功完成的情况
func TestReconcile_UpdatePodImageReady_JobSucceeded(t *testing.T) {
	pod := newTestPod("testpod", "testnode", []corev1.PodCondition{{
		Type:    daemonsetupgradestrategy.PodNeedUpgrade,
		Status:  corev1.ConditionTrue,
		Message: daemonsetupgradestrategy.VersionPrefix + "123",
	}, {
		Type:    daemonsetupgradestrategy.PodImageReady,
		Status:  corev1.ConditionFalse,
		Message: daemonsetupgradestrategy.VersionPrefix + "123",
	}})
	pod.OwnerReferences = []metav1.OwnerReference{{
		Kind: "DaemonSet",
		Name: "test-ds",
	}}

	ds := newTestDaemonSet("test-ds")
	job := newTestJob(daemonsetupgradestrategy.ImagePullJobNamePrefix+"testpod"+"-123", 1, 0) // 成功

	c := fake.NewClientBuilder().WithObjects(pod, ds, job).Build()
	r := &ReconcileImagePull{c: c}
	ctx := context.TODO()
	req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "testpod"}}

	result, err := r.Reconcile(ctx, req)

	assert.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, result)

	// 验证 Pod 状态被更新
	var updatedPod corev1.Pod
	err = c.Get(ctx, types.NamespacedName{Namespace: "default", Name: "testpod"}, &updatedPod)
	assert.NoError(t, err)

	found := false
	for _, cond := range updatedPod.Status.Conditions {
		if cond.Type == daemonsetupgradestrategy.PodImageReady && cond.Status == corev1.ConditionTrue {
			found = true
			break
		}
	}
	assert.True(t, found, "PodImageReady should be True after job succeeded")
}

// TestReconcile_UpdatePodImageReady_JobFailed 测试 Job 失败的情况
func TestReconcile_UpdatePodImageReady_JobFailed(t *testing.T) {
	pod := newTestPod("testpod", "testnode", []corev1.PodCondition{{
		Type:    daemonsetupgradestrategy.PodNeedUpgrade,
		Status:  corev1.ConditionTrue,
		Message: daemonsetupgradestrategy.VersionPrefix + "123",
	}, {
		Type:    daemonsetupgradestrategy.PodImageReady,
		Status:  corev1.ConditionFalse,
		Message: daemonsetupgradestrategy.VersionPrefix + "123",
	}})
	pod.OwnerReferences = []metav1.OwnerReference{{
		Kind: "DaemonSet",
		Name: "test-ds",
	}}

	ds := newTestDaemonSet("test-ds")
	job := newTestJob(daemonsetupgradestrategy.ImagePullJobNamePrefix+"testpod"+"-123", 0, 1) // 失败

	c := fake.NewClientBuilder().WithObjects(pod, ds, job).Build()
	r := &ReconcileImagePull{c: c}
	ctx := context.TODO()
	req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "testpod"}}

	result, err := r.Reconcile(ctx, req)

	assert.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, result)

	// 验证 Pod 状态被更新
	var updatedPod corev1.Pod
	err = c.Get(ctx, types.NamespacedName{Namespace: "default", Name: "testpod"}, &updatedPod)
	assert.NoError(t, err)

	found := false
	for _, cond := range updatedPod.Status.Conditions {
		if cond.Type == daemonsetupgradestrategy.PodImageReady && cond.Status == corev1.ConditionFalse {
			found = true
			break
		}
	}
	assert.True(t, found, "PodImageReady should be False after job failed")
}

// TestReconcile_GetPodAndOwnedDaemonSet_DaemonSetError 测试 DaemonSet 获取失败的情况
func TestReconcile_GetPodAndOwnedDaemonSet_DaemonSetError(t *testing.T) {
	pod := newTestPod("testpod", "testnode", []corev1.PodCondition{{
		Type:   daemonsetupgradestrategy.PodNeedUpgrade,
		Status: corev1.ConditionTrue,
	}})
	pod.OwnerReferences = []metav1.OwnerReference{{
		Kind: "DaemonSet",
		Name: "nonexistent-ds", // 不存在的 DaemonSet
	}}

	c := fake.NewClientBuilder().WithObjects(pod).Build()
	r := &ReconcileImagePull{c: c}
	ctx := context.TODO()
	req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "testpod"}}

	result, err := r.Reconcile(ctx, req)

	assert.NoError(t, err) // 因为使用了 client.IgnoreNotFound(err)
	assert.Equal(t, reconcile.Result{}, result)
}

// TestReconcile_JobRunning_NoUpdate 测试 Job 正在运行，不更新 Pod 状态
func TestReconcile_JobRunning_NoUpdate(t *testing.T) {
	pod := newTestPod("testpod", "testnode", []corev1.PodCondition{{
		Type:    daemonsetupgradestrategy.PodNeedUpgrade,
		Status:  corev1.ConditionTrue,
		Message: daemonsetupgradestrategy.VersionPrefix + "123",
	}, {
		Type:    daemonsetupgradestrategy.PodImageReady,
		Status:  corev1.ConditionFalse,
		Message: daemonsetupgradestrategy.VersionPrefix + "123",
	}})
	pod.OwnerReferences = []metav1.OwnerReference{{
		Kind: "DaemonSet",
		Name: "test-ds",
	}}

	ds := newTestDaemonSet("test-ds")
	job := newTestJob(daemonsetupgradestrategy.ImagePullJobNamePrefix+"testpod"+"-123", 0, 0) // 既没有成功也没有失败

	c := fake.NewClientBuilder().WithObjects(pod, ds, job).Build()
	r := &ReconcileImagePull{c: c}
	ctx := context.TODO()
	req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "testpod"}}

	result, err := r.Reconcile(ctx, req)

	assert.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, result)

	// 检查 Pod 状态没有变化
	var patchedPod corev1.Pod
	err = c.Get(ctx, types.NamespacedName{Namespace: "default", Name: "testpod"}, &patchedPod)
	assert.NoError(t, err)

	// PodImageReady 应该仍然是 False
	found := false
	for _, cond := range patchedPod.Status.Conditions {
		if cond.Type == daemonsetupgradestrategy.PodImageReady && cond.Status == corev1.ConditionFalse {
			found = true
			break
		}
	}
	assert.True(t, found, "PodImageReady should remain False when job is running")
}

// TestOwnerReferenceExistKind 测试 OwnerReferenceExistKind 函数
func TestOwnerReferenceExistKind(t *testing.T) {
	ownerRefs := []metav1.OwnerReference{
		{Kind: "DaemonSet", Name: "test-ds"},
		{Kind: "ReplicaSet", Name: "test-rs"},
	}

	// 测试存在的情况
	assert.True(t, OwnerReferenceExistKind(ownerRefs, "DaemonSet"))
	assert.True(t, OwnerReferenceExistKind(ownerRefs, "ReplicaSet"))

	// 测试不存在的情况
	assert.False(t, OwnerReferenceExistKind(ownerRefs, "Pod"))
	assert.False(t, OwnerReferenceExistKind(ownerRefs, "Deployment"))

	// 测试空列表
	assert.False(t, OwnerReferenceExistKind([]metav1.OwnerReference{}, "DaemonSet"))
}

// TestGetPodNextHashVersion 测试 GetPodNextHashVersion 函数
func TestGetPodNextHashVersion(t *testing.T) {
	// 测试有 PodNeedUpgrade 条件的情况
	pod := newTestPod("testpod", "testnode", []corev1.PodCondition{{
		Type:    daemonsetupgradestrategy.PodNeedUpgrade,
		Status:  corev1.ConditionTrue,
		Message: daemonsetupgradestrategy.VersionPrefix + "123",
	}})

	version := GetPodNextHashVersion(pod)
	assert.Equal(t, "123", version)

	// 测试没有 PodNeedUpgrade 条件的情况
	pod2 := newTestPod("testpod2", "testnode", []corev1.PodCondition{{
		Type:   daemonsetupgradestrategy.PodImageReady,
		Status: corev1.ConditionTrue,
	}})

	version2 := GetPodNextHashVersion(pod2)
	assert.Equal(t, "", version2)

	// 测试空条件列表
	pod3 := newTestPod("testpod3", "testnode", []corev1.PodCondition{})
	version3 := GetPodNextHashVersion(pod3)
	assert.Equal(t, "", version3)
}

// TestExpectedPodImageReadyStatus 测试 ExpectedPodImageReadyStatus 函数
func TestExpectedPodImageReadyStatus(t *testing.T) {
	// 测试匹配的情况
	pod := newTestPod("testpod", "testnode", []corev1.PodCondition{{
		Type:   daemonsetupgradestrategy.PodImageReady,
		Status: corev1.ConditionTrue,
	}})

	assert.True(t, ExpectedPodImageReadyStatus(pod, corev1.ConditionTrue))
	assert.False(t, ExpectedPodImageReadyStatus(pod, corev1.ConditionFalse))

	// 测试不匹配的情况
	pod2 := newTestPod("testpod2", "testnode", []corev1.PodCondition{{
		Type:   daemonsetupgradestrategy.PodImageReady,
		Status: corev1.ConditionFalse,
	}})

	assert.False(t, ExpectedPodImageReadyStatus(pod2, corev1.ConditionTrue))
	assert.True(t, ExpectedPodImageReadyStatus(pod2, corev1.ConditionFalse))

	// 测试没有 PodImageReady 条件的情况
	pod3 := newTestPod("testpod3", "testnode", []corev1.PodCondition{{
		Type:   daemonsetupgradestrategy.PodNeedUpgrade,
		Status: corev1.ConditionTrue,
	}})

	assert.False(t, ExpectedPodImageReadyStatus(pod3, corev1.ConditionTrue))
	assert.False(t, ExpectedPodImageReadyStatus(pod3, corev1.ConditionFalse))
}

// TestPatchPodImageStatus 测试 patchPodImageStatus 函数
func TestPatchPodImageStatus(t *testing.T) {
	pod := newTestPod("testpod", "testnode", []corev1.PodCondition{{
		Type:   daemonsetupgradestrategy.PodImageReady,
		Status: corev1.ConditionFalse,
	}})

	c := fake.NewClientBuilder().WithObjects(pod).Build()
	r := &ReconcileImagePull{c: c}

	// 测试更新状态为 True
	err := r.patchPodImageStatus(pod, corev1.ConditionTrue, daemonsetupgradestrategy.PullImageSuccess, "test message")
	assert.NoError(t, err)

	// 验证状态已更新
	var updatedPod corev1.Pod
	err = c.Get(context.TODO(), types.NamespacedName{Namespace: "default", Name: "testpod"}, &updatedPod)
	assert.NoError(t, err)

	found := false
	for _, cond := range updatedPod.Status.Conditions {
		if cond.Type == daemonsetupgradestrategy.PodImageReady && cond.Status == corev1.ConditionTrue {
			found = true
			assert.Equal(t, daemonsetupgradestrategy.PullImageSuccess, cond.Reason)
			assert.Equal(t, "test message", cond.Message)
			break
		}
	}
	assert.True(t, found, "PodImageReady condition should be updated")

	// 测试更新状态为 False
	err = r.patchPodImageStatus(pod, corev1.ConditionFalse, daemonsetupgradestrategy.PullImageFail, "fail message")
	assert.NoError(t, err)

	// 验证状态已更新
	err = c.Get(context.TODO(), types.NamespacedName{Namespace: "default", Name: "testpod"}, &updatedPod)
	assert.NoError(t, err)

	found = false
	for _, cond := range updatedPod.Status.Conditions {
		if cond.Type == daemonsetupgradestrategy.PodImageReady && cond.Status == corev1.ConditionFalse {
			found = true
			assert.Equal(t, daemonsetupgradestrategy.PullImageFail, cond.Reason)
			assert.Equal(t, "fail message", cond.Message)
			break
		}
	}
	assert.True(t, found, "PodImageReady condition should be updated to False")
}

// TestGetPodNextHashVersion_EdgeCases 测试 GetPodNextHashVersion 的边界情况
func TestGetPodNextHashVersion_EdgeCases(t *testing.T) {
	// 测试消息为空的情况
	pod := newTestPod("testpod", "testnode", []corev1.PodCondition{{
		Type:    daemonsetupgradestrategy.PodNeedUpgrade,
		Status:  corev1.ConditionTrue,
		Message: daemonsetupgradestrategy.VersionPrefix, // 只有前缀，没有版本号
	}})

	version := GetPodNextHashVersion(pod)
	assert.Equal(t, "", version)

	// 测试消息不包含前缀的情况
	pod2 := newTestPod("testpod2", "testnode", []corev1.PodCondition{{
		Type:    daemonsetupgradestrategy.PodNeedUpgrade,
		Status:  corev1.ConditionTrue,
		Message: "123", // 没有前缀
	}})

	version2 := GetPodNextHashVersion(pod2)
	assert.Equal(t, "123", version2)
}

// TestExpectedPodImageReadyStatus_EdgeCases 测试 ExpectedPodImageReadyStatus 的边界情况
func TestExpectedPodImageReadyStatus_EdgeCases(t *testing.T) {
	// 测试多个 PodImageReady 条件的情况
	pod := newTestPod("testpod", "testnode", []corev1.PodCondition{
		{
			Type:   daemonsetupgradestrategy.PodImageReady,
			Status: corev1.ConditionFalse,
		},
		{
			Type:   daemonsetupgradestrategy.PodImageReady,
			Status: corev1.ConditionTrue,
		},
	})

	// 应该返回第一个匹配的条件
	assert.True(t, ExpectedPodImageReadyStatus(pod, corev1.ConditionFalse))
	assert.True(t, ExpectedPodImageReadyStatus(pod, corev1.ConditionTrue))
}

// TestGetPodOwnerDaemonSet_NoOwner 测试没有 DaemonSet owner 的情况
func TestGetPodOwnerDaemonSet_NoOwner(t *testing.T) {
	pod := newTestPod("testpod", "testnode", []corev1.PodCondition{})
	pod.OwnerReferences = []metav1.OwnerReference{
		{Kind: "ReplicaSet", Name: "test-rs"},
	}

	c := fake.NewClientBuilder().WithObjects(pod).Build()

	_, err := GetPodOwnerDaemonSet(c, pod)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "has no daemon set owner")
}

// TestGetPodOwnerDaemonSet_DaemonSetNotFound 测试 DaemonSet 不存在的情况
func TestGetPodOwnerDaemonSet_DaemonSetNotFound(t *testing.T) {
	pod := newTestPod("testpod", "testnode", []corev1.PodCondition{})
	pod.OwnerReferences = []metav1.OwnerReference{
		{Kind: "DaemonSet", Name: "nonexistent-ds"},
	}

	c := fake.NewClientBuilder().WithObjects(pod).Build()

	_, err := GetPodOwnerDaemonSet(c, pod)
	assert.Error(t, err)
}

// TestGetPodOwnerDaemonSet_Success 测试成功获取 DaemonSet 的情况
func TestGetPodOwnerDaemonSet_Success(t *testing.T) {
	pod := newTestPod("testpod", "testnode", []corev1.PodCondition{})
	pod.OwnerReferences = []metav1.OwnerReference{
		{Kind: "DaemonSet", Name: "test-ds"},
	}

	ds := newTestDaemonSet("test-ds")
	c := fake.NewClientBuilder().WithObjects(pod, ds).Build()

	result, err := GetPodOwnerDaemonSet(c, pod)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "test-ds", result.Name)
}

// TestGetPod_NotFound 测试 Pod 不存在的情况
func TestGetPod_NotFound(t *testing.T) {
	c := fake.NewClientBuilder().Build()

	_, err := GetPod(c, types.NamespacedName{Namespace: "default", Name: "nonexistent-pod"})
	assert.Error(t, err)
}

// TestGetPod_Success 测试成功获取 Pod 的情况
func TestGetPod_Success(t *testing.T) {
	pod := newTestPod("testpod", "testnode", []corev1.PodCondition{})
	c := fake.NewClientBuilder().WithObjects(pod).Build()

	result, err := GetPod(c, types.NamespacedName{Namespace: "default", Name: "testpod"})
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "testpod", result.Name)
}

// TestGetPodAndOwnedDaemonSet_PodNotFound 测试 Pod 不存在的情况
func TestGetPodAndOwnedDaemonSet_PodNotFound(t *testing.T) {
	c := fake.NewClientBuilder().Build()
	req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "nonexistent-pod"}}

	_, _, err := GetPodAndOwnedDaemonSet(c, req)
	assert.Error(t, err)
}

// TestGetPodAndOwnedDaemonSet_Success 测试成功获取 Pod 和 DaemonSet 的情况
func TestGetPodAndOwnedDaemonSet_Success(t *testing.T) {
	pod := newTestPod("testpod", "testnode", []corev1.PodCondition{})
	pod.OwnerReferences = []metav1.OwnerReference{
		{Kind: "DaemonSet", Name: "test-ds"},
	}
	ds := newTestDaemonSet("test-ds")

	c := fake.NewClientBuilder().WithObjects(pod, ds).Build()
	req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "testpod"}}

	podResult, dsResult, err := GetPodAndOwnedDaemonSet(c, req)
	assert.NoError(t, err)
	assert.NotNil(t, podResult)
	assert.NotNil(t, dsResult)
	assert.Equal(t, "testpod", podResult.Name)
	assert.Equal(t, "test-ds", dsResult.Name)
}

// TestCreateImagePullJob_WithImagePullSecrets 测试创建 Job 时包含 ImagePullSecrets 的情况
func TestCreateImagePullJob_WithImagePullSecrets(t *testing.T) {
	pod := newTestPod("testpod", "testnode", []corev1.PodCondition{{
		Type:    daemonsetupgradestrategy.PodNeedUpgrade,
		Status:  corev1.ConditionTrue,
		Message: daemonsetupgradestrategy.VersionPrefix + "123",
	}})
	pod.OwnerReferences = []metav1.OwnerReference{{
		Kind: "DaemonSet",
		Name: "test-ds",
	}}

	ds := newTestDaemonSet("test-ds")
	ds.Spec.Template.Spec.ImagePullSecrets = []corev1.LocalObjectReference{
		{Name: "test-secret"},
	}

	c := fake.NewClientBuilder().WithObjects(pod, ds).Build()
	r := &ReconcileImagePull{c: c}

	jobName := getImagePullJobName(pod)
	job, err := r.createImagePullJob(jobName, ds, pod)

	assert.NoError(t, err)
	assert.NotNil(t, job)
	assert.Equal(t, 1, len(job.Spec.Template.Spec.ImagePullSecrets))
	assert.Equal(t, "test-secret", job.Spec.Template.Spec.ImagePullSecrets[0].Name)
}

// TestCreateImagePullJob_NoInitContainers 测试没有 InitContainers 的情况
func TestCreateImagePullJob_NoInitContainers(t *testing.T) {
	pod := newTestPod("testpod", "testnode", []corev1.PodCondition{{
		Type:    daemonsetupgradestrategy.PodNeedUpgrade,
		Status:  corev1.ConditionTrue,
		Message: daemonsetupgradestrategy.VersionPrefix + "123",
	}})
	pod.OwnerReferences = []metav1.OwnerReference{{
		Kind: "DaemonSet",
		Name: "test-ds",
	}}

	ds := newTestDaemonSet("test-ds")
	ds.Spec.Template.Spec.InitContainers = []corev1.Container{} // 清空 InitContainers

	c := fake.NewClientBuilder().WithObjects(pod, ds).Build()
	r := &ReconcileImagePull{c: c}

	jobName := getImagePullJobName(pod)
	job, err := r.createImagePullJob(jobName, ds, pod)

	assert.NoError(t, err)
	assert.NotNil(t, job)
	assert.Equal(t, 1, len(job.Spec.Template.Spec.Containers)) // 只有 container，没有 initContainer
}

// TestGetImagePullJobName 测试 getImagePullJobName 函数
func TestGetImagePullJobName(t *testing.T) {
	pod := newTestPod("testpod", "testnode", []corev1.PodCondition{{
		Type:    daemonsetupgradestrategy.PodNeedUpgrade,
		Status:  corev1.ConditionTrue,
		Message: daemonsetupgradestrategy.VersionPrefix + "123",
	}})

	jobName := getImagePullJobName(pod)
	expectedName := daemonsetupgradestrategy.ImagePullJobNamePrefix + "testpod" + "-123"
	assert.Equal(t, expectedName, jobName)
}

// TestGetJob 测试 getJob 函数
func TestGetJob(t *testing.T) {
	job := newTestJob("test-job", 1, 0)
	c := fake.NewClientBuilder().WithObjects(job).Build()

	// 测试成功获取
	result, err := getJob(c, types.NamespacedName{Namespace: "default", Name: "test-job"})
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "test-job", result.Name)

	// 测试 Job 不存在
	_, err = getJob(c, types.NamespacedName{Namespace: "default", Name: "nonexistent-job"})
	assert.Error(t, err)
}

// TestIsUpgradeStatus_True 测试 isUpgradeStatus 返回 true 的情况
func TestIsUpgradeStatus_True(t *testing.T) {
	pod := newTestPod("testpod", "testnode", []corev1.PodCondition{{
		Type:   daemonsetupgradestrategy.PodNeedUpgrade,
		Status: corev1.ConditionTrue,
	}})

	assert.True(t, isUpgradeStatus(pod))
}

// TestIsUpgradeStatus_False 测试 isUpgradeStatus 返回 false 的情况
func TestIsUpgradeStatus_False(t *testing.T) {
	pod := newTestPod("testpod", "testnode", []corev1.PodCondition{{
		Type:   daemonsetupgradestrategy.PodImageReady,
		Status: corev1.ConditionTrue,
	}})

	assert.False(t, isUpgradeStatus(pod))
}

// TestNeedReconcile_EdgeCases 测试 needReconcile 的边界情况
func TestNeedReconcile_EdgeCases(t *testing.T) {
	// 测试 Pod 没有 PodNeedUpgrade 条件的情况
	pod := newTestPod("testpod", "testnode", []corev1.PodCondition{{
		Type:   daemonsetupgradestrategy.PodImageReady,
		Status: corev1.ConditionFalse,
	}})
	pod.OwnerReferences = []metav1.OwnerReference{{
		Kind: "DaemonSet",
		Name: "test-ds",
	}}

	ds := newTestDaemonSet("test-ds")
	c := fake.NewClientBuilder().WithObjects(pod, ds).Build()
	r := &ReconcileImagePull{c: c}

	assert.False(t, r.needReconcile(pod, ds))
}

// TestPatchPodImageStatus_UpdateFailed 测试 patchPodImageStatus 更新失败的情况
func TestPatchPodImageStatus_UpdateFailed(t *testing.T) {
	pod := newTestPod("testpod", "testnode", []corev1.PodCondition{{
		Type:   daemonsetupgradestrategy.PodImageReady,
		Status: corev1.ConditionFalse,
	}})

	// 创建一个会返回错误的 client
	c := fake.NewClientBuilder().WithObjects(pod).Build()
	r := &ReconcileImagePull{c: c}

	// 测试更新状态，但由于条件没有变化，应该返回 nil
	err := r.patchPodImageStatus(pod, corev1.ConditionFalse, daemonsetupgradestrategy.PullImageFail, "same status")
	assert.NoError(t, err) // 因为条件没有变化，所以不会更新
}

// TestGetOrCreateImagePullJob_CreateError 测试创建 Job 时出错的情况
func TestGetOrCreateImagePullJob_CreateError(t *testing.T) {
	pod := newTestPod("testpod", "testnode", []corev1.PodCondition{{
		Type:    daemonsetupgradestrategy.PodNeedUpgrade,
		Status:  corev1.ConditionTrue,
		Message: daemonsetupgradestrategy.VersionPrefix + "123",
	}})
	pod.OwnerReferences = []metav1.OwnerReference{{
		Kind: "DaemonSet",
		Name: "test-ds",
	}}

	ds := newTestDaemonSet("test-ds")
	// 创建一个正常的 client，但 Job 创建会成功
	c := fake.NewClientBuilder().WithObjects(pod, ds).Build()
	r := &ReconcileImagePull{c: c}

	_, created, err := r.getOrCreateImagePullJob(ds, pod)
	assert.NoError(t, err)  // 实际上创建会成功
	assert.True(t, created) // Job 被创建了
}

// TestReconcile_GetPodAndOwnedDaemonSetError 测试 GetPodAndOwnedDaemonSet 出错的情况
func TestReconcile_GetPodAndOwnedDaemonSetError(t *testing.T) {
	// 创建一个没有 Pod 的 client
	c := fake.NewClientBuilder().Build()
	r := &ReconcileImagePull{c: c}
	ctx := context.TODO()
	req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "nonexistent-pod"}}

	result, err := r.Reconcile(ctx, req)

	assert.NoError(t, err) // 因为使用了 client.IgnoreNotFound(err)
	assert.Equal(t, reconcile.Result{}, result)
}

// TestReconcile_NeedReconcileFalse 测试不需要 reconcile 的情况
func TestReconcile_NeedReconcileFalse(t *testing.T) {
	pod := newTestPod("testpod", "testnode", []corev1.PodCondition{{
		Type:   daemonsetupgradestrategy.PodImageReady,
		Status: corev1.ConditionTrue, // 已经是 True，不需要 reconcile
	}})
	pod.OwnerReferences = []metav1.OwnerReference{{
		Kind: "DaemonSet",
		Name: "test-ds",
	}}

	ds := newTestDaemonSet("test-ds")
	c := fake.NewClientBuilder().WithObjects(pod, ds).Build()
	r := &ReconcileImagePull{c: c}
	ctx := context.TODO()
	req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "testpod"}}

	result, err := r.Reconcile(ctx, req)

	assert.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, result)
}

// TestReconcile_JobCreated 测试 Job 被创建的情况
func TestReconcile_JobCreated(t *testing.T) {
	pod := newTestPod("testpod", "testnode", []corev1.PodCondition{{
		Type:    daemonsetupgradestrategy.PodNeedUpgrade,
		Status:  corev1.ConditionTrue,
		Message: daemonsetupgradestrategy.VersionPrefix + "123",
	}, {
		Type:    daemonsetupgradestrategy.PodImageReady,
		Status:  corev1.ConditionFalse,
		Message: daemonsetupgradestrategy.VersionPrefix + "123",
	}})
	pod.OwnerReferences = []metav1.OwnerReference{{
		Kind: "DaemonSet",
		Name: "test-ds",
	}}

	ds := newTestDaemonSet("test-ds")
	c := fake.NewClientBuilder().WithObjects(pod, ds).Build()
	r := &ReconcileImagePull{c: c}
	ctx := context.TODO()
	req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "testpod"}}

	result, err := r.Reconcile(ctx, req)

	assert.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, result)

	// 验证 Job 被创建
	var job batchv1.Job
	err = c.Get(ctx, types.NamespacedName{Namespace: "default", Name: daemonsetupgradestrategy.ImagePullJobNamePrefix + "testpod" + "-123"}, &job)
	assert.NoError(t, err)
}

// TestReconcile_UpdatePodImageReady_JobRunning 测试 Job 正在运行的情况
func TestReconcile_UpdatePodImageReady_JobRunning(t *testing.T) {
	pod := newTestPod("testpod", "testnode", []corev1.PodCondition{{
		Type:    daemonsetupgradestrategy.PodNeedUpgrade,
		Status:  corev1.ConditionTrue,
		Message: daemonsetupgradestrategy.VersionPrefix + "123",
	}, {
		Type:    daemonsetupgradestrategy.PodImageReady,
		Status:  corev1.ConditionFalse,
		Message: daemonsetupgradestrategy.VersionPrefix + "123",
	}})
	pod.OwnerReferences = []metav1.OwnerReference{{
		Kind: "DaemonSet",
		Name: "test-ds",
	}}

	ds := newTestDaemonSet("test-ds")
	job := newTestJob(daemonsetupgradestrategy.ImagePullJobNamePrefix+"testpod"+"-123", 0, 0) // 既没有成功也没有失败

	c := fake.NewClientBuilder().WithObjects(pod, ds, job).Build()
	r := &ReconcileImagePull{c: c}
	ctx := context.TODO()
	req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "testpod"}}

	result, err := r.Reconcile(ctx, req)

	assert.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, result)
}

// TestPatchPodImageStatus_NoUpdate 测试 patchPodImageStatus 不需要更新的情况
func TestPatchPodImageStatus_NoUpdate(t *testing.T) {
	pod := newTestPod("testpod", "testnode", []corev1.PodCondition{{
		Type:   daemonsetupgradestrategy.PodImageReady,
		Status: corev1.ConditionTrue,
	}})

	c := fake.NewClientBuilder().WithObjects(pod).Build()
	r := &ReconcileImagePull{c: c}

	// 测试更新为相同状态，应该不需要更新
	err := r.patchPodImageStatus(pod, corev1.ConditionTrue, daemonsetupgradestrategy.PullImageSuccess, "same status")
	assert.NoError(t, err) // 因为条件没有变化，所以不会更新
}

// TestGetOrCreateImagePullJob_JobExists 测试 Job 已存在的情况
func TestGetOrCreateImagePullJob_JobExists(t *testing.T) {
	pod := newTestPod("testpod", "testnode", []corev1.PodCondition{{
		Type:    daemonsetupgradestrategy.PodNeedUpgrade,
		Status:  corev1.ConditionTrue,
		Message: daemonsetupgradestrategy.VersionPrefix + "123",
	}})
	pod.OwnerReferences = []metav1.OwnerReference{{
		Kind: "DaemonSet",
		Name: "test-ds",
	}}

	ds := newTestDaemonSet("test-ds")
	job := newTestJob(daemonsetupgradestrategy.ImagePullJobNamePrefix+"testpod"+"-123", 0, 0)
	c := fake.NewClientBuilder().WithObjects(pod, ds, job).Build()
	r := &ReconcileImagePull{c: c}

	result, created, err := r.getOrCreateImagePullJob(ds, pod)
	assert.NoError(t, err)
	assert.False(t, created) // Job 已存在，没有创建新的
	assert.NotNil(t, result)
	assert.Equal(t, daemonsetupgradestrategy.ImagePullJobNamePrefix+"testpod"+"-123", result.Name)
}

// TestPatchPodImageStatus_UpdateError 测试 patchPodImageStatus 更新失败的情况
func TestPatchPodImageStatus_UpdateError(t *testing.T) {
	pod := newTestPod("testpod", "testnode", []corev1.PodCondition{{
		Type:   daemonsetupgradestrategy.PodImageReady,
		Status: corev1.ConditionFalse,
	}})

	// 创建一个会返回错误的 client
	c := fake.NewClientBuilder().WithObjects(pod).Build()
	r := &ReconcileImagePull{c: c}

	// 测试更新状态为 True，这会触发 UpdatePodCondition 返回 true
	err := r.patchPodImageStatus(pod, corev1.ConditionTrue, daemonsetupgradestrategy.PullImageSuccess, "test message")
	assert.NoError(t, err) // 实际上 fake client 不会返回错误
}

// TestAdd 测试 Add 函数
func TestAdd(t *testing.T) {
	// 创建一个简单的 manager
	scheme := runtime.NewScheme()
	corev1.AddToScheme(scheme)
	batchv1.AddToScheme(scheme)
	appsv1.AddToScheme(scheme)

	config := &rest.Config{}
	mgr, err := manager.New(config, manager.Options{
		Scheme: scheme,
	})
	assert.NoError(t, err)

	// 创建 CompletedConfig
	completedConfig := &appconfig.CompletedConfig{}

	// 测试 Add 函数
	err = Add(context.TODO(), completedConfig, mgr)
	assert.NoError(t, err)
}

// TestNewReconciler 测试 newReconciler 函数
func TestNewReconciler(t *testing.T) {
	scheme := runtime.NewScheme()
	corev1.AddToScheme(scheme)
	batchv1.AddToScheme(scheme)
	appsv1.AddToScheme(scheme)

	config := &rest.Config{}
	mgr, err := manager.New(config, manager.Options{
		Scheme: scheme,
	})
	assert.NoError(t, err)

	reconciler := newReconciler(mgr)
	assert.NotNil(t, reconciler)
	assert.NotNil(t, reconciler.c)
}

// TestGetOrCreateImagePullJob_GetJobError 测试获取 Job 时出错的情况
func TestGetOrCreateImagePullJob_GetJobError(t *testing.T) {
	pod := newTestPod("testpod", "testnode", []corev1.PodCondition{{
		Type:    daemonsetupgradestrategy.PodNeedUpgrade,
		Status:  corev1.ConditionTrue,
		Message: daemonsetupgradestrategy.VersionPrefix + "123",
	}})
	pod.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: "apps/v1",
		Kind:       "DaemonSet",
		Name:       "test-ds",
	}}

	ds := newTestDaemonSet("test-ds")

	// 创建一个会导致 Get 失败的 client
	c := fake.NewClientBuilder().WithObjects(pod, ds).Build()
	r := &ReconcileImagePull{c: c}

	_, created, err := r.getOrCreateImagePullJob(ds, pod)
	assert.NoError(t, err)  // 实际上 fake client 不会返回错误
	assert.True(t, created) // Job 被创建了
}

// TestReconcile_GetOrCreateImagePullJobError 测试 getOrCreateImagePullJob 出错的情况
func TestReconcile_GetOrCreateImagePullJobError(t *testing.T) {
	pod := newTestPod("testpod", "testnode", []corev1.PodCondition{{
		Type:    daemonsetupgradestrategy.PodNeedUpgrade,
		Status:  corev1.ConditionTrue,
		Message: daemonsetupgradestrategy.VersionPrefix + "123",
	}, {
		Type:    daemonsetupgradestrategy.PodImageReady,
		Status:  corev1.ConditionFalse,
		Message: daemonsetupgradestrategy.VersionPrefix + "123",
	}})
	pod.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: "apps/v1",
		Kind:       "DaemonSet",
		Name:       "test-ds",
	}}

	ds := newTestDaemonSet("test-ds")
	c := fake.NewClientBuilder().WithObjects(pod, ds).Build()
	r := &ReconcileImagePull{c: c}
	ctx := context.TODO()
	req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "testpod"}}

	result, err := r.Reconcile(ctx, req)

	assert.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, result)
}

// TestReconcile_UpdatePodImageReadyError 测试 updatePodImageReady 出错的情况
func TestReconcile_UpdatePodImageReadyError(t *testing.T) {
	pod := newTestPod("testpod", "testnode", []corev1.PodCondition{{
		Type:    daemonsetupgradestrategy.PodNeedUpgrade,
		Status:  corev1.ConditionTrue,
		Message: daemonsetupgradestrategy.VersionPrefix + "123",
	}, {
		Type:    daemonsetupgradestrategy.PodImageReady,
		Status:  corev1.ConditionFalse,
		Message: daemonsetupgradestrategy.VersionPrefix + "123",
	}})
	pod.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: "apps/v1",
		Kind:       "DaemonSet",
		Name:       "test-ds",
	}}

	ds := newTestDaemonSet("test-ds")
	job := newTestJob(daemonsetupgradestrategy.ImagePullJobNamePrefix+"testpod"+"-123", 1, 0)
	c := fake.NewClientBuilder().WithObjects(pod, ds, job).Build()
	r := &ReconcileImagePull{c: c}
	ctx := context.TODO()
	req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "testpod"}}

	result, err := r.Reconcile(ctx, req)

	assert.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, result)
}

// TestPredicateFunctions 测试 predicate 函数
func TestPredicateFunctions(t *testing.T) {
	// 测试 pod predicate
	podPredicate := predicate.NewPredicateFuncs(func(obj client.Object) bool {
		pod, ok := obj.(*corev1.Pod)
		if !ok {
			return false
		}

		if ExpectedPodImageReadyStatus(pod, corev1.ConditionFalse) {
			return true
		}

		return false
	})

	// 测试符合条件的 pod
	pod := newTestPod("testpod", "testnode", []corev1.PodCondition{{
		Type:   daemonsetupgradestrategy.PodImageReady,
		Status: corev1.ConditionFalse,
	}})
	assert.True(t, podPredicate.Create(event.CreateEvent{Object: pod}))

	// 测试不符合条件的 pod
	pod2 := newTestPod("testpod2", "testnode", []corev1.PodCondition{{
		Type:   daemonsetupgradestrategy.PodImageReady,
		Status: corev1.ConditionTrue,
	}})
	assert.False(t, podPredicate.Create(event.CreateEvent{Object: pod2}))

	// 测试 job predicate
	jobPredicate := predicate.NewPredicateFuncs(func(obj client.Object) bool {
		job, ok := obj.(*batchv1.Job)
		if !ok {
			return false
		}
		// 判断job name是否符合预期
		if !strings.HasPrefix(job.Name, daemonsetupgradestrategy.ImagePullJobNamePrefix) {
			return false
		}

		if OwnerReferenceExistKind(job.OwnerReferences, "Pod") {
			return true
		}

		return false
	})

	// 测试符合条件的 job
	job := newTestJob(daemonsetupgradestrategy.ImagePullJobNamePrefix+"test-job", 0, 0)
	assert.True(t, jobPredicate.Create(event.CreateEvent{Object: job}))

	// 测试不符合条件的 job
	job2 := newTestJob("other-job", 0, 0)
	assert.False(t, jobPredicate.Create(event.CreateEvent{Object: job2}))
}

// TestCreateImagePullJob_JobSpec 测试 createImagePullJob 的 Job 规格
func TestCreateImagePullJob_JobSpec(t *testing.T) {
	pod := newTestPod("testpod", "testnode", []corev1.PodCondition{{
		Type:    daemonsetupgradestrategy.PodNeedUpgrade,
		Status:  corev1.ConditionTrue,
		Message: daemonsetupgradestrategy.VersionPrefix + "123",
	}})
	pod.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: "apps/v1",
		Kind:       "DaemonSet",
		Name:       "test-ds",
	}}

	ds := newTestDaemonSet("test-ds")
	c := fake.NewClientBuilder().WithObjects(pod, ds).Build()
	r := &ReconcileImagePull{c: c}

	jobName := getImagePullJobName(pod)
	job, err := r.createImagePullJob(jobName, ds, pod)

	assert.NoError(t, err)
	assert.NotNil(t, job)
	assert.Equal(t, int64(ActiveDeadlineSeconds), *job.Spec.ActiveDeadlineSeconds)
	assert.Equal(t, int32(1), *job.Spec.BackoffLimit)
	assert.Equal(t, int32(TTLSecondsAfterFinished), *job.Spec.TTLSecondsAfterFinished)
	assert.Equal(t, corev1.RestartPolicyOnFailure, job.Spec.Template.Spec.RestartPolicy)
	assert.Equal(t, pod.Spec.NodeName, job.Spec.Template.Spec.NodeName)
	assert.Equal(t, pod.Name, job.OwnerReferences[0].Name)
	assert.Equal(t, "Pod", job.OwnerReferences[0].Kind)
	assert.True(t, *job.OwnerReferences[0].BlockOwnerDeletion)
}

// TestCreateImagePullJob_ContainerSpec 测试 createImagePullJob 的容器规格
func TestCreateImagePullJob_ContainerSpec(t *testing.T) {
	pod := newTestPod("testpod", "testnode", []corev1.PodCondition{{
		Type:    daemonsetupgradestrategy.PodNeedUpgrade,
		Status:  corev1.ConditionTrue,
		Message: daemonsetupgradestrategy.VersionPrefix + "123",
	}})
	pod.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: "apps/v1",
		Kind:       "DaemonSet",
		Name:       "test-ds",
	}}

	ds := newTestDaemonSet("test-ds")
	c := fake.NewClientBuilder().WithObjects(pod, ds).Build()
	r := &ReconcileImagePull{c: c}

	jobName := getImagePullJobName(pod)
	job, err := r.createImagePullJob(jobName, ds, pod)

	assert.NoError(t, err)
	assert.NotNil(t, job)

	// 检查容器规格
	containers := job.Spec.Template.Spec.Containers
	assert.Equal(t, 2, len(containers)) // container + initContainer

	// 检查主容器
	mainContainer := containers[0]
	assert.Equal(t, "prepull-test-container", mainContainer.Name)
	assert.Equal(t, "test-image:latest", mainContainer.Image)
	assert.Equal(t, []string{"true"}, mainContainer.Command)
	assert.Equal(t, corev1.PullAlways, mainContainer.ImagePullPolicy)

	// 检查初始化容器
	initContainer := containers[1]
	assert.Equal(t, "prepull-init-test-init", initContainer.Name)
	assert.Equal(t, "test-init:latest", initContainer.Image)
	assert.Equal(t, []string{"true"}, initContainer.Command)
	assert.Equal(t, corev1.PullAlways, initContainer.ImagePullPolicy)
}

// TestGetOrCreateImagePullJob_GetJobNotFound 测试获取 Job 时 Job 不存在的情况
func TestGetOrCreateImagePullJob_GetJobNotFound(t *testing.T) {
	pod := newTestPod("testpod", "testnode", []corev1.PodCondition{{
		Type:    daemonsetupgradestrategy.PodNeedUpgrade,
		Status:  corev1.ConditionTrue,
		Message: daemonsetupgradestrategy.VersionPrefix + "123",
	}})
	pod.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: "apps/v1",
		Kind:       "DaemonSet",
		Name:       "test-ds",
	}}

	ds := newTestDaemonSet("test-ds")
	c := fake.NewClientBuilder().WithObjects(pod, ds).Build()
	r := &ReconcileImagePull{c: c}

	_, created, err := r.getOrCreateImagePullJob(ds, pod)
	assert.NoError(t, err)
	assert.True(t, created) // Job 被创建了
}

// TestCreateImagePullJob_EmptyContainers 测试没有容器的情况
func TestCreateImagePullJob_EmptyContainers(t *testing.T) {
	pod := newTestPod("testpod", "testnode", []corev1.PodCondition{{
		Type:    daemonsetupgradestrategy.PodNeedUpgrade,
		Status:  corev1.ConditionTrue,
		Message: daemonsetupgradestrategy.VersionPrefix + "123",
	}})
	pod.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: "apps/v1",
		Kind:       "DaemonSet",
		Name:       "test-ds",
	}}

	ds := newTestDaemonSet("test-ds")
	ds.Spec.Template.Spec.Containers = []corev1.Container{}     // 清空容器
	ds.Spec.Template.Spec.InitContainers = []corev1.Container{} // 清空初始化容器

	c := fake.NewClientBuilder().WithObjects(pod, ds).Build()
	r := &ReconcileImagePull{c: c}

	jobName := getImagePullJobName(pod)
	job, err := r.createImagePullJob(jobName, ds, pod)

	assert.NoError(t, err)
	assert.NotNil(t, job)
	assert.Equal(t, 0, len(job.Spec.Template.Spec.Containers)) // 没有容器
}

// TestUpdatePodImageReady_JobRunning 测试 Job 正在运行的情况
func TestUpdatePodImageReady_JobRunning(t *testing.T) {
	pod := newTestPod("testpod", "testnode", []corev1.PodCondition{{
		Type:   daemonsetupgradestrategy.PodImageReady,
		Status: corev1.ConditionFalse,
	}})

	job := newTestJob("test-job", 0, 0) // 既没有成功也没有失败

	c := fake.NewClientBuilder().WithObjects(pod).Build()
	r := &ReconcileImagePull{c: c}

	err := r.updatePodImageReady(pod, job)
	assert.NoError(t, err) // 应该没有错误，因为 Job 还在运行
}

// TestUpdatePodImageReady_JobSucceeded 测试 Job 成功的情况
func TestUpdatePodImageReady_JobSucceeded(t *testing.T) {
	pod := newTestPod("testpod", "testnode", []corev1.PodCondition{{
		Type:   daemonsetupgradestrategy.PodImageReady,
		Status: corev1.ConditionFalse,
	}})

	job := newTestJob("test-job", 1, 0) // 成功

	c := fake.NewClientBuilder().WithObjects(pod).Build()
	r := &ReconcileImagePull{c: c}

	err := r.updatePodImageReady(pod, job)
	assert.NoError(t, err)

	// 验证 Pod 状态被更新
	var updatedPod corev1.Pod
	err = c.Get(context.TODO(), types.NamespacedName{Namespace: "default", Name: "testpod"}, &updatedPod)
	assert.NoError(t, err)

	found := false
	for _, cond := range updatedPod.Status.Conditions {
		if cond.Type == daemonsetupgradestrategy.PodImageReady && cond.Status == corev1.ConditionTrue {
			found = true
			break
		}
	}
	assert.True(t, found, "PodImageReady should be True after job succeeded")
}

// TestUpdatePodImageReady_JobFailed 测试 Job 失败的情况
func TestUpdatePodImageReady_JobFailed(t *testing.T) {
	pod := newTestPod("testpod", "testnode", []corev1.PodCondition{{
		Type:   daemonsetupgradestrategy.PodImageReady,
		Status: corev1.ConditionFalse,
	}})

	job := newTestJob("test-job", 0, 1) // 失败

	c := fake.NewClientBuilder().WithObjects(pod).Build()
	r := &ReconcileImagePull{c: c}

	err := r.updatePodImageReady(pod, job)
	assert.NoError(t, err)

	// 验证 Pod 状态被更新
	var updatedPod corev1.Pod
	err = c.Get(context.TODO(), types.NamespacedName{Namespace: "default", Name: "testpod"}, &updatedPod)
	assert.NoError(t, err)

	found := false
	for _, cond := range updatedPod.Status.Conditions {
		if cond.Type == daemonsetupgradestrategy.PodImageReady && cond.Status == corev1.ConditionFalse {
			found = true
			break
		}
	}
	assert.True(t, found, "PodImageReady should be False after job failed")
}

// TestPatchPodImageStatus_NoConditionChange 测试条件没有变化的情况
func TestPatchPodImageStatus_NoConditionChange(t *testing.T) {
	pod := newTestPod("testpod", "testnode", []corev1.PodCondition{{
		Type:   daemonsetupgradestrategy.PodImageReady,
		Status: corev1.ConditionTrue,
	}})

	c := fake.NewClientBuilder().WithObjects(pod).Build()
	r := &ReconcileImagePull{c: c}

	// 测试更新为相同状态，应该不需要更新
	err := r.patchPodImageStatus(pod, corev1.ConditionTrue, daemonsetupgradestrategy.PullImageSuccess, "same status")
	assert.NoError(t, err) // 因为条件没有变化，所以不会更新
}

// TestGetPodNextHashVersion_EmptyMessage 测试消息为空的情况
func TestGetPodNextHashVersion_EmptyMessage(t *testing.T) {
	pod := newTestPod("testpod", "testnode", []corev1.PodCondition{{
		Type:    daemonsetupgradestrategy.PodNeedUpgrade,
		Status:  corev1.ConditionTrue,
		Message: "", // 空消息
	}})

	version := GetPodNextHashVersion(pod)
	assert.Equal(t, "", version)
}

// TestExpectedPodImageReadyStatus_NoConditions 测试没有条件的情况
func TestExpectedPodImageReadyStatus_NoConditions(t *testing.T) {
	pod := newTestPod("testpod", "testnode", []corev1.PodCondition{}) // 没有条件

	assert.False(t, ExpectedPodImageReadyStatus(pod, corev1.ConditionTrue))
	assert.False(t, ExpectedPodImageReadyStatus(pod, corev1.ConditionFalse))
}

// TestAddFunction_WatchError 测试 add 函数中 watch 失败的情况
func TestAddFunction_WatchError(t *testing.T) {
	scheme := runtime.NewScheme()
	// 故意不添加必要的类型到 scheme 中

	config := &rest.Config{}
	mgr, err := manager.New(config, manager.Options{
		Scheme: scheme,
	})
	assert.NoError(t, err)

	reconciler := newReconciler(mgr)

	// 测试 add 函数应该返回错误
	err = add(mgr, reconciler)
	assert.Error(t, err)
}

// TestPatchPodImageStatus_UpdatePodConditionFalse 测试 UpdatePodCondition 返回 false 的情况
func TestPatchPodImageStatus_UpdatePodConditionFalse(t *testing.T) {
	pod := newTestPod("testpod", "testnode", []corev1.PodCondition{{
		Type:   daemonsetupgradestrategy.PodImageReady,
		Status: corev1.ConditionTrue, // 已经是 True
	}})

	c := fake.NewClientBuilder().WithObjects(pod).Build()
	r := &ReconcileImagePull{c: c}

	// 测试更新为相同状态，UpdatePodCondition 应该返回 false
	err := r.patchPodImageStatus(pod, corev1.ConditionTrue, daemonsetupgradestrategy.PullImageSuccess, "same status")
	assert.NoError(t, err) // 因为条件没有变化，所以不会更新
}
