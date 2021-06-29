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
	"path/filepath"

	"k8s.io/apimachinery/pkg/util/version"
	clientset "k8s.io/client-go/kubernetes"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	certutil "k8s.io/client-go/util/cert"
	"k8s.io/klog"
	kubeadmapi "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm"
	"k8s.io/kubernetes/cmd/kubeadm/app/cmd/phases/workflow"
	kubeadmconstants "k8s.io/kubernetes/cmd/kubeadm/app/constants"
	kubeletphase "k8s.io/kubernetes/cmd/kubeadm/app/phases/kubelet"
	kubeconfigutil "k8s.io/kubernetes/cmd/kubeadm/app/util/kubeconfig"

	"github.com/openyurtio/openyurt/pkg/yurtctl/util/edgenode"
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
	if data.NodeType() != EdgeNode {
		return nil
	}
	cfg, initCfg, tlsBootstrapCfg, err := getEdgeNodeJoinData(data)
	if err != nil {
		return err
	}

	if err := setKubeleConfigForEdgeNode(); err != nil {
		return err
	}
	if err := addYurthubStaticYaml(cfg, data.YurtHubImage()); err != nil {
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
	if err := getKubeletConfig(cfg, initCfg, tlsClient); err != nil {
		return err
	}
	klog.Info("[kubelet-start] Starting the kubelet")
	kubeletphase.TryStartKubelet()
	return nil
}

//setKubeleConfigForEdgeNode write kubelet.conf for edge-node.
func setKubeleConfigForEdgeNode() error {
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
func addYurthubStaticYaml(cfg *kubeadmapi.JoinConfiguration, yurthubImage string) error {
	klog.Info("[join-node] Adding edge hub static yaml")
	if len(yurthubImage) == 0 {
		yurthubImage = defaultYurthubImage
	}
	kubeletStaticPodDir := filepath.Dir(yurtHubStaticPodYamlFile)
	if _, err := os.Stat(kubeletStaticPodDir); err != nil {
		if os.IsNotExist(err) {
			err = os.MkdirAll(kubeletStaticPodDir, os.ModePerm)
			if err != nil {
				return err
			}
		} else {
			klog.Errorf("Describe dir %s fail: %v", kubeletStaticPodDir, err)
			return err
		}
	}

	yurthubTemplate := edgenode.ReplaceRegularExpression(edgenode.YurthubTemplate,
		map[string]string{
			"__kubernetes_service_addr__": fmt.Sprintf("https://%s", cfg.Discovery.BootstrapToken.APIServerEndpoint),
			"__yurthub_image__":           yurthubImage,
			"__join_token__":              cfg.Discovery.BootstrapToken.Token,
		})

	if err := ioutil.WriteFile(yurtHubStaticPodYamlFile, []byte(yurthubTemplate), 0600); err != nil {
		return err
	}
	klog.Info("[join-node] Add edge hub static yaml is ok")
	return nil
}

//getKubeletConfig get kubelet configure from master.
func getKubeletConfig(cfg *kubeadmapi.JoinConfiguration, initCfg *kubeadmapi.InitConfiguration, tlsClient *clientset.Clientset) error {
	kubeletVersion, err := version.ParseSemantic(initCfg.ClusterConfiguration.KubernetesVersion)
	if err != nil {
		return err
	}

	// Write the configuration for the kubelet (using the bootstrap token credentials) to disk so the kubelet can start
	if err := kubeletphase.DownloadConfig(tlsClient, kubeletVersion, kubeadmconstants.KubeletRunDirectory); err != nil {
		return err
	}
	if err := kubeletphase.WriteKubeletDynamicEnvFile(&initCfg.ClusterConfiguration, &cfg.NodeRegistration, false, kubeadmconstants.KubeletRunDirectory); err != nil {
		return err
	}
	return nil
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
