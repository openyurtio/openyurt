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
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"k8s.io/apiserver/pkg/endpoints/handlers/responsewriters"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/config"
	"github.com/openyurtio/openyurt/pkg/yurthub/cachemanager"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/rest"
)

var nonResourceReqPaths = []string{
	"/version",
	"/apis/discovery.k8s.io/v1",
	"/apis/discovery.k8s.io/v1beta1",
}

type NonResourceHandler func(kubeClient *kubernetes.Clientset, sw cachemanager.StorageWrapper, path string) http.Handler

func wrapNonResourceHandler(proxyHandler http.Handler, config *config.YurtHubConfiguration, restMgr *rest.RestConfigManager) http.Handler {
	wrapMux := mux.NewRouter()
	// register handler for non resource requests
	for i := range nonResourceReqPaths {
		wrapMux.Handle(nonResourceReqPaths[i], localCacheHandler(nonResourceHandler, restMgr, config.StorageWrapper, nonResourceReqPaths[i])).Methods("GET")
	}

	// register handler for other requests
	wrapMux.PathPrefix("/").Handler(proxyHandler)
	return wrapMux
}

func localCacheHandler(handler NonResourceHandler, restMgr *rest.RestConfigManager, sw cachemanager.StorageWrapper, path string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := fmt.Sprintf("non-reosurce-info%s", path)
		restCfg := restMgr.GetRestConfig(true)
		if restCfg == nil {
			klog.Infof("get %s non resource data from local cache when cloud-edge line off", path)
			if nonResourceData, err := sw.GetRaw(key); err != nil {
				writeErrResponse(path, err, w)
			} else {
				writeRawJSON(nonResourceData, w)
			}
			return
		}

		kubeClient, err := kubernetes.NewForConfig(restCfg)
		if err != nil {
			writeErrResponse(path, err, w)
			return
		}
		handler(kubeClient, sw, path).ServeHTTP(w, r)
	})
}

func nonResourceHandler(kubeClient *kubernetes.Clientset, sw cachemanager.StorageWrapper, path string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := fmt.Sprintf("non-reosurce-info%s", path)
		if nonResourceData, err := kubeClient.RESTClient().Get().AbsPath(path).Do(context.TODO()).Raw(); err != nil {
			writeErrResponse(path, err, w)
		} else {
			writeRawJSON(nonResourceData, w)
			sw.UpdateRaw(key, nonResourceData)
		}
	})
}

func writeErrResponse(path string, err error, w http.ResponseWriter) {
	klog.Errorf("failed to handle %s non resource request, %v", path, err)
	status := responsewriters.ErrorToAPIStatus(err)
	output, err := json.Marshal(status)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeRawJSON(output, w)
}

func writeRawJSON(output []byte, w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(output)
}
