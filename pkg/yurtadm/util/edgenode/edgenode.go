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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/node-servant/static-pod-upgrade/util"
	"github.com/openyurtio/openyurt/pkg/yurtadm/constants"
)

const (
	NODE_NAME     = "NODE_NAME"
	NodeNameSplit = "="
)

// FileExists determines whether the file exists
func FileExists(filename string) (bool, error) {
	if _, err := os.Stat(filename); os.IsExist(err) {
		return true, err
	} else if err != nil {
		return false, err
	}
	return true, nil
}

// GetContentFormFile returns all strings that match the regular expression regularExpression
func GetContentFormFile(filename string, regularExpression string) ([]string, error) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	ct := string(content)
	reg := regexp.MustCompile(regularExpression)
	res := reg.FindAllString(ct, -1)
	return res, nil
}

// GetSingleContentFromFile determines whether there is a unique string that matches the
// regular expression regularExpression and returns it
func GetSingleContentFromFile(filename string, regularExpression string) (string, error) {
	contents, err := GetContentFormFile(filename, regularExpression)
	if err != nil {
		return "", fmt.Errorf("could not read file %s, %w", filename, err)
	}
	if contents == nil {
		return "", fmt.Errorf("no matching string %s in file %s", regularExpression, filename)
	}
	if len(contents) > 1 {
		return "", fmt.Errorf("there are multiple matching string %s in file %s", regularExpression, filename)
	}
	return contents[0], nil
}

// EnsureDir make sure dir is exists, if not create
func EnsureDir(dirname string) error {
	s, err := os.Stat(dirname)
	if err == nil && s.IsDir() {
		return nil
	}

	return os.MkdirAll(dirname, 0755)
}

// CopyFile copys sourceFile to destinationFile
func CopyFile(sourceFile string, destinationFile string, perm os.FileMode) error {
	content, err := os.ReadFile(sourceFile)
	if err != nil {
		return fmt.Errorf("could not read source file %s: %w", sourceFile, err)
	}
	err = os.WriteFile(destinationFile, content, perm)
	if err != nil {
		return fmt.Errorf("could not write destination file %s: %w", destinationFile, err)
	}
	return nil
}

// GetNodeName gets the node name based on environment variable, parameters --hostname-override
// in the configuration file or hostname
func GetNodeName(kubeadmConfPath string) (string, error) {
	//1. from env NODE_NAME
	nodename := os.Getenv(NODE_NAME)
	if nodename != "" {
		return nodename, nil
	}

	//2. find --hostname-override in 10-kubeadm.conf
	nodeName, err := GetSingleContentFromFile(kubeadmConfPath, constants.KubeletHostname)
	if nodeName != "" {
		nodeName = strings.Split(nodeName, NodeNameSplit)[1]
		return nodeName, nil
	} else {
		klog.V(4).Info("get nodename err: ", err)
	}

	//3. find --hostname-override in EnvironmentFile
	environmentFiles, err := GetContentFormFile(kubeadmConfPath, constants.KubeletEnvironmentFile)
	if err != nil {
		return "", err
	}
	for _, ef := range environmentFiles {
		ef = strings.Split(ef, "-")[1]
		nodeName, err = GetSingleContentFromFile(ef, constants.KubeletHostname)
		if nodeName != "" {
			nodeName = strings.Split(nodeName, NodeNameSplit)[1]
			return nodeName, nil
		} else {
			klog.V(4).Info("get nodename err: ", err)
		}
	}

	//4. read nodeName from /etc/hostname
	content, err := os.ReadFile(constants.Hostname)
	if err != nil {
		return "", err
	}
	nodeName = strings.Trim(string(content), "\n")
	return nodeName, nil
}

// GetHostname returns OS's hostname if 'hostnameOverride' is empty; otherwise, return 'hostnameOverride'.
// This is compatible with kubelet's hostname processing logic
// https://github.com/kubernetes/kubernetes/blob/c00975370a5bf81328dc56396ee05edc7306e238/pkg/util/node/node.go#L46
var osHostName func() (string, error) = os.Hostname

func GetHostname(hostnameOverride string) (string, error) {
	hostName := hostnameOverride
	if len(hostName) == 0 {
		nodeName, err := osHostName()
		if err != nil {
			return "", fmt.Errorf("couldn't determine hostname: %v", err)
		}
		hostName = nodeName
	}

	// Trim whitespaces first to avoid getting an empty hostname
	// For linux, the hostname is read from file /proc/sys/kernel/hostname directly
	hostName = strings.TrimSpace(hostName)
	if len(hostName) == 0 {
		return "", fmt.Errorf("empty hostname is invalid")
	}
	return strings.ToLower(hostName), nil
}

// Exec execs the command
func Exec(cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	if err := cmd.Wait(); err != nil {
		return err
	}
	return nil
}

// GetPodManifestPath return podManifestPath, use default value of kubeadm/minikube/kind. etc.
func GetPodManifestPath() string {
	return constants.StaticPodPath // /etc/kubernetes/manifests
}

func DeployStaticYaml(manifestList, templateList []string, podManifestPath string) error {
	klog.Info("Deploying user edge static yaml")
	if _, err := os.Stat(podManifestPath); err != nil {
		if os.IsNotExist(err) {
			err = os.MkdirAll(podManifestPath, os.ModePerm)
			if err != nil {
				return err
			}
		} else {
			klog.Errorf("Describe dir %s fail: %v", podManifestPath, err)
			return err
		}
	}

	for i, template := range templateList {
		manifestFile := filepath.Join(podManifestPath, util.WithYamlSuffix(manifestList[i]))
		klog.Infof("static pod template: %s\n%s", manifestFile, template)
		if err := os.WriteFile(manifestFile, []byte(template), 0600); err != nil {
			return err
		}
	}

	klog.Info("Deploy user edge static yaml is ok")
	return nil
}

func RemoveStaticYaml(manifestList []string, podManifestPath string) error {
	klog.Info("Removing user edge static yaml")
	if _, err := os.Stat(podManifestPath); err != nil {
		if os.IsNotExist(err) {
			klog.Errorf("Describe dir %s fail: %v", podManifestPath, err)
			return err
		}
	}

	for _, manifest := range manifestList {
		manifestFile := filepath.Join(podManifestPath, util.WithYamlSuffix(manifest))
		if err := os.Remove(manifestFile); err != nil {
			return err
		}
	}

	klog.Info("Remove user edge static yaml is ok")
	return nil
}
