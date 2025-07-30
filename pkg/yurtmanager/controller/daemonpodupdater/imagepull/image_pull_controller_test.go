// image_pull_controller_test.go
// 单元测试：ImagePullController

package imagepull

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func newTestPod(name string, node string, conds []corev1.PodCondition) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{{
				Kind: "DaemonSet",
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

func TestReconcile_CreateJobAndPatchPod(t *testing.T) {
	pod := newTestPod("testpod", "testnode", []corev1.PodCondition{{
		Type:   "PodImageReady",
		Status: corev1.ConditionFalse,
		Reason: "imagepull start",
	}})
	c := fake.NewClientBuilder().WithObjects(pod).Build()
	r := &ReconcileImagePull{c: c}
	ctx := context.TODO()
	req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "testpod"}}
	_, err := r.Reconcile(ctx, req)
	assert.NoError(t, err)
	// 检查 Job 是否创建
	var job batchv1.Job
	err = c.Get(ctx, types.NamespacedName{Namespace: "default", Name: "image-pre-pull-testpod"}, &job)
	assert.NoError(t, err)
	assert.Equal(t, "image-pre-pull-testpod", job.Name)
	assert.Equal(t, 2, len(job.Spec.Template.Spec.Containers)) // container + initContainer
}

func TestReconcile_JobSucceeded_PatchPodTrue(t *testing.T) {
	pod := newTestPod("testpod", "testnode", []corev1.PodCondition{{
		Type:   "PodImageReady",
		Status: corev1.ConditionFalse,
		Reason: "imagepull start",
	}})
	job := newTestJob("image-pre-pull-testpod", 1, 0)
	c := fake.NewClientBuilder().WithObjects(pod, job).Build()
	r := &ReconcileImagePull{c: c}
	ctx := context.TODO()
	req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "testpod"}}
	_, err := r.Reconcile(ctx, req)
	assert.NoError(t, err)
	var patchedPod corev1.Pod
	err = c.Get(ctx, types.NamespacedName{Namespace: "default", Name: "testpod"}, &patchedPod)
	assert.NoError(t, err)
	found := false
	for _, cond := range patchedPod.Status.Conditions {
		if cond.Type == "PodImageReady" && cond.Status == corev1.ConditionTrue {
			found = true
		}
	}
	assert.True(t, found, "PodImageReady should be True after job succeeded")
}

func TestReconcile_JobFailed_PatchPodFalse(t *testing.T) {
	pod := newTestPod("testpod", "testnode", []corev1.PodCondition{{
		Type:   "PodImageReady",
		Status: corev1.ConditionFalse,
		Reason: "imagepull start",
	}})
	job := newTestJob("image-pre-pull-testpod", 0, 1)
	c := fake.NewClientBuilder().WithObjects(pod, job).Build()
	r := &ReconcileImagePull{c: c}
	ctx := context.TODO()
	req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "testpod"}}
	_, err := r.Reconcile(ctx, req)
	assert.NoError(t, err)
	var patchedPod corev1.Pod
	err = c.Get(ctx, types.NamespacedName{Namespace: "default", Name: "testpod"}, &patchedPod)
	assert.NoError(t, err)
	found := false
	for _, cond := range patchedPod.Status.Conditions {
		if cond.Type == "PodImageReady" && cond.Status == corev1.ConditionFalse && cond.Reason == "imagepull fail" {
			found = true
		}
	}
	assert.True(t, found, "PodImageReady should be False after job failed")
}
