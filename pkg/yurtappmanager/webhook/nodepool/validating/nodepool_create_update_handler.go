/*
Copyright 2020 The Kruise Authors.

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

package validating

import (
	"context"
	"net/http"

	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	appsv1alpha1 "github.com/alibaba/openyurt/pkg/yurtappmanager/apis/apps/v1alpha1"
	webhookutil "github.com/alibaba/openyurt/pkg/yurtappmanager/webhook/util"
)

// NodePoolCreateUpdateHandler handles UnitedDeployment
type NodePoolCreateUpdateHandler struct {
	Client client.Client

	// Decoder decodes objects
	Decoder *admission.Decoder
}

var _ webhookutil.Handler = &NodePoolCreateUpdateHandler{}

func (h *NodePoolCreateUpdateHandler) SetOptions(options webhookutil.Options) {
	return
}

// Handle handles admission requests.
func (h *NodePoolCreateUpdateHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	np := appsv1alpha1.NodePool{}

	switch req.AdmissionRequest.Operation {
	case admissionv1beta1.Create:
		klog.V(4).Info("capture the nodepool creation request")
		err := h.Decoder.Decode(req, &np)
		if err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		if allErrs := validateNodePoolSpec(&np.Spec); len(allErrs) > 0 {
			return admission.Errored(http.StatusUnprocessableEntity,
				allErrs.ToAggregate())
		}
	case admissionv1beta1.Update:
		klog.V(4).Info("capture the nodepool update request")
		err := h.Decoder.Decode(req, &np)
		if err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		onp := appsv1alpha1.NodePool{}
		err = h.Decoder.DecodeRaw(req.OldObject, &onp)
		if err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}

		if allErrs := validateNodePoolSpecUpdate(&np.Spec, &onp.Spec); len(allErrs) > 0 {
			return admission.Errored(http.StatusUnprocessableEntity,
				allErrs.ToAggregate())
		}
	case admissionv1beta1.Delete:
		klog.V(4).Info("capture the nodepool deletion request")
		err := h.Decoder.DecodeRaw(req.OldObject, &np)
		if err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		if allErrs := validateNodePoolDeletion(h.Client, &np); len(allErrs) > 0 {
			return admission.Errored(http.StatusUnprocessableEntity,
				allErrs.ToAggregate())
		}
	}

	return admission.ValidationResponse(true, "")
}

var _ admission.DecoderInjector = &NodePoolCreateUpdateHandler{}

// InjectDecoder injects the decoder into the UnitedDeploymentCreateUpdateHandler
func (h *NodePoolCreateUpdateHandler) InjectDecoder(d *admission.Decoder) error {
	h.Decoder = d
	return nil
}

var _ inject.Client = &NodePoolCreateUpdateHandler{}

// InjectClient injects the client into the PodCreateHandler
func (h *NodePoolCreateUpdateHandler) InjectClient(c client.Client) error {
	h.Client = c
	return nil
}
