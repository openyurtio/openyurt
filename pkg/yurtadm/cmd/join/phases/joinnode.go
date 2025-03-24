/*
Copyright 2021 The OpenYurt Authors.
Copyright 2019 The Kubernetes Authors.

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

	"github.com/openyurtio/openyurt/pkg/yurtadm/cmd/join/joindata"
)

// RunJoinNode executes the node join process.
func RunJoinNode(data joindata.YurtJoinData, out io.Writer, outErr io.Writer) error {
	// var kubeadmJoinConfigFilePath string
	// if data.CfgPath() != "" {
	// 	kubeadmJoinConfigFilePath = data.CfgPath()
	// } else {
	// 	kubeadmJoinConfigFilePath = filepath.Join(constants.KubeletWorkdir, constants.KubeadmJoinConfigFileName)
	// }
	// kubeadmCmd := exec.Command("kubeadm", "join ", fmt.Sprintf("--config=%s", kubeadmJoinConfigFilePath))
	kubeadmCmd := exec.Command("kubeadm", "join", "192.168.24.179:6443",
		"--token", "g3pqux.54hzlfue88ab3ovr",
		"--discovery-token-unsafe-skip-ca-verification",
		"--v=5",
	)
	kubeadmCmd.Stdout = out
	kubeadmCmd.Stderr = outErr

	if err := kubeadmCmd.Run(); err != nil {
		return err
	}

	return nil
}
