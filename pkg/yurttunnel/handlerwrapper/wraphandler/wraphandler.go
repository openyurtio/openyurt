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

	hw "github.com/alibaba/openyurt/pkg/yurttunnel/handlerwrapper"

	// followings are the middleware packages
	//
	// NOTE the package importing order decide the order in which
	// the middleware will be called
	//
	// e.g. there are two middleware mw1 and mw2
	// if the packages are imported in the following order,
	//
	// _ "github.com/alibaba/openyurt/pkg/yurttunnel/handlerwrapper/mw2"
	// _ "github.com/alibaba/openyurt/pkg/yurttunnel/handlerwrapper/mw1"
	//
	// then the middleware m2 will be called before the mw1

	_ "github.com/alibaba/openyurt/pkg/yurttunnel/handlerwrapper/printreqinfo"
)

// WrapWrapHandler wraps the coreHandler with handlerwrapper.Middlewares
func WrapHandler(coreHandler http.Handler) http.Handler {
	handler := coreHandler
	if len(hw.Middlewares) == 0 {
		return handler
	}
	for i := len(hw.Middlewares) - 1; i >= 0; i-- {
		handler = hw.Middlewares[i](handler)
	}
	return handler
}
