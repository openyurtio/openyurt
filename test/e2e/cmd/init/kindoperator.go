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

package init

import (
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"k8s.io/klog/v2"

	strutil "github.com/openyurtio/openyurt/pkg/util/strings"
)

var (
	// enableGO111MODULE will be used when 1.13 <= go version <= 1.16
	enableGO111MODULE = "GO111MODULE=on"
	kindInstallCmd    = "go install sigs.k8s.io/kind@%s"

	defaultKubeConfigPath = "${HOME}/.kube/config"
	defaultKindVersion    = "v0.12.0"
)

type KindOperator struct {
	// kubeconfig represents where to store the kubeconfig of the new created cluster.
	kindCMDPath    string
	kubeconfigPath string

	// KindOperator will use this function to get command.
	// For the convenience of stub in unit tests.
	execCommand func(string, ...string) *exec.Cmd
}

func NewKindOperator(kindCMDPath string, kubeconfigPath string) *KindOperator {
	path := defaultKubeConfigPath
	if kubeconfigPath != "" {
		path = kubeconfigPath
	}
	return &KindOperator{
		kubeconfigPath: path,
		kindCMDPath:    kindCMDPath,
		execCommand:    exec.Command,
	}
}

func (k *KindOperator) SetExecCommand(execCommand func(string, ...string) *exec.Cmd) {
	k.execCommand = execCommand
}

func (k *KindOperator) KindVersion() (string, error) {
	b, err := k.execCommand(k.kindCMDPath, "version").CombinedOutput()
	if err != nil {
		return "", err
	}
	klog.V(1).Infof("get kind version %s", b)
	info := strings.Split(string(b), " ")
	// the output of "kind version" is like:
	// kind v0.11.1 go1.17.7 linux/amd64
	ver := info[1]
	return ver, nil
}

func (k *KindOperator) KindLoadDockerImage(out io.Writer, clusterName, image string, nodeNames []string) error {
	nodeArgs := strings.Join(nodeNames, ",")
	klog.V(1).Infof("load image %s to nodes %s in cluster %s", image, nodeArgs, clusterName)
	cmd := k.execCommand(k.kindCMDPath, "load", "docker-image", image, "--name", clusterName, "--nodes", nodeArgs)
	if out != nil {
		cmd.Stdout = out
		cmd.Stderr = out
	}
	if err := cmd.Run(); err != nil {
		klog.Errorf("failed to load docker image %s to nodes %s in cluster %s, %v", image, nodeArgs, clusterName, err)
		return err
	}
	return nil
}

func (k *KindOperator) KindCreateClusterWithConfig(out io.Writer, configPath string) error {
	cmd := k.execCommand(k.kindCMDPath, "create", "cluster", "--config", configPath, "--kubeconfig", k.kubeconfigPath)
	if out != nil {
		cmd.Stdout = out
		cmd.Stderr = out
	}
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

// KindInstall will install kind of default version.
// If KindOperator has got kindCMDPath, it will do nothing.
func (k *KindOperator) KindInstall() error {
	if k.kindCMDPath != "" {
		return nil
	}

	kindPath, err := findKindPath()
	if err != nil {
		klog.Infof("no kind tool is found, so try to install. %v", err)
	} else {
		k.kindCMDPath = kindPath
		return nil
	}

	minorVer, err := k.goMinorVersion()
	if err != nil {
		return fmt.Errorf("failed to get go minor version, %w", err)
	}
	installCMD := k.getInstallCmd(minorVer, defaultKindVersion)
	klog.V(1).Infof("start to install kind, running command: %s", installCMD)
	if err := k.execCommand("bash", "-c", installCMD).Run(); err != nil {
		return err
	}

	kindPath, err = findKindPath()
	if err != nil {
		return err
	}
	k.kindCMDPath = kindPath

	return nil
}

func (k *KindOperator) getInstallCmd(minorVer int, kindVersion string) string {
	installCMD := fmt.Sprintf(kindInstallCmd, kindVersion)
	if minorVer <= 16 {
		installCMD = strings.Join([]string{enableGO111MODULE, installCMD}, " ")
	}
	return installCMD
}

func (k *KindOperator) goMinorVersion() (int, error) {
	goverInfo, err := k.execCommand("go", "version").CombinedOutput()
	if err != nil {
		return 0, err
	}
	klog.V(1).Infof("get go version: %v", goverInfo)

	// the output of go version is like:
	// go version go1.17.7 linux/amd64
	gover := strings.Split(string(goverInfo), " ")[2]
	minorVer, err := strconv.Atoi(strings.Split(gover[2:], ".")[1])
	if err != nil {
		return 0, err
	}
	return minorVer, nil
}

func getGoBinPath() (string, error) {
	buf, err := exec.Command("bash", "-c", "go env GOPATH").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get GOPATH, %w", err)
	}
	gopath := strings.TrimSuffix(string(buf), "\n")
	return filepath.Join(gopath, "bin"), nil
}

func checkIfKindAt(path string) (bool, string) {
	if p, err := exec.LookPath(path); err == nil {
		return true, p
	}
	return false, ""
}

func findKindPath() (string, error) {

	var kindPath string
	if exist, path := checkIfKindAt("kind"); exist {
		kindPath = path
	} else {
		goBinPath, err := getGoBinPath()
		if err != nil {
			klog.Fatal("failed to get go bin path, %s", err)
		}

		if exist, path := checkIfKindAt(goBinPath + "/kind"); exist {
			kindPath = path
		}
	}

	if len(kindPath) == 0 {
		return kindPath, fmt.Errorf("cannot find valid kind cmd, try to install it")
	}

	if err := validateKindVersion(kindPath); err != nil {
		return "", err
	}
	return kindPath, nil
}

func validateKindVersion(kindCmdPath string) error {
	tmpOperator := NewKindOperator(kindCmdPath, "")
	ver, err := tmpOperator.KindVersion()
	if err != nil {
		return err
	}
	if !strutil.IsInStringLst(validKindVersions, ver) {
		return fmt.Errorf("invalid kind version: %s, all valid kind versions are: %s",
			ver, strings.Join(validKindVersions, ","))
	}
	return nil
}
