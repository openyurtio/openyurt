/*
Copyright 2021 The OpenYurt Authors.

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

package edgenode

import (
	"os"
	"path/filepath"
	"testing"
)

func Test_GetPodManifestPath(t *testing.T) {

	path := GetPodManifestPath()
	if path != "/etc/kubernetes/manifests" {
		t.Fatal("get path err: " + path)
	}
}

func Test_GetHostName(t *testing.T) {
	oldOSHost := osHostName
	defer func() {
		osHostName = oldOSHost
	}()

	osHostName = func() (string, error) {
		return "test_host", nil
	}

	tests := []struct {
		name             string
		hostNameOverride string
		expectedHostName string
		expectError      bool
	}{
		{
			"host name with upper case character",
			"TEST_HOST",
			"test_host",
			false,
		},
		{
			"host name with leading space ",
			"    test_host",
			"test_host",
			false,
		},
		{
			"valid host name",
			"test_host",
			"test_host",
			false,
		},
		{
			"invalid host name",
			"    ",
			"",
			true,
		},
		{
			"get from os envs",
			"",
			"test_host",
			false,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Logf("\tTestCase: %s", test.name)
			{
				hostName, err := GetHostname(test.hostNameOverride)
				if err != nil && !test.expectError {
					t.Errorf("unexpected error: %s", err)
				}
				if err == nil && test.expectError {
					t.Errorf("expected error, got none")
				}
				if test.expectedHostName != hostName {
					t.Errorf("expected output %q, got %q", test.expectedHostName, hostName)
				}
			}
		})
	}
}

func Test_parseEnvironmentFilePath(t *testing.T) {
	tests := []struct {
		name      string
		directive string
		expected  string
	}{
		{
			"kubeadm default, optional '-' prefix and '-' inside the path",
			"EnvironmentFile=-/var/lib/kubelet/kubeadm-flags.env",
			"/var/lib/kubelet/kubeadm-flags.env",
		},
		{
			"no '-' anywhere",
			"EnvironmentFile=/opt/kubelet/config.env",
			"/opt/kubelet/config.env",
		},
		{
			"path without a leading '-' but with hyphens inside",
			"EnvironmentFile=/etc/default/kube-lite-node.env",
			"/etc/default/kube-lite-node.env",
		},
		{
			"trailing whitespace",
			"EnvironmentFile=-/var/lib/kubelet/kubeadm-flags.env   ",
			"/var/lib/kubelet/kubeadm-flags.env",
		},
		{
			"directive without a path",
			"EnvironmentFile=",
			"",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			if got := parseEnvironmentFilePath(test.directive); got != test.expected {
				t.Errorf("expected %q, got %q", test.expected, got)
			}
		})
	}
}

func Test_GetNodeNameFromEnvironmentFile(t *testing.T) {
	tests := []struct {
		name string
		// envFileName is the file the EnvironmentFile directive points at.
		envFileName string
		// dashPrefix reproduces systemd's optional "ignore if missing" marker.
		dashPrefix bool
	}{
		{
			// The kubeadm default. The old code split on every '-' and took
			// index [1], which truncated the path at the hyphen in the file name.
			"path with a '-' inside and the systemd '-' prefix",
			"kubeadm-flags.env",
			true,
		},
		{
			// No '-' anywhere, so the old code indexed a one-element slice and
			// panicked with "index out of range [1]".
			"path without any '-'",
			"config.env",
			false,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(NodeName, "")

			dir := t.TempDir()
			envFile := filepath.Join(dir, test.envFileName)
			if err := os.WriteFile(envFile, []byte(`KUBELET_KUBEADM_ARGS="--hostname-override=edge-node-1"`+"\n"), 0644); err != nil {
				t.Fatalf("could not write env file: %v", err)
			}

			// The kubeadm layout: no --hostname-override in the drop-in itself,
			// only an EnvironmentFile directive pointing at the flags file.
			directive := "EnvironmentFile="
			if test.dashPrefix {
				directive += "-"
			}
			kubeadmConf := filepath.Join(dir, "10-kubeadm.conf")
			if err := os.WriteFile(kubeadmConf, []byte("[Service]\n"+directive+envFile+"\n"), 0644); err != nil {
				t.Fatalf("could not write kubeadm conf: %v", err)
			}

			nodeName, err := GetNodeName(kubeadmConf)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if nodeName != "edge-node-1" {
				t.Errorf("expected node name %q, got %q", "edge-node-1", nodeName)
			}
		})
	}
}

func TestDeployStaticYaml(t *testing.T) {
	tests := []struct {
		name            string
		manifestList    []string
		templateList    []string
		podManifestPath string
		wantErr         bool
	}{
		{
			name:            "test1",
			manifestList:    []string{"nginx"},
			templateList:    []string{"xxxxxx"},
			podManifestPath: "/tmp",
			wantErr:         false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := DeployStaticYaml(tt.manifestList, tt.templateList, tt.podManifestPath); (err != nil) != tt.wantErr {
				t.Errorf("DeployStaticYaml() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRemoveStaticYaml(t *testing.T) {
	tests := []struct {
		name            string
		manifestList    []string
		podManifestPath string
		wantErr         bool
	}{
		{
			name:            "test1",
			manifestList:    []string{"nginx"},
			podManifestPath: "/tmp",
			wantErr:         false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := RemoveStaticYaml(tt.manifestList, tt.podManifestPath); (err != nil) != tt.wantErr {
				t.Errorf("RemoveStaticYaml() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
