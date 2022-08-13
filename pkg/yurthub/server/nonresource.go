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
	"fmt"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"k8s.io/apiserver/pkg/endpoints/handlers/responsewriters"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/config"
	"github.com/openyurtio/openyurt/pkg/yurthub/cachemanager"
)

var nonResourceReqPaths = []string{
	"/version",
	"/apis/discovery.k8s.io/v1",
	"/apis/discovery.k8s.io/v1beta1",
}

var cfg *config.YurtHubConfiguration
var clientSet *kubernetes.Clientset

func wrapNonResourceHandler(proxyHandler http.Handler, config *config.YurtHubConfiguration, cs *kubernetes.Clientset) http.Handler {
	wrapMux := mux.NewRouter()
	cfg = config
	clientSet = cs
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
			cacheNonResourceInfo(w, req, cfg.StorageWrapper, fmt.Sprintf("non-reosurce-info%s", nonResourceReqPaths[i]), nonResourceReqPaths[i], false)
			break
		}
	}

}

func cacheNonResourceInfo(w http.ResponseWriter, req *http.Request, sw cachemanager.StorageWrapper, key string, path string, isFake bool) {
	nonResourceInfo, err := getClientSetQueryInfo(path, isFake)
	copyHeader(w.Header(), req.Header)
	if err == nil {
		_, err = w.Write(nonResourceInfo)
		if err != nil {
			klog.Errorf("failed to write the non-resource info, the error is: %v", err)
			ErrNonResource(err, w, req)
			return
		}

		klog.Infof("success to query the cache non-resource info: %s", key)
		w.WriteHeader(http.StatusOK)
		sw.UpdateRaw(key, nonResourceInfo)
	} else {
		infoCache, err := sw.GetRaw(key)
		if err != nil {
			klog.Errorf("the non-resource info cannot be acquired, the error is: %v", err)
			ErrNonResource(err, w, req)
			return
		}
		_, err = w.Write(infoCache)
		if err != nil {
			klog.Errorf("failed to write the non-resource info, the error is: %v", err)
			ErrNonResource(err, w, req)
			return
		}

		klog.Infof("success to non-resource info: %s", key)
		w.WriteHeader(http.StatusOK)
	}

}

func getClientSetQueryInfo(path string, isFake bool) ([]byte, error) {
	if clientSet == nil {
		// used for unit test
		if isFake {
			return []byte(fmt.Sprintf("fake-non-resource-info-%s", path)), nil
		}
		klog.Errorf("the kubernetes clientSet is nil, and return the fake value for test")
		return nil, errors.New("the kubernetes clientSet is nil")
	}
	nonResourceInfo, err := clientSet.RESTClient().Get().AbsPath(path).Do(context.TODO()).Raw()
	if err != nil {
		return nil, err
	}
	return nonResourceInfo, nil
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

func ErrNonResource(err error, w http.ResponseWriter, req *http.Request) {
	status := responsewriters.ErrorToAPIStatus(err)
	code := int(status.Code)
	// when writing an error, check to see if the status indicates a retry after period
	if status.Details != nil && status.Details.RetryAfterSeconds > 0 {
		delay := strconv.Itoa(int(status.Details.RetryAfterSeconds))
		w.Header().Set("Retry-After", delay)
	}

	if code == http.StatusNoContent {
		w.WriteHeader(code)
	}
	klog.Errorf("%v counter the error %v", req.URL, err)

}
