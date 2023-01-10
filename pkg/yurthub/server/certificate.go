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
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/openyurtio/openyurt/pkg/yurthub/certificate"
)

const (
	tokenKey = "jointoken"
)

// updateTokenHandler returns a http handler that update bootstrap token in the bootstrap-hub.conf file
// in order to update node certificate when both node certificate and old join token expires
func updateTokenHandler(certificateMgr certificate.YurtCertificateManager) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokens := make(map[string]string)
		decoder := json.NewDecoder(r.Body)
		err := decoder.Decode(&tokens)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprintf(w, "could not decode tokens, %v", err)
			return
		}

		joinToken := tokens[tokenKey]
		if len(joinToken) == 0 {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "no join token is set")
			return
		}

		err = certificateMgr.UpdateBootstrapConf(joinToken)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "could not update bootstrap token, %v", err)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, "update bootstrap token successfully")
		return
	})
}
