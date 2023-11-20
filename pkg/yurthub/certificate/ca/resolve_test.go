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

package ca

import (
	"bytes"
	"net/url"
	"testing"

	"github.com/openyurtio/openyurt/pkg/yurthub/certificate/testdata"
)

func TestResolveCADataByToken(t *testing.T) {
	joinToken := "123456.abcdef1234567890"
	u, _ := url.Parse("http://127.0.0.1")
	remoteServers := []*url.URL{u}
	client, err := testdata.CreateCertFakeClient("../testdata")
	if err != nil {
		t.Errorf("could not create cert fake client, %v", err)
		return
	}

	caData, err := ResolveCADataByToken(client, remoteServers, joinToken, []string{})
	if err != nil {
		t.Errorf("could not resolve ca data by token, %v", err)
	}

	if !bytes.Equal(caData, testdata.GetCAData()) {
		t.Errorf("expect ca data %v, but got %v", testdata.GetCAData(), caData)
	}
}

func TestResolveCADataByBootstrapFile(t *testing.T) {
	testcases := map[string]struct {
		bootstrapFile string
		expectCAData  []byte
		expectErr     bool
	}{
		"resolve a normal bootstrap file": {
			bootstrapFile: "../testdata/bootstrap-hub.conf",
			expectCAData:  testdata.GetCAData(),
			expectErr:     false,
		},
		"resolve a not exist file": {
			bootstrapFile: "/tmp/xxx/test/bootstrap-file.",
			expectErr:     true,
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			caData, err := ResolveCADataByBootstrapFile(tc.bootstrapFile)
			if tc.expectErr && err == nil {
				t.Errorf("expect an error, but got nil")
			} else if !tc.expectErr && err != nil {
				t.Errorf("expect no error, but got %v", err)
			}

			if len(tc.expectCAData) != 0 && !bytes.Equal(tc.expectCAData, caData) {
				t.Errorf("expect ca data %v, but got %v", tc.expectCAData, caData)
			}
		})
	}
}
