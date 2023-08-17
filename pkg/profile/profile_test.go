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
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
)

// TestInstall checks Install function correctly sets up routes
func TestInstall(t *testing.T) {
	router := mux.NewRouter()
	Install(router)

	// Define the routes we expect to exist and their expected handlers
	expectedRoutes := map[string]string{
		"/debug/pprof/profile": "profile",
		"/debug/pprof/symbol":  "symbol",
		"/debug/pprof/trace":   "trace",
	}

	// For each route, make a request and check the handler that's called
	for route := range expectedRoutes {
		req, err := http.NewRequest("GET", route, nil)
		if err != nil {
			t.Fatal(err)
		}
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("handler(%s) returned wrong status code: got %v want %v", route, rr.Code, http.StatusOK)
		}
	}
}

// TestRedirectTo checks redirect to the desired location
func TestRedirectTo(t *testing.T) {
	destination := "/destination"
	redirect := redirectTo(destination)

	req, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()

	redirect(rr, req)

	if location := rr.Header().Get("Location"); location != destination {
		t.Errorf("expected redirect to %s, got %s", destination, location)
	}
}
