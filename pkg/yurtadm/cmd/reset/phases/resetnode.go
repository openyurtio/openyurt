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

package phases

import (
	"io"
	"os/exec"
	"strconv"
	"strings"

	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurtadm/cmd/reset/resetdata"
	"github.com/openyurtio/openyurt/pkg/yurtadm/constants"
)

func RunResetNode(data resetdata.YurtResetData, in io.Reader, out io.Writer, outErr io.Writer) error {
	if _, err := exec.LookPath("kubeadm"); err != nil {
		klog.Fatalf("kubeadm is not installed, you can refer to this link for installation: %s.", constants.KubeadmInstallUrl)
		return err
	}

	kubeadmCmd := exec.Command("kubeadm", "reset",
		"--cert-dir="+data.CertificatesDir(),
		"--cri-socket="+data.CRISocketPath(),
		"--force="+strconv.FormatBool(data.ForceReset()),
		"--ignore-preflight-errors="+strings.Join(data.IgnorePreflightErrors(), ","),
	)
	kubeadmCmd.Stdin = in
	kubeadmCmd.Stdout = out
	kubeadmCmd.Stderr = outErr

	if err := kubeadmCmd.Run(); err != nil {
		return err
	}

	return nil
}
