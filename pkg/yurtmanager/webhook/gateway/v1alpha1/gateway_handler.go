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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/openyurtio/openyurt/pkg/apis/raven/v1alpha1"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/webhook/util"
)

// SetupWebhookWithManager sets up Cluster webhooks. 	mutate path, validatepath, error
func (webhook *GatewayHandler) SetupWebhookWithManager(mgr ctrl.Manager) (string, string, error) {
	return util.RegisterWebhook(mgr, &v1alpha1.Gateway{}, webhook)
}

// Cluster implements a validating and defaulting webhook for Cluster.
type GatewayHandler struct {
}

var _ webhook.CustomDefaulter = &GatewayHandler{}
var _ webhook.CustomValidator = &GatewayHandler{}
