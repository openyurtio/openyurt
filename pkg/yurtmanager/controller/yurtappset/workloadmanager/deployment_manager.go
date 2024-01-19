/*
Copyright 2024 The Openyurt Authors.

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

package workloadmanager

import (
	"context"
	"errors"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/openyurtio/openyurt/pkg/apis/apps"
	"github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/refmanager"
)

const updateRetries = 5

type DeploymentManager struct {
	client.Client
	Scheme *runtime.Scheme
}

func (d *DeploymentManager) GetTemplateType() TemplateType {
	return DeploymentTemplateType
}

func (d *DeploymentManager) Delete(yas *v1beta1.YurtAppSet, workload metav1.Object) error {
	klog.V(4).Infof("YurtAppSet[%s/%s] prepare delete [Deployment/%s/%s]", yas.GetNamespace(),
		yas.GetName(), workload.GetNamespace(), workload.GetName())

	workloadObj, ok := workload.(client.Object)
	if !ok {
		return errors.New("could not convert metav1.Object to client.Object")
	}
	return d.Client.Delete(context.TODO(), workloadObj, client.PropagationPolicy(metav1.DeletePropagationBackground))
}

// ApplyTemplate updates the object to the latest revision, depending on the YurtAppSet.
func (d *DeploymentManager) applyTemplate(yas *v1beta1.YurtAppSet, nodepoolName, revision string, workload *appsv1.Deployment) error {

	deployTemplate := yas.Spec.Workload.WorkloadTemplate.DeploymentTemplate
	if deployTemplate == nil {
		return errors.New("no deployment template in workloadTemplate")
	}

	// deployment meta data
	workload.Labels = CombineMaps(workload.Labels, deployTemplate.Labels, map[string]string{
		apps.PoolNameLabelKey:               nodepoolName,
		apps.ControllerRevisionHashLabelKey: revision,
		apps.YurtAppSetOwnerLabelKey:        yas.Name,
	})
	workload.Annotations = CombineMaps(workload.Annotations, deployTemplate.Annotations, map[string]string{
		apps.AnnotationRefNodePool: nodepoolName,
	})

	workload.Namespace = yas.GetNamespace()
	workload.GenerateName = getWorkloadPrefix(yas.GetName(), nodepoolName)
	if err := controllerutil.SetControllerReference(yas, workload, d.Scheme); err != nil {
		return err
	}

	// deployment spec data
	workload.Spec = *deployTemplate.Spec.DeepCopy()
	if workload.Spec.Selector == nil {
		// if no selector, create one
		// add this check, because we have no yas webhook check to ensure deployment template is valid
		workload.Spec.Selector = &metav1.LabelSelector{
			MatchLabels: make(map[string]string, 0),
		}
	}
	workload.Spec.Selector.MatchLabels[apps.PoolNameLabelKey] = nodepoolName
	workload.Spec.Template.Labels = CombineMaps(workload.Spec.Template.Labels, map[string]string{
		apps.PoolNameLabelKey:               nodepoolName,
		apps.ControllerRevisionHashLabelKey: revision,
	})
	workload.Spec.Template.Spec.NodeSelector = CombineMaps(workload.Spec.Template.Spec.NodeSelector, CreateNodeSelectorByNodepoolName(nodepoolName))

	// apply tweaks
	tweaks, err := GetNodePoolTweaksFromYurtAppSet(d.Client, nodepoolName, yas)
	if err != nil {
		return err
	}

	if err = ApplyTweaksToDeployment(workload, tweaks); err != nil {
		return err
	}

	return nil
}

func (d *DeploymentManager) Update(yas *v1beta1.YurtAppSet, workload metav1.Object, nodepoolName, revision string) error {
	klog.V(4).Infof("YurtAppSet[%s/%s] prepare update [Deployment/%s/%s]", yas.GetNamespace(),
		yas.GetName(), workload.GetNamespace(), workload.GetName())

	if nodepoolName == "" {
		klog.Warningf("Deployment[%s/%s] to be updated's nodepool name is empty.", workload.GetNamespace(), workload.GetName())
	}

	deploy := &appsv1.Deployment{}
	var updateError error
	for i := 0; i < updateRetries; i++ {
		getError := d.Client.Get(context.TODO(), types.NamespacedName{Namespace: workload.GetNamespace(), Name: workload.GetName()}, deploy)
		if getError != nil {
			return getError
		}

		if err := d.applyTemplate(yas, nodepoolName, revision, deploy); err != nil {
			return err
		}
		updateError = d.Client.Update(context.TODO(), deploy)
		if updateError == nil {
			break
		}
		klog.V(4).Info("update deployment failed, retry")
	}

	return updateError
}

func (d *DeploymentManager) Create(yas *v1beta1.YurtAppSet, nodepoolName, revision string) error {
	klog.V(4).Infof("YurtAppSet[%s/%s] prepare create new deployment for nodepool %s ", yas.GetNamespace(), yas.GetName(), nodepoolName)

	deploy := appsv1.Deployment{}
	if err := d.applyTemplate(yas, nodepoolName, revision, &deploy); err != nil {
		klog.Errorf("YurtAppSet[%s/%s] could not apply template, when create deployment: %v", yas.GetNamespace(),
			yas.GetName(), err)
		return err
	}
	return d.Client.Create(context.TODO(), &deploy)
}

func (d *DeploymentManager) List(yas *v1beta1.YurtAppSet) ([]metav1.Object, error) {

	// get yas selector from yas name
	yasSelector, err := NewLabelSelectorForYurtAppSet(yas)
	if err != nil {
		return nil, err
	}

	// List all Deployment to include those that don't match the selector anymore but
	// have a ControllerRef pointing to this controller.
	allDeployments := appsv1.DeploymentList{}
	if err := d.Client.List(context.TODO(), &allDeployments); err != nil {
		return nil, err
	}

	manager, err := refmanager.New(d.Client, yasSelector, yas, d.Scheme)
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

	return objs, nil
}

var _ WorkloadManager = &DeploymentManager{}
