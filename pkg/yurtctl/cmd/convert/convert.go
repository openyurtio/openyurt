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

package convert

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	bootstrapapi "k8s.io/cluster-bootstrap/token/api"
	"k8s.io/klog/v2"

	nodeutil "github.com/openyurtio/openyurt/pkg/controller/util/node"
	nodeservant "github.com/openyurtio/openyurt/pkg/node-servant"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/util/kubeadmapi"
	"github.com/openyurtio/openyurt/pkg/yurtctl/constants"
	"github.com/openyurtio/openyurt/pkg/yurtctl/lock"
	enutil "github.com/openyurtio/openyurt/pkg/yurtctl/util/edgenode"
	kubeutil "github.com/openyurtio/openyurt/pkg/yurtctl/util/kubernetes"
	strutil "github.com/openyurtio/openyurt/pkg/yurtctl/util/strings"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

// Provider signifies the provider type
type Provider string

const (
	// ProviderMinikube is used if the target kubernetes is run on minikube
	ProviderMinikube Provider = "minikube"
	// ProviderACK is used if the target kubernetes is run on ack
	ProviderACK     Provider = "ack"
	ProviderKubeadm Provider = "kubeadm"
	// ProviderKind is used if the target kubernetes is run on kind
	ProviderKind Provider = "kind"
)

const (
	// defaultYurthubHealthCheckTimeout defines the default timeout for yurthub health check phase
	defaultYurthubHealthCheckTimeout = 2 * time.Minute
)

// ConvertOptions has the information that required by convert operation
type ConvertOptions struct {
	clientSet  *kubernetes.Clientset
	CloudNodes []string
	// AutonomousNodes stores the names of edge nodes that are going to be marked as autonomous.
	// If empty, all edge nodes will be marked as autonomous.
	AutonomousNodes            []string
	Provider                   Provider
	YurhubImage                string
	YurthubHealthCheckTimeout  time.Duration
	YurtControllerManagerImage string
	NodeServantImage           string
	YurttunnelServerImage      string
	YurttunnelServerAddress    string
	YurttunnelAgentImage       string
	PodMainfestPath            string
	KubeadmConfPath            string
	DeployTunnel               bool
	kubeConfigPath             string
	EnableAppManager           bool
	YurtAppManagerImage        string
	SystemArchitecture         string
	yurtAppManagerClientSet    dynamic.Interface
}

// NewConvertOptions creates a new ConvertOptions
func NewConvertOptions() *ConvertOptions {
	return &ConvertOptions{
		CloudNodes: []string{},
	}
}

// NewConvertCmd generates a new convert command
func NewConvertCmd() *cobra.Command {
	co := NewConvertOptions()
	cmd := &cobra.Command{
		Use:   "convert -c CLOUDNODES",
		Short: "Converts the kubernetes cluster to a yurt cluster",
		Run: func(cmd *cobra.Command, _ []string) {
			if err := co.Complete(cmd.Flags()); err != nil {
				klog.Errorf("fail to complete the convert option: %s", err)
				os.Exit(1)
			}
			if err := co.Validate(); err != nil {
				klog.Errorf("convert option is invalid: %s", err)
				os.Exit(1)
			}
			if err := co.RunConvert(); err != nil {
				klog.Errorf("fail to convert kubernetes to yurt: %s", err)
				os.Exit(1)
			}
		},
	}

	cmd.Flags().StringP("cloud-nodes", "c", "",
		"The list of cloud nodes.(e.g. -c cloudnode1,cloudnode2)")
	cmd.Flags().StringP("provider", "p", "minikube",
		"The provider of the original Kubernetes cluster.")
	cmd.Flags().String("yurthub-image",
		"openyurt/yurthub:latest",
		"The yurthub image.")
	cmd.Flags().Duration("yurthub-healthcheck-timeout", defaultYurthubHealthCheckTimeout,
		"The timeout for yurthub health check.")
	cmd.Flags().Duration("wait-servant-job-timeout", kubeutil.WaitServantJobTimeout,
		"The timeout for servant-job run check.")
	cmd.Flags().String("yurt-controller-manager-image",
		"openyurt/yurt-controller-manager:latest",
		"The yurt-controller-manager image.")
	cmd.Flags().String("node-servant-image",
		"openyurt/node-servant:latest",
		"The node-servant image.")
	cmd.Flags().String("kubeadm-conf-path",
		"/etc/systemd/system/kubelet.service.d/10-kubeadm.conf",
		"The path to kubelet service conf that is used by kubelet component to join the cluster on the edge node.")
	cmd.Flags().String("yurt-tunnel-server-image",
		"openyurt/yurt-tunnel-server:latest",
		"The yurt-tunnel-server image.")
	cmd.Flags().String("yurt-tunnel-server-address",
		"",
		"The yurt-tunnel-server address.")
	cmd.Flags().String("yurt-tunnel-agent-image",
		"openyurt/yurt-tunnel-agent:latest",
		"The yurt-tunnel-agent image.")
	cmd.Flags().BoolP("deploy-yurttunnel", "t", false,
		"If set, yurttunnel will be deployed.")
	cmd.Flags().BoolP("enable-app-manager", "e", false,
		"If set, yurtappmanager will be deployed.")
	cmd.Flags().String("yurt-app-manager-image",
		"openyurt/yurt-app-manager:v0.4.0",
		"The yurt-app-manager image.")
	cmd.Flags().String("system-architecture", "amd64",
		"The system architecture of cloud nodes.")
	cmd.Flags().StringP("autonomous-nodes", "a", "",
		"The list of nodes that will be marked as autonomous. If not set, all edge nodes will be marked as autonomous."+
			"(e.g. -a autonomousnode1,autonomousnode2)")

	return cmd
}

// Complete completes all the required options
func (co *ConvertOptions) Complete(flags *pflag.FlagSet) error {
	cnStr, err := flags.GetString("cloud-nodes")
	if err != nil {
		return err
	}
	if cnStr != "" {
		co.CloudNodes = strings.Split(cnStr, ",")
	} else {
		err := fmt.Errorf("The '--cloud-nodes' parameter cannot be empty.Please specify the cloud node first, and then execute the yurtctl convert command")
		return err
	}

	anStr, err := flags.GetString("autonomous-nodes")
	if err != nil {
		return err
	}
	if anStr == "" {
		co.AutonomousNodes = []string{}
	} else {
		co.AutonomousNodes = strings.Split(anStr, ",")
	}

	dt, err := flags.GetBool("deploy-yurttunnel")
	if err != nil {
		return err
	}
	co.DeployTunnel = dt

	eam, err := flags.GetBool("enable-app-manager")
	if err != nil {
		return err
	}
	co.EnableAppManager = eam

	pStr, err := flags.GetString("provider")
	if err != nil {
		return err
	}
	co.Provider = Provider(pStr)

	yhi, err := flags.GetString("yurthub-image")
	if err != nil {
		return err
	}
	co.YurhubImage = yhi

	yurthubHealthCheckTimeout, err := flags.GetDuration("yurthub-healthcheck-timeout")
	if err != nil {
		return err
	}
	co.YurthubHealthCheckTimeout = yurthubHealthCheckTimeout

	ycmi, err := flags.GetString("yurt-controller-manager-image")
	if err != nil {
		return err
	}
	co.YurtControllerManagerImage = ycmi

	nsi, err := flags.GetString("node-servant-image")
	if err != nil {
		return err
	}
	co.NodeServantImage = nsi

	ytsi, err := flags.GetString("yurt-tunnel-server-image")
	if err != nil {
		return err
	}
	co.YurttunnelServerImage = ytsi

	ytsa, err := flags.GetString("yurt-tunnel-server-address")
	if err != nil {
		return err
	}
	co.YurttunnelServerAddress = ytsa

	ytai, err := flags.GetString("yurt-tunnel-agent-image")
	if err != nil {
		return err
	}
	co.YurttunnelAgentImage = ytai

	yami, err := flags.GetString("yurt-app-manager-image")
	if err != nil {
		return err
	}
	co.YurtAppManagerImage = yami

	kcp, err := flags.GetString("kubeadm-conf-path")
	if err != nil {
		return err
	}
	co.KubeadmConfPath = kcp

	co.PodMainfestPath = enutil.GetPodManifestPath()

	sa, err := flags.GetString("system-architecture")
	if err != nil {
		return err
	}
	co.SystemArchitecture = sa

	// parse kubeconfig and generate the clientset
	co.clientSet, err = kubeutil.GenClientSet(flags)
	if err != nil {
		return err
	}

	// parse kubeconfig and generate the yurtappmanagerclientset
	co.yurtAppManagerClientSet, err = kubeutil.GenDynamicClientSet(flags)
	if err != nil {
		return err
	}

	// prepare path of cluster kubeconfig file
	co.kubeConfigPath, err = kubeutil.PrepareKubeConfigPath(flags)
	if err != nil {
		return err
	}
	return nil
}

// Validate makes sure provided values for ConvertOptions are valid
func (co *ConvertOptions) Validate() error {
	if co.Provider != ProviderMinikube && co.Provider != ProviderACK &&
		co.Provider != ProviderKubeadm && co.Provider != ProviderKind {
		return fmt.Errorf("unknown provider: %s, valid providers are: minikube, ack, kubeadm, kind",
			co.Provider)
	}
	if co.YurttunnelServerAddress != "" {
		if _, _, err := net.SplitHostPort(co.YurttunnelServerAddress); err != nil {
			return fmt.Errorf("invalid --yurt-tunnel-server-address: %s", err)
		}
	}

	return nil
}

// RunConvert performs the conversion
func (co *ConvertOptions) RunConvert() (err error) {
	if err = lock.AcquireLock(co.clientSet); err != nil {
		return
	}
	defer func() {
		if releaseLockErr := lock.ReleaseLock(co.clientSet); releaseLockErr != nil {
			klog.Error(releaseLockErr)
		}
	}()
	klog.V(4).Info("successfully acquire the lock")

	// 1. check the server version
	if err = kubeutil.ValidateServerVersion(co.clientSet); err != nil {
		return
	}
	klog.V(4).Info("the server version is valid")

	// 1.1. get kube-controller-manager HA nodes
	kcmNodeNames, err := kubeutil.GetKubeControllerManagerHANodes(co.clientSet)
	if err != nil {
		return
	}

	// edgeNodeNames stores the node names of all edge nodes.
	var edgeNodeNames []string
	// 1.2 check the nodes status and label
	nodeLst, err := co.clientSet.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return
	}
	nodeNameSet := make(map[string]struct{})
	for _, node := range nodeLst.Items {
		nodeNameSet[node.GetName()] = struct{}{}
		if !isNodeReady(&node.Status) {
			err = fmt.Errorf("the status of node: %s is not 'Ready'", node.Name)
			return
		}
		_, ok := node.Labels[projectinfo.GetEdgeWorkerLabelKey()]
		if ok {
			return fmt.Errorf("the node %s has already been labeled as a OpenYurt node", node.GetName())
		}
		if !strutil.IsInStringLst(co.CloudNodes, node.GetName()) {
			edgeNodeNames = append(edgeNodeNames, node.GetName())
		}
	}
	// make sure all cloud nodes are kubernetes nodes
	for _, v := range co.CloudNodes {
		if _, ok := nodeNameSet[v]; !ok {
			return fmt.Errorf("the node %s is not a kubernetes node", v)
		}
	}
	klog.V(4).Info("the status of all nodes are satisfied")

	var autonomousNodes []string
	// 1.3 check if autonomous nodes are valid
	for _, v := range co.AutonomousNodes {
		if strutil.IsInStringLst(co.CloudNodes, v) {
			return fmt.Errorf("can't make cloud node %s autonomous", v)
		}
		if !strutil.IsInStringLst(edgeNodeNames, v) {
			return fmt.Errorf("can't make unknown node %s autonomous", v)
		}
		autonomousNodes = append(autonomousNodes, v)
	}
	// If empty, mark all edge nodes as autonomous
	if len(co.AutonomousNodes) == 0 {
		autonomousNodes = make([]string, len(edgeNodeNames))
		copy(autonomousNodes, edgeNodeNames)
	}

	// 2. label nodes as cloud node or edge node
	for _, node := range nodeLst.Items {
		if strutil.IsInStringLst(co.CloudNodes, node.GetName()) {
			// label node as cloud node
			klog.Infof("mark %s as the cloud-node", node.GetName())
			if _, err = kubeutil.LabelNode(co.clientSet,
				&node, projectinfo.GetEdgeWorkerLabelKey(), "false"); err != nil {
				return
			}
		} else {
			// label node as edge node
			var updatedNode *v1.Node
			klog.Infof("mark %s as the edge-node", node.GetName())
			if updatedNode, err = kubeutil.LabelNode(co.clientSet,
				&node, projectinfo.GetEdgeWorkerLabelKey(), "true"); err != nil {
				return
			}
			if strutil.IsInStringLst(autonomousNodes, node.GetName()) {
				// mark edge node as autonomous
				klog.Infof("mark %s as autonomous", node.GetName())
				if _, err = kubeutil.AnnotateNode(co.clientSet,
					updatedNode, constants.AnnotationAutonomy, "true"); err != nil {
					return
				}
			}
		}
	}

	// 3. deploy yurt controller manager
	if err = kubeutil.DeployYurtControllerManager(co.clientSet, co.YurtControllerManagerImage); err != nil {
		return fmt.Errorf("fail to deploy yurtcontrollermanager: %s", err)
	}
	// 4. disable node-controller
	if err = kubeutil.RunServantJobs(co.clientSet, func(nodeName string) (*batchv1.Job, error) {
		ctx := map[string]string{
			"node_servant_image": co.NodeServantImage,
			"pod_manifest_path":  co.PodMainfestPath,
		}
		return kubeutil.RenderServantJob("disable", ctx, nodeName)
	}, kcmNodeNames); err != nil {
		return fmt.Errorf("fail to run DisableNodeControllerJobs: %s", err)
	}
	klog.Info("complete disabling node-controller")

	// 5. deploy the yurttunnel if required
	if co.DeployTunnel {
		var certIP string
		if co.YurttunnelServerAddress != "" {
			certIP, _, _ = net.SplitHostPort(co.YurttunnelServerAddress)
		}
		if err = kubeutil.DeployYurttunnelServer(co.clientSet,
			certIP,
			co.YurttunnelServerImage,
			co.SystemArchitecture); err != nil {
			err = fmt.Errorf("fail to deploy the yurt-tunnel-server: %s", err)
			return
		}
		klog.Info("yurt-tunnel-server is deployed")
		// we will deploy yurt-tunnel-agent on every edge node
		if err = kubeutil.DeployYurttunnelAgent(co.clientSet,
			co.YurttunnelServerAddress,
			co.YurttunnelAgentImage); err != nil {
			err = fmt.Errorf("fail to deploy the yurt-tunnel-agent: %s", err)
			return
		}
		klog.Info("yurt-tunnel-agent is deployed")
	}

	// 6. prepare kube-public/cluster-info configmap before convert
	err = prepareClusterInfoConfigMap(co.clientSet, co.kubeConfigPath)
	if err != nil {
		return fmt.Errorf("fail to prepre cluster-info configmap, %v", err)
	}

	//7. deploy the yurtappmanager if required
	if co.EnableAppManager {
		if err = kubeutil.DeployYurtAppManager(co.clientSet,
			co.YurtAppManagerImage,
			co.yurtAppManagerClientSet,
			co.SystemArchitecture); err != nil {
			err = fmt.Errorf("fail to deploy the yurt-app-manager: %s", err)
			return
		}
		klog.Info("yurt-app-manager is deployed")
	}

	// 8. deploy global settings(like RBAC, configmap) for yurthub
	if err = kubeutil.DeployYurthubSetting(co.clientSet); err != nil {
		return err
	}

	joinToken, err := kubeutil.GetOrCreateJoinTokenString(co.clientSet)
	if err != nil {
		return err
	}

	convertCtx := map[string]string{
		"node_servant_image": co.NodeServantImage,
		"yurthub_image":      co.YurhubImage,
		"joinToken":          joinToken,
		"kubeadm_conf_path":  co.KubeadmConfPath,
	}

	if co.YurthubHealthCheckTimeout != defaultYurthubHealthCheckTimeout {
		convertCtx["yurthub_healthcheck_timeout"] = co.YurthubHealthCheckTimeout.String()
	}

	// 9. deploy yurt-hub and reset the kubelet service on edge nodes.
	if len(edgeNodeNames) != 0 {
		klog.Infof("deploying the yurt-hub and resetting the kubelet service on edge nodes...")
		convertCtx["working_mode"] = string(util.WorkingModeEdge)
		if err = kubeutil.RunServantJobs(co.clientSet, func(nodeName string) (*batchv1.Job, error) {
			return nodeservant.RenderNodeServantJob("convert", convertCtx, nodeName)
		}, edgeNodeNames); err != nil {
			return fmt.Errorf("fail to run ServantJobs: %s", err)
		}
		klog.Info("complete deploying yurt-hub on edge nodes")
	}

	// 10. deploy yurt-hub and reset the kubelet service on cloud nodes
	klog.Infof("deploying the yurt-hub and resetting the kubelet service on cloud nodes")
	convertCtx["working_mode"] = string(util.WorkingModeCloud)
	if err = kubeutil.RunServantJobs(co.clientSet, func(nodeName string) (*batchv1.Job, error) {
		return nodeservant.RenderNodeServantJob("convert", convertCtx, nodeName)
	}, co.CloudNodes); err != nil {
		return fmt.Errorf("fail to run ServantJobs: %s", err)
	}
	klog.Info("complete deploying yurt-hub on cloud nodes")

	return
}

// prepareClusterInfoConfigMap will create cluster-info configmap in kube-public namespace if it does not exist
func prepareClusterInfoConfigMap(client *kubernetes.Clientset, file string) error {
	info, err := client.CoreV1().ConfigMaps(metav1.NamespacePublic).Get(context.Background(), bootstrapapi.ConfigMapClusterInfo, metav1.GetOptions{})
	if err != nil && apierrors.IsNotFound(err) {
		// Create the cluster-info ConfigMap with the associated RBAC rules
		if err := kubeadmapi.CreateBootstrapConfigMapIfNotExists(client, file); err != nil {
			return fmt.Errorf("error creating bootstrap ConfigMap, %v", err)
		}
		if err := kubeadmapi.CreateClusterInfoRBACRules(client); err != nil {
			return fmt.Errorf("error creating clusterinfo RBAC rules, %v", err)
		}
	} else if err != nil || info == nil {
		return fmt.Errorf("fail to get configmap, %v", err)
	} else {
		klog.Infof("%s/%s configmap already exists, skip to prepare it", info.Namespace, info.Name)
	}

	return nil
}

func isNodeReady(status *v1.NodeStatus) bool {
	_, condition := nodeutil.GetNodeCondition(status, v1.NodeReady)
	return condition != nil && condition.Status == v1.ConditionTrue
}
