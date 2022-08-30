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

package hubself

import (
	"io/ioutil"
	"net/url"
	"os"
	"testing"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/config"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
	"k8s.io/client-go/util/certificate"
)

var (
	testCaCert = []byte(`-----BEGIN CERTIFICATE-----
MIICRzCCAfGgAwIBAgIJANXr+UzRFq4TMA0GCSqGSIb3DQEBCwUAMH4xCzAJBgNV
BAYTAkdCMQ8wDQYDVQQIDAZMb25kb24xDzANBgNVBAcMBkxvbmRvbjEYMBYGA1UE
CgwPR2xvYmFsIFNlY3VyaXR5MRYwFAYDVQQLDA1JVCBEZXBhcnRtZW50MRswGQYD
VQQDDBJ0ZXN0LWNlcnRpZmljYXRlLTEwIBcNMTcwNDI2MjMyNzMyWhgPMjExNzA0
MDIyMzI3MzJaMH4xCzAJBgNVBAYTAkdCMQ8wDQYDVQQIDAZMb25kb24xDzANBgNV
BAcMBkxvbmRvbjEYMBYGA1UECgwPR2xvYmFsIFNlY3VyaXR5MRYwFAYDVQQLDA1J
VCBEZXBhcnRtZW50MRswGQYDVQQDDBJ0ZXN0LWNlcnRpZmljYXRlLTEwXDANBgkq
hkiG9w0BAQEFAANLADBIAkEAqvbkN4RShH1rL37JFp4fZPnn0JUhVWWsrP8NOomJ
pXdBDUMGWuEQIsZ1Gf9JrCQLu6ooRyHSKRFpAVbMQ3ABJwIDAQABo1AwTjAdBgNV
HQ4EFgQUEGBc6YYheEZ/5MhwqSUYYPYRj2MwHwYDVR0jBBgwFoAUEGBc6YYheEZ/
5MhwqSUYYPYRj2MwDAYDVR0TBAUwAwEB/zANBgkqhkiG9w0BAQsFAANBAIyNmznk
5dgJY52FppEEcfQRdS5k4XFPc22SHPcz77AHf5oWZ1WG9VezOZZPp8NCiFDDlDL8
yma33a5eMyTjLD8=
-----END CERTIFICATE-----`)
)

func TestNewYurtHubCertManager(t *testing.T) {
	testUrl, err := url.Parse("https://127.0.0.1")
	if err != nil {
		t.Fatalf("parse test url failed. err:%v", err)
	}
	testNodeName := "localhost"

	type args struct {
		cfg *config.YurtHubConfiguration
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{"no config", args{}, true},
		{"config has no node name", args{&config.YurtHubConfiguration{
			RemoteServers: []*url.URL{testUrl},
		}}, true},
		{"config has no remote server", args{&config.YurtHubConfiguration{
			NodeName: testNodeName,
		}}, true},
		{"config valid", args{&config.YurtHubConfiguration{
			RemoteServers: []*url.URL{testUrl},
			NodeName:      testNodeName,
		}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err = NewYurtHubCertManager(tt.args.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewYurtHubCertManager() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}

func Test_removeDirContents(t *testing.T) {

	dir, err := ioutil.TempDir("", "yurthub-test-removeDirContents")
	if err != nil {
		t.Fatalf("Unable to create the test directory %q: %v", dir, err)
	}
	defer func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Errorf("Unable to clean up test directory %q: %v", dir, err)
		}
	}()

	tempFile := dir + "/tmp.txt"
	if err = ioutil.WriteFile(tempFile, nil, 0600); err != nil {
		t.Fatalf("Unable to create the test file %q: %v", tempFile, err)
	}

	type args struct {
		dir string
	}
	tests := []struct {
		name    string
		args    args
		file    string
		wantErr bool
	}{
		{"no input dir", args{}, "", true},
		{"input dir exist", args{dir}, tempFile, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err = removeDirContents(tt.args.dir); (err != nil) != tt.wantErr {
				t.Errorf("removeDirContents() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.file != "" {
				if _, err = os.Stat(tt.file); err == nil || !os.IsNotExist(err) {
					t.Errorf("after remote dir content, no file should exist. file:%s", tt.file)
				}
			}
		})
	}
}

func Test_createInsecureRestClientConfig(t *testing.T) {
	testUrl, err := url.Parse("https://127.0.0.1")
	if err != nil {
		t.Fatalf("parse test url failed. err:%v", err)
	}
	type args struct {
		remoteServer *url.URL
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{"no remote server", args{}, true},
		{"remote server exist", args{testUrl}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := createInsecureRestClientConfig(tt.args.remoteServer)
			if (err != nil) != tt.wantErr {
				t.Errorf("createInsecureRestClientConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}

type fakeStore struct {
	data map[string][]byte
}

func newFakeStore() (storage.Store, error) {
	return &fakeStore{data: make(map[string][]byte)}, nil
}

func (s *fakeStore) Create(key string, contents []byte) error {
	if key == "" {
		return storage.ErrKeyIsEmpty
	}
	s.data[key] = contents
	return nil
}
func (s *fakeStore) Delete(key string) error {
	if key == "" {
		return storage.ErrKeyIsEmpty
	}
	s.data[key] = nil
	return nil
}
func (s *fakeStore) Get(key string) ([]byte, error) {
	if key == "" {
		return nil, storage.ErrKeyIsEmpty
	}
	return s.data[key], nil
}
func (s *fakeStore) ListKeys(key string) ([]string, error) {
	// not support for this case
	return nil, nil
}
func (s *fakeStore) List(key string) ([][]byte, error) {
	// not support for this case
	return nil, nil
}
func (s *fakeStore) Update(key string, contents []byte) error {
	return s.Create(key, contents)
}
func (s *fakeStore) Replace(rootKey string, contents map[string][]byte) error {
	// not support for this case
	return nil
}
func (s *fakeStore) DeleteCollection(rootKey string) error {
	// not support for this case
	return nil
}

func Test_yurtHubCertManager_createBootstrapConfFile(t *testing.T) {
	testUrl, err := url.Parse("https://127.0.0.1")
	if err != nil {
		t.Fatalf("parse test url failed. err:%v", err)
	}
	testFakeStore, _ := newFakeStore()
	dir, err := ioutil.TempDir("", "yurthub-test-ca")
	if err != nil {
		t.Fatalf("Unable to create the test directory %q: %v", dir, err)
	}
	defer func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Errorf("Unable to clean up test directory %q: %v", dir, err)
		}
	}()
	testCAFile := dir + "/ca.crt"
	if err = ioutil.WriteFile(testCAFile, testCaCert, 0600); err != nil {
		t.Fatalf("Unable to create the test file %q: %v", testCAFile, err)
	}
	type fields struct {
		remoteServers         []*url.URL
		hubCertOrganizations  []string
		bootstrapConfStore    storage.Store
		hubClientCertManager  certificate.Manager
		hubClientCertPath     string
		joinToken             string
		caFile                string
		nodeName              string
		rootDir               string
		hubName               string
		kubeletRootCAFilePath string
		kubeletPairFilePath   string
		dialer                *util.Dialer
		stopCh                chan struct{}
	}
	type args struct {
		joinToken string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{"illegal remote server", fields{
			remoteServers: []*url.URL{{}},
		}, args{}, true},
		{"ca file not exist", fields{
			remoteServers: []*url.URL{testUrl},
		}, args{}, true},
		{"success", fields{
			remoteServers:      []*url.URL{testUrl},
			bootstrapConfStore: testFakeStore,
			caFile:             testCAFile,
		}, args{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ycm := &yurtHubCertManager{
				remoteServers:         tt.fields.remoteServers,
				hubCertOrganizations:  tt.fields.hubCertOrganizations,
				bootstrapConfStore:    tt.fields.bootstrapConfStore,
				hubClientCertManager:  tt.fields.hubClientCertManager,
				hubClientCertPath:     tt.fields.hubClientCertPath,
				joinToken:             tt.fields.joinToken,
				caFile:                tt.fields.caFile,
				nodeName:              tt.fields.nodeName,
				rootDir:               tt.fields.rootDir,
				hubName:               tt.fields.hubName,
				kubeletRootCAFilePath: tt.fields.kubeletRootCAFilePath,
				kubeletPairFilePath:   tt.fields.kubeletPairFilePath,
				dialer:                tt.fields.dialer,
				stopCh:                tt.fields.stopCh,
			}
			if err = ycm.createBootstrapConfFile(tt.args.joinToken); (err != nil) != tt.wantErr {
				t.Errorf("createBootstrapConfFile() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_yurtHubCertManager_updateBootstrapConfFile(t *testing.T) {
	type fields struct {
		remoteServers         []*url.URL
		hubCertOrganizations  []string
		bootstrapConfStore    storage.Store
		hubClientCertManager  certificate.Manager
		hubClientCertPath     string
		joinToken             string
		caFile                string
		nodeName              string
		rootDir               string
		hubName               string
		kubeletRootCAFilePath string
		kubeletPairFilePath   string
		dialer                *util.Dialer
		stopCh                chan struct{}
	}
	type args struct {
		joinToken string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{"joinToken not exist", fields{}, args{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ycm := &yurtHubCertManager{
				remoteServers:         tt.fields.remoteServers,
				hubCertOrganizations:  tt.fields.hubCertOrganizations,
				bootstrapConfStore:    tt.fields.bootstrapConfStore,
				hubClientCertManager:  tt.fields.hubClientCertManager,
				hubClientCertPath:     tt.fields.hubClientCertPath,
				joinToken:             tt.fields.joinToken,
				caFile:                tt.fields.caFile,
				nodeName:              tt.fields.nodeName,
				rootDir:               tt.fields.rootDir,
				hubName:               tt.fields.hubName,
				kubeletRootCAFilePath: tt.fields.kubeletRootCAFilePath,
				kubeletPairFilePath:   tt.fields.kubeletPairFilePath,
				dialer:                tt.fields.dialer,
				stopCh:                tt.fields.stopCh,
			}
			if err := ycm.updateBootstrapConfFile(tt.args.joinToken); (err != nil) != tt.wantErr {
				t.Errorf("updateBootstrapConfFile() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
