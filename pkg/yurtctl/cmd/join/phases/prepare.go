package phases

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

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"k8s.io/klog"
	"k8s.io/kubernetes/cmd/kubeadm/app/cmd/phases/workflow"

	selinux "github.com/opencontainers/selinux/go-selinux"
	"github.com/openyurtio/openyurt/pkg/yurtctl/constants"
	"github.com/openyurtio/openyurt/pkg/yurtctl/util"
	"github.com/openyurtio/openyurt/pkg/yurtctl/util/edgenode"
)

// NewEdgeNodePhase creates a yurtctl workflow phase that initialize the node environment.
func NewPreparePhase() workflow.Phase {
	return workflow.Phase{
		Name:  "Initialize system environment.",
		Short: "Initialize system environment.",
		Run:   runPrepare,
	}
}

//runPrepare executes the node initialization process.
func runPrepare(c workflow.RunData) error {
	data, ok := c.(YurtJoinData)
	if !ok {
		return fmt.Errorf("Prepare phase invoked with an invalid data struct. ")
	}

	initCfg, err := data.InitCfg()
	if err != nil {
		return err
	}

	if err := setIpv4Forward(); err != nil {
		return err
	}
	if err := setBridgeSetting(); err != nil {
		return err
	}
	if err := setSELinux(); err != nil {
		return err
	}
	if err := checkAndInstallKubelet(initCfg.ClusterConfiguration.KubernetesVersion); err != nil {
		return err
	}
	if err := setKubeletService(); err != nil {
		return err
	}
	if err := setKubeletUnitConfig(data.NodeType()); err != nil {
		return err
	}
	return nil
}

//setIpv4Forward turn on the node ipv4 forward.
func setIpv4Forward() error {
	klog.Infof("Setting ipv4 forward")
	if err := ioutil.WriteFile(ip_forward, []byte("1"), 0644); err != nil {
		return fmt.Errorf("Write content 1 to file %s fail: %v ", ip_forward, err)
	}
	return nil
}

//setBridgeSetting turn on the node bridge-nf-call-iptables.
func setBridgeSetting() error {
	klog.Info("Setting bridge settings for kubernetes.")
	if err := ioutil.WriteFile(constants.Sysctl_k8s_config, []byte(kubernetsBridgeSetting), 0644); err != nil {
		return fmt.Errorf("Write file %s fail: %v ", constants.Sysctl_k8s_config, err)
	}
	if err := ioutil.WriteFile(bridgenf, []byte("1"), 0644); err != nil {
		return fmt.Errorf("Write file %s fail: %v ", bridgenf, err)
	}
	if err := ioutil.WriteFile(bridgenf6, []byte("1"), 0644); err != nil {
		return fmt.Errorf("Write file %s fail: %v ", bridgenf, err)
	}
	return nil
}

// setSELinux turn off the node selinux.
func setSELinux() error {
	klog.Info("Disabling SELinux.")
	selinux.SetDisabled()
	return nil
}

//checkAndInstallKubelet install kubelet and kubernetes-cni, skip install if they exist.
func checkAndInstallKubelet(clusterVersion string) error {
	klog.Info("Check and install kubelet.")
	kubeletExist := false
	if _, err := exec.LookPath("kubelet"); err == nil {
		if b, err := exec.Command("kubelet", "--version").CombinedOutput(); err == nil {
			kubeletVersion := strings.Split(string(b), " ")[1]
			kubeletVersion = strings.TrimSpace(kubeletVersion)
			klog.Infof("kubelet --version: %s", kubeletVersion)
			if strings.Contains(string(b), clusterVersion) {
				klog.Infof("Kubelet %s already exist, skip install.", clusterVersion)
				kubeletExist = true
			} else {
				return fmt.Errorf("The existing kubelet version %s of the node is inconsistent with cluster version %s, please clean it. ", kubeletVersion, clusterVersion)
			}
		}
	}

	if !kubeletExist {
		//download and install kubernetes-node
		packageUrl := fmt.Sprintf(kubeUrlFormat, clusterVersion, runtime.GOARCH)
		savePath := fmt.Sprintf("%s/kubernetes-node-linux-%s.tar.gz", tmpDownloadDir, runtime.GOARCH)
		klog.V(1).Infof("Download kubelet from: %s", packageUrl)
		if err := util.DownloadFile(packageUrl, savePath, 3); err != nil {
			return fmt.Errorf("Download kuelet fail: %v", err)
		}
		if err := util.Untar(savePath, tmpDownloadDir); err != nil {
			return err
		}
		for _, comp := range []string{"kubectl", "kubeadm", "kubelet"} {
			target := fmt.Sprintf("/usr/bin/%s", comp)
			if err := edgenode.CopyFile(tmpDownloadDir+"/kubernetes/node/bin/"+comp, target, 0755); err != nil {
				return err
			}
		}
	}
	if _, err := os.Stat(constants.StaticPodPath); os.IsNotExist(err) {
		if err := os.MkdirAll(constants.StaticPodPath, 0755); err != nil {
			return err
		}
	}

	if _, err := os.Stat(constants.KubeCniDir); err == nil {
		klog.Infof("Cni dir %s already exist, skip install.", constants.KubeCniDir)
		return nil
	}
	//download and install kubernetes-cni
	cniUrl := fmt.Sprintf(cniUrlFormat, constants.KubeCniVersion, runtime.GOARCH, constants.KubeCniVersion)
	savePath := fmt.Sprintf("%s/cni-plugins-linux-%s-%s.tgz", tmpDownloadDir, runtime.GOARCH, constants.KubeCniVersion)
	klog.V(1).Infof("Download cni from: %s", cniUrl)
	if err := util.DownloadFile(cniUrl, savePath, 3); err != nil {
		return err
	}

	if err := os.MkdirAll(constants.KubeCniDir, 0600); err != nil {
		return err
	}
	if err := util.Untar(savePath, constants.KubeCniDir); err != nil {
		return err
	}
	return nil
}

// setKubeletService configure kubelet service.
func setKubeletService() error {
	klog.Info("Setting kubelet service.")
	kubeletServiceDir := filepath.Dir(constants.KubeletServiceFilepath)
	if _, err := os.Stat(kubeletServiceDir); err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(kubeletServiceDir, os.ModePerm); err != nil {
				klog.Errorf("Create dir %s fail: %v", kubeletServiceDir, err)
				return err
			}
		} else {
			klog.Errorf("Describe dir %s fail: %v", kubeletServiceDir, err)
			return err
		}
	}
	if err := ioutil.WriteFile(constants.KubeletServiceFilepath, []byte(kubeletServiceContent), 0644); err != nil {
		klog.Errorf("Write file %s fail: %v", constants.KubeletServiceFilepath, err)
		return err
	}
	return nil
}

//setKubeletUnitConfig configure kubelet startup parameters.
func setKubeletUnitConfig(nodeType string) error {
	kubeletUnitDir := filepath.Dir(edgenode.KubeletSvcPath)
	if _, err := os.Stat(kubeletUnitDir); err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(kubeletUnitDir, os.ModePerm); err != nil {
				klog.Errorf("Create dir %s fail: %v", kubeletUnitDir, err)
				return err
			}
		} else {
			klog.Errorf("Describe dir %s fail: %v", kubeletUnitDir, err)
			return err
		}
	}
	if nodeType == EdgeNode {
		if err := ioutil.WriteFile(edgenode.KubeletSvcPath, []byte(edgeKubeletUnitConfig), 0600); err != nil {
			return err
		}
	} else {
		if err := ioutil.WriteFile(edgenode.KubeletSvcPath, []byte(cloudKubeletUnitConfig), 0600); err != nil {
			return err
		}
	}

	return nil
}
