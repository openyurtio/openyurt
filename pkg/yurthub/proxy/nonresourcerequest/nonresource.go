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

package nonresourcerequest

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"k8s.io/apiserver/pkg/endpoints/handlers/responsewriters"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/config"
	yurtutil "github.com/openyurtio/openyurt/pkg/util"
	"github.com/openyurtio/openyurt/pkg/yurthub/cachemanager"
	"github.com/openyurtio/openyurt/pkg/yurthub/healthchecker"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage"
	"github.com/openyurtio/openyurt/pkg/yurthub/transport"
)

var nonResourceReqPaths = map[string]storage.ClusterInfoType{
	"/version":                         storage.Version,
	"/apis/discovery.k8s.io/v1":        storage.APIResourcesInfo,
	"/apis/discovery.k8s.io/v1beta1":   storage.APIResourcesInfo,
	"/apis/raven.openyurt.io/v1alpha1": storage.APIResourcesInfo,
	"/apis/raven.openyurt.io/v1beta1":  storage.APIResourcesInfo,
}

type NonResourceHandler func(kubeClient *kubernetes.Clientset, sw cachemanager.StorageWrapper, path string) http.Handler

func WrapNonResourceHandler(proxyHandler http.Handler, config *config.YurtHubConfiguration, healthChecker healthchecker.Interface) http.Handler {
	wrapMux := mux.NewRouter()

	// register handler for non resource requests
	if !yurtutil.IsNil(healthChecker) && !yurtutil.IsNil(config.StorageWrapper) {
		for path := range nonResourceReqPaths {
			wrapMux.Handle(path, localCacheHandler(nonResourceHandler, healthChecker, config.TransportAndDirectClientManager, config.StorageWrapper, path)).Methods("GET")
		}
	}

	// register handler for other requests
	wrapMux.PathPrefix("/").Handler(proxyHandler)
	return wrapMux
}

func localCacheHandler(handler NonResourceHandler, healthChecker healthchecker.Interface, clientManager transport.Interface, sw cachemanager.StorageWrapper, path string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// if cloud kube-apiserver is healthy, forward non resource request to cloud kube-apiserver
		// otherwise serve non resource request by local cache.
		u := healthChecker.PickOneHealthyBackend()
		if u != nil {
			kubeClient := clientManager.GetDirectClientset(u)
			if kubeClient != nil {
				clientset, ok := kubeClient.(*kubernetes.Clientset)
				if ok {
					handler(clientset, sw, path).ServeHTTP(w, r)
					return
				}
			}
		}

		klog.Infof("get %s non resource data from local cache when cloud-edge line off", path)
		key := &storage.ClusterInfoKey{
			ClusterInfoType: nonResourceReqPaths[path],
			UrlPath:         path,
		}
		if nonResourceData, err := sw.GetClusterInfo(key); err == nil {
			w.WriteHeader(http.StatusOK)
			writeRawJSON(nonResourceData, w)
		} else if err == storage.ErrStorageNotFound {
			w.WriteHeader(http.StatusNotFound)
			writeErrResponse(path, err, w)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			writeErrResponse(path, err, w)
		}
	})
}

func nonResourceHandler(kubeClient *kubernetes.Clientset, sw cachemanager.StorageWrapper, path string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := &storage.ClusterInfoKey{
			ClusterInfoType: nonResourceReqPaths[path],
			UrlPath:         path,
		}

		result := kubeClient.RESTClient().Get().AbsPath(path).Do(context.TODO())
		code := ptr.To(0)
		result.StatusCode(code)
		if result.Error() != nil {
			err := result.Error()
			w.WriteHeader(*code)
			writeErrResponse(path, err, w)
		} else {
			body, _ := result.Raw()
			w.WriteHeader(*code)
			writeRawJSON(body, w)
			sw.SaveClusterInfo(key, body)
		}
	})
}

func writeErrResponse(path string, err error, w http.ResponseWriter) {
	klog.Errorf("could not handle %s non resource request, %v", path, err)
	status := responsewriters.ErrorToAPIStatus(err)
	output, err := json.Marshal(status)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeRawJSON(output, w)
}

func writeRawJSON(output []byte, w http.ResponseWriter) {
	w.Header().Set(yurtutil.HttpHeaderContentType, yurtutil.HttpContentTypeJson)
	w.Write(output)
}
