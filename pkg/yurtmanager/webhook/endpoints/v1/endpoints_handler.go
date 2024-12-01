/*
Copyright 2024 The OpenYurt Authors.

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

package v1

import (
	yurtClient "github.com/openyurtio/openyurt/cmd/yurt-manager/app/client"
	"github.com/openyurtio/openyurt/cmd/yurt-manager/names"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/webhook/util"
	v1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

const (
	WebhookName = "endpoints"
)

// EndpointsHandler implements a defaulting webhook for Endpoints.
type EndpointsHandler struct {
	Client client.Client
}

// SetupWebhookWithManager sets up Endpoints webhooks.
func (webhook *EndpointsHandler) SetupWebhookWithManager(mgr ctrl.Manager) (string, string, error) {
	// init
	webhook.Client = yurtClient.GetClientByControllerNameOrDie(mgr, names.NodeLifeCycleController)

	return util.RegisterWebhook(mgr, &v1.Endpoints{}, webhook)
}

// +kubebuilder:webhook:path=/mutate-core-openyurt-io-v1-endpoints,mutating=true,failurePolicy=ignore,sideEffects=None,admissionReviewVersions=v1,groups="",resources=endpoints,verbs=update,versions=v1,name=mutate.core.v1.endpoints.openyurt.io

var _ webhook.CustomDefaulter = &EndpointsHandler{}
