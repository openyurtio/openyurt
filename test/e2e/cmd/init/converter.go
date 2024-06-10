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

package init

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	kubeclientset "k8s.io/client-go/kubernetes"
	bootstrapapi "k8s.io/cluster-bootstrap/token/api"
	"k8s.io/klog/v2"

	nodeservant "github.com/openyurtio/openyurt/pkg/node-servant"
	kubeadmapi "github.com/openyurtio/openyurt/pkg/util/kubernetes/kubeadm/app/phases/bootstraptoken/clusterinfo"
	strutil "github.com/openyurtio/openyurt/pkg/util/strings"
	"github.com/openyurtio/openyurt/test/e2e/cmd/init/lock"
	kubeutil "github.com/openyurtio/openyurt/test/e2e/cmd/init/util/kubernetes"
)

const (
	// defaultYurthubHealthCheckTimeout defines the default timeout for yurthub health check phase
	defaultYurthubHealthCheckTimeout = 2 * time.Minute
	yssYurtHubCloudName              = "yurt-static-set-yurt-hub-cloud"
	yssYurtHubName                   = "yurt-static-set-yurt-hub"
)

type ClusterConverter struct {
	RootDir                   string
	ClientSet                 kubeclientset.Interface
	CloudNodes                []string
	EdgeNodes                 []string
	WaitServantJobTimeout     time.Duration
	YurthubHealthCheckTimeout time.Duration
	KubeConfigPath            string
	YurtManagerImage          string
	NodeServantImage          string
	YurthubImage              string
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

	klog.Info("Deploying yurt-manager")
	if err := c.installYurtManagerByHelm(); err != nil {
		klog.Errorf("failed to deploy yurt-manager with image %s, %s", c.YurtManagerImage, err)
		return err
	}

	klog.Info("Running jobs for convert. Job running may take a long time, and job failure will not affect the execution of the next stage")

	klog.Info("Running node-servant-convert jobs to deploy the yurt-hub and reset the kubelet service on edge and cloud nodes")
	if err := c.installYurthubByHelm(); err != nil {
		klog.Errorf("error occurs when deploying Yurthub, %v", err)
		return err
	}
	return nil
}

func (c *ClusterConverter) labelEdgeNodes() error {
	nodeLst, err := c.ClientSet.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
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

func (c *ClusterConverter) installYurthubByHelm() error {
	helmPath := filepath.Join(c.RootDir, "bin", "helm")
	yurthubChartPath := filepath.Join(c.RootDir, "charts", "yurthub")

	parts := strings.Split(c.YurthubImage, "/")
	imageTagParts := strings.Split(parts[len(parts)-1], ":")
	tag := imageTagParts[1]

	// create the yurthub-cloud and yurthub yss
	cmd := exec.Command(helmPath, "install", "yurthub", yurthubChartPath, "--namespace", "kube-system", "--set", fmt.Sprintf("kubernetesServerAddr=KUBERNETES_SERVER_ADDRESS,image.tag=%s", tag))
	output, err := cmd.CombinedOutput()
	if err != nil {
		klog.Errorf("couldn't install yurthub, %v, %s", err, string(output))
		return err
	}
	klog.Infof("start to install yurthub, %s", string(output))
	// deploy yurt-hub and reset the kubelet service on edge nodes.
	joinToken, err := prepareYurthubStart(c.ClientSet, c.KubeConfigPath)
	if err != nil {
		return err
	}
	convertCtx := map[string]string{
		"node_servant_image": c.NodeServantImage,
		"joinToken":          joinToken,
	}
	if c.YurthubHealthCheckTimeout != defaultYurthubHealthCheckTimeout {
		convertCtx["yurthub_healthcheck_timeout"] = c.YurthubHealthCheckTimeout.String()
	}

	if len(c.EdgeNodes) != 0 {
		convertCtx["configmap_name"] = yssYurtHubName
		if err = kubeutil.RunServantJobs(c.ClientSet, c.WaitServantJobTimeout, func(nodeName string) (*batchv1.Job, error) {
			return nodeservant.RenderNodeServantJob("convert", convertCtx, nodeName)
		}, c.EdgeNodes, os.Stderr); err != nil {
			// print logs of yurthub
			for i := range c.EdgeNodes {
				hubPodName := fmt.Sprintf("yurt-hub-%s", c.EdgeNodes[i])
				pod, logErr := c.ClientSet.CoreV1().Pods("kube-system").Get(context.TODO(), hubPodName, metav1.GetOptions{})
				if logErr == nil {
					kubeutil.DumpPod(c.ClientSet, pod, os.Stderr)
				}
			}
			return err
		}
	}

	// deploy yurt-hub and reset the kubelet service on cloud nodes
	convertCtx["configmap_name"] = yssYurtHubCloudName
	klog.Infof("convert context for cloud nodes(%q): %#+v", c.CloudNodes, convertCtx)
	if err = kubeutil.RunServantJobs(c.ClientSet, c.WaitServantJobTimeout, func(nodeName string) (*batchv1.Job, error) {
		return nodeservant.RenderNodeServantJob("convert", convertCtx, nodeName)
	}, c.CloudNodes, os.Stderr); err != nil {
		return err
	}

	klog.Info("If any job fails, you can get job information through 'kubectl get jobs -n kube-system' to debug.\n" +
		"\tNote that before the next conversion, please delete all related jobs so as not to affect the conversion.")

	return nil
}

func prepareYurthubStart(cliSet kubeclientset.Interface, kcfg string) (string, error) {
	// prepare kube-public/cluster-info configmap before convert
	if err := prepareClusterInfoConfigMap(cliSet, kcfg); err != nil {
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
func prepareClusterInfoConfigMap(client kubeclientset.Interface, file string) error {
	info, err := client.CoreV1().ConfigMaps(metav1.NamespacePublic).Get(context.Background(), bootstrapapi.ConfigMapClusterInfo, metav1.GetOptions{})
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

func (c *ClusterConverter) installYurtManagerByHelm() error {
	helmPath := filepath.Join(c.RootDir, "bin", "helm")
	yurtManagerChartPath := filepath.Join(c.RootDir, "charts", "yurt-manager")

	parts := strings.Split(c.YurtManagerImage, "/")
	imageTagParts := strings.Split(parts[len(parts)-1], ":")
	tag := imageTagParts[1]

	cmd := exec.Command(helmPath, "install", "yurt-manager", yurtManagerChartPath, "--namespace", "kube-system", "--set", fmt.Sprintf("image.tag=%s", tag))
	output, err := cmd.CombinedOutput()
	if err != nil {
		klog.Errorf("couldn't install yurt-manager, %v", err)
		return err
	}
	klog.Infof("start to install yurt-manager, %s", string(output))

	// waiting yurt-manager pod ready
	if err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
		podList, err := c.ClientSet.CoreV1().Pods("kube-system").List(context.TODO(), metav1.ListOptions{
			LabelSelector: labels.SelectorFromSet(map[string]string{"app.kubernetes.io/name": "yurt-manager"}).String(),
		})
		if err != nil {
			klog.Errorf("failed to list yurt-manager pod, %v", err)
			return false, nil
		} else if len(podList.Items) == 0 {
			klog.Infof("there is no yurt-manager pod now")
			return false, nil
		} else if podList.Items[0].Status.Phase != corev1.PodRunning {
			klog.Infof("status phase of yurt-manager pod is not running, now is %s", string(podList.Items[0].Status.Phase))
			return false, nil
		}

		for i := range podList.Items[0].Status.Conditions {
			if podList.Items[0].Status.Conditions[i].Type == corev1.PodReady &&
				podList.Items[0].Status.Conditions[i].Status != corev1.ConditionTrue {
				klog.Infof("ready condition of pod(%s/%s) is not true", podList.Items[0].Namespace, podList.Items[0].Name)
				return false, nil
			}
			if podList.Items[0].Status.Conditions[i].Type == corev1.ContainersReady &&
				podList.Items[0].Status.Conditions[i].Status != corev1.ConditionTrue {
				klog.Info("container ready condition is not true")
				return false, nil
			}
		}

		return true, nil
	}); err != nil {
		// print logs of yurt-manager
		podList, logErr := c.ClientSet.CoreV1().Pods("kube-system").List(context.TODO(), metav1.ListOptions{
			LabelSelector: labels.SelectorFromSet(map[string]string{"app.kubernetes.io/name": "yurt-manager"}).String(),
		})
		if logErr != nil {
			klog.Errorf("failed to get yurt-manager pod, %v", logErr)
			return err
		}

		if len(podList.Items) == 0 {
			klog.Errorf("yurt-manager pod doesn't exist")
			return err
		}
		if logErr = kubeutil.DumpPod(c.ClientSet, &podList.Items[0], os.Stderr); logErr != nil {
			return err
		}
		return err
	}

	return nil
}
