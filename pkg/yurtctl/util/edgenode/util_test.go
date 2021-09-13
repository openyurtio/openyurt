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
	"io/ioutil"
	"os"
	"testing"
)

var kubeadmConf = `
[Service]
Environment="KUBELET_KUBECONFIG_ARGS=--kubeconfig=/etc/kubernetes/kubelet.conf"
Environment="KUBELET_CONFIG_ARGS=--config=/tmp/openyurt-test/config.yaml"
EnvironmentFile=-/var/lib/kubelet/kubeadm-flags.env
EnvironmentFile=-/etc/default/kubelet
ExecStart=
ExecStart=/usr/bin/kubelet $KUBELET_KUBECONFIG_ARGS $KUBELET_CONFIG_ARGS $KUBELET_KUBEADM_ARGS $KUBELET_EXTRA_ARGS`

var confYaml = `serializeImagePulls: true
staticPodPath: /etc/kubernetes/manifests
streamingConnectionIdleTimeout: 4h0m0s
syncFrequency: 1m0s`

func Test_GetPodManifestPath(t *testing.T) {
	err := EnsureDir("/tmp/openyurt-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll("/tmp/openyurt-test")

	_ = ioutil.WriteFile("/tmp/openyurt-test/kubeadm.conf", []byte(kubeadmConf), 0755)
	_ = ioutil.WriteFile("/tmp/openyurt-test/config.yaml", []byte(confYaml), 0755)

	path, err := GetPodManifestPath("/tmp/openyurt-test/kubeadm.conf")
	if err != nil {
		t.Fatal(err)
	}
	if path != "/etc/kubernetes/manifests" {
		t.Fatal("get path err: " + path)
	}
}
