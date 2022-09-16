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
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	kubeclientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"

	nodeservant "github.com/openyurtio/openyurt/pkg/node-servant"
	strutil "github.com/openyurtio/openyurt/pkg/util/strings"
	tmplutil "github.com/openyurtio/openyurt/pkg/util/templates"
	"github.com/openyurtio/openyurt/pkg/yurtctl/constants"
	kubeutil "github.com/openyurtio/openyurt/pkg/yurtctl/util/kubernetes"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/servicetopology"
)

var (
	validKubernetesVersions = []string{
		"v1.17",
		"v1.18",
		"v1.19",
		"v1.20",
		"v1.21",
		"v1.22",
		"v1.23",
	}
	validOpenYurtVersions = []string{
		"v0.5.0",
		"v0.6.0",
		"v0.6.1",
		"v0.7.0",
		"v0.7.1",
		"v1.0.0",
		"latest",
	}
	validKindVersions = []string{
		"v0.11.1",
		"v0.12.0",
	}

	kindNodeImageMap = map[string]map[string]string{
		"v0.11.1": {
			"v1.17": "kindest/node:v1.17.17@sha256:66f1d0d91a88b8a001811e2f1054af60eef3b669a9a74f9b6db871f2f1eeed00",
			"v1.18": "kindest/node:v1.18.19@sha256:7af1492e19b3192a79f606e43c35fb741e520d195f96399284515f077b3b622c",
			"v1.19": "kindest/node:v1.19.11@sha256:07db187ae84b4b7de440a73886f008cf903fcf5764ba8106a9fd5243d6f32729",
			"v1.20": "kindest/node:v1.20.7@sha256:cbeaf907fc78ac97ce7b625e4bf0de16e3ea725daf6b04f930bd14c67c671ff9",
			"v1.21": "kindest/node:v1.21.1@sha256:69860bda5563ac81e3c0057d654b5253219618a22ec3a346306239bba8cfa1a6",
		},
		"v0.12.0": {
			"v1.17": "kindest/node:v1.17.17@sha256:e477ee64df5731aa4ef4deabbafc34e8d9a686b49178f726563598344a3898d5",
			"v1.18": "kindest/node:v1.18.20@sha256:e3dca5e16116d11363e31639640042a9b1bd2c90f85717a7fc66be34089a8169",
			"v1.19": "kindest/node:v1.19.16@sha256:81f552397c1e6c1f293f967ecb1344d8857613fb978f963c30e907c32f598467",
			"v1.20": "kindest/node:v1.20.15@sha256:393bb9096c6c4d723bb17bceb0896407d7db581532d11ea2839c80b28e5d8deb",
			"v1.21": "kindest/node:v1.21.10@sha256:84709f09756ba4f863769bdcabe5edafc2ada72d3c8c44d6515fc581b66b029c",
			"v1.22": "kindest/node:v1.22.7@sha256:1dfd72d193bf7da64765fd2f2898f78663b9ba366c2aa74be1fd7498a1873166",
			"v1.23": "kindest/node:v1.23.4@sha256:0e34f0d0fd448aa2f2819cfd74e99fe5793a6e4938b328f657c8e3f81ee0dfb9",
		},
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

func NewKindInitCMD(out io.Writer) *cobra.Command {
	o := newKindOptions()

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Using kind to setup OpenYurt cluster for test",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := o.Validate(); err != nil {
				return err
			}
			initializer := newKindInitializer(out, o.Config())
			if err := initializer.Run(); err != nil {
				return err
			}
			return nil
		},
		Args: cobra.NoArgs,
	}
	cmd.SetOut(out)
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
	DisableDefaultCNI bool
}

func newKindOptions() *kindOptions {
	return &kindOptions{
		KindConfigPath:    fmt.Sprintf("%s/kindconfig.yaml", constants.TmpDownloadDir),
		NodeNum:           2,
		ClusterName:       "openyurt",
		OpenYurtVersion:   constants.DefaultOpenYurtVersion,
		KubernetesVersion: "v1.22",
		UseLocalImages:    false,
		IgnoreError:       false,
		EnableDummyIf:     true,
		DisableDefaultCNI: false,
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
		NodesNum:                   o.NodeNum,
		ClusterName:                o.ClusterName,
		KubernetesVersion:          o.KubernetesVersion,
		UseLocalImage:              o.UseLocalImages,
		YurtHubImage:               fmt.Sprintf(yurtHubImageFormat, o.OpenYurtVersion),
		YurtControllerManagerImage: fmt.Sprintf(yurtControllerManagerImageFormat, o.OpenYurtVersion),
		NodeServantImage:           fmt.Sprintf(nodeServantImageFormat, o.OpenYurtVersion),
		YurtTunnelServerImage:      fmt.Sprintf(yurtTunnelServerImageFormat, o.OpenYurtVersion),
		YurtTunnelAgentImage:       fmt.Sprintf(yurtTunnelAgentImageFormat, o.OpenYurtVersion),
		EnableDummyIf:              o.EnableDummyIf,
		DisableDefaultCNI:          o.DisableDefaultCNI,
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
	flagset.BoolVar(&o.DisableDefaultCNI, "disable-default-cni", o.DisableDefaultCNI,
		"Disable the default cni of kind cluster which is kindnet. "+
			"If this option is set, you should check the ready status of pods by yourself after installing your CNI.")
}

type initializerConfig struct {
	CloudNodes                 []string
	EdgeNodes                  []string
	KindConfigPath             string
	KubeConfig                 string
	NodesNum                   int
	ClusterName                string
	KubernetesVersion          string
	NodeImage                  string
	UseLocalImage              bool
	YurtHubImage               string
	YurtControllerManagerImage string
	NodeServantImage           string
	YurtTunnelServerImage      string
	YurtTunnelAgentImage       string
	EnableDummyIf              bool
	DisableDefaultCNI          bool
}

type Initializer struct {
	initializerConfig
	out        io.Writer
	operator   *KindOperator
	kubeClient kubeclientset.Interface
}

func newKindInitializer(out io.Writer, cfg *initializerConfig) *Initializer {
	return &Initializer{
		initializerConfig: *cfg,
		out:               out,
		operator:          NewKindOperator("", cfg.KubeConfig),
	}
}

func (ki *Initializer) Run() error {
	klog.Info("Start to install kind")
	if err := ki.operator.KindInstall(); err != nil {
		return err
	}

	klog.Info("Start to prepare kind node image")
	if err := ki.prepareKindNodeImage(); err != nil {
		return err
	}

	klog.Info("Start to prepare config file for kind")
	if err := ki.prepareKindConfigFile(ki.KindConfigPath); err != nil {
		return err
	}

	klog.Info("Start to create cluster with kind")
	if err := ki.operator.KindCreateClusterWithConfig(ki.out, ki.KindConfigPath); err != nil {
		return err
	}

	klog.Info("Start to prepare kube client")
	kubeconfig, err := clientcmd.BuildConfigFromFlags("", ki.KubeConfig)
	if err != nil {
		return err
	}
	ki.kubeClient, err = kubeclientset.NewForConfig(kubeconfig)
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

func (ki *Initializer) prepareKindNodeImage() error {
	kindVer, err := ki.operator.KindVersion()
	if err != nil {
		return err
	}
	ki.NodeImage = kindNodeImageMap[kindVer][ki.KubernetesVersion]
	if len(ki.NodeImage) == 0 {
		return fmt.Errorf("failed to get node image by kind version= %s and kubernetes version= %s", kindVer, ki.KubernetesVersion)
	}

	return nil
}

func (ki *Initializer) prepareKindConfigFile(kindConfigPath string) error {
	kindConfigDir := filepath.Dir(kindConfigPath)
	if err := os.MkdirAll(kindConfigDir, constants.DirMode); err != nil {
		return err
	}
	kindConfigContent, err := tmplutil.SubsituteTemplate(constants.OpenYurtKindConfig, map[string]string{
		"kind_node_image":     ki.NodeImage,
		"cluster_name":        ki.ClusterName,
		"disable_default_cni": fmt.Sprintf("%v", ki.DisableDefaultCNI),
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

	// If we disable default cni, nodes will not be ready and the coredns pod always be in pending.
	// The health check for coreDNS should be done by someone who will install CNI.
	if !ki.DisableDefaultCNI {
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
	return nil
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
		if err := ki.operator.KindLoadDockerImage(ki.out, ki.ClusterName, image, nodes); err != nil {
			return err
		}
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

	if !strings.HasPrefix(ver, "v") {
		ver = fmt.Sprintf("v%s", ver)
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
