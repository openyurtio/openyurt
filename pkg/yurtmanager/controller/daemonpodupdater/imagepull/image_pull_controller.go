// image_pull_controller.go
// ImagePullController: watches pod status, creates image pull jobs, updates pod status

package imagepull

import (
	context "context"
	"fmt"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"k8s.io/klog/v2"
)

const (
	ImagePullJobLabel = "app=image-prepull"
)

type ImagePullReconciler struct {
	client.Client
}

func (r *ImagePullReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var pod corev1.Pod
	if err := r.Get(ctx, req.NamespacedName, &pod); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	for _, cond := range pod.Status.Conditions {
		if cond.Type == "PodImageReady" && cond.Status == corev1.ConditionFalse && cond.Reason == "imagepull start" {
			jobName := fmt.Sprintf("image-pre-pull-%s", pod.Name)
			var job batchv1.Job
			err := r.Get(ctx, types.NamespacedName{Namespace: pod.Namespace, Name: jobName}, &job)
			if err != nil {
				// Job 不存在则创建
				job = batchv1.Job{
					ObjectMeta: metav1.ObjectMeta{
						Name:      jobName,
						Namespace: pod.Namespace,
						Labels:    map[string]string{"app": "image-prepull"},
					},
					Spec: batchv1.JobSpec{
						ActiveDeadlineSeconds:   int64Ptr(600),
						TTLSecondsAfterFinished: int32Ptr(300),
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"app": "image-prepull-pod"},
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{{
									Name:            "prepull-image",
									Image:           pod.Spec.Containers[0].Image,
									Command:         []string{"sleep", "30"},
									ImagePullPolicy: corev1.PullAlways,
								}},
								RestartPolicy:    corev1.RestartPolicyOnFailure,
								NodeName:         pod.Spec.NodeName,
								ImagePullSecrets: pod.Spec.ImagePullSecrets,
							},
						},
					},
				}
				if err := r.Create(ctx, &job); err != nil {
					klog.Errorf("create image pull job failed: %v", err)
					return ctrl.Result{}, err
				}
				klog.Infof("created image pull job %s for pod %s", jobName, pod.Name)
			} else {
				// Job 已存在，检查状态
				if job.Status.Succeeded > 0 {
					patchPodImageReady(r.Client, &pod, corev1.ConditionTrue, "imagepull success")
				} else if job.Status.Failed > 0 {
					patchPodImageReady(r.Client, &pod, corev1.ConditionFalse, "imagepull fail")
				}
			}
		}
	}
	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

func (r *ImagePullReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}

func patchPodImageReady(c client.Client, pod *corev1.Pod, status corev1.ConditionStatus, reason string) {
	// 构造 patch 并更新 Pod status condition
	// 可复用 controller util
	// ...
}

func int64Ptr(i int64) *int64 { return &i }
func int32Ptr(i int32) *int32 { return &i }
