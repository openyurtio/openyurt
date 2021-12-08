/*
Copyright 2018 The Kubernetes Authors.
Copyright 2021 The OpenYurt Authors.

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

package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	goruntime "runtime"
	"sync"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	genericapifilters "k8s.io/apiserver/pkg/endpoints/filters"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	apiserver "k8s.io/apiserver/pkg/server"
	genericfilters "k8s.io/apiserver/pkg/server/filters"
	"k8s.io/apiserver/pkg/server/healthz"
	"k8s.io/apiserver/pkg/server/mux"
	"k8s.io/apiserver/pkg/server/routes"
	componentbaseconfig "k8s.io/component-base/config"
	"k8s.io/component-base/metrics/legacyregistry"
	_ "k8s.io/component-base/metrics/prometheus/workqueue" // for workqueue metric registration
)

var (
	Scheme = runtime.NewScheme()
	Codecs = serializer.NewCodecFactory(Scheme)
)

// BuildHandlerChain builds a handler chain with a base handler and CompletedConfig.
func BuildHandlerChain(apiHandler http.Handler, authorizationInfo *apiserver.AuthorizationInfo, authenticationInfo *apiserver.AuthenticationInfo) http.Handler {
	requestInfoResolver := &apirequest.RequestInfoFactory{}
	failedHandler := genericapifilters.Unauthorized(Codecs, false)

	handler := apiHandler
	if authorizationInfo != nil {
		handler = genericapifilters.WithAuthorization(apiHandler, authorizationInfo.Authorizer, Codecs)
	}
	if authenticationInfo != nil {
		handler = genericapifilters.WithAuthentication(handler, authenticationInfo.Authenticator, failedHandler, nil)
	}
	handler = genericapifilters.WithRequestInfo(handler, requestInfoResolver)
	handler = genericapifilters.WithCacheControl(handler)
	handler = genericfilters.WithPanicRecovery(handler)

	return handler
}

// NewBaseHandler takes in CompletedConfig and returns a handler.
func NewBaseHandler(c *componentbaseconfig.DebuggingConfiguration, checks ...healthz.HealthChecker) *mux.PathRecorderMux {
	mux := mux.NewPathRecorderMux("controller-manager")
	healthz.InstallHandler(mux, checks...)
	if c.EnableProfiling {
		routes.Profiling{}.Install(mux)
		if c.EnableContentionProfiling {
			goruntime.SetBlockProfileRate(1)
		}
	}
	InstallHandler(mux)
	//lint:ignore SA1019 See the Metrics Stability Migration KEP
	mux.Handle("/metrics", legacyregistry.Handler())

	return mux
}

// The following comes from k8s.io/kubernetes/pkg/util/configz"
var (
	configsGuard sync.RWMutex
	configs      = map[string]*Config{}
)

// Config is a handle to a ComponentConfig object. Don't create these directly;
// use New() instead.
type Config struct {
	val interface{}
}

// InstallHandler adds an HTTP handler on the given mux for the "/configz"
// endpoint which serves all registered ComponentConfigs in JSON format.
func InstallHandler(m configzmux) {
	m.Handle("/configz", http.HandlerFunc(handle))
}

type configzmux interface {
	Handle(string, http.Handler)
}

func handle(w http.ResponseWriter, r *http.Request) {
	if err := write(w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func write(w http.ResponseWriter) error {
	var b []byte
	var err error
	func() {
		configsGuard.RLock()
		defer configsGuard.RUnlock()
		b, err = json.Marshal(configs)
	}()
	if err != nil {
		return fmt.Errorf("error marshaling json: %v", err)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, err = w.Write(b)
	return err
}
