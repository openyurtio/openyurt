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

package util

import (
	"context"
	"net"
	"sync"
	"time"

	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurthub/metrics"
)

// closableConn is used to remove reference in dialer
// when conn is closed by http transport
type closableConn struct {
	net.Conn
	dialer *Dialer
	addr   string
}

// Close is called by http transport, so remove the conn reference in dialer
// and close conn.
func (c *closableConn) Close() error {
	c.dialer.mu.Lock()
	remain := len(c.dialer.addrConns[c.addr])
	if remain >= 1 {
		delete(c.dialer.addrConns[c.addr], c)
		remain = len(c.dialer.addrConns[c.addr])
	}
	c.dialer.mu.Unlock()
	klog.Infof("close connection from %s to %s for %s dialer, remain %d connections", c.Conn.LocalAddr().String(), c.addr, c.dialer.name, remain)
	metrics.Metrics.SetClosableConns(c.addr, remain)
	return c.Conn.Close()
}

// DialFunc is a shorthand for signature of net.DialContext.
type DialFunc func(ctx context.Context, network, address string) (net.Conn, error)

// Dialer opens connections through Dial and tracks them.
type Dialer struct {
	dial DialFunc
	name string

	mu        sync.Mutex
	addrConns map[string]map[*closableConn]struct{}
}

// NewDialer creates a new Dialer instance.
//
// If dial is not nil, it will be used to create new underlying connections.
// Otherwise, net.DialContext is used.
func NewDialer(name string) *Dialer {
	return &Dialer{
		name:      name,
		dial:      (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		addrConns: make(map[string]map[*closableConn]struct{}),
	}
}

// Name returns the name of dialer
func (d *Dialer) Name() string {
	return d.name
}

// CloseAll forcibly closes all tracked connections.
//
// Note: new connections may get created before CloseAll returns.
func (d *Dialer) CloseAll() {
	d.mu.Lock()
	addrConns := d.addrConns
	d.addrConns = make(map[string]map[*closableConn]struct{})
	d.mu.Unlock()

	for addr, conns := range addrConns {
		for conn := range conns {
			conn.Conn.Close()
			delete(conns, conn)
			metrics.Metrics.DecClosableConns(addr)
		}
		delete(addrConns, addr)
	}
}

// Close forcibly closes all tracked connections that specified by address.
//
// Note: new connections may get created before Close returns.
func (d *Dialer) Close(address string) {
	d.mu.Lock()
	conns := d.addrConns[address]
	delete(d.addrConns, address)
	d.mu.Unlock()

	klog.Infof("forcibly close %d connections on %s for %s dialer", len(conns), address, d.name)
	for conn := range conns {
		conn.Conn.Close()
		delete(conns, conn)
		metrics.Metrics.DecClosableConns(address)
	}
}

// Dial creates a new tracked connection.
func (d *Dialer) Dial(network, address string) (net.Conn, error) {
	return d.DialContext(context.Background(), network, address)
}

// DialContext creates a new tracked connection.
func (d *Dialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	conn, err := d.dial(ctx, network, address)
	if err != nil {
		if klog.V(3).Enabled() {
			d.mu.Lock()
			size := len(d.addrConns[address])
			d.mu.Unlock()
			klog.Infof("%s dialer could not dial: %v, and total connections: %d", d.name, err, size)
		}
		return nil, err
	}

	closable := &closableConn{
		Conn:   conn,
		dialer: d,
		addr:   address,
	}

	// Start tracking the connection
	d.mu.Lock()
	if d.addrConns[address] == nil {
		d.addrConns[address] = make(map[*closableConn]struct{})
	}
	d.addrConns[address][closable] = struct{}{}
	size := len(d.addrConns[address])
	d.mu.Unlock()

	klog.Infof("create a connection from %s to %s, total %d connections in %s dialer", conn.LocalAddr().String(), address, size, d.name)
	metrics.Metrics.IncClosableConns(address)
	return closable, nil
}
