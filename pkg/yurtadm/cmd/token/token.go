/*
Copyright 2022 The OpenYurt Authors.

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

package token

import (
	"io"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurtadm/constants"
)

func NewCmdToken(in io.Reader, out io.Writer, outErr io.Writer) *cobra.Command {

	cmd := &cobra.Command{
		Use:                "token",
		Short:              "Run this command in order to manage openyurt joint token",
		FParseErrWhitelist: cobra.FParseErrWhitelist{UnknownFlags: true},
		RunE: func(cmd *cobra.Command, args []string) error {

			if _, err := exec.LookPath("kubeadm"); err != nil {
				klog.Fatalf("kubeadm is not installed, you can refer to this link for installation: %s.", constants.KubeadmInstallUrl)
				return err
			}

			klog.V(2).InfoS("kubeadm command exec", "args", os.Args[1:])

			kubeadmCmd := exec.Command("kubeadm", os.Args[1:]...)
			kubeadmCmd.Stdout = out
			kubeadmCmd.Stderr = outErr
			if err := kubeadmCmd.Run(); err != nil {
				return err
			}
			return nil
		},
	}

	return cmd
}
