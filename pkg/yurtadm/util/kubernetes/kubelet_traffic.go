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

package kubernetes

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurtadm/constants"
	"github.com/openyurtio/openyurt/pkg/yurtadm/util/edgenode"
)

var (
	kubeletKubeadmArgsRegexp = regexp.MustCompile(`KUBELET_KUBEADM_ARGS="(.+)"`)
	kubeletMkdirAll          = os.MkdirAll
	kubeletReadFile          = os.ReadFile
	kubeletWriteFile         = os.WriteFile
	kubeletRemoveFile        = os.Remove
	kubeletExecCommand       = exec.Command
	kubeletExec              = edgenode.Exec
)

// KubeletYurthubConfig describes the host files involved in redirecting kubelet
// traffic through yurthub.
type KubeletYurthubConfig struct {
	KubeconfigFilePath   string
	KubeadmFlagsFilePath string
	FileMode             os.FileMode
}

// WriteKubeletConfig writes the kubeconfig that redirects kubelet traffic to yurthub.
func (cfg *KubeletYurthubConfig) WriteKubeletConfig() error {
	if len(cfg.KubeconfigFilePath) == 0 {
		return fmt.Errorf("kubeconfig file path is empty")
	}

	kubeletConfigDir := filepath.Dir(cfg.KubeconfigFilePath)
	if _, err := os.Stat(kubeletConfigDir); err != nil {
		if os.IsNotExist(err) {
			if err := kubeletMkdirAll(kubeletConfigDir, os.ModePerm); err != nil {
				klog.Errorf("Create dir %s fail: %v", kubeletConfigDir, err)
				return err
			}
		} else {
			klog.Errorf("Describe dir %s fail: %v", kubeletConfigDir, err)
			return err
		}
	}

	if err := kubeletWriteFile(cfg.KubeconfigFilePath, []byte(constants.KubeletConfForNode), cfg.fileMode()); err != nil {
		return err
	}

	return nil
}

// RemoveKubeletConfig removes the kubeconfig that redirects kubelet traffic to yurthub.
func (cfg *KubeletYurthubConfig) RemoveKubeletConfig() error {
	if len(cfg.KubeconfigFilePath) == 0 {
		return fmt.Errorf("kubeconfig file path is empty")
	}

	if _, err := os.Stat(cfg.KubeconfigFilePath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	return kubeletRemoveFile(cfg.KubeconfigFilePath)
}

// AppendKubeadmFlags updates kubeadm-flags.env so kubelet uses the yurthub kubeconfig.
func (cfg *KubeletYurthubConfig) AppendKubeadmFlags() error {
	if len(cfg.KubeadmFlagsFilePath) == 0 {
		return fmt.Errorf("kubeadm flags file path is empty")
	}

	kubeconfigSetup := cfg.appendSetting()
	content, err := kubeletReadFile(cfg.KubeadmFlagsFilePath)
	if err != nil {
		return err
	}

	args := string(content)
	if strings.Contains(args, kubeconfigSetup) {
		klog.Info("kubeConfigSetup has wrote before")
		return nil
	}

	finding := kubeletKubeadmArgsRegexp.FindStringSubmatch(args)
	if len(finding) != 2 {
		return fmt.Errorf("kubeadm-flags.env error format. %s", args)
	}

	updatedArgs := strings.Replace(args, finding[1], fmt.Sprintf("%s %s", finding[1], kubeconfigSetup), 1)
	return kubeletWriteFile(cfg.KubeadmFlagsFilePath, []byte(updatedArgs), cfg.fileMode())
}

// UndoAppendKubeadmFlags removes the yurthub kubeconfig override from kubeadm-flags.env.
func (cfg *KubeletYurthubConfig) UndoAppendKubeadmFlags() error {
	if len(cfg.KubeadmFlagsFilePath) == 0 {
		return fmt.Errorf("kubeadm flags file path is empty")
	}

	kubeconfigSetup := cfg.appendSetting()
	content, err := kubeletReadFile(cfg.KubeadmFlagsFilePath)
	if err != nil {
		return err
	}

	updatedArgs := strings.ReplaceAll(string(content), kubeconfigSetup, "")
	if err := kubeletWriteFile(cfg.KubeadmFlagsFilePath, []byte(updatedArgs), cfg.fileMode()); err != nil {
		return err
	}

	klog.Info("revertKubelet: undoAppendConfig finished")
	return nil
}

// RedirectTrafficToYurthub writes the kubeconfig, updates kubeadm-flags.env,
// and restarts kubelet so traffic is proxied through yurthub.
func (cfg *KubeletYurthubConfig) RedirectTrafficToYurthub() error {
	if err := cfg.WriteKubeletConfig(); err != nil {
		return err
	}

	if err := cfg.AppendKubeadmFlags(); err != nil {
		return err
	}

	return RestartKubeletService()
}

// UndoRedirectTrafficToYurthub restores kubelet's direct API server access and
// removes the temporary yurthub kubeconfig.
func (cfg *KubeletYurthubConfig) UndoRedirectTrafficToYurthub() error {
	if err := cfg.UndoAppendKubeadmFlags(); err != nil {
		return err
	}

	if err := RestartKubeletService(); err != nil {
		return err
	}

	if err := cfg.RemoveKubeletConfig(); err != nil {
		return err
	}

	klog.Info("revertKubelet: undoWriteYurthubKubeletConfig finished")
	return nil
}

// RestartKubeletService reloads systemd and restarts kubelet.
func RestartKubeletService() error {
	klog.Info("restartKubelet: " + constants.DaemonReload)
	cmd := kubeletExecCommand("bash", "-c", constants.DaemonReload)
	if err := kubeletExec(cmd); err != nil {
		return err
	}

	klog.Info("restartKubelet: " + constants.RestartKubeletSvc)
	cmd = kubeletExecCommand("bash", "-c", constants.RestartKubeletSvc)
	if err := kubeletExec(cmd); err != nil {
		return err
	}

	klog.Infof("restartKubelet: kubelet has been restarted")
	return nil
}

func (cfg *KubeletYurthubConfig) appendSetting() string {
	return fmt.Sprintf(" --kubeconfig=%s --bootstrap-kubeconfig= ", cfg.KubeconfigFilePath)
}

func (cfg *KubeletYurthubConfig) fileMode() os.FileMode {
	if cfg.FileMode != 0 {
		return cfg.FileMode
	}

	return constants.DirMode
}
