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
