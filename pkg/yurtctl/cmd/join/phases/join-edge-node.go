/*
Copyright 2021 The OpenYurt Authors.
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

package phases

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/version"
	clientset "k8s.io/client-go/kubernetes"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	certutil "k8s.io/client-go/util/cert"
	"k8s.io/klog"
	kubeletconfigv1beta1 "k8s.io/kubelet/config/v1beta1"
	kubeadmapi "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm"
	"k8s.io/kubernetes/cmd/kubeadm/app/cmd/phases/workflow"
	kubeadmconstants "k8s.io/kubernetes/cmd/kubeadm/app/constants"
	kubeletphase "k8s.io/kubernetes/cmd/kubeadm/app/phases/kubelet"
	"k8s.io/kubernetes/cmd/kubeadm/app/util/apiclient"
	kubeconfigutil "k8s.io/kubernetes/cmd/kubeadm/app/util/kubeconfig"
	kubeletconfig "k8s.io/kubernetes/pkg/kubelet/apis/config"
	kubeletscheme "k8s.io/kubernetes/pkg/kubelet/apis/config/scheme"
	utilcodec "k8s.io/kubernetes/pkg/kubelet/kubeletconfig/util/codec"

	"github.com/openyurtio/openyurt/pkg/yurtctl/constants"
	"github.com/openyurtio/openyurt/pkg/yurtctl/util/edgenode"
	"github.com/pkg/errors"
)

// NewEdgeNodePhase creates a yurtctl workflow phase that start kubelet on a edge node.
func NewEdgeNodePhase() workflow.Phase {
	return workflow.Phase{
		Name:  "Join edge-node to OpenYurt cluster. ",
		Short: "Join edge-node",
		Run:   runJoinEdgeNode,
	}
}

//runJoinEdgeNode executes the edge node join process.
func runJoinEdgeNode(c workflow.RunData) error {
	data, ok := c.(YurtJoinData)
	if !ok {
		return fmt.Errorf("Join edge-node phase invoked with an invalid data struct. ")
	}
	if data.NodeType() != constants.EdgeNode {
		return nil
	}
	cfg, initCfg, tlsBootstrapCfg, err := getEdgeNodeJoinData(data)
	if err != nil {
		return err
	}

	if err := setKubeletConfigForEdgeNode(); err != nil {
		return err
	}
	clusterinfo := kubeconfigutil.GetClusterFromKubeConfig(tlsBootstrapCfg)
	if err := certutil.WriteCert(edgenode.KubeCaFile, clusterinfo.CertificateAuthorityData); err != nil {
		return err
	}

	tlsClient, err := kubeconfigutil.ToClientSet(tlsBootstrapCfg)
	if err != nil {
		return err
	}
	kc, err := getKubeletConfig(cfg, initCfg, tlsClient)
	if err != nil {
		return err
	}
	if err := addYurthubStaticYaml(cfg, kc.StaticPodPath, data.YurtHubImage()); err != nil {
		return err
	}
	klog.Info("[kubelet-start] Starting the kubelet")
	kubeletphase.TryStartKubelet()
	return nil
}

//setKubeleConfigForEdgeNode write kubelet.conf for edge-node.
func setKubeletConfigForEdgeNode() error {
	kubeletConfigDir := filepath.Dir(edgenode.KubeCondfigPath)
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
	if err := ioutil.WriteFile(edgenode.KubeCondfigPath, []byte(kubeletConfForEdgeNode), 0755); err != nil {
		return err
	}
	return nil
}

//addYurthubStaticYaml generate YurtHub static yaml for edge-node.
func addYurthubStaticYaml(cfg *kubeadmapi.JoinConfiguration, podManifestPath string, yurthubImage string) error {
	klog.Info("[join-node] Adding edge hub static yaml")
	if len(yurthubImage) == 0 {
		yurthubImage = fmt.Sprintf("%s/%s:%s", constants.DefaultOpenYurtImageRegistry, constants.Yurthub, constants.DefaultOpenYurtVersion)
	}
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

	yurthubTemplate := edgenode.ReplaceRegularExpression(edgenode.YurthubTemplate,
		map[string]string{
			"__kubernetes_service_addr__": fmt.Sprintf("https://%s", cfg.Discovery.BootstrapToken.APIServerEndpoint),
			"__yurthub_image__":           yurthubImage,
			"__join_token__":              cfg.Discovery.BootstrapToken.Token,
		})

	if err := ioutil.WriteFile(filepath.Join(podManifestPath, defaultYurthubStaticPodFileName), []byte(yurthubTemplate), 0600); err != nil {
		return err
	}
	klog.Info("[join-node] Add edge hub static yaml is ok")
	return nil
}

//getKubeletConfig get kubelet configure from master.
func getKubeletConfig(cfg *kubeadmapi.JoinConfiguration, initCfg *kubeadmapi.InitConfiguration, tlsClient *clientset.Clientset) (*kubeletconfig.KubeletConfiguration, error) {
	kubeletVersion, err := version.ParseSemantic(initCfg.ClusterConfiguration.KubernetesVersion)
	if err != nil {
		return nil, err
	}

	// Write the configuration for the kubelet (using the bootstrap token credentials) to disk so the kubelet can start
	kc, err := downloadConfig(tlsClient, kubeletVersion, kubeadmconstants.KubeletRunDirectory)
	if err != nil {
		return nil, err
	}
	if err := kubeletphase.WriteKubeletDynamicEnvFile(&initCfg.ClusterConfiguration, &cfg.NodeRegistration, false, kubeadmconstants.KubeletRunDirectory); err != nil {
		return kc, err
	}
	return kc, nil
}

// downloadConfig downloads the kubelet configuration from a ConfigMap and writes it to disk.
// Used at "kubeadm join" time
func downloadConfig(client clientset.Interface, kubeletVersion *version.Version, kubeletDir string) (*kubeletconfig.KubeletConfiguration, error) {

	// Download the ConfigMap from the cluster based on what version the kubelet is
	configMapName := kubeadmconstants.GetKubeletConfigMapName(kubeletVersion)

	fmt.Printf("[kubelet-start] Downloading configuration for the kubelet from the %q ConfigMap in the %s namespace\n",
		configMapName, metav1.NamespaceSystem)

	kubeletCfg, err := apiclient.GetConfigMapWithRetry(client, metav1.NamespaceSystem, configMapName)
	// If the ConfigMap wasn't found and the kubelet version is v1.10.x, where we didn't support the config file yet
	// just return, don't error out
	if apierrors.IsNotFound(err) && kubeletVersion.Minor() == 10 {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	_, kubeletCodecs, err := kubeletscheme.NewSchemeAndCodecs()
	if err != nil {
		return nil, err
	}
	kc, err := utilcodec.DecodeKubeletConfiguration(kubeletCodecs, []byte(kubeletCfg.Data[kubeadmconstants.KubeletBaseConfigurationConfigMapKey]))
	if err != nil {
		return nil, err
	}
	if kc.StaticPodPath == "" {
		kc.StaticPodPath = constants.StaticPodPath
	}
	encoder, err := utilcodec.NewKubeletconfigYAMLEncoder(kubeletconfigv1beta1.SchemeGroupVersion)
	if err != nil {
		return nil, err
	}
	data, err := runtime.Encode(encoder, kc)
	if err != nil {
		return nil, err
	}
	return kc, writeConfigBytesToDisk(data, kubeletDir)
}

//getEdgeNodeJoinData get edge-node join configuration.
func getEdgeNodeJoinData(data YurtJoinData) (*kubeadmapi.JoinConfiguration, *kubeadmapi.InitConfiguration, *clientcmdapi.Config, error) {
	cfg := data.Cfg()
	initCfg, err := data.InitCfg()
	if err != nil {
		return nil, nil, nil, err
	}
	tlsBootstrapCfg, err := data.TLSBootstrapCfg()
	if err != nil {
		return nil, nil, nil, err
	}
	return cfg, initCfg, tlsBootstrapCfg, nil
}

// writeConfigBytesToDisk writes a byte slice down to disk at the specific location of the kubelet config file
func writeConfigBytesToDisk(b []byte, kubeletDir string) error {
	configFile := filepath.Join(kubeletDir, kubeadmconstants.KubeletConfigurationFileName)
	fmt.Printf("[kubelet-start] Writing kubelet configuration to file %q\n", configFile)

	// creates target folder if not already exists
	if err := os.MkdirAll(kubeletDir, 0700); err != nil {
		return errors.Wrapf(err, "failed to create directory %q", kubeletDir)
	}

	if err := ioutil.WriteFile(configFile, b, 0644); err != nil {
		return errors.Wrapf(err, "failed to write kubelet configuration to the file %q", configFile)
	}
	return nil
}
