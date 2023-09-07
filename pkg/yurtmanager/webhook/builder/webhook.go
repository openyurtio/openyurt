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

package builder

import (
	"errors"
	"net/http"
	"net/url"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/controller-runtime/pkg/webhook/conversion"

	"github.com/openyurtio/openyurt/pkg/yurtmanager/webhook/util"
)

// WebhookBuilder builds a Webhook.
type WebhookBuilder struct {
	apiType       runtime.Object
	withDefaulter CustomDefaulter
	withValidator CustomValidator
	gvk           schema.GroupVersionKind
	mgr           manager.Manager
	config        *rest.Config
}

// WebhookManagedBy allows inform its manager.Manager.
func WebhookManagedBy(m manager.Manager) *WebhookBuilder {
	return &WebhookBuilder{mgr: m}
}

// For takes a runtime.Object which should be a CR.
// If the given object implements the admission.Defaulter interface, a MutatingWebhook will be wired for this type.
// If the given object implements the admission.Validator interface, a ValidatingWebhook will be wired for this type.
func (blder *WebhookBuilder) For(apiType runtime.Object) *WebhookBuilder {
	blder.apiType = apiType
	return blder
}

// WithDefaulter takes a WithDefaulter interface, a MutatingWebhook will be wired for this type.
func (blder *WebhookBuilder) WithDefaulter(defaulter CustomDefaulter) *WebhookBuilder {
	blder.withDefaulter = defaulter
	return blder
}

// WithValidator takes a WithValidator interface, a ValidatingWebhook will be wired for this type.
func (blder *WebhookBuilder) WithValidator(validator CustomValidator) *WebhookBuilder {
	blder.withValidator = validator
	return blder
}

// Complete builds the webhook.
func (blder *WebhookBuilder) Complete() error {
	// Set the Config
	blder.loadRestConfig()

	// Set the Webhook if needed
	return blder.registerWebhooks()
}

func (blder *WebhookBuilder) loadRestConfig() {
	if blder.config == nil {
		blder.config = blder.mgr.GetConfig()
	}
}

func (blder *WebhookBuilder) registerWebhooks() error {
	typ, err := blder.getType()
	if err != nil {
		return err
	}

	// Create webhook(s) for each type
	blder.gvk, err = apiutil.GVKForObject(typ, blder.mgr.GetScheme())
	if err != nil {
		return err
	}

	blder.registerDefaultingWebhook()
	blder.registerValidatingWebhook()

	err = blder.registerConversionWebhook()
	if err != nil {
		return err
	}
	return nil
}

func (blder *WebhookBuilder) getType() (runtime.Object, error) {
	if blder.apiType != nil {
		return blder.apiType, nil
	}
	return nil, errors.New("for() must be called with a valid object")
}

// registerDefaultingWebhook registers a defaulting webhook if th.
func (blder *WebhookBuilder) registerDefaultingWebhook() {
	mwh := blder.getDefaultingWebhook()
	if mwh != nil {
		path := util.GenerateMutatePath(blder.gvk)

		// Checking if the path is already registered.
		// If so, just skip it.
		if !blder.isAlreadyHandled(path) {
			klog.Info("Registering a mutating webhook",
				"GVK", blder.gvk,
				"path", path)
			blder.mgr.GetWebhookServer().Register(path, mwh)
		}
	}
}

func (blder *WebhookBuilder) getDefaultingWebhook() *admission.Webhook {
	if defaulter := blder.withDefaulter; defaulter != nil {
		return WithCustomDefaulter(blder.apiType, defaulter)
	}
	if defaulter, ok := blder.apiType.(admission.Defaulter); ok {
		return admission.DefaultingWebhookFor(defaulter)
	}
	klog.Info(
		"skip registering a mutating webhook, object does not implement admission.Defaulter or WithDefaulter wasn't called",
		"GVK", blder.gvk)
	return nil
}

func (blder *WebhookBuilder) isAlreadyHandled(path string) bool {
	if blder.mgr.GetWebhookServer().WebhookMux == nil {
		return false
	}
	h, p := blder.mgr.GetWebhookServer().WebhookMux.Handler(&http.Request{URL: &url.URL{Path: path}})
	if p == path && h != nil {
		return true
	}
	return false
}

func (blder *WebhookBuilder) registerValidatingWebhook() {
	vwh := blder.getValidatingWebhook()
	if vwh != nil {
		path := util.GenerateValidatePath(blder.gvk)

		// Checking if the path is already registered.
		// If so, just skip it.
		if !blder.isAlreadyHandled(path) {
			klog.Info("Registering a validating webhook",
				"GVK", blder.gvk,
				"path", path)
			blder.mgr.GetWebhookServer().Register(path, vwh)
		}
	}
}

func (blder *WebhookBuilder) getValidatingWebhook() *admission.Webhook {
	if validator := blder.withValidator; validator != nil {
		return WithCustomValidator(blder.apiType, validator)
	}
	if validator, ok := blder.apiType.(admission.Validator); ok {
		return admission.ValidatingWebhookFor(validator)
	}
	klog.Info(
		"skip registering a validating webhook, object does not implement admission.Validator or WithValidator wasn't called",
		"GVK", blder.gvk)
	return nil
}

func (blder *WebhookBuilder) registerConversionWebhook() error {
	ok, err := conversion.IsConvertible(blder.mgr.GetScheme(), blder.apiType)
	if err != nil {
		klog.Error(err, "conversion check failed", "GVK", blder.gvk)
		return err
	}
	if ok {
		if !blder.isAlreadyHandled("/convert") {
			blder.mgr.GetWebhookServer().Register("/convert", &conversion.Webhook{})
		}
		klog.Info("Conversion webhook enabled", "GVK", blder.gvk)
	}

	return nil
}
