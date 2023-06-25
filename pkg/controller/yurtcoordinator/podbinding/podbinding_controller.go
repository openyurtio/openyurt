/*
Copyright 2023 The OpenYurt Authors.

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

package podbinding

import (
	"context"
	"flag"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	appconfig "github.com/openyurtio/openyurt/cmd/yurt-manager/app/config"
	"github.com/openyurtio/openyurt/pkg/controller/yurtcoordinator/constant"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
)

func init() {
	flag.IntVar(&concurrentReconciles, "podbinding-controller", concurrentReconciles, "Max concurrent workers for podbinding-controller controller.")
}

const (
	ControllerName = "podbinding"
)

var (
	concurrentReconciles = 5

	notReadyToleration = corev1.Toleration{
		Key:      corev1.TaintNodeNotReady,
		Operator: corev1.TolerationOpExists,
		Effect:   corev1.TaintEffectNoExecute,
	}

	unreachableToleration = corev1.Toleration{
		Key:      corev1.TaintNodeUnreachable,
		Operator: corev1.TolerationOpExists,
		Effect:   corev1.TaintEffectNoExecute,
	}
	defaultTolerationSeconds = 300
)

type ReconcilePodBinding struct {
	client.Client
}

// Add creates a PodBingding controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(_ *appconfig.CompletedConfig, mgr manager.Manager) error {
	r := &ReconcilePodBinding{}
	c, err := controller.New(ControllerName, mgr, controller.Options{
		Reconciler: r, MaxConcurrentReconciles: concurrentReconciles,
	})
	if err != nil {
		return err
	}
	err = c.Watch(&source.Kind{Type: &corev1.Node{}}, &handler.EnqueueRequestForObject{})
	return err
}

// Reconcile reads that state of Node in cluster and makes changes if node autonomy state has been changed
func (r *ReconcilePodBinding) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	var err error
	node := &corev1.Node{}
	if err = r.Get(ctx, req.NamespacedName, node); err != nil {
		klog.V(4).Infof("node not found for %q\n", req.NamespacedName)
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}
	klog.V(4).Infof("node request: %s\n", node.Name)

	if err := r.processNode(node); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *ReconcilePodBinding) processNode(node *corev1.Node) error {
	// if node has autonomy annotation, we need to see if pods on this node except DaemonSet/Static ones need a treat
	pods, err := r.getPodsAssignedToNode(node.Name)
	if err != nil {
		return err
	}

	for i := range pods {
		pod := &pods[i]
		klog.V(5).Infof("pod %d on node %s: %s", i, node.Name, pod.Name)
		// skip DaemonSet pods and static pod
		if isDaemonSetPodOrStaticPod(pod) {
			continue
		}

		// skip not running pods
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}

		// pod binding takes precedence against node autonomy
		if isPodBoundenToNode(node) {
			return r.configureTolerationForPod(pod, nil)
		} else {
			tolerationSeconds := int64(defaultTolerationSeconds)
			return r.configureTolerationForPod(pod, &tolerationSeconds)
		}
	}
	return nil
}

func (r *ReconcilePodBinding) getPodsAssignedToNode(name string) ([]corev1.Pod, error) {
	listOptions := &client.ListOptions{
		FieldSelector: fields.SelectorFromSet(fields.Set{
			"spec.nodeName": name,
		}),
	}

	podList := &corev1.PodList{}
	err := r.List(context.TODO(), podList, listOptions)
	if err != nil {
		klog.Errorf("failed to get podList for node(%s), %v", name, err)
		return nil, err
	}
	return podList.Items, nil
}

func (r *ReconcilePodBinding) configureTolerationForPod(pod *corev1.Pod, tolerationSeconds *int64) error {
	// reset toleration seconds
	notReadyToleration.TolerationSeconds = tolerationSeconds
	unreachableToleration.TolerationSeconds = tolerationSeconds
	toleratesNodeNotReady := addOrUpdateTolerationInPodSpec(&pod.Spec, &notReadyToleration)
	toleratesNodeUnreachable := addOrUpdateTolerationInPodSpec(&pod.Spec, &unreachableToleration)

	if toleratesNodeNotReady || toleratesNodeUnreachable {
		klog.V(4).Infof("pod(%s/%s) => toleratesNodeNotReady=%v, toleratesNodeUnreachable=%v, tolerationSeconds=%d", pod.Namespace, pod.Name, toleratesNodeNotReady, toleratesNodeUnreachable, *tolerationSeconds)
		err := r.Update(context.TODO(), pod, &client.UpdateOptions{})
		if err != nil {
			klog.Errorf("failed to update toleration of pod(%s/%s), %v", pod.Namespace, pod.Name, err)
			return err
		}
	}

	return nil
}

func isPodBoundenToNode(node *corev1.Node) bool {
	if node.Annotations != nil &&
		(node.Annotations[projectinfo.GetAutonomyAnnotation()] == "true" ||
			node.Annotations[constant.PodBindingAnnotation] == "true") {
		return true
	}

	return false
}

func isDaemonSetPodOrStaticPod(pod *corev1.Pod) bool {
	if pod != nil {
		for i := range pod.OwnerReferences {
			if pod.OwnerReferences[i].Kind == "DaemonSet" {
				return true
			}
		}

		if pod.Annotations != nil && len(pod.Annotations[corev1.MirrorPodAnnotationKey]) != 0 {
			return true
		}
	}

	return false
}

// addOrUpdateTolerationInPodSpec tries to add a toleration to the toleration list in PodSpec.
// Returns true if something was updated, false otherwise.
func addOrUpdateTolerationInPodSpec(spec *corev1.PodSpec, toleration *corev1.Toleration) bool {
	podTolerations := spec.Tolerations

	var newTolerations []corev1.Toleration
	updated := false
	for i := range podTolerations {
		if toleration.MatchToleration(&podTolerations[i]) {
			if (toleration.TolerationSeconds == nil && podTolerations[i].TolerationSeconds == nil) ||
				(toleration.TolerationSeconds != nil && podTolerations[i].TolerationSeconds != nil &&
					(*toleration.TolerationSeconds == *podTolerations[i].TolerationSeconds)) {
				return false
			}

			newTolerations = append(newTolerations, *toleration)
			updated = true
			continue
		}

		newTolerations = append(newTolerations, podTolerations[i])
	}

	if !updated {
		newTolerations = append(newTolerations, *toleration)
	}

	spec.Tolerations = newTolerations
	return true
}
