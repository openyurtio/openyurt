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
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"

	nodeservant "github.com/openyurtio/openyurt/pkg/node-servant"
	kubeutil "github.com/openyurtio/openyurt/pkg/yurtadm/util/kubernetes"
	strutil "github.com/openyurtio/openyurt/pkg/yurtadm/util/strings"
	tmplutil "github.com/openyurtio/openyurt/pkg/yurtadm/util/templates"
	"github.com/openyurtio/openyurt/pkg/yurtctl/constants"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/servicetopology"
)

var (
	validKubernetesVersions = []string{
		"v1.17",
		"v1.18",
		"v1.19",
		"v1.20",
		"v1.21",
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

	kindNodeImageMap = map[string]string{
		"v1.17": "kindest/node:v1.17.17@sha256:66f1d0d91a88b8a001811e2f1054af60eef3b669a9a74f9b6db871f2f1eeed00",
		"v1.18": "kindest/node:v1.18.19@sha256:7af1492e19b3192a79f606e43c35fb741e520d195f96399284515f077b3b622c",
		"v1.19": "kindest/node:v1.19.11@sha256:07db187ae84b4b7de440a73886f008cf903fcf5764ba8106a9fd5243d6f32729",
		"v1.20": "kindest/node:v1.20.7@sha256:cbeaf907fc78ac97ce7b625e4bf0de16e3ea725daf6b04f930bd14c67c671ff9",
		"v1.21": "kindest/node:v1.21.1@sha256:69860bda5563ac81e3c0057d654b5253219618a22ec3a346306239bba8cfa1a6",
	}

	yurtHubImageFormat               = "openyurt/yurthub:%s"
	yurtControllerManagerImageFormat = "openyurt/yurt-controller-manager:%s"
	nodeServantImageFormat           = "openyurt/node-servant:%s"
	yurtTunnelServerImageFormat      = "openyurt/yurt-tunnel-server:%s"
	yurtTunnelAgentImageFormat       = "openyurt/yurt-tunnel-agent:%s"

	hostsSettingForCoreFile = []string{
		"    hosts /etc/edge/tunnel-nodes {",
		"       reload 300ms",
		"       fallthrough",
		"    }",
	}
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
	EnableDummyIf     bool
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
		EnableDummyIf:     true,
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
		EnableDummyIf:              o.EnableDummyIf,
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
		"Ignore error when using openyurt version that is not officially released.")
	flagset.BoolVar(&o.EnableDummyIf, "enable-dummy-if", o.EnableDummyIf,
		"Enable dummy interface for yurthub component or not. and recommend to set false on mac env")
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
	EnableDummyIf              bool
}

type Initializer struct {
	initializerConfig
	operator   *KindOperator
	kubeClient *kubernetes.Clientset
}

func newKindInitializer(cfg *initializerConfig) *Initializer {
	return &Initializer{
		initializerConfig: *cfg,
		operator:          NewKindOperator(cfg.KindPath, cfg.KubeConfig),
	}
}

func (ki *Initializer) Run() error {
	klog.Info("Start to install kind")
	if err := ki.operator.KindInstall(); err != nil {
		return err
	}

	klog.Info("Start to prepare config file for kind")
	if err := ki.prepareKindConfigFile(ki.KindConfigPath); err != nil {
		return err
	}

	klog.Info("Start to create cluster with kind")
	if err := ki.operator.KindCreateClusterWithConfig(ki.KindConfigPath); err != nil {
		return err
	}

	klog.Info("Start to prepare kube client")
	kubeconfig, err := clientcmd.BuildConfigFromFlags("", ki.KubeConfig)
	if err != nil {
		return err
	}
	ki.kubeClient, err = kubernetes.NewForConfig(kubeconfig)
	if err != nil {
		return err
	}

	klog.Info("Start to prepare OpenYurt images for kind cluster")
	if err := ki.prepareImages(); err != nil {
		return err
	}

	klog.Info("Start to configure kube-apiserver and kube-controller-manager")
	if err := ki.configureControlPlane(); err != nil {
		return err
	}

	klog.Info("Start to deploy OpenYurt components")
	if err := ki.deployOpenYurt(); err != nil {
		return err
	}

	klog.Infof("Start to configure coredns and kube-proxy to adapt OpenYurt")
	if err := ki.configureAddons(); err != nil {
		return err
	}
	return nil
}

func (ki *Initializer) prepareImages() error {
	if !ki.UseLocalImage {
		return nil
	}
	// load images of cloud components to cloud nodes
	if err := ki.loadImagesToKindNodes([]string{
		ki.YurtHubImage,
		ki.YurtControllerManagerImage,
		ki.NodeServantImage,
		ki.YurtTunnelServerImage,
	}, ki.CloudNodes); err != nil {
		return err
	}

	// load images of edge components to edge nodes
	if err := ki.loadImagesToKindNodes([]string{
		ki.YurtHubImage,
		ki.NodeServantImage,
		ki.YurtTunnelAgentImage,
	}, ki.EdgeNodes); err != nil {
		return err
	}

	return nil
}

func (ki *Initializer) prepareKindConfigFile(kindConfigPath string) error {
	kindConfigDir := filepath.Dir(kindConfigPath)
	if err := os.MkdirAll(kindConfigDir, constants.DirMode); err != nil {
		return err
	}
	kindConfigContent, err := tmplutil.SubsituteTemplate(constants.OpenYurtKindConfig, map[string]string{
		"kind_node_image": ki.NodeImage,
		"cluster_name":    ki.ClusterName,
	})
	if err != nil {
		return err
	}

	// add additional worker entries into kind config file according to NodesNum
	for num := 1; num < ki.NodesNum; num++ {
		worker, err := tmplutil.SubsituteTemplate(constants.KindWorkerRole, map[string]string{
			"kind_node_image": ki.NodeImage,
		})
		if err != nil {
			return err
		}
		kindConfigContent = strings.Join([]string{kindConfigContent, worker}, "\n")
	}

	if err = os.WriteFile(kindConfigPath, []byte(kindConfigContent), constants.FileMode); err != nil {
		return err
	}
	klog.V(1).Infof("generated new kind config file at %s", kindConfigPath)
	return nil
}

func (ki *Initializer) configureControlPlane() error {
	convertCtx := map[string]string{
		"node_servant_image": ki.NodeServantImage,
	}

	return kubeutil.RunServantJobs(ki.kubeClient, kubeutil.DefaultWaitServantJobTimeout, func(nodeName string) (*batchv1.Job, error) {
		return nodeservant.RenderNodeServantJob("config-control-plane", convertCtx, nodeName)
	}, ki.CloudNodes, os.Stderr, true)
}

func (ki *Initializer) configureAddons() error {
	if err := ki.configureCoreDnsAddon(); err != nil {
		return err
	}

	if err := ki.ConfigureKubeProxyAddon(); err != nil {
		return err
	}

	// re-construct kube-proxy pods
	podList, err := ki.kubeClient.CoreV1().Pods("kube-system").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}
	for i := range podList.Items {
		switch {
		case strings.HasPrefix(podList.Items[i].Name, "kube-proxy"):
			// delete pod
			propagation := metav1.DeletePropagationForeground
			err = ki.kubeClient.CoreV1().Pods("kube-system").Delete(context.TODO(), podList.Items[i].Name, metav1.DeleteOptions{
				PropagationPolicy: &propagation,
			})
			if err != nil {
				klog.Errorf("failed to delete pod(%s), %v", podList.Items[i].Name, err)
			}
		default:
		}
	}

	// wait for coredns pods available
	for {
		select {
		case <-time.After(10 * time.Second):
			dnsDp, err := ki.kubeClient.AppsV1().Deployments("kube-system").Get(context.TODO(), "coredns", metav1.GetOptions{})
			if err != nil {
				return fmt.Errorf("failed to get coredns deployment when waiting for available, %v", err)
			}

			if dnsDp.Status.ObservedGeneration < dnsDp.Generation {
				klog.Infof("waiting for coredns generation(%d) to be observed. now observed generation is %d", dnsDp.Generation, dnsDp.Status.ObservedGeneration)
				continue
			}

			if *dnsDp.Spec.Replicas != dnsDp.Status.AvailableReplicas {
				klog.Infof("waiting for coredns replicas(%d) to be ready, now %d pods available", *dnsDp.Spec.Replicas, dnsDp.Status.AvailableReplicas)
				continue
			}
			klog.Info("coredns deployment configuration is completed")
			return nil
		}
	}
}

func (ki *Initializer) configureCoreDnsAddon() error {
	// config configmap kube-system/coredns in order to add hosts setting for resolving hostname to x-tunnel-server-internal-svc service
	cm, err := ki.kubeClient.CoreV1().ConfigMaps("kube-system").Get(context.TODO(), "coredns", metav1.GetOptions{})
	if err != nil {
		return err
	}
	if cm != nil && !strings.Contains(cm.Data["Corefile"], "hosts /etc/edge/tunnel-nodes") {
		lines := strings.Split(cm.Data["Corefile"], "\n")
		for i := range lines {
			if strings.Contains(lines[i], "kubernetes cluster.local") && strings.Contains(lines[i], "{") {
				lines = append(lines[:i], append(hostsSettingForCoreFile, lines[i:]...)...)
				break
			}
		}
		cm.Data["Corefile"] = strings.Join(lines, "\n")

		// update coredns configmap
		_, err = ki.kubeClient.CoreV1().ConfigMaps("kube-system").Update(context.TODO(), cm, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to configure coredns configmap, %w", err)
		}
	}

	// add annotation(openyurt.io/topologyKeys=kubernetes.io/hostname) for service kube-system/kube-dns in order to use the
	// local coredns instance for resolving.
	svc, err := ki.kubeClient.CoreV1().Services("kube-system").Get(context.TODO(), "kube-dns", metav1.GetOptions{})
	if err != nil {
		return err
	}
	if svc != nil && len(svc.Annotations[servicetopology.AnnotationServiceTopologyKey]) == 0 {
		svc.Annotations[servicetopology.AnnotationServiceTopologyKey] = servicetopology.AnnotationServiceTopologyValueNode
		if _, err := ki.kubeClient.CoreV1().Services("kube-system").Update(context.TODO(), svc, metav1.UpdateOptions{}); err != nil {
			return err
		}
	}

	// kubectl patch deployment coredns -n kube-system  -p '{"spec": {"template": {"spec": {"volumes": [{"configMap":{"name":"yurt-tunnel-nodes"},"name": "edge"}]}}}}'
	// kubectl patch deployment coredns -n kube-system   -p '{"spec": { "template": { "spec": { "containers": [{"name":"coredns","volumeMounts": [{"mountPath": "/etc/edge", "name": "edge", "readOnly": true }]}]}}}}'
	dp, err := ki.kubeClient.AppsV1().Deployments("kube-system").Get(context.TODO(), "coredns", metav1.GetOptions{})
	if err != nil {
		return err
	}

	if dp != nil {
		dp.Spec.Template.Spec.HostNetwork = true
		hasEdgeVolume := false
		for i := range dp.Spec.Template.Spec.Volumes {
			if dp.Spec.Template.Spec.Volumes[i].Name == "edge" {
				hasEdgeVolume = true
				break
			}
		}
		if !hasEdgeVolume {
			dp.Spec.Template.Spec.Volumes = append(dp.Spec.Template.Spec.Volumes, v1.Volume{
				Name: "edge",
				VolumeSource: v1.VolumeSource{
					ConfigMap: &v1.ConfigMapVolumeSource{
						LocalObjectReference: v1.LocalObjectReference{
							Name: "yurt-tunnel-nodes",
						},
					},
				},
			})
		}
		hasEdgeVolumeMount := false
		containerIndex := 0
		for i := range dp.Spec.Template.Spec.Containers {
			if dp.Spec.Template.Spec.Containers[i].Name == "coredns" {
				for j := range dp.Spec.Template.Spec.Containers[i].VolumeMounts {
					if dp.Spec.Template.Spec.Containers[i].VolumeMounts[j].Name == "edge" {
						hasEdgeVolumeMount = true
						containerIndex = i
						break
					}
				}
			}
			if hasEdgeVolumeMount {
				break
			}
		}
		if !hasEdgeVolumeMount {
			dp.Spec.Template.Spec.Containers[containerIndex].VolumeMounts = append(dp.Spec.Template.Spec.Containers[containerIndex].VolumeMounts,
				v1.VolumeMount{
					Name:      "edge",
					MountPath: "/etc/edge",
					ReadOnly:  true,
				})
		}

		if !hasEdgeVolume || !hasEdgeVolumeMount {
			_, err = ki.kubeClient.AppsV1().Deployments("kube-system").Update(context.TODO(), dp, metav1.UpdateOptions{})
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (ki *Initializer) ConfigureKubeProxyAddon() error {
	// configure configmap kube-system/kube-proxy in order to make kube-proxy access kube-apiserver by going through yurthub
	cm, err := ki.kubeClient.CoreV1().ConfigMaps("kube-system").Get(context.TODO(), "kube-proxy", metav1.GetOptions{})
	if err != nil {
		return err
	}
	if cm != nil && strings.Contains(cm.Data["config.conf"], "kubeconfig") {
		lines := strings.Split(cm.Data["config.conf"], "\n")
		for i := range lines {
			if strings.Contains(lines[i], "kubeconfig:") {
				lines = append(lines[:i], lines[i+1:]...)
				break
			}
		}
		cm.Data["config.conf"] = strings.Join(lines, "\n")

		// update kube-proxy configmap
		_, err = ki.kubeClient.CoreV1().ConfigMaps("kube-system").Update(context.TODO(), cm, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to configure kube-proxy configmap, %w", err)
		}
	}
	return nil
}

func (ki *Initializer) deployOpenYurt() error {
	converter := &ClusterConverter{
		ClientSet:                  ki.kubeClient,
		CloudNodes:                 ki.CloudNodes,
		EdgeNodes:                  ki.EdgeNodes,
		WaitServantJobTimeout:      kubeutil.DefaultWaitServantJobTimeout,
		YurthubHealthCheckTimeout:  defaultYurthubHealthCheckTimeout,
		KubeConfigPath:             ki.KubeConfig,
		YurtTunnelAgentImage:       ki.YurtTunnelAgentImage,
		YurtTunnelServerImage:      ki.YurtTunnelServerImage,
		YurtControllerManagerImage: ki.YurtControllerManagerImage,
		NodeServantImage:           ki.NodeServantImage,
		YurthubImage:               ki.YurtHubImage,
		EnableDummyIf:              ki.EnableDummyIf,
	}
	if err := converter.Run(); err != nil {
		klog.Errorf("errors occurred when deploying openyurt components")
		return err
	}
	return nil
}

func (ki *Initializer) loadImagesToKindNodes(images, nodes []string) error {
	for _, image := range images {
		if image == "" {
			// if image == "", it's the responsibility of kind to pull images from registry.
			continue
		}
		if err := ki.operator.KindLoadDockerImage(ki.ClusterName, image, nodes); err != nil {
			return err
		}
	}
	return nil
}

func getGoBinPath() (string, error) {
	gopath, err := exec.Command("bash", "-c", "go env GOPATH").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get GOPATH, %w", err)
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
	if exist, path := checkIfKindAt("kind"); exist {
		kindPath = path
		return kindPath, nil
	} else {
		goBinPath, err := getGoBinPath()
		if err != nil {
			klog.Fatal("failed to get go bin path, %s", err)
		}

		if exist, path := checkIfKindAt(goBinPath + "/kind"); exist {
			kindPath = path
		}
	}

	if len(kindPath) == 0 {
		return kindPath, fmt.Errorf("cannot find valid kind cmd, try to install it")
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
