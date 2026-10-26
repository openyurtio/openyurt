/*
Copyright 2022 The OpenYurt Authors.

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

package options

import (
	"errors"
	"net"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/yurthub/certificate"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage/disk"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

func TestNewYurtHubOptions(t *testing.T) {
	expectOptions := YurtHubOptions{
		YurtHubHost:               "127.0.0.1",
		YurtHubProxyHost:          "127.0.0.1",
		YurtHubProxyPort:          util.YurtHubProxyPort,
		YurtHubPort:               util.YurtHubPort,
		YurtHubProxySecurePort:    util.YurtHubProxySecurePort,
		PortForMultiplexer:        util.YurtHubMultiplexerPort,
		YurtHubNamespace:          util.YurtHubNamespace,
		GCFrequency:               120,
		YurtHubCertOrganizations:  make([]string, 0),
		LBMode:                    "rr",
		HeartbeatFailedRetry:      3,
		HeartbeatHealthyThreshold: 2,
		HeartbeatTimeoutSeconds:   2,
		HeartbeatIntervalSeconds:  10,
		MaxRequestInFlight:        250,
		BootstrapMode:             "token",
		RootDir:                   filepath.Join("/var/lib/", projectinfo.GetHubName()),
		EnableProfiling:           true,
		EnableDummyIf:             true,
		EnableIptables:            false,
		HubAgentDummyIfName:       "hub-dummy0",
		DiskCachePath:             disk.CacheBaseDir,
		EnableResourceFilter:      true,
		DisabledResourceFilters:   make([]string, 0),
		WorkingMode:               string(util.WorkingModeEdge),
		KubeletHealthGracePeriod:  time.Second * 40,
		EnableNodePool:            true,
		MinRequestTimeout:         time.Second * 1800,
		CACertHashes:              make([]string, 0),
		UnsafeSkipCAVerification:  false,
		PoolScopeResources: []schema.GroupVersionResource{
			{Group: "", Version: "v1", Resource: "services"},
			{Group: "discovery.k8s.io", Version: "v1", Resource: "endpointslices"},
		},
	}

	options := NewYurtHubOptions()
	options.AddFlags((&cobra.Command{}).Flags())
	if !reflect.DeepEqual(expectOptions, *options) {
		t.Errorf("expect options: %#+v, but got %#+v", expectOptions, *options)
	}
}

func TestValidate(t *testing.T) {
	testcases := map[string]struct {
		options *YurtHubOptions
		isErr   bool
	}{
		"don't set node name": {
			options: NewYurtHubOptions(),
			isErr:   true,
		},
		"don't set server addr": {
			options: &YurtHubOptions{
				NodeName: "foo",
			},
			isErr: true,
		},
		"don't set join token": {
			options: &YurtHubOptions{
				NodeName:   "foo",
				ServerAddr: "1.2.3.4:56",
			},
			isErr: true,
		},
		"secure default requires ca cert hashes": {
			options: func() *YurtHubOptions {
				o := NewYurtHubOptions()
				o.NodeName = "foo"
				o.ServerAddr = "1.2.3.4:56"
				o.JoinToken = "xxxx"
				o.LBMode = "rr"
				o.WorkingMode = "cloud"
				o.NodePoolName = "foo"
				return o
			}(),
			isErr: true,
		},
		"kubelet certificate bootstrap does not require ca cert hashes": {
			options: func() *YurtHubOptions {
				o := NewYurtHubOptions()
				o.NodeName = "foo"
				o.ServerAddr = "1.2.3.4:56"
				o.BootstrapMode = certificate.KubeletCertificateBootstrapMode
				o.LBMode = "rr"
				o.WorkingMode = "cloud"
				o.NodePoolName = "foo"
				return o
			}(),
			isErr: false,
		},
		"invalid lb mode": {
			options: &YurtHubOptions{
				NodeName:   "foo",
				ServerAddr: "1.2.3.4:56",
				JoinToken:  "xxxx",
				LBMode:     "invalid mode",
			},
			isErr: true,
		},
		"invalid working mode": {
			options: &YurtHubOptions{
				NodeName:    "foo",
				ServerAddr:  "1.2.3.4:56",
				JoinToken:   "xxxx",
				LBMode:      "rr",
				WorkingMode: "invalid mode",
			},
			isErr: true,
		},
		"invalid dummy ip": {
			options: &YurtHubOptions{
				NodeName:          "foo",
				ServerAddr:        "1.2.3.4:56",
				JoinToken:         "xxxx",
				LBMode:            "rr",
				WorkingMode:       "cloud",
				HubAgentDummyIfIP: "invalid ip",
			},
			isErr: true,
		},
		"not in dummy cidr": {
			options: &YurtHubOptions{
				NodeName:          "foo",
				ServerAddr:        "1.2.3.4:56",
				JoinToken:         "xxxx",
				LBMode:            "rr",
				WorkingMode:       "cloud",
				HubAgentDummyIfIP: "169.250.0.0",
			},
			isErr: true,
		},
		"in exclusive cidr": {
			options: &YurtHubOptions{
				NodeName:          "foo",
				ServerAddr:        "1.2.3.4:56",
				JoinToken:         "xxxx",
				LBMode:            "rr",
				WorkingMode:       "cloud",
				HubAgentDummyIfIP: "169.254.31.1",
			},
			isErr: true,
		},
		"ip is 169.254.1.1": {
			options: &YurtHubOptions{
				NodeName:          "foo",
				ServerAddr:        "1.2.3.4:56",
				JoinToken:         "xxxx",
				LBMode:            "rr",
				WorkingMode:       "cloud",
				HubAgentDummyIfIP: "169.254.1.1",
			},
			isErr: true,
		},
		"no ca cert hashes": {
			options: &YurtHubOptions{
				NodeName:                 "foo",
				ServerAddr:               "1.2.3.4:56",
				JoinToken:                "xxxx",
				LBMode:                   "rr",
				WorkingMode:              "cloud",
				UnsafeSkipCAVerification: false,
			},
			isErr: true,
		},
		"normal options": {
			options: &YurtHubOptions{
				NodeName:                 "foo",
				ServerAddr:               "1.2.3.4:56",
				JoinToken:                "xxxx",
				LBMode:                   "rr",
				WorkingMode:              "cloud",
				UnsafeSkipCAVerification: true,
				NodePoolName:             "foo",
			},
			isErr: false,
		},
		"normal options with ca cert hashes": {
			options: &YurtHubOptions{
				NodeName:                 "foo",
				ServerAddr:               "1.2.3.4:56",
				JoinToken:                "xxxx",
				LBMode:                   "rr",
				WorkingMode:              "cloud",
				CACertHashes:             []string{"sha256:abcdef"},
				UnsafeSkipCAVerification: false,
				NodePoolName:             "foo",
			},
			isErr: false,
		},
		"normal options with ipv6": {
			options: &YurtHubOptions{
				NodeName:                 "foo",
				ServerAddr:               "1.2.3.4:56",
				JoinToken:                "xxxx",
				LBMode:                   "rr",
				WorkingMode:              "cloud",
				UnsafeSkipCAVerification: true,
				HubAgentDummyIfIP:        "fd00::2:1",
				NodePoolName:             "foo",
			},
			isErr: false,
		},
		"normal options with ipv4": {
			options: &YurtHubOptions{
				NodeName:                 "foo",
				ServerAddr:               "1.2.3.4:56",
				JoinToken:                "xxxx",
				LBMode:                   "rr",
				WorkingMode:              "cloud",
				UnsafeSkipCAVerification: true,
				HubAgentDummyIfIP:        "169.254.2.1",
				NodePoolName:             "foo",
			},
			isErr: false,
		},
		"host-control-plane-address in local mode": {
			options: &YurtHubOptions{
				NodeName:             "foo",
				WorkingMode:          "local",
				ServerAddr:           "1.2.3.4:56",
				HostControlPlaneAddr: "123.123.123.123",
			},
			isErr: false,
		},
		"no host-control-plane-address in local mode": {
			options: &YurtHubOptions{
				NodeName:             "foo",
				WorkingMode:          "local",
				ServerAddr:           "1.2.3.4:56",
				HostControlPlaneAddr: "",
			},
			isErr: true,
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			err := tc.options.Validate()
			if tc.isErr && err == nil {
				t.Errorf("expect return err, but got nil")
			} else if !tc.isErr && err != nil {
				t.Errorf("expect return nil, but got %v", err)
			}
		})
	}
}

func testIPNet(ip string) *net.IPNet {
	parsed := net.ParseIP(ip)
	if parsed.To4() != nil {
		return &net.IPNet{IP: parsed, Mask: net.CIDRMask(24, 32)}
	}
	return &net.IPNet{IP: parsed, Mask: net.CIDRMask(64, 128)}
}

func TestNodeIPFromAddrs(t *testing.T) {
	testcases := map[string]struct {
		addrs []net.Addr
		want  string
	}{
		"prefers IPv4 over IPv6": {
			addrs: []net.Addr{testIPNet("fd00::5"), testIPNet("10.0.0.5")},
			want:  "10.0.0.5",
		},
		"skips loopback and link-local": {
			addrs: []net.Addr{testIPNet("127.0.0.1"), testIPNet("169.254.1.1"), testIPNet("fe80::1"), testIPNet("192.168.1.10")},
			want:  "192.168.1.10",
		},
		"falls back to IPv6": {
			addrs: []net.Addr{testIPNet("fd00::5")},
			want:  "fd00::5",
		},
		"ignores non-IPNet addresses": {
			addrs: []net.Addr{&net.IPAddr{IP: net.ParseIP("10.0.0.9")}, testIPNet("10.0.0.5")},
			want:  "10.0.0.5",
		},
		"no usable address": {
			addrs: []net.Addr{testIPNet("127.0.0.1"), testIPNet("169.254.1.1")},
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			got := nodeIPFromAddrs(tc.addrs)
			if tc.want == "" {
				if got != nil {
					t.Fatalf("expected nil, got %v", got)
				}
				return
			}
			if got == nil || got.String() != tc.want {
				t.Fatalf("expected %s, got %v", tc.want, got)
			}
		})
	}
}

func TestResolveNodeIPWith(t *testing.T) {
	defaultRouteErr := errors.New("no default routes found in /proc/net/route")

	testcases := map[string]struct {
		resolveBindAddress func(net.IP) (net.IP, error)
		interfaceAddrs     func() ([]net.Addr, error)
		want               string
		wantErr            string
	}{
		"uses default route address when available": {
			resolveBindAddress: func(net.IP) (net.IP, error) { return net.ParseIP("10.0.0.1"), nil },
			interfaceAddrs: func() ([]net.Addr, error) {
				t.Fatal("interface addresses should not be consulted")
				return nil, nil
			},
			want: "10.0.0.1",
		},
		"falls back to interface address without default route": {
			resolveBindAddress: func(net.IP) (net.IP, error) { return nil, defaultRouteErr },
			interfaceAddrs:     func() ([]net.Addr, error) { return []net.Addr{testIPNet("10.0.0.2")}, nil },
			want:               "10.0.0.2",
		},
		"reports interface address failure": {
			resolveBindAddress: func(net.IP) (net.IP, error) { return nil, defaultRouteErr },
			interfaceAddrs:     func() ([]net.Addr, error) { return nil, errors.New("list failed") },
			wantErr:            "list failed",
		},
		"returns default route error when no usable address exists": {
			resolveBindAddress: func(net.IP) (net.IP, error) { return nil, defaultRouteErr },
			interfaceAddrs:     func() ([]net.Addr, error) { return []net.Addr{testIPNet("127.0.0.1")}, nil },
			wantErr:            defaultRouteErr.Error(),
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			got, err := resolveNodeIPWith(tc.resolveBindAddress, tc.interfaceAddrs)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("expected nil error, got %v", err)
			}
			if got == nil || got.String() != tc.want {
				t.Fatalf("expected %s, got %v", tc.want, got)
			}
		})
	}
}
