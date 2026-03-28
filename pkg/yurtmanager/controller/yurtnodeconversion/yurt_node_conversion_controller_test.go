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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	nodeservant "github.com/openyurtio/openyurt/pkg/node-servant"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
	conversionconfig "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/yurtnodeconversion/config"
)

const testWorkingNamespace = "openyurt-test-system"

func TestReconcileCreateConvertJob(t *testing.T) {
	r, cli := newReconcilerForTest(t, newNode("node-a", map[string]string{
		projectinfo.GetNodePoolLabel(): "pool-a",
	}, false, nil))

	_, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "node-a"}})
	require.NoError(t, err)

	node := &corev1.Node{}
	require.NoError(t, cli.Get(context.Background(), types.NamespacedName{Name: "node-a"}, node))
	assert.True(t, node.Spec.Unschedulable)

	cond := getConversionCondition(node)
	require.NotNil(t, cond)
	assert.Equal(t, corev1.ConditionFalse, cond.Status)
	assert.Equal(t, reasonConverting, cond.Reason)

	job := &batchv1.Job{}
	require.NoError(t, cli.Get(context.Background(), types.NamespacedName{
		Namespace: r.conversionJobNamespace(),
		Name:      conversionJobName("node-a"),
	}, job))
	assert.Equal(t, "node-a", job.Labels[nodeservant.ConversionNodeLabelKey])
	assert.Contains(t, job.Spec.Template.Spec.Containers[0].Args[0], "convert")
	assert.Contains(t, job.Spec.Template.Spec.Containers[0].Args[0], "--nodepool-name=pool-a")
}

func TestReconcileConvertSuccess(t *testing.T) {
	node := newNode("node-a", map[string]string{
		projectinfo.GetNodePoolLabel(): "pool-a",
	}, true, newNodeCondition(reasonConverting, corev1.ConditionFalse))
	job := newConversionJobForTest(t, actionConvert, "node-a")
	job.Status.Conditions = []batchv1.JobCondition{{
		Type:   batchv1.JobComplete,
		Status: corev1.ConditionTrue,
	}}
	job.Status.Succeeded = 1

	r, cli := newReconcilerForTest(t, node, job)

	_, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "node-a"}})
	require.NoError(t, err)

	updatedNode := &corev1.Node{}
	require.NoError(t, cli.Get(context.Background(), types.NamespacedName{Name: "node-a"}, updatedNode))
	assert.False(t, updatedNode.Spec.Unschedulable)
	assert.Equal(t, "true", updatedNode.Labels[projectinfo.GetEdgeWorkerLabelKey()])

	cond := getConversionCondition(updatedNode)
	require.NotNil(t, cond)
	assert.Equal(t, corev1.ConditionFalse, cond.Status)
	assert.Equal(t, reasonConverted, cond.Reason)
}

func TestReconcileConvertFailure(t *testing.T) {
	node := newNode("node-a", map[string]string{
		projectinfo.GetNodePoolLabel(): "pool-a",
	}, true, newNodeCondition(reasonConverting, corev1.ConditionFalse))
	job := newConversionJobForTest(t, actionConvert, "node-a")
	job.Status.Conditions = []batchv1.JobCondition{{
		Type:    batchv1.JobFailed,
		Status:  corev1.ConditionTrue,
		Message: "convert failed",
	}}

	r, cli := newReconcilerForTest(t, node, job)

	_, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "node-a"}})
	require.NoError(t, err)

	updatedNode := &corev1.Node{}
	require.NoError(t, cli.Get(context.Background(), types.NamespacedName{Name: "node-a"}, updatedNode))
	assert.True(t, updatedNode.Spec.Unschedulable)
	assert.Empty(t, updatedNode.Labels[projectinfo.GetEdgeWorkerLabelKey()])

	cond := getConversionCondition(updatedNode)
	require.NotNil(t, cond)
	assert.Equal(t, corev1.ConditionTrue, cond.Status)
	assert.Equal(t, reasonConvertFailed, cond.Reason)
	assert.Equal(t, "convert failed", cond.Message)
}

func TestReconcileRecreateConvertJobAfterFailedConditionWhenJobMissing(t *testing.T) {
	node := newNode("node-a", map[string]string{
		projectinfo.GetNodePoolLabel(): "pool-a",
	}, false, newNodeCondition(reasonConvertFailed, corev1.ConditionTrue))

	r, cli := newReconcilerForTest(t, node)

	_, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "node-a"}})
	require.NoError(t, err)

	updatedNode := &corev1.Node{}
	require.NoError(t, cli.Get(context.Background(), types.NamespacedName{Name: "node-a"}, updatedNode))
	assert.True(t, updatedNode.Spec.Unschedulable)

	cond := getConversionCondition(updatedNode)
	require.NotNil(t, cond)
	assert.Equal(t, corev1.ConditionFalse, cond.Status)
	assert.Equal(t, reasonConverting, cond.Reason)

	job := &batchv1.Job{}
	require.NoError(t, cli.Get(context.Background(), types.NamespacedName{
		Namespace: r.conversionJobNamespace(),
		Name:      conversionJobName("node-a"),
	}, job))
	assert.Contains(t, job.Spec.Template.Spec.Containers[0].Args[0], "convert")
}

func TestReconcileCreateRevertJob(t *testing.T) {
	r, cli := newReconcilerForTest(t, newNode("node-a", map[string]string{
		projectinfo.GetEdgeWorkerLabelKey(): "true",
	}, false, nil))

	_, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "node-a"}})
	require.NoError(t, err)

	node := &corev1.Node{}
	require.NoError(t, cli.Get(context.Background(), types.NamespacedName{Name: "node-a"}, node))
	assert.True(t, node.Spec.Unschedulable)

	cond := getConversionCondition(node)
	require.NotNil(t, cond)
	assert.Equal(t, corev1.ConditionFalse, cond.Status)
	assert.Equal(t, reasonReverting, cond.Reason)

	job := &batchv1.Job{}
	require.NoError(t, cli.Get(context.Background(), types.NamespacedName{
		Namespace: r.conversionJobNamespace(),
		Name:      conversionJobName("node-a"),
	}, job))
	assert.Equal(t, "/usr/local/bin/entry.sh revert --node-name=node-a", job.Spec.Template.Spec.Containers[0].Args[0])
}

func TestReconcileRevertSuccess(t *testing.T) {
	node := newNode("node-a", map[string]string{
		projectinfo.GetEdgeWorkerLabelKey(): "true",
	}, true, newNodeCondition(reasonReverting, corev1.ConditionFalse))
	job := newConversionJobForTest(t, actionRevert, "node-a")
	job.Status.Conditions = []batchv1.JobCondition{{
		Type:   batchv1.JobComplete,
		Status: corev1.ConditionTrue,
	}}
	job.Status.Succeeded = 1

	r, cli := newReconcilerForTest(t, node, job)

	_, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "node-a"}})
	require.NoError(t, err)

	updatedNode := &corev1.Node{}
	require.NoError(t, cli.Get(context.Background(), types.NamespacedName{Name: "node-a"}, updatedNode))
	assert.False(t, updatedNode.Spec.Unschedulable)
	assert.NotContains(t, updatedNode.Labels, projectinfo.GetEdgeWorkerLabelKey())

	cond := getConversionCondition(updatedNode)
	require.NotNil(t, cond)
	assert.Equal(t, corev1.ConditionFalse, cond.Status)
	assert.Equal(t, reasonReverted, cond.Reason)
}

func TestReconcileRevertFailure(t *testing.T) {
	node := newNode("node-a", map[string]string{
		projectinfo.GetEdgeWorkerLabelKey(): "true",
	}, true, newNodeCondition(reasonReverting, corev1.ConditionFalse))
	job := newConversionJobForTest(t, actionRevert, "node-a")
	job.Status.Conditions = []batchv1.JobCondition{{
		Type:    batchv1.JobFailed,
		Status:  corev1.ConditionTrue,
		Message: "revert failed",
	}}

	r, cli := newReconcilerForTest(t, node, job)

	_, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "node-a"}})
	require.NoError(t, err)

	updatedNode := &corev1.Node{}
	require.NoError(t, cli.Get(context.Background(), types.NamespacedName{Name: "node-a"}, updatedNode))
	assert.True(t, updatedNode.Spec.Unschedulable)
	assert.Equal(t, "true", updatedNode.Labels[projectinfo.GetEdgeWorkerLabelKey()])

	cond := getConversionCondition(updatedNode)
	require.NotNil(t, cond)
	assert.Equal(t, corev1.ConditionTrue, cond.Status)
	assert.Equal(t, reasonRevertFailed, cond.Reason)
	assert.Equal(t, "revert failed", cond.Message)
}

func TestReconcileResumeInterruptedFinalizationWithoutJob(t *testing.T) {
	node := newNode("node-a", map[string]string{
		projectinfo.GetNodePoolLabel():      "pool-a",
		projectinfo.GetEdgeWorkerLabelKey(): "true",
	}, true, newNodeCondition(reasonConverting, corev1.ConditionFalse))

	r, cli := newReconcilerForTest(t, node)

	_, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "node-a"}})
	require.NoError(t, err)

	updatedNode := &corev1.Node{}
	require.NoError(t, cli.Get(context.Background(), types.NamespacedName{Name: "node-a"}, updatedNode))
	assert.False(t, updatedNode.Spec.Unschedulable)
	assert.Equal(t, "true", updatedNode.Labels[projectinfo.GetEdgeWorkerLabelKey()])

	cond := getConversionCondition(updatedNode)
	require.NotNil(t, cond)
	assert.Equal(t, corev1.ConditionFalse, cond.Status)
	assert.Equal(t, reasonConverted, cond.Reason)
}

func TestReconcileDeleteStaleFinishedJob(t *testing.T) {
	node := newNode("node-a", map[string]string{
		projectinfo.GetEdgeWorkerLabelKey(): "true",
	}, false, nil)
	job := newConversionJobForTest(t, actionConvert, "node-a")
	job.Status.Conditions = []batchv1.JobCondition{{
		Type:   batchv1.JobComplete,
		Status: corev1.ConditionTrue,
	}}
	job.Status.Succeeded = 1

	r, cli := newReconcilerForTest(t, node, job)

	result, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "node-a"}})
	require.NoError(t, err)
	assert.True(t, result.Requeue)

	deletedJob := &batchv1.Job{}
	err = cli.Get(context.Background(), types.NamespacedName{
		Namespace: r.conversionJobNamespace(),
		Name:      conversionJobName("node-a"),
	}, deletedJob)
	assert.Error(t, err)
}

func TestReconcileKeepRunningJobInProgress(t *testing.T) {
	node := newNode("node-a", map[string]string{
		projectinfo.GetNodePoolLabel(): "pool-a",
	}, false, nil)
	job := newConversionJobForTest(t, actionConvert, "node-a")

	r, cli := newReconcilerForTest(t, node, job)

	_, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "node-a"}})
	require.NoError(t, err)

	updatedNode := &corev1.Node{}
	require.NoError(t, cli.Get(context.Background(), types.NamespacedName{Name: "node-a"}, updatedNode))
	assert.True(t, updatedNode.Spec.Unschedulable)

	cond := getConversionCondition(updatedNode)
	require.NotNil(t, cond)
	assert.Equal(t, corev1.ConditionFalse, cond.Status)
	assert.Equal(t, reasonConverting, cond.Reason)
}

func TestReconcileIgnoreDeletingNode(t *testing.T) {
	now := metav1.Now()
	node := newNode("node-a", map[string]string{
		projectinfo.GetNodePoolLabel(): "pool-a",
	}, false, nil)
	node.DeletionTimestamp = &now
	node.Finalizers = []string{"openyurt.io/test-finalizer"}

	r, cli := newReconcilerForTest(t, node)

	_, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "node-a"}})
	require.NoError(t, err)

	job := &batchv1.Job{}
	err = cli.Get(context.Background(), types.NamespacedName{
		Namespace: r.conversionJobNamespace(),
		Name:      conversionJobName("node-a"),
	}, job)
	assert.Error(t, err)
}

func TestJobAction(t *testing.T) {
	convertJob := newConversionJobForTest(t, actionConvert, "node-a")
	action, err := jobAction(convertJob)
	require.NoError(t, err)
	assert.Equal(t, actionConvert, action)

	revertJob := newConversionJobForTest(t, actionRevert, "node-a")
	action, err = jobAction(revertJob)
	require.NoError(t, err)
	assert.Equal(t, actionRevert, action)

	malformedJob := newConversionJobForTest(t, actionRevert, "node-a")
	malformedJob.Spec.Template.Spec.Containers[0].Args = []string{"/usr/local/bin/entry.sh unknown"}
	action, err = jobAction(malformedJob)
	require.Error(t, err)
	assert.Equal(t, actionNone, action)
}

func TestValidateConversionJobRequest(t *testing.T) {
	require.NoError(t, validateConversionJobRequest("node-a", "pool-a", actionConvert, "openyurt/node-servant:latest"))
	require.NoError(t, validateConversionJobRequest("node-a", "", actionRevert, "openyurt/node-servant:latest"))

	err := validateConversionJobRequest("node-a", "", actionConvert, "openyurt/node-servant:latest")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nodepool name is empty")

	err = validateConversionJobRequest("node-a", "pool-a", actionConvert, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "node-servant image is empty")
}

func TestInterruptedFinalizationAction(t *testing.T) {
	node := newNode("node-a", map[string]string{
		projectinfo.GetNodePoolLabel():      "pool-a",
		projectinfo.GetEdgeWorkerLabelKey(): "true",
	}, true, newNodeCondition(reasonConverting, corev1.ConditionFalse))
	assert.Equal(t, actionConvert, interruptedFinalizationAction(node, getConversionCondition(node)))

	node = newNode("node-a", map[string]string{
		projectinfo.GetNodePoolLabel():      "pool-a",
		projectinfo.GetEdgeWorkerLabelKey(): "true",
	}, true, nil)
	assert.Equal(t, actionNone, interruptedFinalizationAction(node, getConversionCondition(node)))

	node = newNode("node-a", nil, true, newNodeCondition(reasonReverting, corev1.ConditionFalse))
	assert.Equal(t, actionRevert, interruptedFinalizationAction(node, getConversionCondition(node)))
}

func TestNodePoolLabelPredicate(t *testing.T) {
	predicate := nodePoolLabelPredicate()

	assert.True(t, predicate.Create(event.CreateEvent{
		Object: newNode("node-a", map[string]string{
			projectinfo.GetNodePoolLabel(): "pool-a",
		}, false, nil),
	}))
	assert.False(t, predicate.Create(event.CreateEvent{
		Object: newNode("node-a", nil, false, nil),
	}))
	assert.True(t, predicate.Create(event.CreateEvent{
		Object: newNode("node-a", map[string]string{
			projectinfo.GetEdgeWorkerLabelKey(): "true",
		}, false, nil),
	}))

	assert.True(t, predicate.Update(event.UpdateEvent{
		ObjectOld: newNode("node-a", nil, false, nil),
		ObjectNew: newNode("node-a", map[string]string{
			projectinfo.GetNodePoolLabel(): "pool-a",
		}, false, nil),
	}))
	assert.True(t, predicate.Update(event.UpdateEvent{
		ObjectOld: newNode("node-a", map[string]string{
			projectinfo.GetNodePoolLabel(): "pool-a",
		}, false, nil),
		ObjectNew: newNode("node-a", nil, false, nil),
	}))
	assert.False(t, predicate.Update(event.UpdateEvent{
		ObjectOld: newNode("node-a", map[string]string{
			projectinfo.GetNodePoolLabel(): "pool-a",
		}, false, nil),
		ObjectNew: newNode("node-a", map[string]string{
			projectinfo.GetNodePoolLabel(): "pool-b",
		}, false, nil),
	}))
	assert.True(t, predicate.Update(event.UpdateEvent{
		ObjectOld: newNode("node-a", nil, false, nil),
		ObjectNew: newNode("node-a", map[string]string{
			projectinfo.GetEdgeWorkerLabelKey(): "true",
		}, false, nil),
	}))
	assert.True(t, predicate.Update(event.UpdateEvent{
		ObjectOld: newNode("node-a", map[string]string{
			projectinfo.GetEdgeWorkerLabelKey(): "true",
		}, false, nil),
		ObjectNew: newNode("node-a", nil, false, nil),
	}))
	assert.False(t, predicate.Update(event.UpdateEvent{
		ObjectOld: newNode("node-a", map[string]string{
			projectinfo.GetEdgeWorkerLabelKey(): "true",
		}, false, nil),
		ObjectNew: newNode("node-a", map[string]string{
			projectinfo.GetEdgeWorkerLabelKey(): "true",
			"example.com/marker":                "stable",
		}, false, nil),
	}))
}

func TestConversionJobPredicate(t *testing.T) {
	predicate := conversionJobPredicate()

	conversionJob := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{
		Name: "job-a",
		Labels: map[string]string{
			nodeservant.ConversionNodeLabelKey: "node-a",
		},
	}}
	plainJob := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "job-b"}}

	assert.True(t, predicate.Create(event.CreateEvent{Object: conversionJob}))
	assert.False(t, predicate.Create(event.CreateEvent{Object: plainJob}))

	assert.True(t, predicate.Update(event.UpdateEvent{
		ObjectOld: conversionJob,
		ObjectNew: plainJob,
	}))
	assert.True(t, predicate.Update(event.UpdateEvent{
		ObjectOld: plainJob,
		ObjectNew: conversionJob,
	}))
	assert.False(t, predicate.Update(event.UpdateEvent{
		ObjectOld: plainJob,
		ObjectNew: plainJob,
	}))

	assert.True(t, predicate.Delete(event.DeleteEvent{Object: conversionJob}))
	assert.False(t, predicate.Delete(event.DeleteEvent{Object: plainJob}))
	assert.True(t, predicate.Generic(event.GenericEvent{Object: conversionJob}))
	assert.False(t, predicate.Generic(event.GenericEvent{Object: plainJob}))
}

func newReconcilerForTest(t *testing.T, objs ...client.Object) (*ReconcileYurtNodeConversion, client.Client) {
	t.Helper()

	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))

	builder := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...)
	builder = builder.WithStatusSubresource(&corev1.Node{})
	cli := builder.Build()

	return &ReconcileYurtNodeConversion{
		Client:           cli,
		nodeServantImage: "openyurt/node-servant:latest",
		jobNamespace:     testWorkingNamespace,
		cfg: conversionconfig.YurtNodeConversionControllerConfiguration{
			ConcurrentYurtNodeConversionWorkers: 1,
		},
	}, cli
}

func newNode(name string, labels map[string]string, unschedulable bool, cond *corev1.NodeCondition) *corev1.Node {
	nodeLabels := map[string]string{}
	for k, v := range labels {
		nodeLabels[k] = v
	}

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: nodeLabels,
		},
		Spec: corev1.NodeSpec{
			Unschedulable: unschedulable,
		},
	}
	if cond != nil {
		node.Status.Conditions = append(node.Status.Conditions, *cond)
	}
	return node
}

func newNodeCondition(reason string, status corev1.ConditionStatus) *corev1.NodeCondition {
	now := metav1.Now()
	return &corev1.NodeCondition{
		Type:               conversionConditionType,
		Status:             status,
		Reason:             reason,
		Message:            strings.ToLower(reason),
		LastHeartbeatTime:  now,
		LastTransitionTime: now,
	}
}

func newConversionJobForTest(t *testing.T, action, nodeName string) *batchv1.Job {
	t.Helper()

	renderCtx := map[string]string{
		"jobNamespace":     testWorkingNamespace,
		"nodeServantImage": "openyurt/node-servant:latest",
	}
	if action == actionConvert {
		renderCtx["nodePoolName"] = "pool-a"
	}

	job, err := nodeservant.RenderNodeServantJob(action, renderCtx, nodeName)
	require.NoError(t, err)
	return job
}
