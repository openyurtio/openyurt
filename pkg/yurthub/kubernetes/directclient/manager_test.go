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

package directclient

import (
	"context"
	"net/url"
	"os"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/options"
	"github.com/openyurtio/openyurt/pkg/yurthub/certificate/manager"
	"github.com/openyurtio/openyurt/pkg/yurthub/certificate/testdata"
	"github.com/openyurtio/openyurt/pkg/yurthub/healthchecker"
	"github.com/openyurtio/openyurt/pkg/yurthub/transport"
)

func TestGetDirectClientset(t *testing.T) {
	testDir, err := os.MkdirTemp("", "test-client")
	if err != nil {
		t.Fatalf("failed to make temp dir, %v", err)
	}
	nodeName := "foo"
	servers := map[string]int{"https://10.10.10.113:6443": 2}
	u, _ := url.Parse("https://10.10.10.113:6443")
	remoteServers := []*url.URL{u}

	client, err := testdata.CreateCertFakeClient("../../certificate/testdata")
	if err != nil {
		t.Errorf("failed to create cert fake client, %v", err)
		return
	}
	certManager, err := manager.NewYurtHubCertManager(&options.YurtHubOptions{
		NodeName:      nodeName,
		RootDir:       testDir,
		YurtHubHost:   "127.0.0.1",
		JoinToken:     "123456.abcdef1234567890",
		ClientForTest: client,
	}, remoteServers)
	if err != nil {
		t.Errorf("failed to create certManager, %v", err)
		return
	}
	certManager.Start()
	defer certManager.Stop()
	defer os.RemoveAll(testDir)

	err = wait.PollUntilContextTimeout(context.Background(), 2*time.Second, 1*time.Minute, true, func(ctx context.Context) (done bool, err error) {
		if certManager.Ready() {
			return true, nil
		}
		return false, nil
	})

	if err != nil {
		t.Errorf("certificates are not ready, %v", err)
	}

	transportManager, err := transport.NewTransportManager(certManager, context.Background().Done())
	if err != nil {
		t.Fatalf("could not new transport manager, %v", err)
	}

	testcases := map[string]struct {
		healthy           bool
		needHealthyServer bool
		clientIsNil       bool
	}{
		"get client from healthy servers": {
			healthy:           true,
			needHealthyServer: true,
			clientIsNil:       false,
		},
		"get client from unhealthy servers": {
			healthy:           false,
			needHealthyServer: true,
			clientIsNil:       true,
		},
		"get client at random": {
			healthy:           true,
			needHealthyServer: false,
			clientIsNil:       false,
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			fakeHealthyChecker := healthchecker.NewFakeChecker(tc.healthy, servers)
			rcm, _ := NewRestClientManager(remoteServers, transportManager, fakeHealthyChecker)

			client := rcm.GetDirectClientset(tc.needHealthyServer)
			if tc.clientIsNil {
				if client != nil {
					t.Errorf("expect rest client is nil, but got %v", client)
				}
			} else {
				if client == nil {
					t.Errorf("expect non nil rest client, but got nil")
				}
			}
		})
	}
}
