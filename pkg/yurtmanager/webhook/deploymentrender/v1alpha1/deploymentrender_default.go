/*
Copyright 2023 The OpenYurt Authors.

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

package v1alpha1

import (
	"context"
	"fmt"
	"strings"

	v1 "k8s.io/api/apps/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/yurtappset/adapter"
)

const (
	DeploymentMutatingWebhook = "deployment-webhook"
)

var (
	resources = []string{"YurtAppSet", "YurtAppDaemon"}
)

func contain(kind string, resources []string) bool {
	for _, v := range resources {
		if kind == v {
			return true
		}
	}
	return false
}

// Default satisfies the defaulting webhook interface.
func (webhook *DeploymentRenderHandler) Default(ctx context.Context, obj runtime.Object) error {

	deployment, ok := obj.(*v1.Deployment)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a YurtAppOverrider but got a %T", obj))
	}
	if v, ok := deployment.Annotations[DeploymentMutatingWebhook]; ok && v == "mutated" {
		delete(deployment.Annotations, DeploymentMutatingWebhook)
		return nil
	}
	if deployment.OwnerReferences == nil {
		return nil
	}
	if !contain(deployment.OwnerReferences[0].Kind, resources) {
		return nil
	}
	klog.Info("start to validate deployment")
	// Get YurtAppSet/YurtAppDaemon resource of this deployment
	app := deployment.OwnerReferences[0]
	var instance client.Object
	switch app.Kind {
	case "YurtAppSet":
		instance = &v1alpha1.YurtAppSet{}

	case "YurtAppDaemon":
		instance = &v1alpha1.YurtAppDaemon{}
	default:
		return nil
	}
	if err := webhook.Client.Get(ctx, client.ObjectKey{
		Namespace: deployment.Namespace,
		Name:      app.Name,
	}, instance); err != nil {
		return err
	}
	klog.Info("Successfully get the owner of deployment")
	// Get nodepool of deployment
	nodepool := deployment.Labels["apps.openyurt.io/pool-name"]

	// resume deployment
	switch app.Kind {
	case "YurtAppSet":
		var replicas int32
		yas := instance.(*v1alpha1.YurtAppSet)
		revision := yas.Status.CurrentRevision
		deploymentAdapter := adapter.DeploymentAdapter{
			Client: webhook.Client,
			Scheme: webhook.Scheme,
		}
		for _, pool := range yas.Spec.Topology.Pools {
			if pool.Name == nodepool {
				replicas = *pool.Replicas
			}
		}
		if err := deploymentAdapter.ApplyPoolTemplate(yas, nodepool, revision, replicas, deployment); err != nil {
			return err
		}
	case "YurtAppDaemon":
		yad := instance.(*v1alpha1.YurtAppDaemon)
		yad.Spec.WorkloadTemplate.DeploymentTemplate.Spec.DeepCopyInto(&deployment.Spec)
	}

	// Get YurtAppOverrider resource of app(1 to 1)
	var allConfigRenderList v1alpha1.YurtAppOverriderList
	//listOptions := client.MatchingFields{"spec.subject.kind": app.Kind, "spec.subject.name": app.Name, "spec.subject.APIVersion": app.APIVersion}
	if err := webhook.Client.List(ctx, &allConfigRenderList, client.InNamespace(deployment.Namespace)); err != nil {
		klog.Info("error in listing YurtAppOverrider")
		return err
	}
	var overriderList = v1alpha1.YurtAppOverriderList{}
	for _, configRender := range allConfigRenderList.Items {
		if configRender.Subject.Kind == app.Kind && configRender.Subject.Name == app.Name && configRender.Subject.APIVersion == app.APIVersion {
			overriderList.Items = append(overriderList.Items, configRender)
		}
	}
	klog.Info("Successfully list YurtAppOverrider")

	if len(overriderList.Items) == 0 {
		return nil
	}
	render := overriderList.Items[0]

	klog.Info("start to render deployment")
	for _, entry := range render.Entries {
		pools := entry.Pools
		for _, pool := range pools {
			if pool == nodepool || pool == "*" {
				items := entry.Items
				// Replace items
				if err := replaceItems(deployment, items); err != nil {
					return err
				}
				klog.Info("Successfully replace items for deployment")
				// json patch and strategic merge
				patches := entry.Patches
				for i, patch := range patches {
					if strings.Contains(string(patch.Value.Raw), "{{nodepool}}") {
						newPatchString := strings.ReplaceAll(string(patch.Value.Raw), "{{nodepool}}", nodepool)
						patches[i].Value = apiextensionsv1.JSON{Raw: []byte(newPatchString)}
					}
				}

				// Implement injection
				dataStruct := v1.Deployment{}
				pc := PatchControl{
					patches:     patches,
					patchObject: deployment,
					dataStruct:  dataStruct,
				}
				if err := pc.updatePatches(); err != nil {
					return err
				}
				klog.Info("Successfully update patches for deployment")
				klog.Infof("name of deployment: %v", deployment.Name)
			}
		}
	}
	deployment.Annotations[DeploymentMutatingWebhook] = "mutated"
	if err := webhook.Client.Update(ctx, deployment); err != nil {
		klog.Infof("error to update deployment: %v", err)
		return err
	}
	klog.Info("Successfully render deployment")
	return nil
}