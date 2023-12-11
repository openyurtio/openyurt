/*
Copyright 2021 The OpenYurt Authors.

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

package workloadcontroller

import (
	"context"
	"errors"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/openyurtio/openyurt/pkg/apis/apps"
	"github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/refmanager"
)

const updateRetries = 5

type DeploymentControllor struct {
	client.Client
	Scheme *runtime.Scheme
}

func (d *DeploymentControllor) GetTemplateType() v1alpha1.TemplateType {
	return v1alpha1.DeploymentTemplateType
}

func (d *DeploymentControllor) DeleteWorkload(yda *v1alpha1.YurtAppDaemon, load *Workload) error {
	klog.Infof("YurtAppDaemon[%s/%s] prepare delete Deployment[%s/%s]", yda.GetNamespace(),
		yda.GetName(), load.Namespace, load.Name)

	set := load.Spec.Ref.(runtime.Object)
	cliSet, ok := set.(client.Object)
	if !ok {
		return errors.New("could not convert runtime.Object to client.Object")
	}
	return d.Delete(context.TODO(), cliSet, client.PropagationPolicy(metav1.DeletePropagationBackground))
}

// ApplyTemplate updates the object to the latest revision, depending on the YurtAppDaemon.
func (d *DeploymentControllor) ApplyTemplate(scheme *runtime.Scheme, yad *v1alpha1.YurtAppDaemon, nodepool v1alpha1.NodePool, revision string, set *appsv1.Deployment) error {

	if set.Labels == nil {
		set.Labels = map[string]string{}
	}
	for k, v := range yad.Spec.WorkloadTemplate.DeploymentTemplate.Labels {
		set.Labels[k] = v
	}
	for k, v := range yad.Spec.Selector.MatchLabels {
		set.Labels[k] = v
	}
	set.Labels[apps.ControllerRevisionHashLabelKey] = revision
	set.Labels[apps.PoolNameLabelKey] = nodepool.GetName()

	if set.Annotations == nil {
		set.Annotations = map[string]string{}
	}
	for k, v := range yad.Spec.WorkloadTemplate.DeploymentTemplate.Annotations {
		set.Annotations[k] = v
	}
	set.Annotations[apps.AnnotationRefNodePool] = nodepool.GetName()

	set.Namespace = yad.GetNamespace()
	set.GenerateName = getWorkloadPrefix(yad.GetName(), nodepool.GetName())

	set.Spec = *yad.Spec.WorkloadTemplate.DeploymentTemplate.Spec.DeepCopy()
	set.Spec.Selector.MatchLabels[apps.PoolNameLabelKey] = nodepool.GetName()

	// set RequiredDuringSchedulingIgnoredDuringExecution nil
	if set.Spec.Template.Spec.Affinity != nil && set.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil {
		set.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = nil
	}

	if set.Spec.Template.Labels == nil {
		set.Spec.Template.Labels = map[string]string{}
	}
	set.Spec.Template.Labels[apps.PoolNameLabelKey] = nodepool.GetName()
	set.Spec.Template.Labels[apps.ControllerRevisionHashLabelKey] = revision

	// use nodeSelector
	set.Spec.Template.Spec.NodeSelector = CreateNodeSelectorByNodepoolName(nodepool.GetName())

	// toleration
	nodePoolTaints := TaintsToTolerations(nodepool.Spec.Taints)
	set.Spec.Template.Spec.Tolerations = append(set.Spec.Template.Spec.Tolerations, nodePoolTaints...)

	if err := controllerutil.SetControllerReference(yad, set, scheme); err != nil {
		return err
	}
	return nil
}

func (d *DeploymentControllor) ObjectKey(load *Workload) client.ObjectKey {
	return types.NamespacedName{
		Namespace: load.Namespace,
		Name:      load.Name,
	}
}

func (d *DeploymentControllor) UpdateWorkload(load *Workload, yad *v1alpha1.YurtAppDaemon, nodepool v1alpha1.NodePool, revision string) error {
	klog.Infof("YurtAppDaemon[%s/%s] prepare update Deployment[%s/%s]", yad.GetNamespace(),
		yad.GetName(), load.Namespace, load.Name)

	deploy := &appsv1.Deployment{}
	var updateError error
	for i := 0; i < updateRetries; i++ {
		getError := d.Client.Get(context.TODO(), d.ObjectKey(load), deploy)
		if getError != nil {
			return getError
		}

		if err := d.ApplyTemplate(d.Scheme, yad, nodepool, revision, deploy); err != nil {
			return err
		}
		updateError = d.Client.Update(context.TODO(), deploy)
		if updateError == nil {
			break
		}
	}

	return updateError
}

func (d *DeploymentControllor) CreateWorkload(yad *v1alpha1.YurtAppDaemon, nodepool v1alpha1.NodePool, revision string) error {
	klog.Infof("YurtAppDaemon[%s/%s] prepare create new deployment by nodepool %s ", yad.GetNamespace(), yad.GetName(), nodepool.GetName())

	deploy := appsv1.Deployment{}
	if err := d.ApplyTemplate(d.Scheme, yad, nodepool, revision, &deploy); err != nil {
		klog.Errorf("YurtAppDaemon[%s/%s] could not apply template, when create deployment: %v", yad.GetNamespace(),
			yad.GetName(), err)
		return err
	}
	return d.Client.Create(context.TODO(), &deploy)
}

func (d *DeploymentControllor) GetAllWorkloads(yad *v1alpha1.YurtAppDaemon) ([]*Workload, error) {
	allDeployments := appsv1.DeploymentList{}
	// 获得 YurtAppDaemon 对应的 所有Deployment, 根据OwnerRef
	selector, err := metav1.LabelSelectorAsSelector(yad.Spec.Selector)
	if err != nil {
		return nil, err
	}
	// List all Deployment to include those that don't match the selector anymore but
	// have a ControllerRef pointing to this controller.
	if err := d.Client.List(context.TODO(), &allDeployments, &client.ListOptions{LabelSelector: selector}); err != nil {
		return nil, err
	}

	manager, err := refmanager.New(d.Client, yad.Spec.Selector, yad, d.Scheme)
	if err != nil {
		return nil, err
	}

	selected := make([]metav1.Object, 0, len(allDeployments.Items))
	for i := 0; i < len(allDeployments.Items); i++ {
		t := allDeployments.Items[i]
		selected = append(selected, &t)
	}

	objs, err := manager.ClaimOwnedObjects(selected)
	if err != nil {
		return nil, err
	}

	workloads := make([]*Workload, 0, len(objs))
	for i, o := range objs {
		deploy := o.(*appsv1.Deployment)
		spec := deploy.Spec
		var availableCondition corev1.ConditionStatus
		for _, condition := range deploy.Status.Conditions {
			if condition.Type == appsv1.DeploymentAvailable {
				availableCondition = condition.Status
				break
			}
		}
		w := &Workload{
			Name:      o.GetName(),
			Namespace: o.GetNamespace(),
			Kind:      deploy.Kind,
			Spec: WorkloadSpec{
				Ref:          objs[i],
				NodeSelector: spec.Template.Spec.NodeSelector,
				Tolerations:  spec.Template.Spec.Tolerations,
			},
			Status: WorkloadStatus{
				Replicas:           deploy.Status.Replicas,
				ReadyReplicas:      deploy.Status.ReadyReplicas,
				AvailableCondition: availableCondition,
			},
		}
		workloads = append(workloads, w)
	}
	return workloads, nil
}

var _ WorkloadController = &DeploymentControllor{}
