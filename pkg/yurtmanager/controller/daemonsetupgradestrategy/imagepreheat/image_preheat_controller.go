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
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	klog "k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	yurtClient "github.com/openyurtio/openyurt/cmd/yurt-manager/app/client"
	appconfig "github.com/openyurtio/openyurt/cmd/yurt-manager/app/config"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/daemonsetupgradestrategy"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/daemonsetupgradestrategy/imagepreheat/config"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/daemonsetupgradestrategy/imagepreheat/dragonfly"
)

const (
	ControllerName          = "image-preheat-controller"
	ActiveDeadlineSeconds   = 600
	TTLSecondsAfterFinished = 30
)

func Add(ctx context.Context, c *appconfig.CompletedConfig, mgr manager.Manager) error {
	klog.Info("add image-pull-controller")
	r, err := newReconcilerWithConfig(mgr, c.ComponentConfig.ImagePreheatController)
	if err != nil {
		return err
	}
	return add(mgr, r)
}

var _ reconcile.Reconciler = &ReconcileImagePull{}

type ReconcileImagePull struct {
	c client.Client
	// dragonfly is set when the dragonfly backend is selected; nil means the job backend.
	dragonfly *dragonflyPreheater
}

func newReconciler(mgr manager.Manager) *ReconcileImagePull {
	return &ReconcileImagePull{
		c: yurtClient.GetClientByControllerNameOrDie(mgr, ControllerName),
	}
}

func newReconcilerWithConfig(mgr manager.Manager, cfg config.ImagePreheatControllerConfiguration) (*ReconcileImagePull, error) {
	r := newReconciler(mgr)
	if cfg.Backend != config.BackendDragonfly {
		return r, nil
	}

	ns, name, ok := strings.Cut(cfg.DragonflyTokenSecret, "/")
	if !ok || ns == "" || name == "" {
		return nil, fmt.Errorf("invalid dragonfly token secret %q, want namespace/name", cfg.DragonflyTokenSecret)
	}
	// Secrets are not in the shared informer cache, so read the token directly with the controller's own identity.
	secrets, err := client.New(yurtClient.GetConfigByControllerNameOrDie(mgr, ControllerName), client.Options{
		Scheme: mgr.GetScheme(),
		Mapper: mgr.GetRESTMapper(),
	})
	if err != nil {
		return nil, err
	}

	r.dragonfly = &dragonflyPreheater{
		c:                  r.c,
		secrets:            secrets,
		df:                 dragonfly.NewClient(cfg.DragonflyManagerAddress, nil),
		tokenSecret:        types.NamespacedName{Namespace: ns, Name: name},
		insecureRegistries: cfg.DragonflyInsecureRegistries,
		now:                time.Now,
	}
	klog.Infof("image preheat uses the dragonfly backend at %s", cfg.DragonflyManagerAddress)
	return r, nil
}

func add(mgr manager.Manager, r reconcile.Reconciler) error {
	c, err := controller.New(ControllerName, mgr, controller.Options{
		Reconciler: r, MaxConcurrentReconciles: 1,
	})
	if err != nil {
		return err
	}

	if err := c.Watch(
		source.Kind[client.Object](mgr.GetCache(), &corev1.Pod{}, &handler.EnqueueRequestForObject{}, predicate.NewPredicateFuncs(PodFilter)),
	); err != nil {
		return errors.Wrap(err, "failed to watch pod")
	}

	if ri, ok := r.(*ReconcileImagePull); ok && ri.dragonfly != nil {
		return nil
	}

	if err := c.Watch(
		source.Kind[client.Object](mgr.GetCache(), &batchv1.Job{}, handler.EnqueueRequestForOwner(mgr.GetScheme(), mgr.GetRESTMapper(), &corev1.Pod{}), predicate.NewPredicateFuncs(JobFilter)),
	); err != nil {
		return errors.Wrap(err, "failed to watch job")
	}

	return nil
}

// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=pods/status,verbs=update
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;create
// +kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list
// +kubebuilder:rbac:groups=core,resources=pods,verbs=list
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get

func (r *ReconcileImagePull) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	klog.Infof("reconcile pod %s", req.String())
	pod, ds, err := GetPodAndOwnedDaemonSet(r.c, req)
	if err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	if !r.needReconcile(pod, ds) {
		return reconcile.Result{}, nil
	}

	if r.dragonfly != nil {
		return r.reconcileDragonfly(ctx, ds, pod)
	}

	job, created, err := r.getOrCreateImagePullJob(ds, pod)
	if err != nil {
		return reconcile.Result{}, err
	}
	if created {
		return reconcile.Result{}, nil
	}

	return reconcile.Result{}, r.updatePodImageReady(pod, job)
}

func (r *ReconcileImagePull) needReconcile(pod *corev1.Pod, ds *appsv1.DaemonSet) bool {
	if pod.DeletionTimestamp != nil {
		return false
	}
	if ds.DeletionTimestamp != nil {
		return false
	}

	if !isUpgradeStatus(pod) {
		return false
	}

	return ExpectedPodImageReadyStatus(pod, corev1.ConditionFalse)
}

func isUpgradeStatus(pod *corev1.Pod) bool {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == daemonsetupgradestrategy.PodNeedUpgrade && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func (r *ReconcileImagePull) getOrCreateImagePullJob(ds *appsv1.DaemonSet, pod *corev1.Pod) (*batchv1.Job, bool, error) {
	jobName := getImagePullJobName(pod)
	job, err := getJob(r.c, types.NamespacedName{Namespace: pod.Namespace, Name: jobName})
	if err != nil && !apierrors.IsNotFound(err) {
		return job, false, err
	}

	if err == nil {
		return job, false, nil
	}

	job, err = r.createImagePullJob(jobName, ds, pod)
	return job, true, err
}

func getImagePullJobName(pod *corev1.Pod) string {
	return daemonsetupgradestrategy.ImagePullJobNamePrefix + pod.Name + "-" + strings.TrimPrefix(GetPodNextHashVersion(pod), daemonsetupgradestrategy.VersionPrefix)
}

func getJob(c client.Client, namespacedName types.NamespacedName) (*batchv1.Job, error) {
	job := &batchv1.Job{}
	err := c.Get(context.TODO(), namespacedName, job)
	return job, err
}

func (r *ReconcileImagePull) createImagePullJob(jobName string, ds *appsv1.DaemonSet, pod *corev1.Pod) (*batchv1.Job, error) {
	var containers []corev1.Container
	for _, c := range ds.Spec.Template.Spec.Containers {
		containers = append(containers, corev1.Container{
			Name:            "prepull-" + c.Name,
			Image:           c.Image,
			Command:         []string{"true"},
			ImagePullPolicy: corev1.PullAlways,
		})
	}
	for _, c := range ds.Spec.Template.Spec.InitContainers {
		containers = append(containers, corev1.Container{
			Name:            "prepull-init-" + c.Name,
			Image:           c.Image,
			Command:         []string{"true"},
			ImagePullPolicy: corev1.PullAlways,
		})
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: ds.Namespace,
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion:         "v1",
				Kind:               "Pod",
				Name:               pod.Name,
				UID:                pod.UID,
				BlockOwnerDeletion: boolPtr(true),
			}},
		},
		Spec: batchv1.JobSpec{
			ActiveDeadlineSeconds:   int64Ptr(ActiveDeadlineSeconds),
			BackoffLimit:            int32Ptr(1),
			TTLSecondsAfterFinished: int32Ptr(TTLSecondsAfterFinished),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers:       containers,
					RestartPolicy:    corev1.RestartPolicyOnFailure,
					NodeName:         pod.Spec.NodeName,
					ImagePullSecrets: ds.Spec.Template.Spec.ImagePullSecrets,
				},
			},
		},
	}

	return job, r.c.Create(context.TODO(), job)
}

func (r *ReconcileImagePull) updatePodImageReady(pod *corev1.Pod, job *batchv1.Job) error {
	if job.Status.Succeeded > 0 {
		mesg := daemonsetupgradestrategy.VersionPrefix + GetPodNextHashVersion(pod)
		return r.patchPodImageStatus(pod, corev1.ConditionTrue, daemonsetupgradestrategy.PullImageSuccess, mesg)
	}
	if job.Status.Failed > 0 {
		mesg := fmt.Sprintf("pull image job %s failed", job.Name)
		return r.patchPodImageStatus(pod, corev1.ConditionFalse, daemonsetupgradestrategy.PullImageFail, mesg)
	}
	return nil
}

func (r *ReconcileImagePull) reconcileDragonfly(ctx context.Context, ds *appsv1.DaemonSet, pod *corev1.Pod) (reconcile.Result, error) {
	phase, msg, err := r.dragonfly.Ensure(ctx, ds, pod)
	if err != nil {
		return reconcile.Result{}, err
	}

	switch phase {
	case phaseSucceeded:
		return reconcile.Result{}, r.patchPodImageStatus(pod, corev1.ConditionTrue, daemonsetupgradestrategy.PullImageSuccess,
			daemonsetupgradestrategy.VersionPrefix+GetPodNextHashVersion(pod))
	case phaseFailed:
		return reconcile.Result{}, r.patchPodImageStatus(pod, corev1.ConditionFalse, daemonsetupgradestrategy.PullImageFail, msg)
	default:
		return reconcile.Result{RequeueAfter: dragonflyPollInterval}, nil
	}
}
