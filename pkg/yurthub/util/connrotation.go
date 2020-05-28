package util

import (
	"context"
	"net"
	"sync"
	"time"

	"k8s.io/klog"
)

// DialFunc is a shorthand for signature of net.DialContext.
type DialFunc func(ctx context.Context, network, address string) (net.Conn, error)

// Dialer opens connections through Dial and tracks them.
type Dialer struct {
	dial DialFunc
	name string

	mu        sync.Mutex
	addrConns map[string]map[net.Conn]struct{}
}

// NewDialer creates a new Dialer instance.
//
// If dial is not nil, it will be used to create new underlying connections.
// Otherwise net.DialContext is used.
func NewDialer(name string) *Dialer {
	return &Dialer{
		name:      name,
		dial:      (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		addrConns: make(map[string]map[net.Conn]struct{}),
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
	d.addrConns = make(map[string]map[net.Conn]struct{})
	d.mu.Unlock()

	for addr, conns := range addrConns {
		for conn := range conns {
			conn.Close()
			delete(conns, conn)
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

	klog.Infof("%s dialer forcibly close %d connections on %s", d.name, len(conns), address)
	for conn := range conns {
		conn.Close()
		delete(conns, conn)
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
		klog.V(3).Infof("%s dialer failed to dial: %v, and total connections: %d", d.name, err, len(d.addrConns[address]))
		return nil, err
	}

	// Start tracking the connection
	cnt := 0
	d.mu.Lock()
	if d.addrConns[address] == nil {
		d.addrConns[address] = make(map[net.Conn]struct{})
	}
	d.addrConns[address][conn] = struct{}{}
	cnt = len(d.addrConns[address])
	d.mu.Unlock()

	klog.Infof("%s dialer create a new connection from %s to %s, total connections: %d", d.name, conn.LocalAddr().String(), address, cnt)
	return conn, nil
}
