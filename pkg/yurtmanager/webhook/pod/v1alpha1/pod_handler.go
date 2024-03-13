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
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	"github.com/openyurtio/openyurt/pkg/yurtmanager/webhook/builder"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/webhook/util"
)

const (
	WebhookName = "pod"
)

// SetupWebhookWithManager sets up Cluster webhooks. mutate path, validate path, error
func (webhook *PodHandler) SetupWebhookWithManager(mgr ctrl.Manager) (string, string, error) {
	// init
	webhook.Client = mgr.GetClient()

	gvk, err := apiutil.GVKForObject(&corev1.Pod{}, mgr.GetScheme())
	if err != nil {
		return "", "", err
	}
	return util.GenerateMutatePath(gvk),
		util.GenerateValidatePath(gvk),
		builder.WebhookManagedBy(mgr).
			For(&corev1.Pod{}).
			WithDefaulter(webhook).
			Complete()
}

// +kubebuilder:webhook:path=/mutate-core-openyurt-io-v1-pod,mutating=true,failurePolicy=ignore,sideEffects=None,admissionReviewVersions=v1;v1beta1,groups="",resources=pods,verbs=create,versions=v1,name=mutate.core.v1.pod.openyurt.io

// PodHandler implements a validating and defaulting webhook for Cluster.
type PodHandler struct {
	Client client.Client
}

var _ builder.CustomDefaulter = &PodHandler{}
