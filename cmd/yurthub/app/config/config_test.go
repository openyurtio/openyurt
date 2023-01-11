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
	"testing"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/options"
	"github.com/openyurtio/openyurt/pkg/yurthub/certificate/token/testdata"
)

func TestComplete(t *testing.T) {
	options := options.NewYurtHubOptions()
	client, err := testdata.CreateCertFakeClient("../../../../pkg/yurthub/certificate/token/testdata")
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
	cfg, err := Complete(options)
	if err != nil {
		t.Errorf("expect no err, but got %v", err)
	} else if cfg == nil {
		t.Errorf("expect cfg not nil, but got nil")
	}
}
