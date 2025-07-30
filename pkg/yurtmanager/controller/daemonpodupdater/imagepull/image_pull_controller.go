// image_pull_controller.go
// ImagePullController: watches pod status, creates image pull jobs, updates pod status

package imagepull

import (
	"context"
	"fmt"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
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
	podutil "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/pod"
)

const (
	ImagePullJobLabel       = "app=image-prepull"
	ControllerName          = "image-pull-controller"
	ActiveDeadlineSeconds   = 600
	TTLSecondsAfterFinished = 300
)

func Add(ctx context.Context, c *appconfig.CompletedConfig, mgr manager.Manager) error {
	klog.Info("add image-pull-controller")
	r := newReconciler(mgr)
	return add(mgr, r)
}

var _ reconcile.Reconciler = &ReconcileImagePull{}

type ReconcileImagePull struct {
	c        client.Client
	recorder record.EventRecorder
}

func newReconciler(mgr manager.Manager) *ReconcileImagePull {
	return &ReconcileImagePull{
		c:        yurtClient.GetClientByControllerNameOrDie(mgr, ControllerName),
		recorder: mgr.GetEventRecorderFor(ControllerName),
	}
}

func add(mgr manager.Manager, r reconcile.Reconciler) error {
	c, err := controller.New(ControllerName, mgr, controller.Options{
		Reconciler: r, MaxConcurrentReconciles: 1,
	})
	if err != nil {
		return err
	}
	// 只关注 DS 管理的特定类型 Pod update 事件
	podPredicate := predicate.NewPredicateFuncs(func(obj client.Object) bool {
		pod, ok := obj.(*corev1.Pod)
		if !ok {
			return false
		}
		if pod.OwnerReferences == nil {
			return false
		}

		for _, owner := range pod.OwnerReferences {
			if owner.Kind != "DaemonSet" {
				continue
			}

			for _, cond := range pod.Status.Conditions {
				if cond.Type == "PodImageReady" && cond.Status == corev1.ConditionFalse {
					return true
				}
			}
		}

		return false
	})
	jobPredicate := predicate.NewPredicateFuncs(func(obj client.Object) bool {
		job, ok := obj.(*batchv1.Job)
		if !ok {
			return false
		}
		// 判断job name是否符合预期
		if !strings.HasPrefix(job.Name, "image-pre-pull-") {
			return false
		}
		// 判断job label是否符合预期
		if job.Labels == nil || job.Labels["app"] != "image-prepull-pod" {
			return false
		}
		// 只处理为 Pod 创建的 Job
		for _, owner := range job.OwnerReferences {
			if owner.Kind == "Pod" {
				return true
			}
		}
		return false
	})
	// 监听 Pod 变更
	err = c.Watch(
		source.Kind[client.Object](mgr.GetCache(), &corev1.Pod{}, &handler.EnqueueRequestForObject{}, podPredicate),
	)
	if err != nil {
		return err
	}
	// 监听 Job 变更
	err = c.Watch(
		source.Kind[client.Object](mgr.GetCache(), &batchv1.Job{}, handler.EnqueueRequestForOwner(mgr.GetScheme(), mgr.GetRESTMapper(), &corev1.Pod{}), jobPredicate),
	)
	if err != nil {
		return err
	}
	return nil
}

// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=pods/status,verbs=update
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;create

func (r *ReconcileImagePull) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	pod, err := getPod(r.c, req.NamespacedName)
	if err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}
	if pod.DeletionTimestamp != nil {
		return reconcile.Result{}, nil
	}

	// 1.获取pod对应的ds，判断ds是否存在，不存在则返回，存在则拿到目标镜像和secret列表
	// 1. 如果Job不存在，则创建新的job
	// 2. 如果Job存在：
	// 2.1 判断Job的镜像与目标镜像是否一致，不一致则删除job
	// 2.2 判断Job的状态，根据Pod状态，来更新Pod状态
	for _, cond := range pod.Status.Conditions {
		if cond.Type == "PodImageReady" && cond.Status == corev1.ConditionFalse {
			jobName := fmt.Sprintf("image-pre-pull-%s", pod.Name)
			var job batchv1.Job
			err := r.c.Get(ctx, types.NamespacedName{Namespace: pod.Namespace, Name: jobName}, &job)
			if err != nil {
				// Job 不存在则创建
				var containers []corev1.Container
				for _, c := range pod.Spec.Containers {
					containers = append(containers, corev1.Container{
						Name:            "prepull-" + c.Name,
						Image:           c.Image,
						Command:         []string{"true"},
						ImagePullPolicy: corev1.PullAlways,
					})
				}
				for _, c := range pod.Spec.InitContainers {
					containers = append(containers, corev1.Container{
						Name:            "prepull-init-" + c.Name,
						Image:           c.Image,
						Command:         []string{"true"},
						ImagePullPolicy: corev1.PullAlways,
					})
				}
				job = batchv1.Job{
					ObjectMeta: metav1.ObjectMeta{
						Name:      jobName,
						Namespace: pod.Namespace,
						OwnerReferences: []metav1.OwnerReference{{
							APIVersion: "v1",
							Kind:       "Pod",
							Name:       pod.Name,
							UID:        pod.UID,
						}},
					},
					Spec: batchv1.JobSpec{
						ActiveDeadlineSeconds:   int64Ptr(ActiveDeadlineSeconds),
						TTLSecondsAfterFinished: int32Ptr(TTLSecondsAfterFinished),
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"app": "image-prepull-pod"},
							},
							Spec: corev1.PodSpec{
								Containers:       containers,
								RestartPolicy:    corev1.RestartPolicyOnFailure,
								NodeName:         pod.Spec.NodeName,
								ImagePullSecrets: pod.Spec.ImagePullSecrets,
							},
						},
					},
				}
				if err := r.c.Create(ctx, &job); err != nil {
					klog.Errorf("create image pull job failed: %v", err)
					return reconcile.Result{}, err
				}
				klog.Infof("created image pull job %s for pod %s", jobName, pod.Name)
			} else {
				// Job 已存在，检查状态， succeeded 则更新 Pod 状态
				klog.Infof("found existing image pull job %s for pod %s", jobName, pod.Name)
				if job.Status.Succeeded > 0 {
					patchPodImageReady(r.c, &pod, corev1.ConditionTrue, "imagepull success")
				} else if job.Status.Failed > 0 {
					patchPodImageReady(r.c, &pod, corev1.ConditionFalse, "imagepull fail")
				}
			}
		}
	}
	return reconcile.Result{RequeueAfter: 5 * time.Second}, nil
}

func getPod(c client.Client, namespacedName types.NamespacedName) (*corev1.Pod, error) {
	var pod corev1.Pod
	if err := c.Get(context.TODO(), namespacedName, &pod); err != nil {
		return nil, err
	}

	return &pod, nil
}

func getDaemonSet(c client.Client, namespacedName types.NamespacedName) (*appsv1.DaemonSet, error) {
	var ds appsv1.DaemonSet
	if err := c.Get(context.TODO(), namespacedName, &ds); err != nil {
		return nil, err
	}
	return &ds, nil
}

func patchPodImageReady(c client.Client, pod *corev1.Pod, status corev1.ConditionStatus, reason string) {
	cond := corev1.PodCondition{
		Type:    "PodImageReady",
		Status:  status,
		Reason:  reason,
		Message: "Image pull job result",
	}
	changed := podutil.UpdatePodCondition(&pod.Status, &cond)
	if !changed {
		return
	}
	if err := c.Status().Update(context.TODO(), pod); err != nil {
		klog.Errorf("update pod status failed: %v", err)
	}
}

func int64Ptr(i int64) *int64 { return &i }
func int32Ptr(i int32) *int32 { return &i }
