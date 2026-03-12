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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurtadm/constants"
	enutil "github.com/openyurtio/openyurt/pkg/yurtadm/util/edgenode"
)

const (
	kubeletConfigRegularExpression = "\\-\\-kubeconfig=.*kubelet.conf"
	apiserverAddrRegularExpression = "server: (http(s)?:\\/\\/)?[\\w][-\\w]{0,62}(\\.[\\w][-\\w]{0,62})*(:[\\d]{1,5})?"

	kubeAdmFlagsEnvFile = "/var/lib/kubelet/kubeadm-flags.env"
	dirMode             = 0755
)

type kubeletOperator struct {
	openyurtDir string
}

// NewKubeletOperator create kubeletOperator
func NewKubeletOperator(openyurtDir string) *kubeletOperator {
	return &kubeletOperator{
		openyurtDir: openyurtDir,
	}
}

// RedirectTrafficToYurtHub
// add env config leads kubelet to visit yurtHub as apiServer
// This function is idempotent: if kubelet is already redirected, it will not restart kubelet.
func (op *kubeletOperator) RedirectTrafficToYurtHub() error {
	// 1. create a working dir to store revised kubelet.conf
	configChanged, err := op.writeYurthubKubeletConfig()
	if err != nil {
		return err
	}

	// 2. append /var/lib/kubelet/kubeadm-flags.env
	envChanged, err := op.appendConfig()
	if err != nil {
		return err
	}

	// 3. restart only if configuration was actually changed
	if configChanged || envChanged {
		return restartKubeletService()
	}

	klog.Info("kubelet already configured to use YurtHub, skipping restart")
	return nil
}

// UndoRedirectTrafficToYurtHub
// undo what's done to kubelet and restart
// This function is idempotent: if kubelet is already using the original config, it will not restart kubelet.
func (op *kubeletOperator) UndoRedirectTrafficToYurtHub() error {
	envChanged, err := op.undoAppendConfig()
	if err != nil {
		return err
	}

	// Only restart kubelet if the environment config was actually changed
	if envChanged {
		if err := restartKubeletService(); err != nil {
			return err
		}
	} else {
		klog.Info("kubelet config already reverted, skipping restart")
	}

	if err := op.undoWriteYurthubKubeletConfig(); err != nil {
		return err
	}
	klog.Info("revertKubelet: undoWriteYurthubKubeletConfig finished")

	return nil
}

// writeYurthubKubeletConfig writes the YurtHub kubelet config file.
// Returns (changed, error) where changed indicates if the file content was actually modified.
func (op *kubeletOperator) writeYurthubKubeletConfig() (bool, error) {
	err := os.MkdirAll(op.openyurtDir, dirMode)
	if err != nil {
		return false, err
	}
	fullPath := op.getYurthubKubeletConf()

	// Check if file already exists with the correct content
	existingContent, err := os.ReadFile(fullPath)
	if err == nil && string(existingContent) == constants.KubeletConfForNode {
		klog.Infof("kubeconfig %s already has correct content, skipping write", fullPath)
		return false, nil
	}

	err = os.WriteFile(fullPath, []byte(constants.KubeletConfForNode), fileMode)
	if err != nil {
		return false, err
	}
	klog.Infof("revised kubeconfig %s is generated", fullPath)
	return true, nil
}

func (op *kubeletOperator) undoWriteYurthubKubeletConfig() error {
	yurtKubeletConf := op.getYurthubKubeletConf()
	if _, err := enutil.FileExists(yurtKubeletConf); err != nil && os.IsNotExist(err) {
		return nil
	}

	return os.Remove(yurtKubeletConf)
}

// appendConfig appends the YurtHub kubeconfig setting to kubelet's kubeadm-flags.env.
// Returns (changed, error) where changed indicates if the file was actually modified.
func (op *kubeletOperator) appendConfig() (bool, error) {
	// set env KUBELET_KUBEADM_ARGS, args set later will override before
	// ExecStart: kubelet $KUBELET_KUBECONFIG_ARGS $KUBELET_CONFIG_ARGS $KUBELET_KUBEADM_ARGS $KUBELET_EXTRA_ARGS
	// append setup: " --kubeconfig=$yurthubKubeletConf -bootstrap-kubeconfig= "
	kubeConfigSetup := op.getAppendSetting()

	// if already configured, return false (no change needed)
	content, err := os.ReadFile(kubeAdmFlagsEnvFile)
	if err != nil {
		return false, err
	}
	args := string(content)
	if strings.Contains(args, kubeConfigSetup) {
		klog.Info("kubeConfigSetup has been written before, no change needed")
		return false, nil
	}

	// append KUBELET_KUBEADM_ARGS
	argsRegexp := regexp.MustCompile(`KUBELET_KUBEADM_ARGS="(.+)"`)
	finding := argsRegexp.FindStringSubmatch(args)
	if len(finding) != 2 {
		return false, fmt.Errorf("kubeadm-flags.env error format. %s", args)
	}

	r := strings.Replace(args, finding[1], fmt.Sprintf("%s %s", finding[1], kubeConfigSetup), 1)
	err = os.WriteFile(kubeAdmFlagsEnvFile, []byte(r), fileMode)
	if err != nil {
		return false, err
	}

	klog.Info("kubeConfigSetup has been appended to kubeadm-flags.env")
	return true, nil
}

// undoAppendConfig removes the YurtHub kubeconfig setting from kubelet's kubeadm-flags.env.
// Returns (changed, error) where changed indicates if the file was actually modified.
func (op *kubeletOperator) undoAppendConfig() (bool, error) {
	kubeConfigSetup := op.getAppendSetting()
	contentbyte, err := os.ReadFile(kubeAdmFlagsEnvFile)
	if err != nil {
		return false, err
	}

	originalContent := string(contentbyte)
	// Check if the config is present before removing
	if !strings.Contains(originalContent, kubeConfigSetup) {
		klog.Info("revertKubelet: kubeConfigSetup not present, no change needed")
		return false, nil
	}

	content := strings.ReplaceAll(originalContent, kubeConfigSetup, "")
	err = os.WriteFile(kubeAdmFlagsEnvFile, []byte(content), fileMode)
	if err != nil {
		return false, err
	}
	klog.Info("revertKubelet: undoAppendConfig finished")

	return true, nil
}

func (op *kubeletOperator) getAppendSetting() string {
	configPath := op.getYurthubKubeletConf()
	return fmt.Sprintf(" --kubeconfig=%s --bootstrap-kubeconfig= ", configPath)
}

func (op *kubeletOperator) getYurthubKubeletConf() string {
	return filepath.Join(op.openyurtDir, constants.KubeletKubeConfigFileName)
}

func restartKubeletService() error {
	klog.Info("restartKubelet: " + constants.DaemonReload)
	cmd := exec.Command("bash", "-c", constants.DaemonReload)
	if err := enutil.Exec(cmd); err != nil {
		return err
	}
	klog.Info("restartKubelet: " + constants.RestartKubeletSvc)
	cmd = exec.Command("bash", "-c", constants.RestartKubeletSvc)
	if err := enutil.Exec(cmd); err != nil {
		return err
	}
	klog.Infof("restartKubelet: kubelet has been restarted")
	return nil
}

// GetApiServerAddress parse apiServer address from conf file
func GetApiServerAddress(kubeadmConfPaths []string) (string, error) {
	var kbcfg string
	for _, path := range kubeadmConfPaths {
		if exist, _ := enutil.FileExists(path); exist {
			kbcfg = path
			break
		}
	}
	if kbcfg == "" {
		return "", fmt.Errorf("get apiserverAddr err: no file exists in list %s", kubeadmConfPaths)
	}

	kubeletConfPath, err := enutil.GetSingleContentFromFile(kbcfg, kubeletConfigRegularExpression)
	if err != nil {
		return "", err
	}

	confArr := strings.Split(kubeletConfPath, "=")
	if len(confArr) != 2 {
		return "", fmt.Errorf("get kubeletConfPath format err:%s", kubeletConfPath)
	}
	kubeletConfPath = confArr[1]
	apiserverAddr, err := enutil.GetSingleContentFromFile(kubeletConfPath, apiserverAddrRegularExpression)
	if err != nil {
		return "", err
	}

	addrArr := strings.Split(apiserverAddr, " ")
	if len(addrArr) != 2 {
		return "", fmt.Errorf("get apiserverAddr format err:%s", apiserverAddr)
	}
	apiserverAddr = addrArr[1]
	return apiserverAddr, nil
}
