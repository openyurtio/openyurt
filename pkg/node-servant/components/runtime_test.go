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

func TestParseCRIContainerListOutputIgnoresWarningPrefix(t *testing.T) {
	out := []byte(`time="2026-03-25T21:09:45+08:00" level=warning msg="Config \"/etc/crictl.yaml\" does not exist, trying next: \"/usr/bin/crictl.yaml\""
{
  "containers": [
    {
      "id": "container-a",
      "metadata": {
        "name": "probe"
      },
      "labels": {
        "io.kubernetes.container.name": "probe",
        "io.kubernetes.pod.name": "probe-test",
        "io.kubernetes.pod.namespace": "default"
      }
    }
  ]
}
`)

	containers, err := parseCRIContainerListOutput(out)
	if err != nil {
		t.Fatalf("parseCRIContainerListOutput() returned error: %v", err)
	}
	if len(containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(containers))
	}
	if containers[0].ID != "container-a" || containers[0].Namespace != "default" || containers[0].PodName != "probe-test" || containers[0].ContainerName != "probe" {
		t.Fatalf("unexpected container %+v", containers[0])
	}
}

func TestParseCRIContainerListOutputFallsBackToMetadataName(t *testing.T) {
	out := []byte(`{
  "containers": [
    {
      "id": "container-b",
      "metadata": {
        "name": "pause"
      },
      "labels": {
        "io.kubernetes.pod.name": "static-probe-test",
        "io.kubernetes.pod.namespace": "kube-system"
      }
    }
  ]
}`)

	containers, err := parseCRIContainerListOutput(out)
	if err != nil {
		t.Fatalf("parseCRIContainerListOutput() returned error: %v", err)
	}
	if len(containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(containers))
	}
	if containers[0].ContainerName != "pause" {
		t.Fatalf("expected metadata name fallback, got %+v", containers[0])
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
