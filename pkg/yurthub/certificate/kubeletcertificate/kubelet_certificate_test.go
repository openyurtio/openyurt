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

package kubeletcertificate

import "testing"

func TestNewKubeletCertManager(t *testing.T) {
	testcases := map[string]struct {
		kubeConfFile   string
		kubeletCAFile  string
		kubeletPemFile string
		err            error
	}{
		"kubelet.conf doesn't exist": {
			kubeConfFile: "invalid file",
			err:          KubeConfNotExistErr,
		},
		"ca.crt file doesn't exist": {
			kubeConfFile:  "../testdata/kubelet.conf",
			kubeletCAFile: "invalid file",
			err:           KubeletCANotExistErr,
		},
		"kubelet.pem doesn't exist": {
			kubeConfFile:   "../testdata/kubelet.conf",
			kubeletCAFile:  "../testdata/ca.crt",
			kubeletPemFile: "invalid file",
			err:            KubeletPemNotExistErr,
		},
		"normal kubelet cert manager": {
			kubeConfFile:   "../testdata/kubelet.conf",
			kubeletCAFile:  "../testdata/ca.crt",
			kubeletPemFile: "../testdata/kubelet.pem",
			err:            nil,
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			_, err := NewKubeletCertManager(tc.kubeConfFile, tc.kubeletCAFile, tc.kubeletPemFile)
			if err != tc.err {
				t.Errorf("expect error is %v, but got %v", tc.err, err)
			}
		})
	}
}
