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

package validating

import (
	"context"
	"fmt"
	"net/http"

	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	appsv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
)

// NodePoolCreateUpdateHandler handles NodePool
type NodePoolCreateUpdateHandler struct {
	// Decoder decodes objects
	Decoder *admission.Decoder
}

const (
	webhookName = "NodePool-validate-webhook"
)

func Format(format string, args ...interface{}) string {
	s := fmt.Sprintf(format, args...)
	return fmt.Sprintf("%s: %s", webhookName, s)
}

var _ admission.Handler = &NodePoolCreateUpdateHandler{}

// Handle handles admission requests.
func (h *NodePoolCreateUpdateHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	// Note !!!!!!!!!!
	// We strongly recommend use Format() to  encapsulation because Format() can print logs by module
	// @kadisi
	klog.Infof(Format("Handle NodePool %s/%s", req.Namespace, req.Name))

	obj := &appsv1alpha1.NodePool{}

	err := h.Decoder.Decode(req, obj)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	if err := validate(obj); err != nil {
		klog.Warningf("Error validate NodePool %s: %v", obj.Name, err)
		return admission.Errored(http.StatusBadRequest, err)
	}

	return admission.ValidationResponse(true, "allowed")
}

func validate(obj *appsv1alpha1.NodePool) error {

	klog.Infof(Format("Validate NodePool %s successfully ...", klog.KObj(obj)))

	return nil
}

var _ admission.DecoderInjector = &NodePoolCreateUpdateHandler{}

// InjectDecoder injects the decoder into the NodePoolCreateUpdateHandler
func (h *NodePoolCreateUpdateHandler) InjectDecoder(d *admission.Decoder) error {
	h.Decoder = d
	return nil
}
