/*
Copyright 2020 The OpenYurt Authors.

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
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	pkgerrors "github.com/pkg/errors"
	batchv1 "k8s.io/api/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/klog/v2"

	kubeadmconstants "github.com/openyurtio/openyurt/pkg/util/kubernetes/kubeadm/app/constants"
	"github.com/openyurtio/openyurt/pkg/util/kubernetes/kubeadm/app/util/apiclient"
	kubeconfigutil "github.com/openyurtio/openyurt/pkg/util/kubernetes/kubeadm/app/util/kubeconfig"
	"github.com/openyurtio/openyurt/pkg/yurtadm/constants"
	"github.com/openyurtio/openyurt/pkg/yurtadm/util"
	"github.com/openyurtio/openyurt/pkg/yurtadm/util/edgenode"
)

const (
	// DisableNodeControllerJobNameBase is the prefix of the DisableNodeControllerJob name
	DisableNodeControllerJobNameBase = "yurtctl-disable-node-controller"
)

var (
	// PropagationPolicy defines the propagation policy used when deleting a resource
	PropagationPolicy = metav1.DeletePropagationBackground

	ErrClusterVersionEmpty = errors.New("cluster version should not be empty")
)

// RunJobAndCleanup runs the job, wait for it to be complete, and delete it
func RunJobAndCleanup(cliSet *kubernetes.Clientset, job *batchv1.Job, timeout, period time.Duration, waitForTimeout bool) error {
	job, err := cliSet.BatchV1().Jobs(job.GetNamespace()).Create(context.Background(), job, metav1.CreateOptions{})
	if err != nil {
		return err
	}
	waitJobTimeout := time.After(timeout)
	for {
		select {
		case <-waitJobTimeout:
			return errors.New("wait for job to be complete timeout")
		case <-time.After(period):
			newJob, err := cliSet.BatchV1().Jobs(job.GetNamespace()).
				Get(context.Background(), job.GetName(), metav1.GetOptions{})
			if err != nil {
				if apierrors.IsNotFound(err) {
					return err
				}

				if waitForTimeout {
					klog.Infof("continue to wait for job(%s) to complete until timeout, even if failed to get job, %v", job.GetName(), err)
					continue
				}
				return err
			}

			if newJob.Status.Succeeded == *newJob.Spec.Completions {
				if err := cliSet.BatchV1().Jobs(job.GetNamespace()).
					Delete(context.Background(), job.GetName(), metav1.DeleteOptions{
						PropagationPolicy: &PropagationPolicy,
					}); err != nil {
					klog.Errorf("fail to delete succeeded servant job(%s): %s", job.GetName(), err)
					return err
				}
				return nil
			}
		}
	}
}

//CheckAndInstallKubelet install kubelet and kubernetes-cni, skip install if they exist.
func CheckAndInstallKubelet(kubernetesResourceServer, clusterVersion string) error {
	if strings.Contains(clusterVersion, "-") {
		clusterVersion = strings.Split(clusterVersion, "-")[0]
	}

	klog.Infof("Check and install kubelet %s", clusterVersion)
	if clusterVersion == "" {
		return ErrClusterVersionEmpty
	}
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
		packageUrl := fmt.Sprintf(constants.KubeUrlFormat, kubernetesResourceServer, clusterVersion, runtime.GOARCH)
		savePath := fmt.Sprintf("%s/kubernetes-node-linux-%s.tar.gz", constants.TmpDownloadDir, runtime.GOARCH)
		klog.V(1).Infof("Download kubelet from: %s", packageUrl)
		if err := util.DownloadFile(packageUrl, savePath, 3); err != nil {
			return fmt.Errorf("Download kuelet fail: %w", err)
		}
		if err := util.Untar(savePath, constants.TmpDownloadDir); err != nil {
			return err
		}
		for _, comp := range []string{"kubectl", "kubeadm", "kubelet"} {
			target := fmt.Sprintf("/usr/bin/%s", comp)
			if err := edgenode.CopyFile(constants.TmpDownloadDir+"/kubernetes/node/bin/"+comp, target, constants.DirMode); err != nil {
				return err
			}
		}
	}

	if _, err := os.Stat(constants.KubeCniDir); err == nil {
		klog.Infof("Cni dir %s already exist, skip install.", constants.KubeCniDir)
		return nil
	}
	//download and install kubernetes-cni
	cniUrl := fmt.Sprintf(constants.CniUrlFormat, constants.KubeCniVersion, runtime.GOARCH, constants.KubeCniVersion)
	savePath := fmt.Sprintf("%s/cni-plugins-linux-%s-%s.tgz", constants.TmpDownloadDir, runtime.GOARCH, constants.KubeCniVersion)
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

// SetKubeletService configure kubelet service.
func SetKubeletService() error {
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
	if err := os.WriteFile(constants.KubeletServiceFilepath, []byte(constants.KubeletServiceContent), 0644); err != nil {
		klog.Errorf("Write file %s fail: %v", constants.KubeletServiceFilepath, err)
		return err
	}
	return nil
}

// SetKubeletUnitConfig configure kubelet startup parameters.
func SetKubeletUnitConfig() error {
	kubeletUnitDir := filepath.Dir(constants.KubeletServiceConfPath)
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

	if err := os.WriteFile(constants.KubeletServiceConfPath, []byte(constants.KubeletUnitConfig), 0600); err != nil {
		return err
	}

	return nil
}

// SetKubeletConfigForNode write kubelet.conf for join node.
func SetKubeletConfigForNode() error {
	kubeconfigFilePath := filepath.Join(kubeadmconstants.KubernetesDir, kubeadmconstants.KubeletKubeConfigFileName)
	kubeletConfigDir := filepath.Dir(kubeconfigFilePath)
	if _, err := os.Stat(kubeletConfigDir); err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(kubeletConfigDir, os.ModePerm); err != nil {
				klog.Errorf("Create dir %s fail: %v", kubeletConfigDir, err)
				return err
			}
		} else {
			klog.Errorf("Describe dir %s fail: %v", kubeletConfigDir, err)
			return err
		}
	}
	if err := os.WriteFile(kubeconfigFilePath, []byte(constants.KubeletConfForNode), constants.DirMode); err != nil {
		return err
	}
	return nil
}

// SetKubeletCaCert write ca.crt for join node.
func SetKubeletCaCert(config *clientcmdapi.Config) error {
	kubeletCaCertPath := filepath.Join(kubeadmconstants.KubernetesDir, "pki", kubeadmconstants.CACertName)
	kubeletCaCertDir := filepath.Dir(kubeletCaCertPath)
	if _, err := os.Stat(kubeletCaCertDir); err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(kubeletCaCertDir, os.ModePerm); err != nil {
				klog.Errorf("Create dir %s fail: %v", kubeletCaCertDir, err)
				return err
			}
		} else {
			klog.Errorf("Describe dir %s fail: %v", kubeletCaCertDir, err)
			return err
		}
	}

	clusterinfo := kubeconfigutil.GetClusterFromKubeConfig(config)
	if err := os.WriteFile(kubeletCaCertPath, []byte(clusterinfo.CertificateAuthorityData), constants.DirMode); err != nil {
		return err
	}
	return nil
}

// GetKubernetesVersionFromCluster get kubernetes cluster version from master.
func GetKubernetesVersionFromCluster(client kubernetes.Interface) (string, error) {
	var kubernetesVersion string
	// Also, the config map really should be KubeadmConfigConfigMap...
	configMap, err := apiclient.GetConfigMapWithRetry(client, metav1.NamespaceSystem, kubeadmconstants.KubeadmConfigConfigMap)
	if err != nil {
		return kubernetesVersion, pkgerrors.Wrap(err, "failed to get config map")
	}

	// gets ClusterConfiguration from kubeadm-config
	clusterConfigurationData, ok := configMap.Data[kubeadmconstants.ClusterConfigurationConfigMapKey]
	if !ok {
		return kubernetesVersion, pkgerrors.Errorf("unexpected error when reading kubeadm-config ConfigMap: %s key value pair missing", kubeadmconstants.ClusterConfigurationConfigMapKey)
	}

	scanner := bufio.NewScanner(strings.NewReader(clusterConfigurationData))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, ":")
		if len(parts) != 2 {
			continue
		}

		if strings.Contains(parts[0], "kubernetesVersion") {
			kubernetesVersion = strings.TrimSpace(parts[1])
			break
		}
	}

	if len(kubernetesVersion) == 0 {
		return kubernetesVersion, errors.New("failed to get Kubernetes version")
	}

	klog.Infof("kubernetes version: %s", kubernetesVersion)
	return kubernetesVersion, nil
}
