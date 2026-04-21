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

package phases

import (
	"errors"
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/util/sets"
	clientset "k8s.io/client-go/kubernetes"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/openyurtio/openyurt/pkg/yurtadm/cmd/join/joindata"
	"github.com/openyurtio/openyurt/pkg/yurtadm/constants"
	yurthubutil "github.com/openyurtio/openyurt/pkg/yurtadm/util/yurthub"
)

type fakeJoinData struct {
	cfgPath               string
	serverAddr            string
	joinToken             string
	pauseImage            string
	yurthubImage          string
	yurthubBinaryURL      string
	hostControlPlaneAddr  string
	yurthubServer         string
	yurthubTemplate       string
	yurthubManifest       string
	kubernetesVersion     string
	tlsBootstrapCfg       *clientcmdapi.Config
	bootstrapClient       *clientset.Clientset
	nodeRegistration      *joindata.NodeRegistration
	caCertHashes          []string
	nodeLabels            map[string]string
	ignorePreflightErrors sets.Set[string]
	kubernetesResourceSrv string
	reuseCNIBin           bool
	namespace             string
	staticPodTemplates    []string
	staticPodManifests    []string
}

func (f fakeJoinData) CfgPath() string                              { return f.cfgPath }
func (f fakeJoinData) ServerAddr() string                           { return f.serverAddr }
func (f fakeJoinData) JoinToken() string                            { return f.joinToken }
func (f fakeJoinData) PauseImage() string                           { return f.pauseImage }
func (f fakeJoinData) YurtHubImage() string                         { return f.yurthubImage }
func (f fakeJoinData) YurtHubBinaryUrl() string                     { return f.yurthubBinaryURL }
func (f fakeJoinData) HostControlPlaneAddr() string                 { return f.hostControlPlaneAddr }
func (f fakeJoinData) YurtHubServer() string                        { return f.yurthubServer }
func (f fakeJoinData) YurtHubTemplate() string                      { return f.yurthubTemplate }
func (f fakeJoinData) YurtHubManifest() string                      { return f.yurthubManifest }
func (f fakeJoinData) KubernetesVersion() string                    { return f.kubernetesVersion }
func (f fakeJoinData) TLSBootstrapCfg() *clientcmdapi.Config        { return f.tlsBootstrapCfg }
func (f fakeJoinData) BootstrapClient() *clientset.Clientset        { return f.bootstrapClient }
func (f fakeJoinData) NodeRegistration() *joindata.NodeRegistration { return f.nodeRegistration }
func (f fakeJoinData) CaCertHashes() []string                       { return f.caCertHashes }
func (f fakeJoinData) NodeLabels() map[string]string                { return f.nodeLabels }
func (f fakeJoinData) IgnorePreflightErrors() sets.Set[string]      { return f.ignorePreflightErrors }
func (f fakeJoinData) KubernetesResourceServer() string             { return f.kubernetesResourceSrv }
func (f fakeJoinData) ReuseCNIBin() bool                            { return f.reuseCNIBin }
func (f fakeJoinData) Namespace() string                            { return f.namespace }
func (f fakeJoinData) StaticPodTemplateList() []string              { return f.staticPodTemplates }
func (f fakeJoinData) StaticPodManifestList() []string              { return f.staticPodManifests }

func TestRunConvert(t *testing.T) {
	originalCreateYurthubSystemdServiceFunc := createYurthubSystemdServiceFunc
	originalCheckYurthubServiceHealthFunc := checkYurthubServiceHealthFunc
	originalCheckYurthubReadyzFunc := checkYurthubReadyzFunc
	originalRedirectKubeletTrafficFunc := redirectKubeletTrafficFunc
	originalCheckKubeletStatusFunc := checkKubeletStatusFunc
	t.Cleanup(func() {
		createYurthubSystemdServiceFunc = originalCreateYurthubSystemdServiceFunc
		checkYurthubServiceHealthFunc = originalCheckYurthubServiceHealthFunc
		checkYurthubReadyzFunc = originalCheckYurthubReadyzFunc
		redirectKubeletTrafficFunc = originalRedirectKubeletTrafficFunc
		checkKubeletStatusFunc = originalCheckKubeletStatusFunc
	})

	testCases := []struct {
		name         string
		mode         string
		wantCalls    []string
		expectCreate bool
	}{
		{
			name:         "edge node runs full convert flow",
			mode:         constants.EdgeNode,
			wantCalls:    []string{"create", "health", "ready", "redirect", "kubelet"},
			expectCreate: true,
		},
		{
			name:         "cloud node runs full convert flow",
			mode:         constants.CloudNode,
			wantCalls:    []string{"create", "health", "ready", "redirect", "kubelet"},
			expectCreate: true,
		},
		{
			name:      "local node skips convert flow",
			mode:      constants.LocalNode,
			wantCalls: nil,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			var gotCalls []string
			var gotConfigNodeName string
			var gotConfigNodePool string
			var gotConfigMode string
			var gotConfigNamespace string
			var gotConfigServerAddr string
			var gotBootstrapMode string

			createYurthubSystemdServiceFunc = func(cfg *yurthubutil.YurthubHostConfig) error {
				gotCalls = append(gotCalls, "create")
				gotConfigNodeName = cfg.NodeName
				gotConfigNodePool = cfg.NodePoolName
				gotConfigMode = cfg.WorkingMode
				gotConfigNamespace = cfg.Namespace
				gotConfigServerAddr = cfg.ServerAddr
				gotBootstrapMode = cfg.BootstrapMode
				return nil
			}
			checkYurthubServiceHealthFunc = func(addr string) error {
				gotCalls = append(gotCalls, "health")
				if addr != "127.0.0.1" {
					t.Fatalf("CheckYurthubServiceHealth() addr got %q, want %q", addr, "127.0.0.1")
				}
				return nil
			}
			checkYurthubReadyzFunc = func(addr string) error {
				gotCalls = append(gotCalls, "ready")
				if addr != "127.0.0.1" {
					t.Fatalf("CheckYurthubReadyz() addr got %q, want %q", addr, "127.0.0.1")
				}
				return nil
			}
			redirectKubeletTrafficFunc = func(openyurtDir string) error {
				gotCalls = append(gotCalls, "redirect")
				if openyurtDir != constants.OpenyurtDir {
					t.Fatalf("RedirectTrafficToYurtHub() openyurtDir got %q, want %q", openyurtDir, constants.OpenyurtDir)
				}
				return nil
			}
			checkKubeletStatusFunc = func() error {
				gotCalls = append(gotCalls, "kubelet")
				return nil
			}

			data := fakeJoinData{
				serverAddr:    "10.0.0.1:6443",
				yurthubServer: "127.0.0.1",
				namespace:     "kube-system",
				nodeRegistration: &joindata.NodeRegistration{
					Name:         "node-a",
					NodePoolName: "pool-a",
					WorkingMode:  tt.mode,
				},
			}

			err := RunConvert(data)
			if err != nil {
				t.Fatalf("RunConvert() unexpected error: %v", err)
			}

			if !reflect.DeepEqual(tt.wantCalls, gotCalls) {
				t.Fatalf("RunConvert() calls got %v, want %v", gotCalls, tt.wantCalls)
			}
			if !tt.expectCreate {
				return
			}

			if gotBootstrapMode != bootstrapModeKubeletCertificate {
				t.Fatalf("RunConvert() bootstrap mode got %q, want %q", gotBootstrapMode, bootstrapModeKubeletCertificate)
			}
			if gotConfigMode != tt.mode {
				t.Fatalf("RunConvert() working mode got %q, want %q", gotConfigMode, tt.mode)
			}
			if gotConfigNodeName != "node-a" || gotConfigNodePool != "pool-a" {
				t.Fatalf("RunConvert() cfg node got (%q,%q), want (%q,%q)", gotConfigNodeName, gotConfigNodePool, "node-a", "pool-a")
			}
			if gotConfigNamespace != "kube-system" {
				t.Fatalf("RunConvert() namespace got %q, want %q", gotConfigNamespace, "kube-system")
			}
			if gotConfigServerAddr != "10.0.0.1:6443" {
				t.Fatalf("RunConvert() server addr got %q, want %q", gotConfigServerAddr, "10.0.0.1:6443")
			}
		})
	}
}

func TestRunJoinCheck(t *testing.T) {
	originalCheckKubeletStatusFunc := checkKubeletStatusFunc
	t.Cleanup(func() {
		checkKubeletStatusFunc = originalCheckKubeletStatusFunc
	})

	t.Run("checks kubelet status", func(t *testing.T) {
		called := false
		checkKubeletStatusFunc = func() error {
			called = true
			return nil
		}

		if err := RunJoinCheck(fakeJoinData{}); err != nil {
			t.Fatalf("RunJoinCheck() unexpected error: %v", err)
		}
		if !called {
			t.Fatal("RunJoinCheck() did not check kubelet status")
		}
	})

	t.Run("propagates kubelet status error", func(t *testing.T) {
		expectedErr := errors.New("kubelet down")
		checkKubeletStatusFunc = func() error {
			return expectedErr
		}

		err := RunJoinCheck(fakeJoinData{})
		if !errors.Is(err, expectedErr) {
			t.Fatalf("RunJoinCheck() error got %v, want %v", err, expectedErr)
		}
	})
}
