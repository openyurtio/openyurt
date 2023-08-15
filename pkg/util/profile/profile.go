/*
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

package profile

import (
	"net/http"
	"net/http/pprof"

	"github.com/gorilla/mux"
)

// Install adds the Profiling webservice to the given mux.
func Install(c *mux.Router) {
	c.HandleFunc("/debug/pprof/profile", pprof.Profile)
	c.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	c.HandleFunc("/debug/pprof/trace", pprof.Trace)
	c.HandleFunc("/debug/pprof", redirectTo("/debug/pprof/"))
	c.PathPrefix("/debug/pprof/").HandlerFunc(pprof.Index)
}

// redirectTo redirects request to a certain destination.
func redirectTo(to string) func(http.ResponseWriter, *http.Request) {
	return func(rw http.ResponseWriter, req *http.Request) {
		http.Redirect(rw, req, to, http.StatusFound)
	}
}

func GetPprofHandlers() map[string]http.Handler {
	handlers := make(map[string]http.Handler)

	handlers["/debug/pprof/"] = http.HandlerFunc(pprof.Index)
	handlers["/debug/pprof/cmdline"] = http.HandlerFunc(pprof.Cmdline)
	handlers["/debug/pprof/profile"] = http.HandlerFunc(pprof.Profile)
	handlers["/debug/pprof/symbol"] = http.HandlerFunc(pprof.Symbol)
	handlers["/debug/pprof/goroutine"] = pprof.Handler("goroutine")
	handlers["/debug/pprof/heap"] = pprof.Handler("heap")
	handlers["/debug/pprof/threadcreate"] = pprof.Handler("threadcreate")
	handlers["/debug/pprof/block"] = pprof.Handler("block")
	handlers["/debug/pprof/allocs"] = pprof.Handler("allocs")

	return handlers
}
