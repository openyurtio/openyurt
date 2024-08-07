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

package v1alpha1

import (
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	yurtClient "github.com/openyurtio/openyurt/cmd/yurt-manager/app/client"
	"github.com/openyurtio/openyurt/cmd/yurt-manager/names"
	"github.com/openyurtio/openyurt/pkg/apis/network/v1alpha1"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/webhook/util"
)

const (
	WebhookName = "service"
)

// SetupWebhookWithManager sets up Cluster webhooks. 	mutate path, validatepath, error
func (webhook *ServiceHandler) SetupWebhookWithManager(mgr ctrl.Manager) (string, string, error) {
	// init
	webhook.Client = yurtClient.GetClientByControllerNameOrDie(mgr, names.VipLoadBalancerController)

	return util.RegisterWebhook(mgr, &v1alpha1.PoolService{}, webhook)
}

// +kubebuilder:webhook:path=/mutate-core-openyurt-io-v1-service,mutating=true,failurePolicy=ignore,sideEffects=None,admissionReviewVersions=v1,groups="",resources=services,verbs=create,versions=v1,name=mutate.core.v1.service.openyurt.io

// Cluster implements a validating and defaulting webhook for Cluster.
type ServiceHandler struct {
	Client client.Client
}

var _ webhook.CustomDefaulter = &ServiceHandler{}
