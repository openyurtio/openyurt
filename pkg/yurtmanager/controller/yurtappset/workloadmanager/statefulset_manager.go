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

type StatefulSetManager struct {
	client.Client
	Scheme *runtime.Scheme
}

func (s *StatefulSetManager) GetTemplateType() TemplateType { return StatefulSetTemplateType }

func (s *StatefulSetManager) Delete(yas *v1beta1.YurtAppSet, workload metav1.Object) error {
	klog.V(4).Infof("YurtAPpSet[%s/%s] prepare to delete StatefulSet[/%s/%s]", yas.Namespace, yas.Name, workload.GetNamespace(), workload.GetName())

	workloadObj, ok := workload.(client.Object)
	if !ok {
		return errors.New("failed to convert metav1.Object to client.Object")
	}
	return s.Client.Delete(context.TODO(), workloadObj, client.PropagationPolicy(metav1.DeletePropagationBackground))
}

func (s *StatefulSetManager) ApplyTemplate(yas *v1beta1.YurtAppSet, nodepoolName, revision string, workload *appsv1.StatefulSet) error {
	statefulsetTemplate := yas.Spec.Workload.WorkloadTemplate.StatefulSetTemplate
	if statefulsetTemplate == nil {
		return errors.New("no statefulset template in workloadTemplate")
	}

	// statefulset meta data
	workload.Labels = CombineMaps(workload.Labels, statefulsetTemplate.Labels, map[string]string{
		apps.PoolNameLabelKey:               nodepoolName,
		apps.ControllerRevisionHashLabelKey: revision,
		apps.YurtAppSetOwnerLabelKey:        yas.Name,
	})
	workload.Annotations = CombineMaps(workload.Annotations, statefulsetTemplate.Annotations, map[string]string{
		apps.AnnotationRefNodePool: nodepoolName,
	})
	workload.Namespace = yas.Namespace
	workload.GenerateName = getWorkloadPrefix(yas.GetName(), nodepoolName)
	if err := controllerutil.SetControllerReference(yas, workload, s.Scheme); err != nil {
		return err
	}

	// statefulset spec data
	// TODO: remove this check after adding validation webhook
	workload.Spec = *statefulsetTemplate.Spec.DeepCopy()
	if workload.Spec.Selector == nil {
		workload.Spec.Selector = &metav1.LabelSelector{
			MatchLabels: map[string]string{},
		}
	}
	workload.Spec.Selector.MatchLabels[apps.PoolNameLabelKey] = nodepoolName
	workload.Spec.Template.Labels = CombineMaps(workload.Spec.Template.Labels, statefulsetTemplate.Labels, map[string]string{
		apps.PoolNameLabelKey:               nodepoolName,
		apps.ControllerRevisionHashLabelKey: revision,
	})
	workload.Spec.Template.Spec.NodeSelector = CombineMaps(workload.Spec.Template.Spec.NodeSelector, CreateNodeSelectorByNodepoolName(nodepoolName))

	tweaks, err := GetNodePoolTweaksFromYurtAppSet(s.Client, nodepoolName, yas)
	if err != nil {
		return err
	}
	if err = ApplyTweaksToStatefulSet(workload, tweaks); err != nil {
		return err
	}
	return nil
}

func (s *StatefulSetManager) Create(yas *v1beta1.YurtAppSet, nodepoolName, revision string) error {
	klog.V(4).Infof("YurtAppSet[%s/%s] prepare to create new StatefulSet for nodepool[%s]", yas.Namespace, yas.Name, nodepoolName)

	stateful := appsv1.StatefulSet{}
	if err := s.ApplyTemplate(yas, nodepoolName, revision, &stateful); err != nil {
		klog.Errorf("YurtAppSet[%s/%s] failed to apply template for StatefulSet when creating statefulset: %v", yas.Namespace, yas.Name, err)
		return err
	}
	return s.Client.Create(context.TODO(), &stateful)
}

func (s *StatefulSetManager) Update(yas *v1beta1.YurtAppSet, workload metav1.Object, nodepoolName, revision string) error {
	klog.V(4).Infof("YurtAppSet[%s/%s] prepare to update StatefulSet[/%s/%s]", yas.Namespace, yas.Name, workload.GetNamespace(), workload.GetName())

	if nodepoolName == "" {
		klog.Warningf("nodepool name of statefulset[%s/%s] to be updated is empty", workload.GetNamespace(), workload.GetName())
	}

	stateful := &appsv1.StatefulSet{}
	var updateError error
	for i := 0; i < updateRetries; i++ {
		getError := s.Client.Get(context.TODO(), types.NamespacedName{Namespace: workload.GetNamespace(), Name: workload.GetName()}, stateful)
		if getError != nil {
			return getError
		}

		if err := s.ApplyTemplate(yas, nodepoolName, revision, stateful); err != nil {
			return err
		}
		updateError = s.Client.Update(context.TODO(), stateful)
		if updateError == nil {
			break
		}
		klog.Errorf("YurtAppSet[%s/%s] failed to update StatefulSet[%s/%s]: %v, retry", yas.Namespace, yas.Name, stateful.Namespace, stateful.Name, updateError)
	}
	return updateError
}

func (s *StatefulSetManager) List(yas *v1beta1.YurtAppSet) ([]metav1.Object, error) {
	yasSelector, err := NewLabelSelectorForYurtAppSet(yas)
	if err != nil {
		return nil, err
	}

	allStatefulSets := appsv1.StatefulSetList{}
	if err := s.Client.List(context.TODO(), &allStatefulSets); err != nil {
		return nil, err
	}

	manager, err := refmanager.New(s.Client, yasSelector, yas, s.Scheme)
	if err != nil {
		return nil, err
	}

	selected := make([]metav1.Object, 0, len(allStatefulSets.Items))
	for i := 0; i < len(allStatefulSets.Items); i++ {
		item := allStatefulSets.Items[i]
		selected = append(selected, &item)
	}

	objs, err := manager.ClaimOwnedObjects(selected)
	if err != nil {
		return nil, err
	}

	return objs, nil
}

var _ WorkloadManager = &StatefulSetManager{}
