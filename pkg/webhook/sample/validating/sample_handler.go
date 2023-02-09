/*
Copyright 2023 The OpenYurt Authors.

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
	"fmt"
	"net/http"

	appsv1beta1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var (
	defaultMaxImagesPerNode = 256
)

// SampleCreateUpdateHandler handles Sample
type SampleCreateUpdateHandler struct {
	// Decoder decodes objects
	Decoder *admission.Decoder
}

const (
	webhookName = "Sample-validate-webhook"
)

func Format(format string, args ...interface{}) string {
	s := fmt.Sprintf(format, args...)
	return fmt.Sprintf("%s: %s", webhookName, s)
}

var _ admission.Handler = &SampleCreateUpdateHandler{}

// Handle handles admission requests.
func (h *SampleCreateUpdateHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	// Note !!!!!!!!!!
	// We strongly recommend use Format() to  encapsulation because Format() can print logs by module
	// @kadisi
	klog.Infof(Format("Handle Sample %s/%s", req.Namespace, req.Name))

	obj := &appsv1beta1.Sample{}

	err := h.Decoder.Decode(req, obj)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	if err := validate(obj); err != nil {
		klog.Warningf("Error validate Sample %s: %v", obj.Name, err)
		return admission.Errored(http.StatusBadRequest, err)
	}

	return admission.ValidationResponse(true, "allowed")
}

func validate(obj *appsv1beta1.Sample) error {

	klog.Infof(Format("Validate Sample %s sucessfully ...", klog.KObj(obj)))

	return nil
}

var _ admission.DecoderInjector = &SampleCreateUpdateHandler{}

// InjectDecoder injects the decoder into the SampleCreateUpdateHandler
func (h *SampleCreateUpdateHandler) InjectDecoder(d *admission.Decoder) error {
	h.Decoder = d
	return nil
}
