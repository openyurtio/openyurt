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

package config

import (
	"reflect"
	"testing"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/options"
	"github.com/openyurtio/openyurt/pkg/yurthub/certificate/testdata"
)

func TestComplete(t *testing.T) {
	options := options.NewYurtHubOptions()
	client, err := testdata.CreateCertFakeClient("../../../../pkg/yurthub/certificate/testdata")
	if err != nil {
		t.Errorf("failed to create cert fake client, %v", err)
		return
	}
	options.ClientForTest = client
	options.ServerAddr = "https://127.0.0.1:6443"
	options.JoinToken = "123456.abcdef1234567890"
	options.DiskCachePath = "/tmp/cache"
	options.RootDir = "/tmp/cert"
	options.NodeName = "foo"
	options.EnableDummyIf = false
	options.HubAgentDummyIfIP = "169.254.2.1"
	options.NodeIP = "127.0.0.1"
	cfg, err := Complete(options, nil)
	if err != nil {
		t.Errorf("expect no err, but got %v", err)
	} else if cfg == nil {
		t.Errorf("expect cfg not nil, but got nil")
	}
}

func TestParseRemoteServers(t *testing.T) {
	testcases := map[string]struct {
		serverAddr string
		expected   []string
		expectErr  bool
	}{
		"https url": {
			serverAddr: "https://1.2.3.4:6443",
			expected:   []string{"https://1.2.3.4:6443"},
		},
		"multiple https urls": {
			serverAddr: "https://1.2.3.4:6443,https://5.6.7.8:6443",
			expected:   []string{"https://1.2.3.4:6443", "https://5.6.7.8:6443"},
		},
		"ip and port without scheme": {
			serverAddr: "1.2.3.4:6443",
			expected:   []string{"https://1.2.3.4:6443"},
		},
		"dns name and port without scheme": {
			serverAddr: "apiserver.example.com:6443",
			expected:   []string{"https://apiserver.example.com:6443"},
		},
		"ipv6 and port without scheme": {
			serverAddr: "[::1]:6443",
			expected:   []string{"https://[::1]:6443"},
		},
		"with and without scheme": {
			serverAddr: "https://1.2.3.4:6443,5.6.7.8:6443",
			expected:   []string{"https://1.2.3.4:6443", "https://5.6.7.8:6443"},
		},
		"scheme relative address": {
			serverAddr: "//1.2.3.4:6443",
			expected:   []string{"https://1.2.3.4:6443"},
		},
		"http scheme is not supported": {
			serverAddr: "http://1.2.3.4:6443",
			expectErr:  true,
		},
		"not set": {
			serverAddr: "",
			expectErr:  true,
		},
		"empty entry": {
			serverAddr: "https://1.2.3.4:6443,",
			expectErr:  true,
		},
	}

	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			servers, err := parseRemoteServers(tt.serverAddr)
			if tt.expectErr {
				if err == nil {
					t.Fatalf("expect an error for %q, but got servers %v", tt.serverAddr, servers)
				}
				return
			}
			if err != nil {
				t.Fatalf("expect no error for %q, but got %v", tt.serverAddr, err)
			}

			got := make([]string, 0, len(servers))
			for _, u := range servers {
				got = append(got, u.String())
			}
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("expect servers %v, but got %v", tt.expected, got)
			}
		})
	}
}

func TestCompleteWithLocalMode(t *testing.T) {
	client, err := testdata.CreateCertFakeClient("../../../../pkg/yurthub/certificate/testdata")
	if err != nil {
		t.Errorf("failed to create cert fake client, %v", err)
		return
	}
	options := options.NewYurtHubOptions()
	options.ClientForTest = client
	options.WorkingMode = "local"
	options.HostControlPlaneAddr = "127.0.0.1:8443"
	options.ServerAddr = "127.0.0.1:7443"
	options.NodeName = "foo-local"
	cfg, err := Complete(options, nil)
	if err != nil {
		t.Errorf("expected no error for local mode, but got %v", err)
	}
	if cfg == nil {
		t.Errorf("expected cfg not to be nil for local mode, but got nil")
	}
}
