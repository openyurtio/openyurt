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

package integration

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	anpserver "sigs.k8s.io/apiserver-network-proxy/pkg/server"

	ta "github.com/openyurtio/openyurt/pkg/yurttunnel/agent"
	hw "github.com/openyurtio/openyurt/pkg/yurttunnel/handlerwrapper"
	tr "github.com/openyurtio/openyurt/pkg/yurttunnel/handlerwrapper/tracerequest"
	ts "github.com/openyurtio/openyurt/pkg/yurttunnel/server"
)

const (
	DummyServerPort          = 9515
	ServerMasterPort         = 9516
	ServerMasterInsecurePort = 9517
	ServerAgentPort          = 9518
	HTTPPath                 = "yurttunel-test"
	DummyServerResponse      = "dummy server"
	RootCAFile               = "ca.pem"
	GeneircCertFile          = "cert.pem"
	GenericKeyFile           = "key.pem"
	InterceptorServerUDSFile = "interceptor-proxier.sock"
)

func genCAPool(t *testing.T, CAPath string) *x509.CertPool {
	caCertPEM, err := os.ReadFile(CAPath)
	if err != nil {
		t.Fatalf("fail to load the CA: %v", err)
	}
	roots := x509.NewCertPool()
	ok := roots.AppendCertsFromPEM(caCertPEM)
	if !ok {
		t.Fatal("fail to append the ca PEM to cert pool")
	}
	return roots
}

func genCert(t *testing.T, certPath, keyPath string) tls.Certificate {
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		t.Fatalf("fail to load the dummy server cert: %v", err)
	}
	return cert
}

func startDummyServer(t *testing.T) {
	m := http.NewServeMux()
	m.HandleFunc("/"+HTTPPath, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		n, err := w.Write([]byte(DummyServerResponse))
		if err != nil {
			t.Errorf("fail to write response: %v", err)
		}
		if n != len([]byte(DummyServerResponse)) {
			t.Errorf("fail to write response: write %d of the %d bytes",
				n, len([]byte(DummyServerResponse)))
		}
	})

	s := &http.Server{
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{
				genCert(t, GeneircCertFile, GenericKeyFile),
			},
			ClientCAs: genCAPool(t, RootCAFile),
		},
		Addr:    fmt.Sprintf(":%d", DummyServerPort),
		Handler: m,
	}

	klog.Infof("[TEST] dummy-server is listening on :%d", DummyServerPort)
	if err := s.ListenAndServeTLS("", ""); err != nil {
		t.Errorf("the dummy-server failed: %v", err)
	}
}

func startDummyClient(t *testing.T, wg *sync.WaitGroup) {
	defer wg.Done()
	c := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				Certificates: []tls.Certificate{
					genCert(t, GeneircCertFile, GenericKeyFile),
				},
				RootCAs: genCAPool(t, RootCAFile),
			},
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("tcp", "127.0.0.1:9516")
			},
		},
	}

	r, err := http.NewRequest(http.MethodGet, "https://127.0.0.1:9515/"+HTTPPath, nil)
	if err != nil {
		t.Errorf("fail to generate http request: %v", err)
	}

	rep, err := c.Do(r)
	if err != nil {
		t.Errorf("fail to send request to the server: %v", err)
	}
	if rep.StatusCode != http.StatusOK {
		t.Errorf("the response status code is incorrect, expect: %d, get: %d",
			http.StatusOK, r.Response.StatusCode)
	}
	defer rep.Body.Close()
	content, err := io.ReadAll(rep.Body)
	if err != nil {
		t.Errorf("fail to read from the response body: %v", err)
	}
	if string(content) != DummyServerResponse {
		t.Errorf("fail to read from the response body: expect %s, get %s",
			DummyServerResponse, string(content))
	}

	klog.Info("[TEST] successfully send request to the Dummy server")
}

func startTunnelServer(t *testing.T) {
	tlsCfg := tls.Config{
		Certificates: []tls.Certificate{
			genCert(t, GeneircCertFile, GenericKeyFile),
		},
		ClientCAs: genCAPool(t, RootCAFile),
	}
	wrappers := hw.HandlerWrappers([]hw.Middleware{
		tr.NewTraceReqMiddleware(),
	})

	_, err := os.Stat(InterceptorServerUDSFile)
	if !os.IsNotExist(err) {
		os.Remove(InterceptorServerUDSFile)
	}

	tunnelServer := ts.NewTunnelServer(
		false,                                /*egressSelectorEnabled*/
		InterceptorServerUDSFile,             /* interceptorServerUDSFile*/
		fmt.Sprintf(":%d", ServerMasterPort), /* serverMasterAddr */
		fmt.Sprintf(":%d", ServerMasterInsecurePort), /* serverMasterInsecureAddr */
		fmt.Sprintf(":%d", ServerAgentPort),          /* serverAgentAddr */
		1,                                            /* serverCount */
		&tlsCfg,                                      /* tlsCfg */
		&tlsCfg,
		wrappers,                                /* hw.HandlerWrappers */
		string(anpserver.ProxyStrategyDestHost), /* proxyStrategy */
	)
	tunnelServer.Run()
	klog.Info("[TEST] Yurttunnel Server is running")
}

func startTunnelAgent(t *testing.T) {
	tlsCfg := tls.Config{
		Certificates: []tls.Certificate{
			genCert(t, GeneircCertFile, GenericKeyFile),
		},
		RootCAs:    genCAPool(t, RootCAFile),
		ServerName: "127.0.0.1",
	}
	tunnelAgent := ta.NewTunnelAgent(
		&tlsCfg,                             /* tlsCfg */
		fmt.Sprintf(":%d", ServerAgentPort), /* tunnelServerAddr */
		"dummy-agent",                       /* nodeName */
		"ipv4=127.0.0.1&host=localhost",     /* agentIdentifiers */
	)
	tunnelAgent.Run(wait.NeverStop)
	klog.Info("[TEST] Yurttunnel Agent is running")
}

func TestYurttunnel(t *testing.T) {
	defer func() {
		_, err := os.Stat(InterceptorServerUDSFile)
		if !os.IsNotExist(err) {
			os.Remove(InterceptorServerUDSFile)
		}
	}()
	var wg sync.WaitGroup
	// 1. start a dummy server
	go startDummyServer(t)

	// 2. setup a tunnel server
	go startTunnelServer(t)
	time.Sleep(1 * time.Second)

	// // 3. setup a tunnel agent
	go startTunnelAgent(t)
	time.Sleep(1 * time.Second)

	// 4. create a dummy client and send requests to the tunnel server
	wg.Add(1)
	go startDummyClient(t, &wg)
	wg.Wait()
}
