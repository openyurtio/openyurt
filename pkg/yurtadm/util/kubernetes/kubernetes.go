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
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/hashicorp/go-version"
	pkgerrors "github.com/pkg/errors"
	batchv1 "k8s.io/api/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
	kubeconfigutil "github.com/openyurtio/openyurt/pkg/util/kubeconfig"
	"github.com/openyurtio/openyurt/pkg/util/kubernetes/kubeadm/app/util/apiclient"
	"github.com/openyurtio/openyurt/pkg/util/templates"
	"github.com/openyurtio/openyurt/pkg/util/token"
	"github.com/openyurtio/openyurt/pkg/yurtadm/cmd/join/joindata"
	"github.com/openyurtio/openyurt/pkg/yurtadm/constants"
	"github.com/openyurtio/openyurt/pkg/yurtadm/util"
	"github.com/openyurtio/openyurt/pkg/yurtadm/util/edgenode"
	"github.com/openyurtio/openyurt/pkg/yurtadm/util/initsystem"
)

const (
	TmpDownloadDir = "/tmp"
	// TokenUser defines token user
	TokenUser = "tls-bootstrap-token-user"
)

var (
	// PropagationPolicy defines the propagation policy used when deleting a resource
	PropagationPolicy = metav1.DeletePropagationBackground

	ErrClusterVersionEmpty = errors.New("cluster version should not be empty")

	// BootstrapTokenRegexp is a compiled regular expression of TokenRegexpString
	BootstrapTokenRegexp = regexp.MustCompile(constants.BootstrapTokenPattern)
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
					klog.Infof("continue to wait for job(%s) to complete until timeout, even if could not get job, %v", job.GetName(), err)
					continue
				}
				return err
			}

			if newJob.Status.Succeeded == *newJob.Spec.Completions {
				if err := cliSet.BatchV1().Jobs(job.GetNamespace()).
					Delete(context.Background(), job.GetName(), metav1.DeleteOptions{
						PropagationPolicy: &PropagationPolicy,
					}); err != nil {
					klog.Errorf("could not delete succeeded servant job(%s): %s", job.GetName(), err)
					return err
				}
				return nil
			}
		}
	}
}

// CheckAndInstallKubelet install kubelet and kubernetes-cni, skip install if they exist.
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
			v1, err := version.NewVersion(kubeletVersion)
			if err != nil {
				return err
			}
			v2, err := version.NewVersion(clusterVersion)
			if err != nil {
				return err
			}
			s1 := v1.Segments()
			s2 := v2.Segments()
			if s1[0] == s2[0] && s1[1] == s2[1] {
				klog.Infof("Kubelet %s already exist, skip install.", clusterVersion)
				kubeletExist = true
			} else {
				return fmt.Errorf("The existing kubelet version %s of the node is inconsistent with cluster version %s, please clean it. ", kubeletVersion, clusterVersion)
			}
		}
	}

	if !kubeletExist {
		//download and install kubelet
		packageUrl := fmt.Sprintf(constants.KubeletUrlFormat, kubernetesResourceServer, clusterVersion, runtime.GOARCH)
		savePath := fmt.Sprintf("%s/kubelet", constants.TmpDownloadDir)
		klog.V(1).Infof("Download kubelet from: %s", packageUrl)
		if err := util.DownloadFile(packageUrl, savePath, 3); err != nil {
			return fmt.Errorf("download kubelet fail: %w", err)
		}
		if err := edgenode.CopyFile(savePath, "/usr/bin/kubelet", constants.DirMode); err != nil {
			return err
		}
	}

	return nil
}

// CheckAndInstallKubernetesCni install kubernetes-cni, skip install if they exist.
func CheckAndInstallKubernetesCni(reuseCNIBin bool) error {
	if reuseCNIBin {
		if _, err := os.Stat(constants.KubeCniDir); err == nil {
			klog.Infof("Cni dir %s exists, reuse the CNI binaries.", constants.KubeCniDir)
			return nil
		} else {
			klog.Errorf("Cni dir %s does not exist, cannot reuse the CNI binaries!", constants.KubeCniDir)
			return err
		}
	}

	//download and install kubernetes-cni
	cniUrl := fmt.Sprintf(constants.CniUrlFormat, constants.KubeCniVersion, runtime.GOARCH, constants.KubeCniVersion)
	savePath := fmt.Sprintf("%s/cni-plugins-linux-%s-%s.tgz", constants.TmpDownloadDir, runtime.GOARCH, constants.KubeCniVersion)
	if _, err := os.Stat(savePath); errors.Is(err, os.ErrNotExist) {
		klog.V(1).Infof("Download cni from: %s", cniUrl)
		if err := util.DownloadFile(cniUrl, savePath, 3); err != nil {
			return err
		}
	} else {
		klog.V(1).Infof("Skip download cni, use already exist file: %s", savePath)
	}

	if err := os.MkdirAll(constants.KubeCniDir, 0755); err != nil {
		return err
	}
	if err := util.Untar(savePath, constants.KubeCniDir); err != nil {
		return err
	}
	return nil
}

type kubeadmVersion struct {
	ClientVersion clientVersion `json:"clientVersion"`
}
type clientVersion struct {
	GitVersion string `json:"gitVersion"`
}

// CheckAndInstallKubeadm install kubeadm, skip install if it exist.
func CheckAndInstallKubeadm(kubernetesResourceServer, clusterVersion string) error {
	if strings.Contains(clusterVersion, "-") {
		clusterVersion = strings.Split(clusterVersion, "-")[0]
	}

	klog.Infof("Check and install kubeadm %s", clusterVersion)
	if clusterVersion == "" {
		return ErrClusterVersionEmpty
	}
	kubeadmExist := false
	if _, err := exec.LookPath("kubeadm"); err == nil {
		if b, err := exec.Command("kubeadm", "version", "-o", "json").CombinedOutput(); err == nil {
			klog.V(1).InfoS("kubeadm", "version", string(b))
			info := kubeadmVersion{}
			if err := json.Unmarshal(b, &info); err != nil {
				return fmt.Errorf("can't get the existing kubeadm version: %w", err)
			}
			kubeadmVersion := info.ClientVersion.GitVersion
			v1, err := version.NewVersion(kubeadmVersion)
			if err != nil {
				return err
			}
			v2, err := version.NewVersion(clusterVersion)
			if err != nil {
				return err
			}
			s1 := v1.Segments()
			s2 := v2.Segments()
			if s1[0] == s2[0] && s1[1] == s2[1] {
				klog.Infof("Kubeadm %s already exist, skip install.", clusterVersion)
				kubeadmExist = true
			} else {
				return fmt.Errorf("The existing kubeadm version %s of the node is inconsistent with cluster version %s, please clean it. ", kubeadmVersion, clusterVersion)
			}
		}
	}

	if !kubeadmExist {
		// download and install kubeadm
		packageUrl := fmt.Sprintf(constants.KubeadmUrlFormat, kubernetesResourceServer, clusterVersion, runtime.GOARCH)
		savePath := fmt.Sprintf("%s/kubeadm", TmpDownloadDir)
		klog.V(1).Infof("Download kubeadm from %s", packageUrl)
		if err := util.DownloadFile(packageUrl, savePath, 3); err != nil {
			return fmt.Errorf("download kubeadm fail: %w", err)
		}
		if err := edgenode.CopyFile(savePath, "/usr/bin/kubeadm", constants.DirMode); err != nil {
			return err
		}
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

// EnableKubeletService enable kubelet service
func EnableKubeletService() error {
	initSystem, err := initsystem.GetInitSystem()
	if err != nil {
		return err
	}

	if !initSystem.ServiceIsEnabled("kubelet") {
		if err = initSystem.ServiceEnable("kubelet"); err != nil {
			return fmt.Errorf("enable kubelet service failed")
		}
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

	if err := os.WriteFile(constants.KubeletServiceConfPath, []byte(constants.KubeletUnitConfig), 0640); err != nil {
		return err
	}

	return nil
}

// SetKubeletConfigForNode write kubelet.conf for join node.
func SetKubeletConfigForNode() error {
	kubeconfigFilePath := filepath.Join(constants.KubeletConfigureDir, constants.KubeletKubeConfigFileName)
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

// GetKubernetesVersionFromCluster get kubernetes cluster version from master.
func GetKubernetesVersionFromCluster(client kubernetes.Interface) (string, error) {
	var kubernetesVersion string
	// Also, the config map really should be KubeadmConfigConfigMap...
	configMap, err := apiclient.GetConfigMapWithRetry(client, metav1.NamespaceSystem, constants.KubeadmConfigConfigMap)
	if err != nil {
		return kubernetesVersion, pkgerrors.Wrap(err, "could not get config map")
	}

	// gets ClusterConfiguration from kubeadm-config
	clusterConfigurationData, ok := configMap.Data[constants.ClusterConfigurationConfigMapKey]
	if !ok {
		return kubernetesVersion, pkgerrors.Errorf("unexpected error when reading kubeadm-config ConfigMap: %s key value pair missing", constants.ClusterConfigurationConfigMapKey)
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
		return kubernetesVersion, errors.New("could not get Kubernetes version")
	}

	klog.Infof("kubernetes version: %s", kubernetesVersion)
	return kubernetesVersion, nil
}

func SetKubeadmJoinConfig(data joindata.YurtJoinData) error {
	kubeadmJoinConfigFilePath := filepath.Join(constants.KubeletWorkdir, constants.KubeadmJoinConfigFileName)
	kubeadmJoinConfigDir := filepath.Dir(kubeadmJoinConfigFilePath)
	if _, err := os.Stat(kubeadmJoinConfigDir); err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(kubeadmJoinConfigDir, os.ModePerm); err != nil {
				klog.Errorf("Create dir %s fail: %v", kubeadmJoinConfigDir, err)
				return err
			}
		} else {
			klog.Errorf("Describe dir %s fail: %v", kubeadmJoinConfigDir, err)
			return err
		}
	}

	nodeReg := data.NodeRegistration()
	KubeadmJoinDiscoveryFilePath := filepath.Join(constants.KubeletWorkdir, constants.KubeadmJoinDiscoveryFileName)
	ctx := map[string]interface{}{
		"kubeConfigPath":         KubeadmJoinDiscoveryFilePath,
		"tlsBootstrapToken":      data.JoinToken(),
		"ignorePreflightErrors":  data.IgnorePreflightErrors().List(),
		"podInfraContainerImage": data.PauseImage(),
		"nodeLabels":             constructNodeLabels(data.NodeLabels(), nodeReg.WorkingMode, projectinfo.GetEdgeWorkerLabelKey()),
		"criSocket":              nodeReg.CRISocket,
		"name":                   nodeReg.Name,
	}

	v1, err := version.NewVersion(data.KubernetesVersion())
	if err != nil {
		return err
	}

	if nodeReg.CRISocket == constants.DefaultDockerCRISocket {
		ctx["networkPlugin"] = "cni"
	} else {
		v124alpha, err := version.NewVersion("1.24.0-alpha.0")
		if err != nil {
			return err
		}
		if v1.LessThan(v124alpha) {
			ctx["containerRuntime"] = "remote"
		}
		ctx["containerRuntimeEndpoint"] = nodeReg.CRISocket
	}

	v2, err := version.NewVersion("v1.22.0")
	if err != nil {
		return err
	}
	// This is to adapt the apiVersion of JoinConfiguration
	// https://kubernetes.io/docs/reference/config-api/kubeadm-config.v1beta3/
	if v1.LessThan(v2) {
		ctx["apiVersion"] = "kubeadm.k8s.io/v1beta2"
	} else {
		ctx["apiVersion"] = "kubeadm.k8s.io/v1beta3"
	}

	kubeadmJoinTemplate, err := templates.SubsituteTemplate(constants.KubeadmJoinConf, ctx)
	if err != nil {
		return err
	}

	if err := os.WriteFile(kubeadmJoinConfigFilePath, []byte(kubeadmJoinTemplate), constants.DirMode); err != nil {
		return err
	}
	return nil
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

func SetDiscoveryConfig(data joindata.YurtJoinData) error {
	cfg, err := token.RetrieveValidatedConfigInfo(nil, &token.BootstrapData{
		ServerAddr:   data.ServerAddr(),
		JoinToken:    data.JoinToken(),
		CaCertHashes: data.CaCertHashes(),
	})
	if err != nil {
		return err
	}

	cluster := kubeconfigutil.GetClusterFromKubeConfig(cfg)
	cluster.Server = fmt.Sprintf("https://%s", strings.Split(data.ServerAddr(), ",")[0])

	discoveryConfigFilePath := filepath.Join(constants.KubeletWorkdir, constants.KubeadmJoinDiscoveryFileName)
	if err := kubeconfigutil.WriteToDisk(discoveryConfigFilePath, cfg); err != nil {
		return pkgerrors.Wrap(err, "couldn't save discovery.conf to disk")
	}

	return nil
}

// RetrieveBootstrapConfig get clientcmdapi config by bootstrap token
func RetrieveBootstrapConfig(data joindata.YurtJoinData) (*clientcmdapi.Config, error) {
	cfg, err := token.RetrieveValidatedConfigInfo(nil, &token.BootstrapData{
		ServerAddr:   data.ServerAddr(),
		JoinToken:    data.JoinToken(),
		CaCertHashes: data.CaCertHashes(),
	})
	if err != nil {
		return nil, err
	}

	clusterinfo := kubeconfigutil.GetClusterFromKubeConfig(cfg)
	return kubeconfigutil.CreateWithToken(
		// If there are multiple master IP addresses, take the first one here
		fmt.Sprintf("https://%s", strings.Split(data.ServerAddr(), ",")[0]),
		"kubernetes",
		TokenUser,
		clusterinfo.CertificateAuthorityData,
		data.JoinToken(),
	), nil
}

// CheckKubeletStatus check if kubelet is healthy.
func CheckKubeletStatus() error {
	initSystem, err := initsystem.GetInitSystem()
	if err != nil {
		return err
	}
	if ok := initSystem.ServiceIsActive("kubelet"); !ok {
		return fmt.Errorf("kubelet is not active. ")
	}
	return nil
}

// GetStaticPodTemplateFromConfigMap get static pod template from configmap
func GetStaticPodTemplateFromConfigMap(client kubernetes.Interface, namespace, name string) (string, string, error) {
	configMap, err := apiclient.GetConfigMapWithRetry(
		client,
		namespace,
		name)
	if err != nil {
		return "", "", pkgerrors.Errorf("could not get configmap of %s/%s yurtstaticset, err: %+v", namespace, name, err)
	}

	if len(configMap.Data) == 1 {
		for manifest, data := range configMap.Data {
			return manifest, data, nil
		}
	}

	return "", "", fmt.Errorf("invalid manifest in configmap %s", name)
}

// GetDefaultClientSet return client set created by /etc/kubernetes/kubelet.conf
func GetDefaultClientSet() (*kubernetes.Clientset, error) {
	kubeConfig := filepath.Join(constants.KubeletConfigureDir, constants.KubeletKubeConfigFileName)
	if _, err := os.Stat(kubeConfig); err != nil && os.IsNotExist(err) {
		return nil, err
	}

	cfg, err := clientcmd.BuildConfigFromFlags("", kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("could not create the clientset based on %s: %w", kubeConfig, err)
	}
	cliSet, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	return cliSet, nil
}

// IsValidBootstrapToken returns whether the given string is valid as a Bootstrap Token and
// in other words satisfies the BootstrapTokenRegexp
func IsValidBootstrapToken(token string) bool {
	return BootstrapTokenRegexp.MatchString(token)
}
