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
	v1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	yurtClient "github.com/openyurtio/openyurt/cmd/yurt-manager/app/client"
	"github.com/openyurtio/openyurt/cmd/yurt-manager/names"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/webhook/util"
)

// SetupWebhookWithManager sets up Cluster webhooks. 	mutate path, validatepath, error
func (webhook *DeploymentRenderHandler) SetupWebhookWithManager(mgr ctrl.Manager) (string, string, error) {
	// init
	webhook.Client = yurtClient.GetClientByControllerNameOrDie(mgr, names.YurtAppOverriderController)
	webhook.Scheme = mgr.GetScheme()

	return util.RegisterWebhook(mgr, &v1.Deployment{}, webhook)
}

// +kubebuilder:webhook:path=/mutate-apps-v1-deployment,mutating=true,failurePolicy=ignore,groups=apps,resources=deployments,verbs=create;update,versions=v1,name=mutate.apps.v1.deployment,sideEffects=None,admissionReviewVersions=v1

// Cluster implements a validating and defaulting webhook for Cluster.
type DeploymentRenderHandler struct {
	Client client.Client
	Scheme *runtime.Scheme
}

var _ webhook.CustomDefaulter = &DeploymentRenderHandler{}
