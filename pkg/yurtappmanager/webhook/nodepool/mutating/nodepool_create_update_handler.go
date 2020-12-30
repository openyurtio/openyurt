/*
Copyright 2020 The OpenYurt Authors.

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

package mutating

import (
	"context"
	"encoding/json"
	"net/http"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	appsv1alpha1 "github.com/alibaba/openyurt/pkg/yurtappmanager/apis/apps/v1alpha1"
	"github.com/alibaba/openyurt/pkg/yurtappmanager/util"
	webhookutil "github.com/alibaba/openyurt/pkg/yurtappmanager/webhook/util"
)

// NodePoolCreateUpdateHandler handles UnitedDeployment
type NodePoolCreateUpdateHandler struct {
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
	err := h.Decoder.Decode(req, &np)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	klog.V(5).Infof("set the nodepool(%s) selector", np.Name)
	np.Spec.Selector = &metav1.LabelSelector{
		MatchLabels: map[string]string{appsv1alpha1.LabelCurrentNodePool: np.Name},
	}

	marshalled, err := json.Marshal(&np)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	resp := admission.PatchResponseFromRaw(req.AdmissionRequest.Object.Raw,
		marshalled)
	if len(resp.Patches) > 0 {
		klog.V(5).Infof("Admit NodePool %s patches: %v", np.Name, util.DumpJSON(resp.Patches))
	}
	return resp
}

var _ admission.DecoderInjector = &NodePoolCreateUpdateHandler{}

// InjectDecoder injects the decoder into the UnitedDeploymentCreateUpdateHandler
func (h *NodePoolCreateUpdateHandler) InjectDecoder(d *admission.Decoder) error {
	h.Decoder = d
	return nil
}
