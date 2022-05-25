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
	"bufio"
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/util/httpstream"
	"k8s.io/apiserver/pkg/util/flushwriter"
	"k8s.io/apiserver/pkg/util/wsstream"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurttunnel/constants"
)

var (
	supportedHeaders       = []string{constants.ProxyHostHeaderKey, "User-Agent"}
	HeaderTransferEncoding = "Transfer-Encoding"
	HeaderChunked          = "chunked"
	bufioReaderPool        sync.Pool
)

// newBufioReader retrieves a cached Reader from the pool if the pool is not empty,
// otherwise creates a new one
func newBufioReader(r io.Reader) *bufio.Reader {
	if v := bufioReaderPool.Get(); v != nil {
		br := v.(*bufio.Reader)
		br.Reset(r)
		return br
	}
	return bufio.NewReader(r)
}

// putBufioReader puts the Reader to the pool.
func putBufioReader(br *bufio.Reader) {
	br.Reset(nil)
	bufioReaderPool.Put(br)
}

// ReqRequestInterceptor intercepts http/https requests sent from the master,
// prometheus and metric server, setup proxy tunnel to kubelet, sends requests
// through the tunnel and sends responses back to the master
type RequestInterceptor struct {
	contextDialer func(addr string, header http.Header, isTLS bool) (net.Conn, error)
}

// NewRequestInterceptor creates a interceptor object that intercept request from kube-apiserver
func NewRequestInterceptor(udsSockFile string, cfg *tls.Config) *RequestInterceptor {
	if len(udsSockFile) == 0 || cfg == nil {
		return nil
	}

	cfg.InsecureSkipVerify = true
	contextDialer := func(addr string, header http.Header, isTLS bool) (net.Conn, error) {
		klog.V(4).Infof("Sending request to %q.", addr)
		proxyConn, err := net.Dial("unix", udsSockFile)
		if err != nil {
			return nil, fmt.Errorf("dialing proxy %q failed: %w", udsSockFile, err)
		}

		var connectHeaders string
		for _, h := range supportedHeaders {
			if v := header.Get(h); len(v) != 0 {
				connectHeaders = fmt.Sprintf("%s\r\n%s: %s", connectHeaders, h, v)
			}
		}

		fmt.Fprintf(proxyConn, "CONNECT %s HTTP/1.1\r\nHost: localhost%s\r\n\r\n", addr, connectHeaders)
		br := newBufioReader(proxyConn)
		defer putBufioReader(br)
		res, err := http.ReadResponse(br, nil)
		if err != nil {
			proxyConn.Close()
			return nil, fmt.Errorf("reading HTTP response from CONNECT to %s via proxy %s failed: %w", addr, udsSockFile, err)
		}
		if res.StatusCode != 200 {
			proxyConn.Close()
			return nil, fmt.Errorf("proxy error from %s while dialing %s, code %d: %v", udsSockFile, addr, res.StatusCode, res.Status)
		}

		// if the request scheme is https, setup a tls connection over the
		// proxy tunnel (i.e. interceptor <--tls--> kubelet)
		if isTLS {
			tlsTunnelConn := tls.Client(proxyConn, cfg)
			if err := tlsTunnelConn.Handshake(); err != nil {
				proxyConn.Close()
				return nil, fmt.Errorf("fail to setup TLS handshake through the Tunnel: %w", err)
			}
			klog.V(4).Infof("successfully setup TLS connection to %q with headers: %s", addr, connectHeaders)
			return tlsTunnelConn, nil
		}
		klog.V(2).Infof("successfully setup connection to %q with headers: %q", addr, connectHeaders)
		return proxyConn, nil
	}

	return &RequestInterceptor{
		contextDialer: contextDialer,
	}
}

// copyHeader copy header from src to dst
func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

// klogAndHTTPError logs the error message and write back to client
func klogAndHTTPError(w http.ResponseWriter, errCode int, format string, i ...interface{}) {
	errMsg := fmt.Sprintf(format, i...)
	klog.Error(errMsg)
	http.Error(w, errMsg, errCode)
}

// ServeHTTP will proxy the request to the tunnel and return response from tunnel back
// to the client
func (ri *RequestInterceptor) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 1. setup the tunnel
	tunnelConn, err := ri.contextDialer(r.Host, r.Header, r.TLS != nil)
	if err != nil {
		klogAndHTTPError(w, http.StatusServiceUnavailable,
			"fail to setup the tunnel: %s", err)
		return
	}
	defer tunnelConn.Close()

	// 2. proxy the request to tunnel
	if err := r.Write(tunnelConn); err != nil {
		klogAndHTTPError(w, http.StatusServiceUnavailable,
			"fail to write request to tls connection: %s", err)
		return
	}

	// 3.1 handling the upgrade request
	if httpstream.IsUpgradeRequest(r) {
		serveUpgradeRequest(tunnelConn, w, r)
		return
	}

	// 3.2 handling the request
	serveRequest(tunnelConn, w, r)
}

// getResponseCode reads a http response from the given reader, returns the response,
// the bytes read from the reader, and any error encountered
func getResponse(r io.Reader) (*http.Response, []byte, error) {
	rawResponse := bytes.NewBuffer(make([]byte, 0, 256))
	// Save the bytes read while reading the response headers into the rawResponse buffer
	br := newBufioReader(io.TeeReader(r, rawResponse))
	defer putBufioReader(br)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		return nil, nil, err
	}
	// return the http response and the raw bytes consumed from the reader in the process
	return resp, rawResponse.Bytes(), nil
}

// serveUpgradeRequest serves the request that needs to be upgraded
// i.e. request requires bidirection httpstreaming
func serveUpgradeRequest(tunnelConn net.Conn, w http.ResponseWriter, r *http.Request) {
	klog.V(4).Infof("interceptor: start serving streaming request %s with Headers: %v", r.URL.String(), r.Header)
	tunnelHTTPResp, rawResponse, err := getResponse(tunnelConn)
	if err != nil {
		klogAndHTTPError(w, http.StatusServiceUnavailable, "tunnel connection error: %v", err)
		return
	}

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		klogAndHTTPError(w, http.StatusServiceUnavailable,
			"can't assert response to http.Hijacker")
		return
	}
	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		klogAndHTTPError(w, http.StatusServiceUnavailable,
			"fail to hijack response: %s", err)
		return
	}
	defer clientConn.Close()

	if tunnelHTTPResp.StatusCode != http.StatusSwitchingProtocols {
		deadline := time.Now().Add(10 * time.Second)
		tunnelConn.SetReadDeadline(deadline)
		clientConn.SetWriteDeadline(deadline)
		// write the response to the client
		err := tunnelHTTPResp.Write(clientConn)
		if err != nil && !strings.Contains(err.Error(), "use of closed network connection") {
			klog.Errorf("error proxying un-upgrade response from tunnel to client: %v", err)
		}
		return
	}

	// write the response bytes back to client
	if len(rawResponse) > 0 {
		if _, err = clientConn.Write(rawResponse); err != nil {
			klog.Errorf("error proxying response bytes from tunnel to client: %v", err)
		}
	}

	// start bidirectional connection proxy, so start two goroutines
	// to copy in each direction.
	readerComplete, writerComplete :=
		make(chan struct{}), make(chan struct{})

	go func() {
		_, err := io.Copy(tunnelConn, clientConn)
		if err != nil && !strings.Contains(err.Error(), "use of closed network connection") {
			klog.Errorf("error proxying data from client to tunnel: %v", err)
		}
		close(writerComplete)
	}()

	go func() {
		_, err := io.Copy(clientConn, tunnelConn)
		if err != nil && !strings.Contains(err.Error(), "use of closed network connection") {
			klog.Errorf("error proxying data from tunnel to client: %v", err)
		}
		close(readerComplete)
	}()

	select {
	case <-writerComplete:
	case <-readerComplete:
	}
	klog.V(4).Infof("interceptor: stop serving streaming request %s with Headers: %v", r.URL.String(), r.Header)
	return
}

// isChunked verify the specified response is chunked stream or not.
func isChunked(response *http.Response) bool {
	for _, h := range response.Header[http.CanonicalHeaderKey(HeaderTransferEncoding)] {
		if strings.Contains(strings.ToLower(h), strings.ToLower(HeaderChunked)) {
			return true
		}
	}

	for _, te := range response.TransferEncoding {
		if strings.Contains(strings.ToLower(te), strings.ToLower(HeaderChunked)) {
			return true
		}
	}

	return false
}

// serverRequest serves the normal requests, e.g., kubectl logs
func serveRequest(tunnelConn net.Conn, w http.ResponseWriter, r *http.Request) {
	br := newBufioReader(tunnelConn)
	defer putBufioReader(br)
	tunnelHTTPResp, err := http.ReadResponse(br, r)
	if err != nil {
		klogAndHTTPError(w, http.StatusServiceUnavailable, "fail to read response from the tunnel: %v", err)
		return
	}
	klog.V(4).Infof("interceptor: successfully read the http response from the proxy tunnel for request %s", r.URL.String())
	defer tunnelHTTPResp.Body.Close()

	if wsstream.IsWebSocketRequest(r) {
		wsReader := wsstream.NewReader(tunnelHTTPResp.Body, true, wsstream.NewDefaultReaderProtocols())
		if err := wsReader.Copy(w, r); err != nil {
			klog.ErrorS(err, "error encountered while streaming results via websocket")
		}
		return
	}

	copyHeader(w.Header(), tunnelHTTPResp.Header)
	w.WriteHeader(tunnelHTTPResp.StatusCode)

	writer := w.(io.Writer)
	if isChunked(tunnelHTTPResp) {
		stopCh := make(chan struct{})
		defer close(stopCh)
		go func(r *http.Request, conn net.Conn, stopCh <-chan struct{}) {
			ctx := r.Context()
			select {
			case <-stopCh:
				klog.V(3).Infof("chunked request(%s) normally exit", r.URL.String())
			case <-ctx.Done():
				klog.V(2).Infof("chunked request(%s) to agent(%s) closed by cloud client, %v", r.URL.String(), r.Header.Get(constants.ProxyHostHeaderKey), ctx.Err())
				// close connection with tunnel
				conn.Close()
			}
		}(r, tunnelConn, stopCh)

		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		writer = flushwriter.Wrap(w)
	}

	_, err = io.Copy(writer, tunnelHTTPResp.Body)
	if err != nil && !strings.Contains(err.Error(), "use of closed network connection") {
		klog.ErrorS(err, "fail to copy response from the tunnel back to the client")
	}

	klog.V(4).Infof("interceptor: stop serving request %s with headers: %v", r.URL.String(), r.Header)

}
