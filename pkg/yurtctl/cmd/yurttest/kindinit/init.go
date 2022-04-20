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

package kindinit

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurtctl/constants"
	enutil "github.com/openyurtio/openyurt/pkg/yurtctl/util/edgenode"
	kubeutil "github.com/openyurtio/openyurt/pkg/yurtctl/util/kubernetes"
	strutil "github.com/openyurtio/openyurt/pkg/yurtctl/util/strings"
	tmplutil "github.com/openyurtio/openyurt/pkg/yurtctl/util/templates"
)

var (
	validKubernetesVersions = []string{
		"v1.17",
		"v1.18",
		"v1.19",
		"v1.20",
		"v1.21",
		"v1.22",
	}
	validOpenYurtVersions = []string{
		"v0.5.0",
		"v0.6.0",
		"v0.6.1",
		"latest",
	}
	validKindVersions = []string{
		"v0.11.1",
	}
	//https://github.com/kubernetes-sigs/kind/releases
	kindNodeImageMap = map[string]string{
		"v1.17": "kindest/node:v1.17.17@sha256:66f1d0d91a88b8a001811e2f1054af60eef3b669a9a74f9b6db871f2f1eeed00",
		"v1.18": "kindest/node:v1.18.19@sha256:7af1492e19b3192a79f606e43c35fb741e520d195f96399284515f077b3b622c",
		"v1.19": "kindest/node:v1.19.11@sha256:07db187ae84b4b7de440a73886f008cf903fcf5764ba8106a9fd5243d6f32729",
		"v1.20": "kindest/node:v1.20.7@sha256:cbeaf907fc78ac97ce7b625e4bf0de16e3ea725daf6b04f930bd14c67c671ff9",
		"v1.21": "kindest/node:v1.21.1@sha256:69860bda5563ac81e3c0057d654b5253219618a22ec3a346306239bba8cfa1a6",
		"v1.22": "kindest/node:v1.22.0@sha256:b8bda84bb3a190e6e028b1760d277454a72267a5454b57db34437c34a588d047",
	}

	yurtHubImageFormat               = "openyurt/yurthub:%s"
	yurtControllerManagerImageFormat = "openyurt/yurt-controller-manager:%s"
	nodeServantImageFormat           = "openyurt/node-servant:%s"
	yurtTunnelServerImageFormat      = "openyurt/yurt-tunnel-server:%s"
	yurtTunnelAgentImageFormat       = "openyurt/yurt-tunnel-agent:%s"
)

func NewKindInitCMD() *cobra.Command {
	o := newKindOptions()

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Using kind to setup OpenYurt cluster for test",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := o.Validate(); err != nil {
				return err
			}
			initializer := newKindInitializer(o.Config())
			if err := initializer.Run(); err != nil {
				return err
			}
			return nil
		},
		Args: cobra.NoArgs,
	}

	addFlags(cmd.Flags(), o)
	return cmd
}

type kindOptions struct {
	KindConfigPath    string
	NodeNum           int
	ClusterName       string
	CloudNodes        string
	OpenYurtVersion   string
	KubernetesVersion string
	UseLocalImages    bool
	KubeConfig        string
	IgnoreError       bool
}

func newKindOptions() *kindOptions {
	return &kindOptions{
		KindConfigPath:    fmt.Sprintf("%s/kindconfig.yaml", constants.TmpDownloadDir),
		NodeNum:           2,
		ClusterName:       "openyurt",
		OpenYurtVersion:   constants.DefaultOpenYurtVersion,
		KubernetesVersion: "v1.21",
		UseLocalImages:    false,
		IgnoreError:       false,
	}
}

func (o *kindOptions) Validate() error {
	if o.NodeNum <= 0 {
		return fmt.Errorf("the number of nodes must be greater than 0")
	}
	if err := validateKubernetesVersion(o.KubernetesVersion); err != nil {
		return err
	}
	if err := validateOpenYurtVersion(o.OpenYurtVersion, o.IgnoreError); err != nil {
		return err
	}
	return nil
}

// Config should be called after Validate
// It will generate a config for Initializer
func (o *kindOptions) Config() *initializerConfig {
	controlPlaneNode, workerNodes := getNodeNamesOfKindCluster(o.ClusterName, o.NodeNum)
	allNodes := append(workerNodes, controlPlaneNode)

	// prepare kindConfig.CloudNodes and kindConfig.EdgeNodes
	cloudNodes := sets.NewString()
	cloudNodes = cloudNodes.Insert(controlPlaneNode)
	if o.CloudNodes != "" {
		for _, node := range strings.Split(o.CloudNodes, ",") {
			if !strutil.IsInStringLst(allNodes, node) {
				klog.Fatalf("node %s will not be in the cluster", node)
			}
			cloudNodes = cloudNodes.Insert(node)
		}
	}
	// any node not be specified as cloud node will be recognized as edge node
	edgeNodes := sets.NewString()
	for _, node := range allNodes {
		if !cloudNodes.Has(node) {
			edgeNodes = edgeNodes.Insert(node)
		}
	}

	// prepare kindConfig.KindPath
	kindPath, err := findKindPath()
	if err != nil {
		kindPath = ""
		klog.Warningf("%s, will try to go install kind", err)
	}

	// prepare kindConfig.KubeConfig
	kubeConfigPath := o.KubeConfig
	if kubeConfigPath == "" {
		if home := os.Getenv("HOME"); home != "" {
			kubeConfigPath = fmt.Sprintf("%s/.kube/config", home)
			klog.V(1).Infof("--kube-config is not specified, %s will be used.", kubeConfigPath)
		} else {
			klog.Fatal("failed to get ${HOME} env when using default kubeconfig path")
		}
	}

	return &initializerConfig{
		CloudNodes:                 cloudNodes.List(),
		EdgeNodes:                  edgeNodes.List(),
		KindConfigPath:             o.KindConfigPath,
		KubeConfig:                 kubeConfigPath,
		KindPath:                   kindPath,
		NodesNum:                   o.NodeNum,
		ClusterName:                o.ClusterName,
		NodeImage:                  kindNodeImageMap[o.KubernetesVersion],
		UseLocalImage:              o.UseLocalImages,
		YurtHubImage:               fmt.Sprintf(yurtHubImageFormat, o.OpenYurtVersion),
		YurtControllerManagerImage: fmt.Sprintf(yurtControllerManagerImageFormat, o.OpenYurtVersion),
		NodeServantImage:           fmt.Sprintf(nodeServantImageFormat, o.OpenYurtVersion),
		YurtTunnelServerImage:      fmt.Sprintf(yurtTunnelServerImageFormat, o.OpenYurtVersion),
		YurtTunnelAgentImage:       fmt.Sprintf(yurtTunnelAgentImageFormat, o.OpenYurtVersion),
	}
}

func addFlags(flagset *pflag.FlagSet, o *kindOptions) {
	flagset.StringVar(&o.KindConfigPath, "kind-config-path", o.KindConfigPath,
		"Specify the path where the kind config file will be generated.")
	flagset.IntVar(&o.NodeNum, "node-num", o.NodeNum,
		"Specify the node number of the kind cluster.")
	flagset.StringVar(&o.ClusterName, "cluster-name", o.ClusterName,
		"The cluster name of the new-created kind cluster.")
	flagset.StringVar(&o.CloudNodes, "cloud-nodes", "",
		"Comma separated list of cloud nodes. The control-plane will always be cloud node."+
			"If no cloud node specified, the control-plane node will be the only one cloud node.")
	flagset.StringVar(&o.OpenYurtVersion, "openyurt-version", o.OpenYurtVersion,
		"The version of openyurt components.")
	flagset.StringVar(&o.KubernetesVersion, "kubernetes-version", o.KubernetesVersion,
		"The version of kubernetes that the openyurt cluster is based on.")
	flagset.BoolVar(&o.UseLocalImages, "use-local-images", o.UseLocalImages,
		"If set, local images stored by docker will be used first.")
	flagset.StringVar(&o.KubeConfig, "kube-config", o.KubeConfig,
		"Path where the kubeconfig file of new cluster will be stored. The default is ${HOME}/.kube/config.")
	flagset.BoolVar(&o.IgnoreError, "ignore-error", o.IgnoreError,
		"Igore error when using openyurt version that is not officially released.")
}

type initializerConfig struct {
	CloudNodes                 []string
	EdgeNodes                  []string
	KindConfigPath             string
	KubeConfig                 string
	KindPath                   string
	NodesNum                   int
	ClusterName                string
	NodeImage                  string
	UseLocalImage              bool
	YurtHubImage               string
	YurtControllerManagerImage string
	NodeServantImage           string
	YurtTunnelServerImage      string
	YurtTunnelAgentImage       string
}

type Initializer struct {
	initializerConfig
	operator *KindOperator
}

func newKindInitializer(cfg *initializerConfig) *Initializer {
	return &Initializer{
		initializerConfig: *cfg,
		operator:          NewKindOperator(cfg.KindPath, cfg.KubeConfig),
	}
}

func (i *Initializer) Run() error {
	klog.Info("Start to install kind")
	if err := i.operator.KindInstall(); err != nil {
		return err
	}

	klog.Info("Start to prepare config file for kind")
	if err := i.prepareKindConfigFile(i.KindConfigPath); err != nil {
		return err
	}

	klog.Info("Start to create cluster with kind")
	if err := i.operator.KindCreateClusterWithConfig(i.KindConfigPath); err != nil {
		return err
	}

	klog.Infof("Start to prepare OpenYurt images for kind cluster")
	if err := i.prepareImages(); err != nil {
		return err
	}

	klog.Infof("Start to deploy OpenYurt components")
	if err := i.deployOpenYurt(); err != nil {
		return err
	}
	return nil
}

func (i *Initializer) prepareImages() error {
	if !i.UseLocalImage {
		return nil
	}
	// load images of cloud components to cloud nodes
	if err := i.loadImagesToKindNodes([]string{
		i.YurtHubImage,
		i.YurtControllerManagerImage,
		i.NodeServantImage,
		i.YurtTunnelServerImage,
	}, i.CloudNodes); err != nil {
		return err
	}

	// load images of edge components to edge nodes
	if err := i.loadImagesToKindNodes([]string{
		i.YurtHubImage,
		i.NodeServantImage,
		i.YurtTunnelAgentImage,
	}, i.EdgeNodes); err != nil {
		return err
	}

	return nil
}

func (i *Initializer) prepareKindConfigFile(kindConfigPath string) error {
	kindConfigDir := filepath.Dir(kindConfigPath)
	if err := os.MkdirAll(kindConfigDir, constants.DirMode); err != nil {
		return err
	}
	kindConfigContent, err := tmplutil.SubsituteTemplate(constants.OpenYurtKindConfig, map[string]string{
		"kind_node_image": i.NodeImage,
		"cluster_name":    i.ClusterName,
	})
	if err != nil {
		return err
	}

	// add additional worker entries into kind config file according to NodesNum
	for num := 1; num < i.NodesNum; num++ {
		worker, err := tmplutil.SubsituteTemplate(constants.KindWorkerRole, map[string]string{
			"kind_node_image": i.NodeImage,
		})
		if err != nil {
			return err
		}
		kindConfigContent = strings.Join([]string{kindConfigContent, worker}, "\n")
	}

	if err = ioutil.WriteFile(kindConfigPath, []byte(kindConfigContent), constants.FileMode); err != nil {
		return err
	}
	klog.V(1).Infof("generated new kind config file at %s", kindConfigPath)
	return nil
}

func (i *Initializer) deployOpenYurt() error {
	kubeconfig, err := clientcmd.BuildConfigFromFlags("", i.KubeConfig)
	if err != nil {
		return err
	}
	client, err := kubernetes.NewForConfig(kubeconfig)
	if err != nil {
		return err
	}

	converter := &ClusterConverter{
		ClientSet:                  client,
		CloudNodes:                 i.CloudNodes,
		EdgeNodes:                  i.EdgeNodes,
		WaitServantJobTimeout:      kubeutil.DefaultWaitServantJobTimeout,
		YurthubHealthCheckTimeout:  defaultYurthubHealthCheckTimeout,
		PodManifestPath:            enutil.GetPodManifestPath(),
		KubeConfigPath:             i.KubeConfig,
		YurtTunnelAgentImage:       i.YurtTunnelAgentImage,
		YurtTunnelServerImage:      i.YurtTunnelServerImage,
		YurtControllerManagerImage: i.YurtControllerManagerImage,
		NodeServantImage:           i.NodeServantImage,
		YurthubImage:               i.YurtHubImage,
	}
	if err := converter.Run(); err != nil {
		klog.Errorf("errors occurred when deploying openyurt components")
		return err
	}
	return nil
}

func (i *Initializer) loadImagesToKindNodes(images, nodes []string) error {
	for _, image := range images {
		if image == "" {
			// if image == "", it's the responsibility of kind to pull images from registry.
			continue
		}
		if err := i.operator.KindLoadDockerImage(i.ClusterName, image, nodes); err != nil {
			return err
		}
	}
	return nil
}

func getGoBinPath() (string, error) {
	gopath, err := exec.Command("bash", "-c", "go env GOPATH").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get GOPATH, %s", err)
	}
	return filepath.Join(string(gopath), "bin"), nil
}

func checkIfKindAt(path string) (bool, string) {
	if p, err := exec.LookPath(path); err == nil {
		return true, p
	}
	return false, ""
}

func findKindPath() (string, error) {
	var kindPath string
	switch {
	case true:
		if exist, path := checkIfKindAt("kind"); exist {
			kindPath = path
			break
		}
		fallthrough
	case true:
		goBinPath, err := getGoBinPath()
		if err != nil {
			klog.Fatal("failed to get go bin path, %s", err)
		}
		if exist, path := checkIfKindAt(goBinPath + "/kind"); exist {
			kindPath = path
			break
		}
		fallthrough
	default:
		return "", fmt.Errorf("cannot find valid kind cmd, try to install it")
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

func validateKubernetesVersion(ver string) error {
	s := strings.Split(ver, ".")
	var originVer = ver
	if len(s) < 2 || len(s) > 3 {
		return fmt.Errorf("invalid format of kubernetes version: %s", ver)
	}
	if len(s) == 3 {
		// v1.xx.xx
		ver = strings.Join(s[:2], ".")
	}

	// v1.xx
	if !strutil.IsInStringLst(validKubernetesVersions, ver) {
		return fmt.Errorf("unsupported kubernetes version: %s", originVer)
	}
	return nil
}

func validateOpenYurtVersion(ver string, ignoreError bool) error {
	if !strutil.IsInStringLst(validOpenYurtVersions, ver) && !ignoreError {
		return fmt.Errorf("%s is not a valid openyurt version, all valid versions are %s. If you know what you're doing, you can set --ignore-error",
			ver, strings.Join(validOpenYurtVersions, ","))
	}
	return nil
}

// getNodeNamesOfKindCluster will generate all nodes will be in the kind cluster.
// It depends on the naming machanism of kind:
// one control-plane node: ${clusterName}-control-plane
// serval worker nodes: ${clusterName}-worker, ${clusterName}-worker2, ${clusterName}-worker3...
func getNodeNamesOfKindCluster(clusterName string, nodeNum int) (string, []string) {
	controlPlaneNode := fmt.Sprintf("%s-control-plane", clusterName)
	workerNodes := []string{
		strings.Join([]string{clusterName, "worker"}, "-"),
	}
	for cnt := 2; cnt < nodeNum; cnt++ {
		workerNodes = append(workerNodes, fmt.Sprintf("%s-worker%d", clusterName, cnt))
	}
	return controlPlaneNode, workerNodes
}
