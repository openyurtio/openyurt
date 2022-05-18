/*
Copyright 2018 The Kubernetes Authors.
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

package kubelet

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/yurtadm/cmd/join/joindata"
	"github.com/openyurtio/openyurt/pkg/yurtadm/kubernetes/kubeadm/app/constants"
	kubeadmutil "github.com/openyurtio/openyurt/pkg/yurtadm/kubernetes/kubeadm/app/util"
)

// WriteKubeletDynamicEnvFile writes an environment file with dynamic flags to the kubelet.
// Used at "kubeadm init" and "kubeadm join" time.
func WriteKubeletDynamicEnvFile(data joindata.YurtJoinData, kubeletDir string) error {
	stringMap := buildKubeletArgMap(data)
	argList := kubeadmutil.BuildArgumentListFromMap(stringMap, map[string]string{})
	envFileContent := fmt.Sprintf("%s=%q\n", constants.KubeletEnvFileVariableName, strings.Join(argList, " "))

	return writeKubeletFlagBytesToDisk([]byte(envFileContent), kubeletDir)
}

//buildKubeletArgMapCommon takes a kubeletFlagsOpts object and builds based on that a string-string map with flags
//that are common to both Linux and Windows
func buildKubeletArgMapCommon(data joindata.YurtJoinData) map[string]string {
	kubeletFlags := map[string]string{}

	nodeReg := data.NodeRegistration()
	if nodeReg.CRISocket == constants.DefaultDockerCRISocket {
		// These flags should only be set when running docker
		kubeletFlags["network-plugin"] = "cni"
		if data.PauseImage() != "" {
			kubeletFlags["pod-infra-container-image"] = data.PauseImage()
		}
	} else {
		kubeletFlags["container-runtime"] = "remote"
		kubeletFlags["container-runtime-endpoint"] = nodeReg.CRISocket
	}

	hostname, err := os.Hostname()
	if err != nil {
		klog.Warning(err)
	}
	if nodeReg.Name != hostname {
		klog.V(1).Infof("setting kubelet hostname-override to %q", nodeReg.Name)
		kubeletFlags["hostname-override"] = nodeReg.Name
	}

	kubeletFlags["node-labels"] = constructNodeLabels(data.NodeLabels(), nodeReg.WorkingMode, projectinfo.GetEdgeWorkerLabelKey())

	kubeletFlags["rotate-certificates"] = "false"

	return kubeletFlags
}

// constructNodeLabels make up node labels string
func constructNodeLabels(nodeLabels map[string]string, workingMode, edgeWorkerLabel string) string {
	if nodeLabels == nil {
		nodeLabels = make(map[string]string)
	}
	if _, ok := nodeLabels[edgeWorkerLabel]; !ok {
		if workingMode == "cloud" {
			nodeLabels[edgeWorkerLabel] = "false"
		} else {
			nodeLabels[edgeWorkerLabel] = "true"
		}
	}
	var labelsStr string
	for k, v := range nodeLabels {
		if len(labelsStr) == 0 {
			labelsStr = fmt.Sprintf("%s=%s", k, v)
		} else {
			labelsStr = fmt.Sprintf("%s,%s=%s", labelsStr, k, v)
		}
	}

	return labelsStr
}

// writeKubeletFlagBytesToDisk writes a byte slice down to disk at the specific location of the kubelet flag overrides file
func writeKubeletFlagBytesToDisk(b []byte, kubeletDir string) error {
	kubeletEnvFilePath := filepath.Join(kubeletDir, constants.KubeletEnvFileName)
	fmt.Printf("[kubelet-start] Writing kubelet environment file with flags to file %q\n", kubeletEnvFilePath)

	// creates target folder if not already exists
	if err := os.MkdirAll(kubeletDir, 0700); err != nil {
		return errors.Wrapf(err, "failed to create directory %q", kubeletDir)
	}
	if err := os.WriteFile(kubeletEnvFilePath, b, 0644); err != nil {
		return errors.Wrapf(err, "failed to write kubelet configuration to the file %q", kubeletEnvFilePath)
	}
	return nil
}
