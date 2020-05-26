package profile

import (
	"net/http"
	"net/http/pprof"

	"github.com/gorilla/mux"
)

// Install adds the Profiling webservice to the given mux.
func Install(c *mux.Router) {
	c.HandleFunc("/debug/pprof", redirectTo("/debug/pprof/"))
	c.HandleFunc("/debug/pprof/", http.HandlerFunc(pprof.Index))
	c.HandleFunc("/debug/pprof/profile", pprof.Profile)
	c.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	c.HandleFunc("/debug/pprof/trace", pprof.Trace)
}

// redirectTo redirects request to a certain destination.
func redirectTo(to string) func(http.ResponseWriter, *http.Request) {
	return func(rw http.ResponseWriter, req *http.Request) {
		http.Redirect(rw, req, to, http.StatusFound)
	}
}
