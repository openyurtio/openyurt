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

package imagepreheat

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appconfig "github.com/openyurtio/openyurt/cmd/yurt-manager/app/config"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/daemonsetupgradestrategy"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/daemonsetupgradestrategy/imagepreheat/config"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/daemonsetupgradestrategy/imagepreheat/dragonfly"
)

func reconcileDF(t *testing.T, r *ReconcileImagePull, node string) reconcile.Result {
	t.Helper()
	res, err := r.Reconcile(context.TODO(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "pod-" + node}})
	require.NoError(t, err)
	return res
}

func imageReadyCond(t *testing.T, c client.Client, node string) corev1.PodCondition {
	t.Helper()
	for _, cond := range getTestPod(t, c, node).Status.Conditions {
		if cond.Type == daemonsetupgradestrategy.PodImageReady {
			return cond
		}
	}
	t.Fatalf("no PodImageReady condition on pod-%s", node)
	return corev1.PodCondition{}
}

func TestReconcile_DragonflyBackend_PendingRequeues(t *testing.T) {
	d, c, _ := newTestPreheater(t, dfStandardObjects()...)
	r := &ReconcileImagePull{c: c, dragonfly: d}

	res := reconcileDF(t, r, "a1")
	assert.Equal(t, 10*time.Second, res.RequeueAfter)

	jobs := &batchv1.JobList{}
	require.NoError(t, c.List(context.TODO(), jobs))
	assert.Empty(t, jobs.Items, "dragonfly backend must not create image pull Jobs")
}

func TestReconcile_DragonflyBackend_SuccessSetsImageReady(t *testing.T) {
	d, c, _ := newTestPreheater(t, dfStandardObjects()...)
	r := &ReconcileImagePull{c: c, dragonfly: d}
	reconcileDF(t, r, "a1")
	setJobState(101, dragonfly.StateSuccess)
	setJobState(102, dragonfly.StateSuccess)

	res := reconcileDF(t, r, "a1")
	assert.Equal(t, reconcile.Result{}, res)
	cond := imageReadyCond(t, c, "a1")
	assert.Equal(t, corev1.ConditionTrue, cond.Status)
	assert.Equal(t, daemonsetupgradestrategy.PullImageSuccess, cond.Reason)
	assert.Equal(t, daemonsetupgradestrategy.VersionPrefix+testRev, cond.Message)
}

func TestReconcile_DragonflyBackend_FailureSetsReason(t *testing.T) {
	d, c, _ := newTestPreheater(t, dfStandardObjects()...)
	r := &ReconcileImagePull{c: c, dragonfly: d}
	reconcileDF(t, r, "a1")
	setJobState(101, dragonfly.StateFailure)
	setJobState(102, dragonfly.StateSuccess)

	res := reconcileDF(t, r, "a1")
	assert.Equal(t, reconcile.Result{}, res)
	cond := imageReadyCond(t, c, "a1")
	assert.Equal(t, corev1.ConditionFalse, cond.Status)
	assert.Equal(t, daemonsetupgradestrategy.PullImageFail, cond.Reason)
	assert.Contains(t, cond.Message, "101")
}

func newTestManager(t *testing.T) manager.Manager {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, batchv1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))
	// Skip name validation so this test can register the controller again after TestAdd.
	mgr, err := manager.New(&rest.Config{}, manager.Options{
		Scheme:     scheme,
		Controller: ctrlconfig.Controller{SkipNameValidation: ptr.To(true)},
	})
	require.NoError(t, err)
	return mgr
}

func TestAdd_DragonflyBackend(t *testing.T) {
	cfg := &appconfig.Config{}
	cfg.ComponentConfig.ImagePreheatController = config.ImagePreheatControllerConfiguration{
		Backend:                 config.BackendDragonfly,
		DragonflyManagerAddress: "http://dfm:8080",
		DragonflyTokenSecret:    "dragonfly-system/df-token",
	}
	assert.NoError(t, Add(context.TODO(), cfg.Complete(), newTestManager(t)))
}

func TestNewReconcilerWithConfig_Dragonfly(t *testing.T) {
	r, err := newReconcilerWithConfig(newTestManager(t), config.ImagePreheatControllerConfiguration{
		Backend:                     config.BackendDragonfly,
		DragonflyManagerAddress:     "http://dfm:8080",
		DragonflyTokenSecret:        "dragonfly-system/df-token",
		DragonflyInsecureRegistries: []string{"kind-registry:5000"},
	})
	require.NoError(t, err)
	require.NotNil(t, r.dragonfly)
	assert.Equal(t, types.NamespacedName{Namespace: "dragonfly-system", Name: "df-token"}, r.dragonfly.tokenSecret)
	assert.Equal(t, []string{"kind-registry:5000"}, r.dragonfly.insecureRegistries)
	assert.NotNil(t, r.dragonfly.secrets)
}

func TestNewReconcilerWithConfig_JobAndEmptyDefaultToJob(t *testing.T) {
	for _, backend := range []string{"", config.BackendJob} {
		r, err := newReconcilerWithConfig(newTestManager(t), config.ImagePreheatControllerConfiguration{Backend: backend})
		require.NoError(t, err)
		assert.Nil(t, r.dragonfly, backend)
	}
}

func TestNewReconcilerWithConfig_BadSecretRef(t *testing.T) {
	_, err := newReconcilerWithConfig(newTestManager(t), config.ImagePreheatControllerConfiguration{
		Backend: config.BackendDragonfly, DragonflyManagerAddress: "http://dfm:8080", DragonflyTokenSecret: "bad",
	})
	assert.Error(t, err)
}
