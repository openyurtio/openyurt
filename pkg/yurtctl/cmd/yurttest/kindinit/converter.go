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
	"strconv"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	bootstrapapi "k8s.io/cluster-bootstrap/token/api"
	"k8s.io/klog/v2"

	nodeservant "github.com/openyurtio/openyurt/pkg/node-servant"
	kubeadmapi "github.com/openyurtio/openyurt/pkg/util/kubernetes/kubeadm/app/phases/bootstraptoken/clusterinfo"
	strutil "github.com/openyurtio/openyurt/pkg/util/strings"
	"github.com/openyurtio/openyurt/pkg/yurtctl/lock"
	kubeutil "github.com/openyurtio/openyurt/pkg/yurtctl/util/kubernetes"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

const (
	// defaultYurthubHealthCheckTimeout defines the default timeout for yurthub health check phase
	defaultYurthubHealthCheckTimeout = 2 * time.Minute
)

type ClusterConverter struct {
	ClientSet                  *kubernetes.Clientset
	CloudNodes                 []string
	EdgeNodes                  []string
	WaitServantJobTimeout      time.Duration
	YurthubHealthCheckTimeout  time.Duration
	KubeConfigPath             string
	YurtTunnelAgentImage       string
	YurtTunnelServerImage      string
	YurtControllerManagerImage string
	NodeServantImage           string
	YurthubImage               string
	EnableDummyIf              bool
}

func (c *ClusterConverter) Run() error {
	if err := lock.AcquireLock(c.ClientSet); err != nil {
		return err
	}
	defer func() {
		if releaseLockErr := lock.ReleaseLock(c.ClientSet); releaseLockErr != nil {
			klog.Error(releaseLockErr)
		}
	}()

	klog.Info("Add edgework label and autonomy annotation to edge nodes")
	if err := c.labelEdgeNodes(); err != nil {
		klog.Errorf("failed to label and annotate edge nodes, %s", err)
		return err
	}

	klog.Info("Deploying yurt-controller-manager")
	if err := kubeutil.DeployYurtControllerManager(c.ClientSet, c.YurtControllerManagerImage); err != nil {
		klog.Errorf("failed to deploy yurt-controller-manager with image %s, %s", c.YurtControllerManagerImage, err)
		return err
	}

	klog.Info("Deploying yurt-tunnel")
	if err := c.deployYurtTunnel(); err != nil {
		klog.Errorf("failed to deploy yurt tunnel, %v", err)
		return err
	}

	klog.Info("Running jobs for convert. Job running may take a long time, and job failure will not affect the execution of the next stage")

	klog.Info("Running node-servant-convert jobs to deploy the yurt-hub and reset the kubelet service on edge and cloud nodes")
	if err := c.deployYurthub(); err != nil {
		klog.Errorf("error occurs when deploying Yurthub, %v", err)
		return err
	}
	return nil
}

func (c *ClusterConverter) labelEdgeNodes() error {
	nodeLst, err := c.ClientSet.CoreV1().Nodes().List(context.Background(), v1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list nodes, %w", err)
	}
	for _, node := range nodeLst.Items {
		isEdge := strutil.IsInStringLst(c.EdgeNodes, node.Name)
		if _, err = kubeutil.AddEdgeWorkerLabelAndAutonomyAnnotation(
			c.ClientSet, &node, strconv.FormatBool(isEdge), "false"); err != nil {
			return fmt.Errorf("failed to add label to edge node %s, %w", node.Name, err)
		}
	}
	return nil
}

func (c *ClusterConverter) deployYurtTunnel() error {
	if err := kubeutil.DeployYurttunnelServer(c.ClientSet,
		"", c.YurtTunnelServerImage, "amd64"); err != nil {
		klog.Errorf("failed to deploy yurt-tunnel-server, %s", err)
		return err
	}

	if err := kubeutil.DeployYurttunnelAgent(c.ClientSet,
		"", c.YurtTunnelAgentImage); err != nil {
		klog.Errorf("failed to deploy yurt-tunnel-agent, %s", err)
		return err
	}
	return nil
}

func (c *ClusterConverter) deployYurthub() error {
	// deploy yurt-hub and reset the kubelet service on edge nodes.
	joinToken, err := prepareYurthubStart(c.ClientSet, c.KubeConfigPath)
	if err != nil {
		return err
	}
	convertCtx := map[string]string{
		"node_servant_image": c.NodeServantImage,
		"yurthub_image":      c.YurthubImage,
		"joinToken":          joinToken,
		// The node-servant will detect the kubeadm_conf_path automatically
		// It will be either "/usr/lib/systemd/system/kubelet.service.d/10-kubeadm.conf"
		// or "/etc/systemd/system/kubelet.service.d/10-kubeadm.conf".
		"kubeadm_conf_path": "",
		"working_mode":      string(util.WorkingModeEdge),
		"enable_dummy_if":   strconv.FormatBool(c.EnableDummyIf),
	}
	if c.YurthubHealthCheckTimeout != defaultYurthubHealthCheckTimeout {
		convertCtx["yurthub_healthcheck_timeout"] = c.YurthubHealthCheckTimeout.String()
	}

	npExist, err := nodePoolResourceExists(c.ClientSet)
	if err != nil {
		return err
	}
	convertCtx["enable_node_pool"] = strconv.FormatBool(npExist)
	klog.Infof("convert context for edge nodes(%q): %#+v", c.EdgeNodes, convertCtx)

	if len(c.EdgeNodes) != 0 {
		convertCtx["working_mode"] = string(util.WorkingModeEdge)
		if err = kubeutil.RunServantJobs(c.ClientSet, c.WaitServantJobTimeout, func(nodeName string) (*batchv1.Job, error) {
			return nodeservant.RenderNodeServantJob("convert", convertCtx, nodeName)
		}, c.EdgeNodes, os.Stderr, false); err != nil {
			return err
		}
	}

	// deploy yurt-hub and reset the kubelet service on cloud nodes
	convertCtx["working_mode"] = string(util.WorkingModeCloud)
	klog.Infof("convert context for cloud nodes(%q): %#+v", c.CloudNodes, convertCtx)
	if err = kubeutil.RunServantJobs(c.ClientSet, c.WaitServantJobTimeout, func(nodeName string) (*batchv1.Job, error) {
		return nodeservant.RenderNodeServantJob("convert", convertCtx, nodeName)
	}, c.CloudNodes, os.Stderr, false); err != nil {
		return err
	}

	klog.Info("If any job fails, you can get job information through 'kubectl get jobs -n kube-system' to debug.\n" +
		"\tNote that before the next conversion, please delete all related jobs so as not to affect the conversion.")

	return nil
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
		return "", fmt.Errorf("fail to get join token: %w", err)
	}
	return joinToken, nil
}

// prepareClusterInfoConfigMap will create cluster-info configmap in kube-public namespace if it does not exist
func prepareClusterInfoConfigMap(client *kubernetes.Clientset, file string) error {
	info, err := client.CoreV1().ConfigMaps(v1.NamespacePublic).Get(context.Background(), bootstrapapi.ConfigMapClusterInfo, v1.GetOptions{})
	if err != nil && apierrors.IsNotFound(err) {
		// Create the cluster-info ConfigMap with the associated RBAC rules
		if err := kubeadmapi.CreateBootstrapConfigMapIfNotExists(client, file); err != nil {
			return fmt.Errorf("error creating bootstrap ConfigMap, %w", err)
		}
		if err := kubeadmapi.CreateClusterInfoRBACRules(client); err != nil {
			return fmt.Errorf("error creating clusterinfo RBAC rules, %w", err)
		}
	} else if err != nil || info == nil {
		return fmt.Errorf("fail to get configmap, %w", err)
	} else {
		klog.V(4).Infof("%s/%s configmap already exists, skip to prepare it", info.Namespace, info.Name)
	}

	return nil
}

func nodePoolResourceExists(client *kubernetes.Clientset) (bool, error) {
	groupVersion := schema.GroupVersion{
		Group:   "apps.openyurt.io",
		Version: "v1alpha1",
	}
	apiResourceList, err := client.Discovery().ServerResourcesForGroupVersion(groupVersion.String())
	if err != nil && !apierrors.IsNotFound(err) {
		klog.Errorf("failed to discover nodepool resource, %v", err)
		return false, err
	} else if apiResourceList == nil {
		return false, nil
	}

	for i := range apiResourceList.APIResources {
		if apiResourceList.APIResources[i].Name == "nodepools" && apiResourceList.APIResources[i].Kind == "NodePool" {
			return true, nil
		}
	}

	return false, nil
}
