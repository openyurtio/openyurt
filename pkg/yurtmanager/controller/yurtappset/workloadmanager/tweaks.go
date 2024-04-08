/*
Copyright 2024 The OpenYurt Authors.

Licensed under the Apache License, Version 2.0 (the License);
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an AS IS BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package workloadmanager

import (
	"context"
	"encoding/json"
	"strings"

	jsonpatch "github.com/evanphx/json-patch"
	v1 "k8s.io/api/apps/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openyurtio/openyurt/pkg/apis/apps"
	"github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
)

func GetNodePoolTweaksFromYurtAppSet(cli client.Client, nodepoolName string, yas *v1beta1.YurtAppSet) (tweaksList []*v1beta1.Tweaks, err error) {
	tweaksList = []*v1beta1.Tweaks{}

	np := v1beta1.NodePool{}
	if err = cli.Get(context.TODO(), client.ObjectKey{Name: nodepoolName}, &np); err != nil {
		return
	}

	for _, yasTweak := range yas.Spec.Workload.WorkloadTweaks {
		if isNodePoolRelated(&np, yasTweak.Pools, yasTweak.NodePoolSelector) {
			klog.V(4).Infof("nodepool %s is related to yurtappset %s/%s, add tweaks", nodepoolName, yas.Namespace, yas.Name)
			tweaksList = append(tweaksList, &yasTweak.Tweaks)
		}
	}
	return
}

// TODO: optimize implementation of applying tweaks
func ApplyTweaksToDeployment(deployment *v1.Deployment, tweaks []*v1beta1.Tweaks) error {
	if len(tweaks) > 0 {
		applyBasicTweaksToDeployment(deployment, tweaks)
		if err := applyAdvancedTweaksToDeployment(deployment, tweaks); err != nil {
			return err
		}
	}
	return nil
}

func ApplyTweaksToStatefulSet(statefulset *v1.StatefulSet, tweaks []*v1beta1.Tweaks) error {
	if len(tweaks) > 0 {
		applyBasicTweaksToStatefulSet(statefulset, tweaks)
		if err := applyAdvancedTweaksToStatefulSet(statefulset, tweaks); err != nil {
			return err
		}
	}
	return nil
}

func applyBasicTweaksToDeployment(deployment *v1.Deployment, basicTweaks []*v1beta1.Tweaks) {
	for _, item := range basicTweaks {
		if item.Replicas != nil {
			klog.V(4).Infof("Apply BasicTweaks successfully: overwrite replicas to %d in deployment %s/%s", *item.Replicas, deployment.Name, deployment.Namespace)
			deployment.Spec.Replicas = item.Replicas
		}

		for _, item := range item.ContainerImages {
			for i := range deployment.Spec.Template.Spec.Containers {
				if deployment.Spec.Template.Spec.Containers[i].Name == item.Name {
					klog.V(5).Infof("Apply BasicTweaks successfully: overwrite container %s 's image to %s in deployment %s/%s", item.Name, item.TargetImage, deployment.Name, deployment.Namespace)
					deployment.Spec.Template.Spec.Containers[i].Image = item.TargetImage
				}
			}
			for i := range deployment.Spec.Template.Spec.InitContainers {
				if deployment.Spec.Template.Spec.InitContainers[i].Name == item.Name {
					klog.V(5).Infof("Apply BasicTweaks successfully: overwrite init container %s 's image to %s in deployment %s/%s", item.Name, item.TargetImage, deployment.Name, deployment.Namespace)
					deployment.Spec.Template.Spec.InitContainers[i].Image = item.TargetImage
				}
			}
		}

	}
}

func applyBasicTweaksToStatefulSet(statefulset *v1.StatefulSet, basicTweaks []*v1beta1.Tweaks) {
	for _, item := range basicTweaks {
		if item.Replicas != nil {
			klog.V(4).Infof("Apply BasicTweaks successfully: overwrite replicas to %d in statefulset %s/%s", *item.Replicas, statefulset.Name, statefulset.Namespace)
			statefulset.Spec.Replicas = item.Replicas
		}
		for _, item := range item.ContainerImages {
			for i := range statefulset.Spec.Template.Spec.Containers {
				if statefulset.Spec.Template.Spec.Containers[i].Name == item.Name {
					klog.V(5).Infof("Apply BasicTweaks successfully: overwrite container %s 's image to %s in statefulset %s/%s", item.Name, item.TargetImage, statefulset.Name, statefulset.Namespace)
					statefulset.Spec.Template.Spec.Containers[i].Image = item.TargetImage
				}
			}
			for i := range statefulset.Spec.Template.Spec.InitContainers {
				if statefulset.Spec.Template.Spec.InitContainers[i].Name == item.Name {
					klog.V(5).Infof("Apply BasicTweaks successfully: overwrite init container %s 's image to %s in statefulset %s/%s", item.Name, item.TargetImage, statefulset.Name, statefulset.Namespace)
					statefulset.Spec.Template.Spec.InitContainers[i].Image = item.TargetImage
				}
			}
		}
	}
}

type patchOperation struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
}

func applyAdvancedTweaksToDeployment(deployment *v1.Deployment, tweaks []*v1beta1.Tweaks) error {
	// convert into json patch format
	nodepoolName := deployment.Labels[apps.PoolNameLabelKey]
	patchOperations := preparePatchOperations(tweaks, nodepoolName)

	if len(patchOperations) == 0 {
		return nil
	}

	patchBytes, err := json.Marshal(patchOperations)
	if err != nil {
		return err
	}
	patchedData, err := json.Marshal(deployment)
	if err != nil {
		return err
	}

	// conduct json patch
	patchObj, err := jsonpatch.DecodePatch(patchBytes)
	if err != nil {
		return err
	}
	patchedData, err = patchObj.Apply(patchedData)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(patchedData, deployment); err != nil {
		return err
	}

	klog.V(5).Infof("Apply AdvancedTweaks %v successfully: patched deployment %+v", patchOperations, deployment)
	return nil
}

func applyAdvancedTweaksToStatefulSet(statefulset *v1.StatefulSet, tweaks []*v1beta1.Tweaks) error {
	// convert into json patch format
	nodepoolName := statefulset.Labels[apps.PoolNameLabelKey]
	patchOperations := preparePatchOperations(tweaks, nodepoolName)
	if len(patchOperations) == 0 {
		return nil
	}
	patchBytes, err := json.Marshal(patchOperations)
	if err != nil {
		return err
	}
	patchedData, err := json.Marshal(statefulset)
	if err != nil {
		return err
	}
	// conduct json patch
	patchObj, err := jsonpatch.DecodePatch(patchBytes)
	if err != nil {
		return err
	}
	patchedData, err = patchObj.Apply(patchedData)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(patchedData, statefulset); err != nil {
		return err
	}

	klog.V(5).Infof("Apply AdvancedTweaks %v successfully: patched statefulset %+v", patchOperations, statefulset)
	return nil
}

func preparePatchOperations(tweaks []*v1beta1.Tweaks, poolName string) []patchOperation {
	var patchOperations []patchOperation
	for _, tweak := range tweaks {
		for _, patch := range tweak.Patches {
			if strings.Contains(string(patch.Value.Raw), "{{nodepool-name}}") {
				patch.Value = apiextensionsv1.JSON{Raw: []byte(strings.ReplaceAll(string(patch.Value.Raw), "{{nodepool-name}}", poolName))}
			}
			patchOperations = append(patchOperations, patchOperation{
				Op:    string(patch.Operation),
				Path:  patch.Path,
				Value: patch.Value,
			})
		}
	}
	return patchOperations
}
