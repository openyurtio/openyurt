/*
Copyright 2020 The OpenYurt Authors.
Copyright 2019 The Kruise Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

@CHANGELOG
OpenYurt Authors:
change statefulset adapter
*/

package adapter

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	alpha1 "github.com/alibaba/openyurt/pkg/yurtappmanager/apis/apps/v1alpha1"
	yurtctlutil "github.com/alibaba/openyurt/pkg/yurtappmanager/controller/util"
	"github.com/alibaba/openyurt/pkg/yurtappmanager/util/refmanager"
)

type StatefulSetAdapter struct {
	client.Client

	Scheme *runtime.Scheme
}

var _ Adapter = &StatefulSetAdapter{}

// NewResourceObject creates a empty StatefulSet object.
func (a *StatefulSetAdapter) NewResourceObject() runtime.Object {
	return &appsv1.StatefulSet{}
}

// NewResourceListObject creates a empty StatefulSetList object.
func (a *StatefulSetAdapter) NewResourceListObject() runtime.Object {
	return &appsv1.StatefulSetList{}
}

// GetStatusObservedGeneration returns the observed generation of the pool.
func (a *StatefulSetAdapter) GetStatusObservedGeneration(obj metav1.Object) int64 {
	return obj.(*appsv1.StatefulSet).Status.ObservedGeneration
}

// GetDetails returns the replicas detail the pool needs.
func (a *StatefulSetAdapter) GetDetails(obj metav1.Object) (ReplicasInfo, error) {
	set := obj.(*appsv1.StatefulSet)

	var specReplicas int32
	if set.Spec.Replicas != nil {
		specReplicas = *set.Spec.Replicas
	}
	replicasInfo := ReplicasInfo{
		Replicas:      specReplicas,
		ReadyReplicas: set.Status.ReadyReplicas,
	}

	return replicasInfo, nil
}

// GetPoolFailure returns the failure information of the pool.
// StatefulSet has no condition.
func (a *StatefulSetAdapter) GetPoolFailure() *string {
	return nil
}

// ApplyPoolTemplate updates the pool to the latest revision, depending on the StatefulSetTemplate.
func (a *StatefulSetAdapter) ApplyPoolTemplate(ud *alpha1.UnitedDeployment, poolName, revision string,
	replicas int32, obj runtime.Object) error {
	set := obj.(*appsv1.StatefulSet)

	var poolConfig *alpha1.Pool
	for _, pool := range ud.Spec.Topology.Pools {
		if pool.Name == poolName {
			poolConfig = &pool
			break
		}
	}
	if poolConfig == nil {
		return fmt.Errorf("fail to find pool config %s", poolName)
	}

	set.Namespace = ud.Namespace

	if set.Labels == nil {
		set.Labels = map[string]string{}
	}
	for k, v := range ud.Spec.WorkloadTemplate.StatefulSetTemplate.Labels {
		set.Labels[k] = v
	}
	for k, v := range ud.Spec.Selector.MatchLabels {
		set.Labels[k] = v
	}
	set.Labels[alpha1.ControllerRevisionHashLabelKey] = revision
	// record the pool name as a label
	set.Labels[alpha1.PoolNameLabelKey] = poolName

	if set.Annotations == nil {
		set.Annotations = map[string]string{}
	}
	for k, v := range ud.Spec.WorkloadTemplate.StatefulSetTemplate.Annotations {
		set.Annotations[k] = v
	}

	set.GenerateName = getPoolPrefix(ud.Name, poolName)

	selectors := ud.Spec.Selector.DeepCopy()
	selectors.MatchLabels[alpha1.PoolNameLabelKey] = poolName

	if err := controllerutil.SetControllerReference(ud, set, a.Scheme); err != nil {
		return err
	}

	set.Spec.Selector = selectors
	set.Spec.Replicas = &replicas

	set.Spec.UpdateStrategy = *ud.Spec.WorkloadTemplate.StatefulSetTemplate.Spec.UpdateStrategy.DeepCopy()
	set.Spec.Template = *ud.Spec.WorkloadTemplate.StatefulSetTemplate.Spec.Template.DeepCopy()
	if set.Spec.Template.Labels == nil {
		set.Spec.Template.Labels = map[string]string{}
	}
	set.Spec.Template.Labels[alpha1.PoolNameLabelKey] = poolName
	set.Spec.Template.Labels[alpha1.ControllerRevisionHashLabelKey] = revision

	set.Spec.RevisionHistoryLimit = ud.Spec.RevisionHistoryLimit
	set.Spec.PodManagementPolicy = ud.Spec.WorkloadTemplate.StatefulSetTemplate.Spec.PodManagementPolicy
	set.Spec.ServiceName = ud.Spec.WorkloadTemplate.StatefulSetTemplate.Spec.ServiceName
	set.Spec.VolumeClaimTemplates = ud.Spec.WorkloadTemplate.StatefulSetTemplate.Spec.VolumeClaimTemplates

	attachNodeAffinityAndTolerations(&set.Spec.Template.Spec, poolConfig)
	return nil
}

// PostUpdate does some works after pool updated. StatefulSet will implement this method to clean stuck pods.
func (a *StatefulSetAdapter) PostUpdate(ud *alpha1.UnitedDeployment, obj runtime.Object, revision string) error {
	/*
		if strategy == nil {
			return nil
		}
		set := obj.(*appsv1.StatefulSet)
		if set.Spec.UpdateStrategy.Type == appsv1.OnDeleteStatefulSetStrategyType {
			return nil
		}

		// If RollingUpdate, work around for issue https://github.com/kubernetes/kubernetes/issues/67250
		return a.deleteStuckPods(set, revision, strategy.GetPartition())
	*/
	return nil
}

// IsExpected checks the pool is the expected revision or not.
// The revision label can tell the current pool revision.
func (a *StatefulSetAdapter) IsExpected(obj metav1.Object, revision string) bool {
	return obj.GetLabels()[alpha1.ControllerRevisionHashLabelKey] != revision
}

func (a *StatefulSetAdapter) getStatefulSetPods(set *appsv1.StatefulSet) ([]*corev1.Pod, error) {
	selector, err := metav1.LabelSelectorAsSelector(set.Spec.Selector)
	if err != nil {
		return nil, err
	}
	podList := &corev1.PodList{}
	err = a.Client.List(context.TODO(), podList, &client.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, err
	}

	manager, err := refmanager.New(a.Client, set.Spec.Selector, set, a.Scheme)
	if err != nil {
		return nil, err
	}
	selected := make([]metav1.Object, len(podList.Items))
	for i, pod := range podList.Items {
		selected[i] = pod.DeepCopy()
	}
	claimed, err := manager.ClaimOwnedObjects(selected)
	if err != nil {
		return nil, err
	}

	claimedPods := make([]*corev1.Pod, len(claimed))
	for i, pod := range claimed {
		claimedPods[i] = pod.(*corev1.Pod)
	}
	return claimedPods, nil
}

// deleteStucckPods tries to work around the blocking issue https://github.com/kubernetes/kubernetes/issues/67250
func (a *StatefulSetAdapter) deleteStuckPods(set *appsv1.StatefulSet, revision string, partition int32) error {
	pods, err := a.getStatefulSetPods(set)
	if err != nil {
		return err
	}

	for i := range pods {
		pod := pods[i]
		// If the pod is considered as stuck, delete it.
		if isPodStuckForRollingUpdate(pod, revision, partition) {
			klog.V(2).Infof("Delete pod %s/%s at stuck state", pod.Namespace, pod.Name)
			err = a.Delete(context.TODO(), pod, client.PropagationPolicy(metav1.DeletePropagationBackground))
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// isPodStuckForRollingUpdate checks whether the pod is stuck under strategy RollingUpdate.
// If a pod needs to upgrade (pod_ordinal >= partition && pod_revision != sts_revision)
// and its readiness is false, or worse status like Pending, ImagePullBackOff, it will be blocked.
func isPodStuckForRollingUpdate(pod *corev1.Pod, revision string, partition int32) bool {
	if yurtctlutil.GetOrdinal(pod) < partition {
		return false
	}

	if getRevision(pod) == revision {
		return false
	}

	return !podutil.IsPodReadyConditionTrue(pod.Status)
}
