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
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	bootstrapapi "k8s.io/cluster-bootstrap/token/api"
	"k8s.io/klog"
	clusterinfophase "k8s.io/kubernetes/cmd/kubeadm/app/phases/bootstraptoken/clusterinfo"
	nodeutil "k8s.io/kubernetes/pkg/controller/util/node"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/yurtctl/lock"
	enutil "github.com/openyurtio/openyurt/pkg/yurtctl/util/edgenode"
	kubeutil "github.com/openyurtio/openyurt/pkg/yurtctl/util/kubernetes"
	strutil "github.com/openyurtio/openyurt/pkg/yurtctl/util/strings"
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
	clientSet                  *kubernetes.Clientset
	CloudNodes                 []string
	Provider                   Provider
	YurhubImage                string
	YurthubHealthCheckTimeout  time.Duration
	YurtControllerManagerImage string
	YurctlServantImage         string
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
				klog.Fatalf("fail to complete the convert option: %s", err)
			}
			if err := co.Validate(); err != nil {
				klog.Fatalf("convert option is invalid: %s", err)
			}
			if err := co.RunConvert(); err != nil {
				klog.Fatalf("fail to convert kubernetes to yurt: %s", err)
			}
		},
	}

	cmd.AddCommand(NewConvertEdgeNodeCmd())

	cmd.Flags().StringP("cloud-nodes", "c", "",
		"The list of cloud nodes.(e.g. -c cloudnode1,cloudnode2)")
	cmd.Flags().StringP("provider", "p", "minikube",
		"The provider of the original Kubernetes cluster.")
	cmd.Flags().String("yurthub-image",
		"openyurt/yurthub:latest",
		"The yurthub image.")
	cmd.Flags().Duration("yurthub-healthcheck-timeout", defaultYurthubHealthCheckTimeout,
		"The timeout for yurthub health check.")
	cmd.Flags().String("yurt-controller-manager-image",
		"openyurt/yurt-controller-manager:latest",
		"The yurt-controller-manager image.")
	cmd.Flags().String("yurtctl-servant-image",
		"openyurt/yurtctl-servant:latest",
		"The yurtctl-servant image.")
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
		"if set, yurttunnel will be deployed.")
	cmd.Flags().BoolP("enable-app-manager", "e", false,
		"if set, yurtappmanager will be deployed.")
	cmd.Flags().String("yurt-app-manager-image",
		"openyurt/yurt-app-manager:v0.4.0",
		"The yurt-app-manager image.")
	cmd.Flags().String("system-architecture", "amd64",
		"The system architecture of cloud nodes.")

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

	ycsi, err := flags.GetString("yurtctl-servant-image")
	if err != nil {
		return err
	}
	co.YurctlServantImage = ycsi

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

	podManifestPath, err := enutil.GetPodManifestPath(co.KubeadmConfPath)
	if err != nil {
		return err
	}
	co.PodMainfestPath = podManifestPath

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

	// 1.2. check the state of worker nodes and kcm nodes
	nodeLst, err := co.clientSet.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return
	}
	for _, node := range nodeLst.Items {
		if !strutil.IsInStringLst(co.CloudNodes, node.GetName()) || strutil.IsInStringLst(kcmNodeNames, node.GetName()) {
			_, condition := nodeutil.GetNodeCondition(&node.Status, v1.NodeReady)
			if condition == nil || condition.Status != v1.ConditionTrue {
				klog.Errorf("Cannot do the convert, the status of worker node or kube-controller-manager node: %s is not 'Ready'.", node.Name)
				return
			}
		}
	}
	klog.V(4).Info("the status of worker nodes and kube-controller-manager nodes are satisfied")

	// 2. label nodes as cloud node or edge node
	var edgeNodeNames []string
	for _, node := range nodeLst.Items {
		if strutil.IsInStringLst(co.CloudNodes, node.GetName()) {
			// label node as cloud node
			klog.Infof("mark %s as the cloud-node", node.GetName())
			if _, err = kubeutil.LabelNode(co.clientSet,
				&node, projectinfo.GetEdgeWorkerLabelKey(), "false"); err != nil {
				return
			}
			continue
		}
		edgeNodeNames = append(edgeNodeNames, node.GetName())
	}

	// 3. deploy yurt controller manager
	if err = kubeutil.DeployYurtControllerManager(co.clientSet, co.YurtControllerManagerImage); err != nil {
		klog.Errorf("fail to deploy yurtcontrollermanager: %s", err)
		return
	}
	// 4. disable node-controller
	ctx := map[string]string{
		"action":                "disable",
		"yurtctl_servant_image": co.YurctlServantImage,
		"pod_manifest_path":     co.PodMainfestPath,
	}
	if err = kubeutil.RunServantJobs(co.clientSet, ctx, kcmNodeNames); err != nil {
		klog.Errorf("fail to run DisableNodeControllerJobs: %s", err)
		return
	}
	klog.Info("complete disabling node-controller")

	// 5. deploy the yurttunnel if required
	if co.DeployTunnel {
		if err = kubeutil.DeployYurttunnelServer(co.clientSet,
			co.CloudNodes,
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
		klog.Errorf("fail to prepre cluster-info configmap, %v", err)
		return
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

	// 9. deploy yurt-hub and reset the kubelet service
	klog.Infof("deploying the yurt-hub and resetting the kubelet service...")
	joinToken, err := kubeutil.GetOrCreateJoinTokenString(co.clientSet)
	if err != nil {
		return err
	}

	ctx = map[string]string{
		"provider":              string(co.Provider),
		"action":                "convert",
		"yurtctl_servant_image": co.YurctlServantImage,
		"yurthub_image":         co.YurhubImage,
		"joinToken":             joinToken,
		"kubeadm_conf_path":     co.KubeadmConfPath,
	}

	if co.YurthubHealthCheckTimeout != defaultYurthubHealthCheckTimeout {
		ctx["yurthub_healthcheck_timeout"] = co.YurthubHealthCheckTimeout.String()
	}

	if err = kubeutil.RunServantJobs(co.clientSet, ctx, edgeNodeNames); err != nil {
		klog.Errorf("fail to run ServantJobs: %s", err)
		return
	}
	klog.Info("complete deploying yurt-hub")

	return
}

// prepareClusterInfoConfigMap will create cluster-info configmap in kube-public namespace if it does not exist
func prepareClusterInfoConfigMap(client *kubernetes.Clientset, file string) error {
	info, err := client.CoreV1().ConfigMaps(metav1.NamespacePublic).Get(context.Background(), bootstrapapi.ConfigMapClusterInfo, metav1.GetOptions{})
	if err != nil && apierrors.IsNotFound(err) {
		// Create the cluster-info ConfigMap with the associated RBAC rules
		if err := clusterinfophase.CreateBootstrapConfigMapIfNotExists(client, file); err != nil {
			return fmt.Errorf("error creating bootstrap ConfigMap, %v", err)
		}
		if err := clusterinfophase.CreateClusterInfoRBACRules(client); err != nil {
			return fmt.Errorf("error creating clusterinfo RBAC rules, %v", err)
		}
	} else if err != nil || info == nil {
		klog.Errorf("fail to get configmap, %v", err)
		return fmt.Errorf("fail to get configmap, %v", err)
	} else {
		klog.Infof("%s/%s configmap already exists, skip to prepare it", info.Namespace, info.Name)
	}

	return nil
}
