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
	"path/filepath"
	"strings"

	"github.com/openyurtio/openyurt/pkg/yurtadm/constants"
	enutil "github.com/openyurtio/openyurt/pkg/yurtadm/util/edgenode"
	kubeletutil "github.com/openyurtio/openyurt/pkg/yurtadm/util/kubernetes"
)

const (
	kubeletConfigRegularExpression = "\\-\\-kubeconfig=.*kubelet.conf"
	apiserverAddrRegularExpression = "server: (http(s)?:\\/\\/)?[\\w][-\\w]{0,62}(\\.[\\w][-\\w]{0,62})*(:[\\d]{1,5})?"
)

type kubeletOperator struct {
	openyurtDir          string
	kubeadmFlagsFilePath string
}

// NewKubeletOperator create kubeletOperator
func NewKubeletOperator(openyurtDir string) *kubeletOperator {
	return &kubeletOperator{
		openyurtDir:          openyurtDir,
		kubeadmFlagsFilePath: constants.KubeadmFlagsEnvFilePath,
	}
}

type kubeletTrafficManager interface {
	RedirectTrafficToYurthub() error
	UndoRedirectTrafficToYurthub() error
}

var newKubeletTrafficManager = func(kubeconfigFilePath, kubeadmFlagsFilePath string) kubeletTrafficManager {
	return &kubeletutil.KubeletYurthubConfig{
		KubeconfigFilePath:   kubeconfigFilePath,
		KubeadmFlagsFilePath: kubeadmFlagsFilePath,
		FileMode:             fileMode,
	}
}

// RedirectTrafficToYurtHub
// add env config leads kubelet to visit yurtHub as apiServer
func (op *kubeletOperator) RedirectTrafficToYurtHub() error {
	return op.trafficManager().RedirectTrafficToYurthub()
}

// UndoRedirectTrafficToYurtHub
// undo what's done to kubelet and restart
// to compatible the old convert way for a while , so do renameSvcBk
func (op *kubeletOperator) UndoRedirectTrafficToYurtHub() error {
	return op.trafficManager().UndoRedirectTrafficToYurthub()
}

func (op *kubeletOperator) getYurthubKubeletConf() string {
	return filepath.Join(op.openyurtDir, constants.KubeletKubeConfigFileName)
}

func (op *kubeletOperator) trafficManager() kubeletTrafficManager {
	return newKubeletTrafficManager(op.getYurthubKubeletConf(), op.kubeadmFlagsFilePath)
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
