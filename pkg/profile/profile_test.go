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
	"testing"

	"github.com/gorilla/mux"
)

func fakeServer(h http.Handler) error {
	err := http.ListenAndServe(":9090", h)
	return err
}

func TestInstall(t *testing.T) {
	m := mux.NewRouter()
	Install(m)
	go fakeServer(m)
	r, err := http.Get("http://localhost:9090/debug/pprof/")
	if err != nil {
		t.Error(" failed to send request to fake server")
	}

	if r.StatusCode != http.StatusOK {
		t.Error(err)
	}
}
