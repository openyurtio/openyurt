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
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	bootstrapapi "k8s.io/cluster-bootstrap/token/api"
	"k8s.io/klog/v2"

	nodeservant "github.com/openyurtio/openyurt/pkg/node-servant"
	"github.com/openyurtio/openyurt/pkg/preflight"
	kubeadmapi "github.com/openyurtio/openyurt/pkg/yurtctl/kubernetes/kubeadm/app/phases/bootstraptoken/clusterinfo"
	"github.com/openyurtio/openyurt/pkg/yurtctl/lock"
	kubeutil "github.com/openyurtio/openyurt/pkg/yurtctl/util/kubernetes"
	strutil "github.com/openyurtio/openyurt/pkg/yurtctl/util/strings"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

const (
	// defaultYurthubHealthCheckTimeout defines the default timeout for yurthub health check phase
	defaultYurthubHealthCheckTimeout = 2 * time.Minute

	latestYurtHubImage               = "openyurt/yurthub:latest"
	latestYurtControllerManagerImage = "openyurt/yurt-controller-manager:latest"
	latestNodeServantImage           = "openyurt/node-servant:latest"
	latestYurtTunnelServerImage      = "openyurt/yurt-tunnel-server:latest"
	latestYurtTunnelAgentImage       = "openyurt/yurt-tunnel-agent:latest"
	versionedYurtAppManagerImage     = "openyurt/yurt-app-manager:v0.4.0"
)

// ClusterConverter do the cluster convert job.
// During the conversion, the pre-check will be performed first, and
// the conversion will be performed only after the pre-check is passed.
type ClusterConverter struct {
	ConvertOptions
}

// NewConvertCmd generates a new convert command
func NewConvertCmd() *cobra.Command {
	co := NewConvertOptions()
	cmd := &cobra.Command{
		Use:   "convert -c CLOUDNODES",
		Short: "Converts the kubernetes cluster to a yurt cluster",
		Run: func(cmd *cobra.Command, _ []string) {
			if err := co.Complete(cmd.Flags()); err != nil {
				klog.Errorf("Fail to complete the convert option: %s", err)
				os.Exit(1)
			}
			if err := co.Validate(); err != nil {
				klog.Errorf("Fail to validate convert option: %s", err)
				os.Exit(1)
			}
			converter := NewClusterConverter(co)
			if err := converter.PreflightCheck(); err != nil {
				klog.Errorf("Fail to run pre-flight checks: %s", err)
				os.Exit(1)
			}
			if err := converter.RunConvert(); err != nil {
				klog.Errorf("Fail to convert the cluster: %s", err)
				os.Exit(1)
			}
		},
		Args: cobra.NoArgs,
	}

	setFlags(cmd)
	return cmd
}

// setFlags sets flags.
func setFlags(cmd *cobra.Command) {
	cmd.Flags().StringP(
		"cloud-nodes", "c", "",
		"The list of cloud nodes.(e.g. -c cloudnode1,cloudnode2)",
	)
	cmd.Flags().StringP(
		"autonomous-nodes", "a", "",
		"The list of nodes that will be marked as autonomous. If not set, all edge nodes will be marked as autonomous."+
			"(e.g. -a autonomousnode1,autonomousnode2)",
	)
	cmd.Flags().String(
		"yurt-tunnel-server-address", "",
		"The yurt-tunnel-server address.",
	)
	cmd.Flags().String(
		"kubeadm-conf-path", "",
		"The path to kubelet service conf that is used by kubelet component to join the cluster on the edge node.",
	)
	cmd.Flags().StringP(
		"provider", "p", "minikube",
		"The provider of the original Kubernetes cluster.",
	)
	cmd.Flags().Duration(
		"yurthub-healthcheck-timeout", defaultYurthubHealthCheckTimeout,
		"The timeout for yurthub health check.",
	)
	cmd.Flags().Duration(
		"wait-servant-job-timeout", kubeutil.DefaultWaitServantJobTimeout,
		"The timeout for servant-job run check.")
	cmd.Flags().String(
		"ignore-preflight-errors", "",
		"A list of checks whose errors will be shown as warnings. Example: 'NodeEdgeWorkerLabel,NodeAutonomy'.Value 'all' ignores errors from all checks.",
	)
	cmd.Flags().BoolP(
		"deploy-yurttunnel", "t", false,
		"If set, yurttunnel will be deployed.",
	)
	cmd.Flags().BoolP(
		"enable-app-manager", "e", false,
		"If set, yurtappmanager will be deployed.",
	)
	cmd.Flags().String(
		"system-architecture", "amd64",
		"The system architecture of cloud nodes.",
	)

	cmd.Flags().String("yurthub-image", latestYurtHubImage, "The yurthub image.")
	cmd.Flags().String("yurt-controller-manager-image", latestYurtControllerManagerImage, "The yurt-controller-manager image.")
	cmd.Flags().String("node-servant-image", latestNodeServantImage, "The node-servant image.")
	cmd.Flags().String("yurt-tunnel-server-image", latestYurtTunnelServerImage, "The yurt-tunnel-server image.")
	cmd.Flags().String("yurt-tunnel-agent-image", latestYurtTunnelAgentImage, "The yurt-tunnel-agent image.")
	cmd.Flags().String("yurt-app-manager-image", versionedYurtAppManagerImage, "The yurt-app-manager image.")
}

func NewClusterConverter(co *ConvertOptions) *ClusterConverter {
	return &ClusterConverter{
		*co,
	}
}

// preflightCheck executes preflight checks logic.
func (c *ClusterConverter) PreflightCheck() error {
	fmt.Println("[preflight] Running pre-flight checks")
	if err := preflight.RunConvertClusterChecks(c.ClientSet, c.IgnorePreflightErrors); err != nil {
		return err
	}

	fmt.Println("[preflight] Running node-servant-preflight-convert jobs to check on edge and cloud nodes. " +
		"Job running may take a long time, and job failure will affect the execution of the next stage")
	jobLst, err := c.generatePreflightJobs()
	if err != nil {
		return err
	}
	if err := preflight.RunNodeServantJobCheck(c.ClientSet, jobLst, c.WaitServantJobTimeout, kubeutil.CheckServantJobPeriod, c.IgnorePreflightErrors); err != nil {
		return err
	}

	return nil
}

// RunConvert performs the conversion
func (c *ClusterConverter) RunConvert() (err error) {
	if err = lock.AcquireLock(c.ClientSet); err != nil {
		return
	}
	defer func() {
		if releaseLockErr := lock.ReleaseLock(c.ClientSet); releaseLockErr != nil {
			klog.Error(releaseLockErr)
		}
	}()

	nodeLst, err := c.ClientSet.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return
	}
	edgeNodeNames := getEdgeNodeNames(nodeLst, c.CloudNodes)

	fmt.Println("[runConvert] Label all nodes with edgeworker label, annotate all nodes with autonomy annotation")
	for _, node := range nodeLst.Items {
		isEdge := strutil.IsInStringLst(edgeNodeNames, node.Name)
		isAuto := strutil.IsInStringLst(c.AutonomousNodes, node.Name)
		if _, err = kubeutil.AddEdgeWorkerLabelAndAutonomyAnnotation(c.ClientSet, &node,
			strconv.FormatBool(isEdge), strconv.FormatBool(isAuto)); err != nil {
			return
		}
	}

	// deploy yurt controller manager
	fmt.Println("[runConvert] Deploying yurt-controller-manager")
	if err = kubeutil.DeployYurtControllerManager(c.ClientSet, c.YurtControllerManagerImage); err != nil {
		return
	}

	// deploy the yurttunnel if required
	if c.DeployTunnel {
		fmt.Println("[runConvert] Deploying yurt-tunnel-server and yurt-tunnel-agent")
		var certIP string
		if c.YurttunnelServerAddress != "" {
			certIP, _, _ = net.SplitHostPort(c.YurttunnelServerAddress)
		}
		if err = kubeutil.DeployYurttunnelServer(c.ClientSet,
			certIP,
			c.YurttunnelServerImage,
			c.SystemArchitecture); err != nil {
			return
		}
		// we will deploy yurt-tunnel-agent on every edge node
		if err = kubeutil.DeployYurttunnelAgent(c.ClientSet,
			c.YurttunnelServerAddress,
			c.YurttunnelAgentImage); err != nil {
			return
		}
	}

	// deploy the yurtappmanager if required
	if c.EnableAppManager {
		fmt.Println("[runConvert] Deploying yurt-app-manager")
		if err = kubeutil.DeployYurtAppManager(c.ClientSet,
			c.YurtAppManagerImage,
			c.YurtAppManagerClientSet,
			c.SystemArchitecture); err != nil {
			return
		}
	}

	fmt.Println("[runConvert] Running jobs for convert. Job running may take a long time, and job failure will not affect the execution of the next stage")

	// disable node-controller
	fmt.Println("[runConvert] Running disable-node-controller jobs to disable node-controller")
	var kcmNodeNames []string
	if kcmNodeNames, err = kubeutil.GetKubeControllerManagerHANodes(c.ClientSet); err != nil {
		return
	}

	if err = kubeutil.RunServantJobs(c.ClientSet, c.WaitServantJobTimeout, func(nodeName string) (*batchv1.Job, error) {
		ctx := map[string]string{
			"node_servant_image": c.NodeServantImage,
			"pod_manifest_path":  c.PodMainfestPath,
		}
		return kubeutil.RenderServantJob("disable", ctx, nodeName)
	}, kcmNodeNames, os.Stderr); err != nil {
		return
	}

	// deploy yurt-hub and reset the kubelet service on edge nodes.
	fmt.Println("[runConvert] Running node-servant-convert jobs to deploy the yurt-hub and reset the kubelet service on edge and cloud nodes")
	var joinToken string
	if joinToken, err = prepareYurthubStart(c.ClientSet, c.KubeConfigPath); err != nil {
		return
	}

	convertCtx := map[string]string{
		"node_servant_image": c.NodeServantImage,
		"yurthub_image":      c.YurthubImage,
		"joinToken":          joinToken,
		"kubeadm_conf_path":  c.KubeadmConfPath,
		"working_mode":       string(util.WorkingModeEdge),
	}
	if c.YurthubHealthCheckTimeout != defaultYurthubHealthCheckTimeout {
		convertCtx["yurthub_healthcheck_timeout"] = c.YurthubHealthCheckTimeout.String()
	}
	if len(edgeNodeNames) != 0 {
		convertCtx["working_mode"] = string(util.WorkingModeEdge)
		if err = kubeutil.RunServantJobs(c.ClientSet, c.WaitServantJobTimeout, func(nodeName string) (*batchv1.Job, error) {
			return nodeservant.RenderNodeServantJob("convert", convertCtx, nodeName)
		}, edgeNodeNames, os.Stderr); err != nil {
			return
		}
	}

	// deploy yurt-hub and reset the kubelet service on cloud nodes
	convertCtx["working_mode"] = string(util.WorkingModeCloud)
	if err = kubeutil.RunServantJobs(c.ClientSet, c.WaitServantJobTimeout, func(nodeName string) (*batchv1.Job, error) {
		return nodeservant.RenderNodeServantJob("convert", convertCtx, nodeName)
	}, c.CloudNodes, os.Stderr); err != nil {
		return
	}

	fmt.Println("[runConvert] If any job fails, you can get job information through 'kubectl get jobs -n kube-system' to debug.\n" +
		"\tNote that before the next conversion, please delete all related jobs so as not to affect the conversion.")

	return

}

func prepareYurthubStart(cliSet *kubernetes.Clientset, kcfg string) (string, error) {
	// prepare kube-public/cluster-info configmap before convert
	if err := prepareClusterInfoConfigMap(cliSet, kcfg); err != nil {
		return "", err
	}

	// prepare global settings(like RBAC, configmap) for yurthub
	if err := kubeutil.DeployYurthubSetting(cliSet); err != nil {
		return "", err
	}

	// prepare join-token for yurthub
	joinToken, err := kubeutil.GetOrCreateJoinTokenString(cliSet)
	if err != nil || joinToken == "" {
		return "", fmt.Errorf("fail to get join token: %v", err)
	}
	return joinToken, nil

}

// generatePreflightJobs generate preflight-check job for each node
func (c *ClusterConverter) generatePreflightJobs() ([]*batchv1.Job, error) {
	jobLst := make([]*batchv1.Job, 0)
	preflightCtx := map[string]string{
		"node_servant_image":      c.NodeServantImage,
		"kubeadm_conf_path":       c.KubeadmConfPath,
		"ignore_preflight_errors": strings.Join(c.IgnorePreflightErrors.List(), ","),
	}

	nodeLst, err := c.ClientSet.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for _, node := range nodeLst.Items {
		job, err := nodeservant.RenderNodeServantJob("preflight-convert", preflightCtx, node.Name)
		if err != nil {
			return nil, fmt.Errorf("fail to get job for node %s: %s", node.Name, err)
		}
		jobLst = append(jobLst, job)
	}

	return jobLst, nil
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
		klog.V(4).Infof("%s/%s configmap already exists, skip to prepare it", info.Namespace, info.Name)
	}

	return nil
}

func getEdgeNodeNames(nodeLst *v1.NodeList, cloudNodeNames []string) (edgeNodeNames []string) {
	for _, node := range nodeLst.Items {
		if !strutil.IsInStringLst(cloudNodeNames, node.GetName()) {
			edgeNodeNames = append(edgeNodeNames, node.GetName())
		}
	}
	return
}
