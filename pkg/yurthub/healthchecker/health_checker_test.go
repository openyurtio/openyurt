package healthchecker

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/alibaba/openyurt/pkg/yurthub/transport"
)

func TestHealthyCheckerWithHealthyServers(t *testing.T) {
	servers := []string{
		"127.0.0.1:18080",
		"127.0.0.1:18081",
		"127.0.0.1:18082",
	}

	// start local http server
	testServers := make([]*httptest.Server, len(servers))
	for i, server := range servers {
		testServers[i] = httptest.NewUnstartedServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
			if req.URL.Path == "/healthz" {
				rw.Write([]byte("OK"))
			}
		}))

		l, err := net.Listen("tcp", server)
		if err != nil {
			t.Fatalf("new listener failed, %v", err)
		}

		testServers[i].Listener.Close()
		testServers[i].Listener = l
		testServers[i].Start()
		defer testServers[i].Close()
	}

	stopCh := make(chan struct{})
	// new transport manager
	transportManager, err := transport.NewTransportManager(2, stopCh)
	if err != nil {
		t.Fatalf("new transport manager failed, %v", err)
	}

	// create urls
	remoteServers := make([]*url.URL, len(servers))
	for i := range servers {
		remoteServers[i], _ = url.Parse(fmt.Sprintf("http://%s", servers[i]))
	}

	// new health checker
	healthChecker, err := NewHealthChecker(remoteServers, transportManager, 3, 2, stopCh)
	if err != nil {
		t.Errorf("new health checker failed, %v", err)
	}

	// wait 10s
	time.Sleep(10 * time.Second)

	// check healthy of all servers
	for i := range remoteServers {
		healthy := healthChecker.IsHealthy(remoteServers[i])
		if !healthy {
			t.Errorf("server: %s is not healthy", remoteServers[i].String())
		}
	}

	close(stopCh)

	// wait some for goroutine exit
	time.Sleep(time.Second)
}

func TestHealthyCheckerWithUnhealthyServers(t *testing.T) {
	servers := []string{
		"127.0.0.1:18080",
		"127.0.0.1:18081",
		"127.0.0.1:18082",
	}

	// start local http server
	testServers := make([]*httptest.Server, len(servers))
	for i, server := range servers {
		testServers[i] = httptest.NewUnstartedServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
			rw.WriteHeader(http.StatusInternalServerError)
		}))

		l, err := net.Listen("tcp", server)
		if err != nil {
			t.Fatalf("new listener failed, %v", err)
		}

		testServers[i].Listener.Close()
		testServers[i].Listener = l
		testServers[i].Start()
		defer testServers[i].Close()
	}

	stopCh := make(chan struct{})
	// new transport manager
	transportManager, err := transport.NewTransportManager(2, stopCh)
	if err != nil {
		t.Fatalf("new transport manager failed, %v", err)
	}

	// create urls
	remoteServers := make([]*url.URL, len(servers))
	for i := range servers {
		remoteServers[i], _ = url.Parse(fmt.Sprintf("http://%s", servers[i]))
	}

	// new health checker
	healthChecker, err := NewHealthChecker(remoteServers, transportManager, 3, 2, stopCh)
	if err != nil {
		t.Errorf("new health checker failed, %v", err)
	}

	// wait 10s
	time.Sleep(10 * time.Second)

	// check healthy of all servers
	for i := range remoteServers {
		healthy := healthChecker.IsHealthy(remoteServers[i])
		if healthy {
			t.Errorf("server: %s is healthy", remoteServers[i].String())
		}
	}
	close(stopCh)

	// wait some for goroutine exit
	time.Sleep(time.Second)
}

func TestHealthyCheckerFromHealthyToUnhealthy(t *testing.T) {
	servers := []string{
		"127.0.0.1:18080",
	}

	reqCnt := 0
	// start local http server
	testServers := make([]*httptest.Server, len(servers))
	for i, server := range servers {
		testServers[i] = httptest.NewUnstartedServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
			reqCnt++

			// after startup 20s, healthy status changed to unhealthy
			if reqCnt < 3 {
				if req.URL.Path == "/healthz" {
					rw.Write([]byte("OK"))
				}
			} else {
				rw.WriteHeader(http.StatusInternalServerError)
			}
		}))

		l, err := net.Listen("tcp", server)
		if err != nil {
			t.Fatalf("new listener failed, %v", err)
		}

		testServers[i].Listener.Close()
		testServers[i].Listener = l
		testServers[i].Start()
		defer testServers[i].Close()
	}

	stopCh := make(chan struct{})
	// new transport manager
	transportManager, err := transport.NewTransportManager(2, stopCh)
	if err != nil {
		t.Fatalf("new transport manager failed, %v", err)
	}

	// create urls
	remoteServers := make([]*url.URL, len(servers))
	for i := range servers {
		remoteServers[i], _ = url.Parse(fmt.Sprintf("http://%s", servers[i]))
	}

	// new health checker
	healthChecker, err := NewHealthChecker(remoteServers, transportManager, 3, 2, stopCh)
	if err != nil {
		t.Errorf("new health checker failed, %v", err)
	}

	// check healthy status lasts 18seconds
	for k := 0; k < 6; k++ {
		time.Sleep(3 * time.Second)
		// check healthy of all servers
		for i := range remoteServers {
			healthy := healthChecker.IsHealthy(remoteServers[i])
			if !healthy {
				t.Errorf("server: %s is unhealthy", remoteServers[i].String())
			}
		}
	}

	// wait 5seconds again
	time.Sleep(5 * time.Second)
	// check healthy of all servers
	for i := range remoteServers {
		healthy := healthChecker.IsHealthy(remoteServers[i])
		if healthy {
			t.Errorf("server: %s is healthy", remoteServers[i].String())
		}
	}

	close(stopCh)

	// wait some for goroutine exit
	time.Sleep(time.Second)
}

func TestHealthyCheckerFromUnHealthyToHealthy(t *testing.T) {
	servers := []string{
		"127.0.0.1:18080",
	}

	reqCnt := 0
	// start local http server
	testServers := make([]*httptest.Server, len(servers))
	for i, server := range servers {
		testServers[i] = httptest.NewUnstartedServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
			reqCnt++

			// after startup 30s, healthy status changed to unhealthy
			if reqCnt > 2 {
				if req.URL.Path == "/healthz" {
					rw.Write([]byte("OK"))
				}
			} else {
				rw.WriteHeader(http.StatusInternalServerError)
			}
		}))

		l, err := net.Listen("tcp", server)
		if err != nil {
			t.Fatalf("new listener failed, %v", err)
		}

		testServers[i].Listener.Close()
		testServers[i].Listener = l
		testServers[i].Start()
		defer testServers[i].Close()
	}

	stopCh := make(chan struct{})
	// new transport manager
	transportManager, err := transport.NewTransportManager(2, stopCh)
	if err != nil {
		t.Fatalf("new transport manager failed, %v", err)
	}

	// create urls
	remoteServers := make([]*url.URL, len(servers))
	for i := range servers {
		remoteServers[i], _ = url.Parse(fmt.Sprintf("http://%s", servers[i]))
	}

	// new health checker
	healthChecker, err := NewHealthChecker(remoteServers, transportManager, 3, 2, stopCh)
	if err != nil {
		t.Errorf("new health checker failed, %v", err)
	}

	// check healthy status lasts 28seconds
	for k := 0; k < 7; k++ {
		time.Sleep(4 * time.Second)
		// check healthy of all servers
		for i := range remoteServers {
			healthy := healthChecker.IsHealthy(remoteServers[i])
			if healthy {
				t.Errorf("server: %s is healthy", remoteServers[i].String())
			}
		}
	}

	// wait 5seconds
	time.Sleep(5 * time.Second)
	// check healthy of all servers
	for i := range remoteServers {
		healthy := healthChecker.IsHealthy(remoteServers[i])
		if !healthy {
			t.Errorf("server: %s is unhealthy", remoteServers[i].String())
		}
	}

	close(stopCh)

	// wait some for goroutine exit
	time.Sleep(time.Second)
}
