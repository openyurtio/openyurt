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

package convert

import (
	"errors"
	"reflect"
	"testing"

	yurthubutil "github.com/openyurtio/openyurt/pkg/yurtadm/util/yurthub"
)

func TestNodeConverterDo(t *testing.T) {
	oldGetAPIServerAddressFunc := getAPIServerAddressFunc
	oldInstallYurthubFunc := installYurthubFunc
	oldRedirectKubeletFunc := redirectKubeletFunc
	oldRestartContainersFunc := restartContainersFunc
	defer func() {
		getAPIServerAddressFunc = oldGetAPIServerAddressFunc
		installYurthubFunc = oldInstallYurthubFunc
		redirectKubeletFunc = oldRedirectKubeletFunc
		restartContainersFunc = oldRestartContainersFunc
	}()

	converter := &nodeConverter{
		Config: Config{
			kubeadmConfPaths: []string{"/etc/systemd/system/kubelet.service.d/10-kubeadm.conf"},
			namespace:        "kube-system",
			nodeName:         "node-a",
			nodePoolName:     "pool-a",
			openyurtDir:      "/var/lib/openyurt",
			workingMode:      "edge",
		},
	}

	var gotCfg *yurthubutil.YurthubHostConfig
	var calls []string
	getAPIServerAddressFunc = func(paths []string) (string, error) {
		if !reflect.DeepEqual(paths, converter.kubeadmConfPaths) {
			t.Fatalf("unexpected kubeadm conf paths: %v", paths)
		}
		calls = append(calls, "discover-apiserver")
		return "10.0.0.1:6443", nil
	}
	installYurthubFunc = func(cfg *yurthubutil.YurthubHostConfig) error {
		gotCfg = cfg
		calls = append(calls, "install-yurthub")
		return nil
	}
	redirectKubeletFunc = func(openyurtDir string) error {
		if openyurtDir != converter.openyurtDir {
			t.Fatalf("unexpected openyurt dir %q", openyurtDir)
		}
		calls = append(calls, "redirect-kubelet")
		return nil
	}
	restartContainersFunc = func(nodeName string) error {
		if nodeName != converter.nodeName {
			t.Fatalf("unexpected node name %q", nodeName)
		}
		calls = append(calls, "restart-containers")
		return nil
	}

	if err := converter.Do(); err != nil {
		t.Fatalf("Do() returned error: %v", err)
	}

	wantCalls := []string{"discover-apiserver", "install-yurthub", "redirect-kubelet", "restart-containers"}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("unexpected call order, got=%v, want=%v", calls, wantCalls)
	}

	wantCfg := &yurthubutil.YurthubHostConfig{
		BootstrapMode: bootstrapModeKubeletCertificate,
		Namespace:     "kube-system",
		NodeName:      "node-a",
		NodePoolName:  "pool-a",
		ServerAddr:    "10.0.0.1:6443",
		WorkingMode:   "edge",
	}
	if !reflect.DeepEqual(gotCfg, wantCfg) {
		t.Fatalf("unexpected yurthub host config, got=%#v, want=%#v", gotCfg, wantCfg)
	}
}

func TestNodeConverterStopsWhenInstallFails(t *testing.T) {
	oldGetAPIServerAddressFunc := getAPIServerAddressFunc
	oldInstallYurthubFunc := installYurthubFunc
	oldRedirectKubeletFunc := redirectKubeletFunc
	oldRestartContainersFunc := restartContainersFunc
	defer func() {
		getAPIServerAddressFunc = oldGetAPIServerAddressFunc
		installYurthubFunc = oldInstallYurthubFunc
		redirectKubeletFunc = oldRedirectKubeletFunc
		restartContainersFunc = oldRestartContainersFunc
	}()

	converter := &nodeConverter{
		Config: Config{
			kubeadmConfPaths: []string{"/etc/systemd/system/kubelet.service.d/10-kubeadm.conf"},
			openyurtDir:      "/var/lib/openyurt",
		},
	}

	getAPIServerAddressFunc = func(paths []string) (string, error) {
		return "10.0.0.1:6443", nil
	}

	expectedErr := errors.New("install failed")
	redirectCalled := false
	restartCalled := false
	installYurthubFunc = func(cfg *yurthubutil.YurthubHostConfig) error {
		return expectedErr
	}
	redirectKubeletFunc = func(openyurtDir string) error {
		redirectCalled = true
		return nil
	}
	restartContainersFunc = func(nodeName string) error {
		restartCalled = true
		return nil
	}

	err := converter.Do()
	if !errors.Is(err, expectedErr) {
		t.Fatalf("unexpected error, got=%v, want=%v", err, expectedErr)
	}
	if redirectCalled {
		t.Fatal("expected kubelet redirection to be skipped after install failure")
	}
	if restartCalled {
		t.Fatal("expected container restart to be skipped after install failure")
	}
}
