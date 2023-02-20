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

package wraphandler

import (
	"net/http"

	"k8s.io/klog/v2"

	hw "github.com/openyurtio/openyurt/pkg/yurttunnel/handlerwrapper"
	"github.com/openyurtio/openyurt/pkg/yurttunnel/handlerwrapper/initializer"
	"github.com/openyurtio/openyurt/pkg/yurttunnel/handlerwrapper/localhostproxy"
	"github.com/openyurtio/openyurt/pkg/yurttunnel/handlerwrapper/tracerequest"
)

func InitHandlerWrappers(mi initializer.MiddlewareInitializer, isIPv6 bool) (hw.HandlerWrappers, error) {
	wrappers := make(hw.HandlerWrappers, 0)
	// register all of middleware here
	//
	// NOTE the register order decide the order in which
	// the middleware will be called
	//
	// e.g. there are two middleware mw1 and mw2
	// if the middlewares are registered in the following order,
	//
	// wrappers = append(wrappers, m2)
	// wrappers = append(wrappers, m1)
	//
	// then the middleware m2 will be called before the mw1
	wrappers = append(wrappers, tracerequest.NewTraceReqMiddleware())
	wrappers = append(wrappers, localhostproxy.NewLocalHostProxyMiddleware(isIPv6))

	// init all of wrappers
	for i := range wrappers {
		if err := mi.Initialize(wrappers[i]); err != nil {
			return wrappers, err
		}
	}

	return wrappers, nil
}

// WrapHandler wraps the coreHandler with all of registered middleware
// and middleware will be initialized before wrap.
func WrapHandler(coreHandler http.Handler, wrappers hw.HandlerWrappers) (http.Handler, error) {
	handler := coreHandler
	klog.V(4).Infof("%d middlewares will be added into wrap handler", len(wrappers))
	if len(wrappers) == 0 {
		return handler, nil
	}
	for i := len(wrappers) - 1; i >= 0; i-- {
		handler = wrappers[i].WrapHandler(handler)
		klog.V(2).Infof("add %s into wrap handler", wrappers[i].Name())
	}
	return handler, nil
}
