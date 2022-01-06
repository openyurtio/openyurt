/*
Copyright 2019 The Kubernetes Authors.

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

package constants

import (
	"fmt"
	"path/filepath"
	"time"

	"k8s.io/apimachinery/pkg/util/version"
	bootstrapapi "k8s.io/cluster-bootstrap/token/api"
)

const (
	// KubernetesDir is the directory Kubernetes owns for storing various configuration files
	KubernetesDir = "/etc/kubernetes"
	// ManifestsSubDirName defines directory name to store manifests
	ManifestsSubDirName = "manifests"

	// CACertAndKeyBaseName defines certificate authority base name
	CACertAndKeyBaseName = "ca"
	// CACertName defines certificate name
	CACertName = "ca.crt"

	// AdminKubeConfigFileName defines name for the kubeconfig aimed to be used by the superuser/admin of the cluster
	AdminKubeConfigFileName = "admin.conf"

	// KubeletKubeConfigFileName defines the file name for the kubeconfig that the control-plane kubelet will use for talking
	// to the API server
	KubeletKubeConfigFileName = "kubelet.conf"
	// ControllerManagerKubeConfigFileName defines the file name for the controller manager's kubeconfig file
	ControllerManagerKubeConfigFileName = "controller-manager.conf"
	// SchedulerKubeConfigFileName defines the file name for the scheduler's kubeconfig file
	SchedulerKubeConfigFileName = "scheduler.conf"

	// Some well-known users and groups in the core Kubernetes authorization system

	// APICallRetryInterval defines how long kubeadm should wait before retrying a failed API operation
	APICallRetryInterval = 500 * time.Millisecond
	// DiscoveryRetryInterval specifies how long kubeadm should wait before retrying to connect to the control-plane when doing discovery
	DiscoveryRetryInterval = 5 * time.Second
	// PatchNodeTimeout specifies how long kubeadm should wait for applying the label and taint on the control-plane before timing out
	PatchNodeTimeout = 2 * time.Minute
	// TLSBootstrapTimeout specifies how long kubeadm should wait for the kubelet to perform the TLS Bootstrap
	TLSBootstrapTimeout = 5 * time.Minute
	// TLSBootstrapRetryInterval specifies how long kubeadm should wait before retrying the TLS Bootstrap check
	TLSBootstrapRetryInterval = 5 * time.Second
	// APICallWithWriteTimeout specifies how long kubeadm should wait for api calls with at least one write
	APICallWithWriteTimeout = 40 * time.Second
	// APICallWithReadTimeout specifies how long kubeadm should wait for api calls with only reads
	APICallWithReadTimeout = 15 * time.Second
	// PullImageRetry specifies how many times ContainerRuntime retries when pulling image failed
	PullImageRetry = 5

	// AnnotationKubeadmCRISocket specifies the annotation kubeadm uses to preserve the crisocket information given to kubeadm at
	// init/join time for use later. kubeadm annotates the node object with this information
	AnnotationKubeadmCRISocket = "kubeadm.alpha.kubernetes.io/cri-socket"

	// KubeadmConfigConfigMap specifies in what ConfigMap in the kube-system namespace the `kubeadm init` configuration should be stored
	KubeadmConfigConfigMap = "kubeadm-config"

	// ClusterConfigurationConfigMapKey specifies in what ConfigMap key the cluster configuration should be stored
	ClusterConfigurationConfigMapKey = "ClusterConfiguration"

	// KubeletBaseConfigurationConfigMapPrefix specifies in what ConfigMap in the kube-system namespace the initial remote configuration of kubelet should be stored
	KubeletBaseConfigurationConfigMapPrefix = "kubelet-config-"

	// KubeletBaseConfigurationConfigMapKey specifies in what ConfigMap key the initial remote configuration of kubelet should be stored
	KubeletBaseConfigurationConfigMapKey = "kubelet"

	// KubeletRunDirectory specifies the directory where the kubelet runtime information is stored.
	KubeletRunDirectory = "/var/lib/kubelet"

	// KubeletConfigurationFileName specifies the file name on the node which stores initial remote configuration of kubelet
	// This file should exist under KubeletRunDirectory
	KubeletConfigurationFileName = "config.yaml"

	// KubeletEnvFileName is a file "kubeadm init" writes at runtime. Using that interface, kubeadm can customize certain
	// kubelet flags conditionally based on the environment at runtime. Also, parameters given to the configuration file
	// might be passed through this file. "kubeadm init" writes one variable, with the name ${KubeletEnvFileVariableName}.
	// This file should exist under KubeletRunDirectory
	KubeletEnvFileName = "kubeadm-flags.env"

	// KubeletEnvFileVariableName specifies the shell script variable name "kubeadm init" should write a value to in KubeletEnvFile
	KubeletEnvFileVariableName = "KUBELET_KUBEADM_ARGS"

	// KubeletHealthzPort is the port of the kubelet healthz endpoint
	KubeletHealthzPort = 10248

	// NodeBootstrapTokenAuthGroup specifies which group a Node Bootstrap Token should be authenticated in
	NodeBootstrapTokenAuthGroup = "system:bootstrappers:kubeadm:default-node-token"

	// KubeAPIServer defines variable used internally when referring to kube-apiserver component
	KubeAPIServer = "kube-apiserver"
	// KubeControllerManager defines variable used internally when referring to kube-controller-manager component
	KubeControllerManager = "kube-controller-manager"
	// KubeScheduler defines variable used internally when referring to kube-scheduler component
	KubeScheduler = "kube-scheduler"

	// KubeletPort is the default port for the kubelet server on each host machine.
	// May be overridden by a flag at startup.
	KubeletPort = 10250
)

var (
	// DefaultTokenUsages specifies the default functions a token will get
	DefaultTokenUsages = bootstrapapi.KnownTokenUsages

	// DefaultTokenGroups specifies the default groups that this token will authenticate as when used for authentication
	DefaultTokenGroups = []string{NodeBootstrapTokenAuthGroup}

	// ControlPlaneComponents defines the control-plane component names
	ControlPlaneComponents = []string{KubeAPIServer, KubeControllerManager, KubeScheduler}

	// MinimumKubeletVersion specifies the minimum version of kubelet which kubeadm supports
	MinimumKubeletVersion = version.MustParseSemantic("v1.17.0")
)

// GetStaticPodDirectory returns the location on the disk where the Static Pod should be present
func GetStaticPodDirectory() string {
	return filepath.Join(KubernetesDir, ManifestsSubDirName)
}

// GetStaticPodFilepath returns the location on the disk where the Static Pod should be present
func GetStaticPodFilepath(componentName, manifestsDir string) string {
	return filepath.Join(manifestsDir, componentName+".yaml")
}

// GetAdminKubeConfigPath returns the location on the disk where admin kubeconfig is located by default
func GetAdminKubeConfigPath() string {
	return filepath.Join(KubernetesDir, AdminKubeConfigFileName)
}

// GetKubeletConfigMapName returns the right ConfigMap name for the right branch of k8s
func GetKubeletConfigMapName(k8sVersion *version.Version) string {
	return fmt.Sprintf("%s%d.%d", KubeletBaseConfigurationConfigMapPrefix, k8sVersion.Major(), k8sVersion.Minor())
}
