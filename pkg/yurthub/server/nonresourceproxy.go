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
	"context"
	"github.com/gorilla/mux"
	"github.com/openyurtio/openyurt/cmd/yurthub/app/config"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/rest"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"net/http"
)

var nonResourceReqPaths = []string{
	"/version",
	"/apis/discovery.k8s.io/v1",
	"/apis/discovery.k8s.io/v1beta1",
}

var cfg *config.YurtHubConfiguration
var rcm *rest.RestConfigManager

func wrapNonResourceHandler(proxyHandler http.Handler, config *config.YurtHubConfiguration, restConfigMgr *rest.RestConfigManager) http.Handler {
	wrapMux := mux.NewRouter()
	cfg = config
	rcm = restConfigMgr
	// register handler for non resource requests
	for i := range nonResourceReqPaths {
		wrapMux.HandleFunc(nonResourceReqPaths[i], withNonResourceRequest).Methods("GET")
	}

	// register handler for other requests
	wrapMux.PathPrefix("/").Handler(proxyHandler)
	return wrapMux
}

func withNonResourceRequest(w http.ResponseWriter, req *http.Request) {
	for i := range nonResourceReqPaths {
		if req.URL.Path == nonResourceReqPaths[i] {
			cacheNonResourceInfo(w, req, "non-resource-info"+nonResourceReqPaths[i], nonResourceReqPaths[i])
		}
	}

}

func cacheNonResourceInfo(w http.ResponseWriter, req *http.Request, key string, path string) {
	infoCache, err := cfg.StorageWrapper.GetRaw(key)
	copyHeader(w.Header(), req.Header)
	if err == nil {
		w.WriteHeader(http.StatusOK)
		klog.Infof("success to query the cache non-resource info: %s", key)
		_, err = w.Write(infoCache)
	} else {
		restCfg := rcm.GetRestConfig(false)
		clientSet, err := kubernetes.NewForConfig(restCfg)
		if err != nil {
			klog.Errorf("the cluster cannot be connected, the error is: %v", err)
			w.WriteHeader(http.StatusBadRequest)
		} else {
			versionInfo, err := clientSet.RESTClient().Get().AbsPath(path).Do(context.TODO()).Raw()
			if err != nil {
				klog.Errorf("the version info cannot be acquired, the error is: %v", err)
				w.WriteHeader(http.StatusBadRequest)
			}

			klog.Infof("success to non-resource info: %s", key)
			w.WriteHeader(http.StatusOK)
			_, err = w.Write(versionInfo)
			cfg.StorageWrapper.UpdateRaw(key, versionInfo)
		}

	}

}

func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		if k == "Content-Type" || k == "Content-Length" {
			for _, v := range vv {
				dst.Add(k, v)
			}
		}
	}
}
