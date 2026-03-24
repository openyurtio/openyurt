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

package components

import (
	"os"
	"testing"
)

func TestDetectCRISocketFromKubeletArgsPrefersConfiguredRuntimeEndpoint(t *testing.T) {
	files := map[string]string{
		KubelerSvcPathSystemEtc: `[Service]
Environment="KUBELET_CONFIG_ARGS=--config=/var/lib/kubelet/config.yaml"
EnvironmentFile=-/var/lib/kubelet/kubeadm-flags.env
EnvironmentFile=-/etc/default/kubelet
ExecStart=/usr/bin/kubelet $KUBELET_CONFIG_ARGS $KUBELET_KUBEADM_ARGS $KUBELET_EXTRA_ARGS
`,
		kubeAdmFlagsEnvFile:   `KUBELET_KUBEADM_ARGS="--container-runtime-endpoint=unix:///run/containerd/containerd.sock --pod-infra-container-image=registry.k8s.io/pause:3.10"`,
		defaultKubeletEnvFile: `KUBELET_EXTRA_ARGS="--node-ip=10.0.0.1"`,
	}

	socket := detectCRISocketFromKubelet(fakeReadFile(files), fakeIsSocket(dockerSocket, containerdSocket), []string{KubelerSvcPathSystemEtc})
	if socket != containerdSocket {
		t.Fatalf("expected %q, got %q", containerdSocket, socket)
	}
}

func TestDetectCRISocketFromKubeletConfigFallback(t *testing.T) {
	const customSocket = "/run/k3s/containerd/containerd.sock"
	const customConfig = "/etc/rancher/k3s/kubelet.config"

	files := map[string]string{
		KubeletSvcPathSystemUsr: `[Service]
Environment="KUBELET_CONFIG_ARGS=--config=/etc/rancher/k3s/kubelet.config"
EnvironmentFile=-/var/lib/kubelet/kubeadm-flags.env
ExecStart=/usr/local/bin/kubelet $KUBELET_CONFIG_ARGS $KUBELET_KUBEADM_ARGS
`,
		customConfig: `kind: KubeletConfiguration
apiVersion: kubelet.config.k8s.io/v1beta1
containerRuntimeEndpoint: "unix:///run/k3s/containerd/containerd.sock"
`,
	}

	socket := detectCRISocketFromKubelet(fakeReadFile(files), fakeIsSocket(customSocket), []string{KubeletSvcPathSystemUsr})
	if socket != customSocket {
		t.Fatalf("expected %q, got %q", customSocket, socket)
	}
}

func TestDetectCRISocketFromKubeletIgnoresMissingRuntimeEndpoint(t *testing.T) {
	files := map[string]string{
		KubeletSvcPathSystemUsr: `[Service]
EnvironmentFile=-/var/lib/kubelet/kubeadm-flags.env
`,
		kubeAdmFlagsEnvFile: `KUBELET_KUBEADM_ARGS="--container-runtime-endpoint=unix:///run/unknown/unknown.sock"`,
	}

	socket := detectCRISocketFromKubelet(fakeReadFile(files), fakeIsSocket(containerdSocket), []string{KubeletSvcPathSystemUsr})
	if socket != "" {
		t.Fatalf("expected empty socket, got %q", socket)
	}
}

func fakeReadFile(files map[string]string) func(string) ([]byte, error) {
	return func(path string) ([]byte, error) {
		content, ok := files[path]
		if !ok {
			return nil, os.ErrNotExist
		}
		return []byte(content), nil
	}
}

func fakeIsSocket(paths ...string) func(string) bool {
	known := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		known[path] = struct{}{}
	}
	return func(path string) bool {
		_, ok := known[path]
		return ok
	}
}
