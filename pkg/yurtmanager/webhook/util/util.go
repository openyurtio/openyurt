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

package util

import (
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	yurtutil "github.com/openyurtio/openyurt/pkg/util"
)

const (
	MutatingWebhookConfigurationName   = "yurt-manager-mutating-webhook-configuration"
	ValidatingWebhookConfigurationName = "yurt-manager-validating-webhook-configuration"
	emptyGroupName                     = "core.openyurt.io"
)

var namespace = "kube-system"

func GetHost() string {
	return os.Getenv("WEBHOOK_HOST")
}

func GetNamespace() string {
	return namespace
}

func SetNamespace(ns string) {
	namespace = ns
}

func GetSecretName() string {
	if name := os.Getenv("SECRET_NAME"); len(name) > 0 {
		return name
	}
	return "yurt-manager-webhook-certs"
}

func GetServiceName() string {
	if name := os.Getenv("SERVICE_NAME"); len(name) > 0 {
		return name
	}
	return "yurt-manager-webhook-service"
}

func GetWebHookPort() int {
	port := 10273
	if p := os.Getenv("WEBHOOK_PORT"); len(p) > 0 {
		if p, err := strconv.ParseInt(p, 10, 32); err == nil {
			port = int(p)
		} else {
			klog.Fatalf("could not convert WEBHOOK_PORT=%v in env: %v", p, err)
		}
	}
	return port
}

func GetCertDir() string {
	if p := os.Getenv("WEBHOOK_CERT_DIR"); len(p) > 0 {
		return p
	}
	return "/tmp/yurt-manager-webhook-certs"
}

func GetCertWriter() string {
	return os.Getenv("WEBHOOK_CERT_WRITER")
}

func GenerateMutatePath(gvk schema.GroupVersionKind) string {
	groupName := gvk.Group
	if groupName == "" {
		groupName = emptyGroupName
	}

	return "/mutate-" + strings.ReplaceAll(groupName, ".", "-") + "-" +
		gvk.Version + "-" + strings.ToLower(gvk.Kind)
}

func GenerateValidatePath(gvk schema.GroupVersionKind) string {
	groupName := gvk.Group
	if groupName == "" {
		groupName = emptyGroupName
	}
	return "/validate-" + strings.ReplaceAll(groupName, ".", "-") + "-" +
		gvk.Version + "-" + strings.ToLower(gvk.Kind)
}

func RegisterIndependentWebhook(mgr ctrl.Manager, obj runtime.Object, defaulter admission.CustomDefaulter, validator admission.CustomValidator) (string, string, error) {
	var mutatePath, validatorPath string
	gvk, err := apiutil.GVKForObject(obj, mgr.GetScheme())
	if err != nil {
		return mutatePath, validatorPath, err
	}

	if !yurtutil.IsNil(defaulter) {
		mutateWebhook := admission.WithCustomDefaulter(mgr.GetScheme(), obj, defaulter).WithRecoverPanic(true)
		mutatePath = GenerateMutatePath(gvk)
		if !isAlreadyHandled(mgr, mutatePath) {
			klog.Infof("Registering a mutating webhook, GVK: %s with path: %s", gvk.String(), mutatePath)
			mgr.GetWebhookServer().Register(mutatePath, mutateWebhook)
		}
	}

	if !yurtutil.IsNil(validator) {
		validatorWebhook := admission.WithCustomValidator(mgr.GetScheme(), obj, validator).WithRecoverPanic(true)
		validatorPath = GenerateValidatePath(gvk)
		if !isAlreadyHandled(mgr, validatorPath) {
			klog.Infof("Registering a validating webhook, GVK: %s with path: %s", gvk.String(), validatorPath)
			mgr.GetWebhookServer().Register(validatorPath, validatorWebhook)
		}
	}

	return mutatePath, validatorPath, nil
}

func isAlreadyHandled(mgr ctrl.Manager, path string) bool {
	if mgr.GetWebhookServer().WebhookMux() == nil {
		return false
	}
	h, p := mgr.GetWebhookServer().WebhookMux().Handler(&http.Request{URL: &url.URL{Path: path}})
	if p == path && h != nil {
		return true
	}
	return false
}

// IsWebhookDisabled check if a specified webhook disabled or not.
func IsWebhookDisabled(name string, webhooks []string) bool {
	hasStar := false
	for _, ctrl := range webhooks {
		if ctrl == name {
			return true
		}
		if ctrl == "*" {
			hasStar = true
		}
	}
	return hasStar
}
