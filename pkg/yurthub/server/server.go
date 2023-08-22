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

package server

import (
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/config"
	"github.com/openyurtio/openyurt/pkg/util/profile"
	"github.com/openyurtio/openyurt/pkg/yurthub/certificate"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/rest"
	ota "github.com/openyurtio/openyurt/pkg/yurthub/otaupdate"
	otautil "github.com/openyurtio/openyurt/pkg/yurthub/otaupdate/util"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

// RunYurtHubServers is used to start up all servers for yurthub
func RunYurtHubServers(cfg *config.YurtHubConfiguration,
	proxyHandler http.Handler,
	rest *rest.RestConfigManager,
	stopCh <-chan struct{}) error {
	hubServerHandler := mux.NewRouter()
	registerHandlers(hubServerHandler, cfg, rest)

	// start yurthub http server for serving metrics, pprof.
	if cfg.YurtHubServerServing != nil {
		if err := cfg.YurtHubServerServing.Serve(hubServerHandler, 0, stopCh); err != nil {
			return err
		}
	}

	// start yurthub proxy servers for forwarding requests to cloud kube-apiserver
	if cfg.WorkingMode == util.WorkingModeEdge {
		proxyHandler = wrapNonResourceHandler(proxyHandler, cfg, rest)
	}
	if cfg.YurtHubProxyServerServing != nil {
		if err := cfg.YurtHubProxyServerServing.Serve(proxyHandler, 0, stopCh); err != nil {
			return err
		}
	}

	if cfg.YurtHubDummyProxyServerServing != nil {
		if err := cfg.YurtHubDummyProxyServerServing.Serve(proxyHandler, 0, stopCh); err != nil {
			return err
		}
	}

	if cfg.YurtHubSecureProxyServerServing != nil {
		if _, err := cfg.YurtHubSecureProxyServerServing.Serve(proxyHandler, 0, stopCh); err != nil {
			return err
		}
	}

	return nil
}

// registerHandler registers handlers for yurtHubServer, and yurtHubServer can handle requests like profiling, healthz, update token.
func registerHandlers(c *mux.Router, cfg *config.YurtHubConfiguration, rest *rest.RestConfigManager) {
	// register handlers for update join token
	c.Handle("/v1/token", updateTokenHandler(cfg.CertManager)).Methods("POST", "PUT")

	// register handler for health check
	c.HandleFunc("/v1/healthz", healthz).Methods("GET")
	c.Handle("/v1/readyz", readyz(cfg.CertManager)).Methods("GET")

	// register handler for profile
	if cfg.EnableProfiling {
		profile.Install(c)
	}

	// register handler for metrics
	c.Handle("/metrics", promhttp.Handler())

	// register handler for ota upgrade
	if cfg.WorkingMode == util.WorkingModeEdge {
		c.Handle("/pods", ota.GetPods(cfg.StorageWrapper)).Methods("GET")
	} else {
		c.Handle("/pods", getPodList(cfg.SharedFactory, cfg.NodeName)).Methods("GET")
	}
	c.Handle("/openyurt.io/v1/namespaces/{ns}/pods/{podname}/upgrade",
		ota.HealthyCheck(rest, cfg.NodeName, ota.UpdatePod)).Methods("POST")
}

// healthz returns ok for healthz request
func healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "OK")
}

// readyz is used for checking certificates are ready or not
func readyz(certificateMgr certificate.YurtCertificateManager) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ready := certificateMgr.Ready()
		if ready {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "OK")
		} else {
			http.Error(w, "certificates are not ready", http.StatusInternalServerError)
		}
	})
}

func getPodList(sharedFactory informers.SharedInformerFactory, nodeName string) http.Handler {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		podLister := sharedFactory.Core().V1().Pods().Lister()
		podList, err := podLister.List(labels.Everything())
		if err != nil {
			klog.Errorf("get pods key failed, %v", err)
			otautil.WriteErr(w, "Get pods key failed", http.StatusInternalServerError)
			return
		}
		pl := new(corev1.PodList)
		for i := range podList {
			pl.Items = append(pl.Items, *podList[i])
		}

		data, err := otautil.EncodePods(pl)
		if err != nil {
			klog.Errorf("Encode pod list failed, %v", err)
			otautil.WriteErr(w, "Encode pod list failed", http.StatusInternalServerError)
		}
		otautil.WriteJSONResponse(w, data)
	})
}
