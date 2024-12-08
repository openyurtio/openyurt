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
	v1 "k8s.io/api/discovery/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	yurtClient "github.com/openyurtio/openyurt/cmd/yurt-manager/app/client"
	"github.com/openyurtio/openyurt/cmd/yurt-manager/names"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/webhook/util"
)

const (
	WebhookName = "endpointslice"
)

// EndpointSliceHandler implements a defaulting webhook for EndpointSlice.
type EndpointSliceHandler struct {
	Client client.Client
}

// SetupWebhookWithManager sets up EndpointSlice webhooks.
func (webhook *EndpointSliceHandler) SetupWebhookWithManager(mgr ctrl.Manager) (string, string, error) {
	// init
	webhook.Client = yurtClient.GetClientByControllerNameOrDie(mgr, names.NodeLifeCycleController)

	return util.RegisterWebhook(mgr, &v1.EndpointSlice{}, webhook)
}

// +kubebuilder:webhook:path=/mutate-core-openyurt-io-v1-endpointslice,mutating=true,failurePolicy=ignore,sideEffects=None,admissionReviewVersions=v1,groups="",resources=endpointslices,verbs=update,versions=v1,name=mutate.core.v1.endpointslice.openyurt.io

var _ webhook.CustomDefaulter = &EndpointSliceHandler{}
