package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/config"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/rest"
	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/informers"
)

func TestRunYurtHubServers(t *testing.T) {
	cfg := &config.YurtHubConfiguration{
		// Set the configuration fields as needed for the test
	}

	proxyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Implement the proxy handler logic for the test
	})

	restConfigManager := &rest.RestConfigManager{
		// Set the rest config manager fields as needed for the test
	}

	stopCh := make(chan struct{})

	err := RunYurtHubServers(cfg, proxyHandler, restConfigManager, stopCh)

	assert.NoError(t, err)
}

func TestHealthzHandler(t *testing.T) {
	req, err := http.NewRequest("GET", "/v1/healthz", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(healthz)

	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	expected := "OK"
	if rr.Body.String() != expected {
		t.Errorf("handler returned unexpected body: got %v want %v", rr.Body.String(), expected)
	}
}

func TestGetPodList(t *testing.T) {
	sharedFactory := informers.NewSharedInformerFactory(nil, 0)
	nodeName := "test-node"

	req, err := http.NewRequest("GET", "/pods", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		getPodList(sharedFactory, nodeName).ServeHTTP(w, r)
	})

	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	// Add your assertions for the response body here
}
