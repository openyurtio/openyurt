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
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"

	"github.com/Masterminds/semver/v3"
	"github.com/spf13/cobra"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurtadm/constants"
	"github.com/openyurtio/openyurt/pkg/yurtadm/util"
	"github.com/openyurtio/openyurt/pkg/yurtadm/util/edgenode"
)

const (
	TmpDownloadDir = "/tmp"

	KubeadmUrlFormat      = "https://storage.googleapis.com/kubernetes-release/release/%s/bin/linux/%s/kubeadm"
	DefaultKubeadmVersion = "v1.22.3"

	MiniumKubeadmVersion = "v1.22.3"
)

func NewCmdToken(in io.Reader, out io.Writer, outErr io.Writer) *cobra.Command {

	cmd := &cobra.Command{
		Use:                "token",
		Short:              "Run this command in order to manage openyurt joint token",
		FParseErrWhitelist: cobra.FParseErrWhitelist{UnknownFlags: true},
		RunE: func(cmd *cobra.Command, args []string) error {

			if err := CheckAndInstallKubeadm(); err != nil {
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

type kubeadmVersion struct {
	ClientVersion clientVersion `json:"clientVersion"`
}
type clientVersion struct {
	GitVersion string `json:"gitVersion"`
}

func CheckAndInstallKubeadm() error {
	klog.V(2).Infof("Check and install kubeadm")
	kubeadmExist := false

	if _, err := exec.LookPath("kubeadm"); err == nil {
		if b, err := exec.Command("kubeadm", "version", "-o", "json").CombinedOutput(); err == nil {
			klog.V(2).InfoS("kubeadm", "version", string(b))
			info := kubeadmVersion{}
			if err := json.Unmarshal(b, &info); err != nil {
				return fmt.Errorf("Can't get the existing kubeadm version: %w", err)
			}
			kubeadmVersion := info.ClientVersion.GitVersion

			c, err := semver.NewConstraint(fmt.Sprintf(">= %s", MiniumKubeadmVersion))
			if err != nil {
				return fmt.Errorf("kubeadm version constraint build fail, err: %s", err.Error())
			}
			v, err := semver.NewVersion(kubeadmVersion)
			if err != nil {
				return fmt.Errorf("current kubeadm version build fail, err: %s", err.Error())
			}

			if c.Check(v) {
				klog.V(2).Infof("Kubeadm %s already exist, skip install.", kubeadmVersion)
				kubeadmExist = true
			} else {
				return fmt.Errorf("The existing kubeadm version %s is not supported, please clean it. Valid server versions are %v.", kubeadmVersion, MiniumKubeadmVersion)
			}
		} else {
			klog.ErrorS(err, "kubeadm version fail")
		}
	} else {
		klog.ErrorS(err, "kubeadm look path fail")
	}

	if !kubeadmExist {
		// download and install kubeadm
		packageUrl := fmt.Sprintf(KubeadmUrlFormat, DefaultKubeadmVersion, runtime.GOARCH)
		savePath := fmt.Sprintf("%s/kubeadm", TmpDownloadDir)
		klog.V(2).Infof("Download kubeadm from: %s", packageUrl)
		if err := util.DownloadFile(packageUrl, savePath, 3); err != nil {
			return fmt.Errorf("Download kubeadm fail: %w", err)
		}
		comp := "kubeadm"
		target := fmt.Sprintf("/usr/bin/%s", comp)
		if err := edgenode.CopyFile(TmpDownloadDir+"/"+comp, target, constants.DirMode); err != nil {
			return err
		}
	}
	return nil
}
