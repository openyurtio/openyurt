/*
Copyright 2023 The OpenYurt Authors.

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

package client

import (
	"net/url"
	"testing"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/options"
	"github.com/openyurtio/openyurt/pkg/yurthub/certificate/testdata"
)

func TestNewYurtClientCertificateManager(t *testing.T) {
	joinToken := "123456.abcdef1234567890"
	rootDir := "/tmp/token/cert"
	nodeName := "foo"
	u, _ := url.Parse("http://127.0.0.1")
	remoteServers := []*url.URL{u}

	client, err := testdata.CreateCertFakeClient("../testdata")
	if err != nil {
		t.Errorf("could not create cert fake client, %v", err)
		return
	}

	_, err = NewYurtClientCertificateManager(&options.YurtHubOptions{
		NodeName:                 nodeName,
		YurtHubHost:              "127.0.0.1",
		RootDir:                  rootDir,
		JoinToken:                joinToken,
		YurtHubCertOrganizations: []string{"yurthub:tenant:foo"},
		ClientForTest:            client,
	}, remoteServers, nil, rootDir)
	if err != nil {
		t.Errorf("could not new client certificate manager, %v", err)
	}
}
