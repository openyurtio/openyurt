package util

import (
	"net/http"

	"github.com/openyurtio/openyurt/pkg/profile"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/klog/v2"

	"github.com/gorilla/mux"
)

// RunMetaServer start a http server for serving metrics and pprof requests.
func RunMetaServer(addr string) {
	muxHandler := mux.NewRouter()
	muxHandler.Handle("/metrics", promhttp.Handler())

	// register handler for pprof
	profile.Install(muxHandler)

	metaServer := &http.Server{
		Addr:           addr,
		Handler:        muxHandler,
		MaxHeaderBytes: 1 << 20,
	}

	klog.InfoS("start handling meta requests(metrics/pprof)", "server endpoint", addr)
	go func() {
		err := metaServer.ListenAndServe()
		if err != nil {
			klog.ErrorS(err, "meta server could not listen")
		}
		klog.InfoS("meta server stopped listening", "server endpoint", addr)
	}()
}
