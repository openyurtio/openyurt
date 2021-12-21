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

package components

import (
	"os"
)

const (
	KubeletSvcEnv           = "KUBELET_SVC"
	KubeletSvcPathSystemUsr = "/usr/lib/systemd/system/kubelet.service.d/10-kubeadm.conf"
	KubelerSvcPathSystemEtc = "/etc/systemd/system/kubelet.service.d/10-kubeadm.conf"
)

func GetDefaultKubeadmConfPath() []string {
	kubeadmConfPath := []string{}
	path := os.Getenv(KubeletSvcEnv)
	if path != "" && path != KubeletSvcPathSystemUsr && path != KubelerSvcPathSystemEtc {
		kubeadmConfPath = append(kubeadmConfPath, path)
	}
	kubeadmConfPath = append(kubeadmConfPath, KubeletSvcPathSystemUsr, KubelerSvcPathSystemEtc)
	return kubeadmConfPath
}
