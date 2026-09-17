/*
Copyright 2026 The OpenYurt Authors.

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

package config

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/options"
)

// fakeCertManager serves a real self-signed certificate file, which is all
// prepareServerServing needs: a server cert/key pair path and a CA bundle path.
type fakeCertManager struct {
	certFile string
}

func (f *fakeCertManager) Start()                                   {}
func (f *fakeCertManager) Stop()                                    {}
func (f *fakeCertManager) Ready() bool                              { return true }
func (f *fakeCertManager) UpdateBootstrapConf(_ string) error       { return nil }
func (f *fakeCertManager) GetHubConfFile() string                   { return "" }
func (f *fakeCertManager) GetCAData() []byte                        { return nil }
func (f *fakeCertManager) GetCaFile() string                        { return f.certFile }
func (f *fakeCertManager) GetAPIServerClientCert() *tls.Certificate { return nil }
func (f *fakeCertManager) GetHubServerCert() *tls.Certificate       { return nil }
func (f *fakeCertManager) GetHubServerCertFile() string             { return f.certFile }

func writeSelfSignedCert(t *testing.T, dir string) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("could not generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "yurthub-test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("could not create certificate: %v", err)
	}
	path := filepath.Join(dir, "yurthub-server-current.pem")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("could not create cert file: %v", err)
	}
	defer f.Close()
	if err := pem.Encode(f, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
		t.Fatalf("could not encode certificate: %v", err)
	}
	if err := pem.Encode(f, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}); err != nil {
		t.Fatalf("could not encode key: %v", err)
	}
	return path
}

// A non-positive port is how a listener is switched off: SecureServingOptions.ApplyTo
// returns early without populating the SecureServingInfo. Configuring the returned
// (nil) SecureServingInfo unconditionally used to panic here, so setting
// --multiplexer-port=0 or --proxy-secure-port=0 stopped yurthub from starting at all —
// which on an edge node means it never comes back from a reboot.
func TestPrepareServerServingWithDisabledPorts(t *testing.T) {
	dir, err := os.MkdirTemp("", "yurthub-serving")
	if err != nil {
		t.Fatalf("could not create temp dir: %v", err)
	}
	defer os.RemoveAll(dir)
	certMgr := &fakeCertManager{certFile: writeSelfSignedCert(t, dir)}

	testcases := map[string]struct {
		multiplexerPort int
		secureProxyPort int
		wantMultiplexer bool
		wantSecureProxy bool
	}{
		"multiplexer port disabled": {
			multiplexerPort: 0, secureProxyPort: 0, wantMultiplexer: false, wantSecureProxy: false,
		},
		"negative ports are disabled too": {
			multiplexerPort: -1, secureProxyPort: -1, wantMultiplexer: false, wantSecureProxy: false,
		},
		// The realistic mitigation: close the routable multiplexer listener while the
		// secure proxy that pods talk to keeps serving. A positive port really binds, so
		// use one that will not collide with a yurthub running on this machine.
		"only the multiplexer port disabled": {
			multiplexerPort: 0, secureProxyPort: 34681, wantMultiplexer: false, wantSecureProxy: true,
		},
	}

	for name, tt := range testcases {
		t.Run(name, func(t *testing.T) {
			opts := options.NewYurtHubOptions()
			opts.NodeIP = "127.0.0.1"
			opts.YurtHubHost = "127.0.0.1"
			opts.YurtHubProxyHost = "127.0.0.1"
			// Insecure listeners actually bind, so keep them off in the unit test.
			opts.YurtHubPort = 0
			opts.YurtHubProxyPort = 0
			opts.EnableDummyIf = false
			opts.PortForMultiplexer = tt.multiplexerPort
			opts.YurtHubProxySecurePort = tt.secureProxyPort

			cfg := &YurtHubConfiguration{}
			// Before the nil guards this panicked rather than returning.
			if err := prepareServerServing(opts, certMgr, cfg); err != nil {
				t.Fatalf("prepareServerServing returned an error: %v", err)
			}
			t.Cleanup(func() { closeServingListeners(cfg) })
			if got := cfg.YurtHubMultiplexerServerServing != nil; got != tt.wantMultiplexer {
				t.Errorf("multiplexer serving configured = %v, want %v", got, tt.wantMultiplexer)
			}
			if got := cfg.YurtHubSecureProxyServerServing != nil; got != tt.wantSecureProxy {
				t.Errorf("secure proxy serving configured = %v, want %v", got, tt.wantSecureProxy)
			}
		})
	}
}

func closeServingListeners(cfg *YurtHubConfiguration) {
	if cfg.YurtHubSecureProxyServerServing != nil && cfg.YurtHubSecureProxyServerServing.Listener != nil {
		cfg.YurtHubSecureProxyServerServing.Listener.Close()
	}
	if cfg.YurtHubMultiplexerServerServing != nil && cfg.YurtHubMultiplexerServerServing.Listener != nil {
		cfg.YurtHubMultiplexerServerServing.Listener.Close()
	}
}

// A positive port must still configure the listener — the guards must not turn a normal
// start into a silently port-less yurthub. The ports here are arbitrary high ports rather
// than the real defaults, so the test cannot collide with a yurthub running on this
// machine; that the shipped defaults are positive is asserted separately below.
func TestPrepareServerServingWithEnabledPorts(t *testing.T) {
	dir, err := os.MkdirTemp("", "yurthub-serving-default")
	if err != nil {
		t.Fatalf("could not create temp dir: %v", err)
	}
	defer os.RemoveAll(dir)
	certMgr := &fakeCertManager{certFile: writeSelfSignedCert(t, dir)}

	opts := options.NewYurtHubOptions()
	opts.NodeIP = "127.0.0.1"
	opts.YurtHubHost = "127.0.0.1"
	opts.YurtHubProxyHost = "127.0.0.1"
	opts.YurtHubPort = 0
	opts.YurtHubProxyPort = 0
	opts.EnableDummyIf = false
	opts.YurtHubProxySecurePort = 34682
	opts.PortForMultiplexer = 34683

	cfg := &YurtHubConfiguration{}
	if err := prepareServerServing(opts, certMgr, cfg); err != nil {
		t.Fatalf("prepareServerServing returned an error: %v", err)
	}
	defer closeServingListeners(cfg)

	if cfg.YurtHubMultiplexerServerServing == nil {
		t.Fatalf("the multiplexer listener must be configured for port %d", opts.PortForMultiplexer)
	}
	if cfg.YurtHubSecureProxyServerServing == nil {
		t.Fatalf("the secure proxy listener must be configured for port %d", opts.YurtHubProxySecurePort)
	}
	// The ClientCA is what the nil guard now protects; assert it was actually set, so a
	// guard that skipped the assignment on an enabled port would be caught.
	if cfg.YurtHubMultiplexerServerServing.ClientCA == nil {
		t.Errorf("the multiplexer listener must have its ClientCA configured")
	}
	if cfg.YurtHubSecureProxyServerServing.ClientCA == nil {
		t.Errorf("the secure proxy listener must have its ClientCA configured")
	}
}

// The shipped defaults must stay positive, otherwise the nil guards above would silently
// turn a default start into a yurthub with no secure listeners at all.
func TestDefaultServingPortsArePositive(t *testing.T) {
	opts := options.NewYurtHubOptions()
	if opts.PortForMultiplexer <= 0 {
		t.Errorf("the default multiplexer port must be positive, got %d", opts.PortForMultiplexer)
	}
	if opts.YurtHubProxySecurePort <= 0 {
		t.Errorf("the default secure proxy port must be positive, got %d", opts.YurtHubProxySecurePort)
	}
}
