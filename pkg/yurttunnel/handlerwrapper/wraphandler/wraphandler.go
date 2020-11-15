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

	hw "github.com/alibaba/openyurt/pkg/yurttunnel/handlerwrapper"
	"github.com/alibaba/openyurt/pkg/yurttunnel/handlerwrapper/initializer"
	"github.com/alibaba/openyurt/pkg/yurttunnel/handlerwrapper/tracerequest"
)

func init() {
	// register all of middleware here
	//
	// NOTE the register order decide the order in which
	// the middleware will be called
	//
	// e.g. there are two middleware mw1 and mw2
	// if the middlewares are registered in the following order,
	//
	// hw.Register(mw2)
	// hw.Register(mw1)
	//
	// then the middleware m2 will be called before the mw1
	hw.Register(tracerequest.NewTraceReqMiddleware())
}

// WrapWrapHandler wraps the coreHandler with all of registered middleware
// and middleware will be initialized before wrap.
func WrapHandler(coreHandler http.Handler, mi initializer.MiddlewareInitializer) (http.Handler, error) {
	handler := coreHandler
	klog.V(4).Infof("%d middlewares will be added into wrap handler", len(hw.Middlewares))
	if len(hw.Middlewares) == 0 {
		return handler, nil
	}
	for i := len(hw.Middlewares) - 1; i >= 0; i-- {
		if err := mi.Initialize(hw.Middlewares[i]); err != nil {
			return handler, err
		}

		handler = hw.Middlewares[i].WrapHandler(handler)
		klog.V(2).Infof("add %s into wrap handler", hw.Middlewares[i].Name())
	}
	return handler, nil
}
