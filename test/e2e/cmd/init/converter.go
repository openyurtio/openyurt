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
	"path/filepath"
	"strconv"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	kubeclientset "k8s.io/client-go/kubernetes"
	bootstrapapi "k8s.io/cluster-bootstrap/token/api"
	"k8s.io/klog/v2"

	nodeservant "github.com/openyurtio/openyurt/pkg/node-servant"
	kubeadmapi "github.com/openyurtio/openyurt/pkg/util/kubernetes/kubeadm/app/phases/bootstraptoken/clusterinfo"
	strutil "github.com/openyurtio/openyurt/pkg/util/strings"
	tmplutil "github.com/openyurtio/openyurt/pkg/util/templates"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
	"github.com/openyurtio/openyurt/test/e2e/cmd/init/constants"
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
	ComponentsBuilder         *kubeutil.Builder
	ClientSet                 kubeclientset.Interface
	CloudNodes                []string
	EdgeNodes                 []string
	WaitServantJobTimeout     time.Duration
	YurthubHealthCheckTimeout time.Duration
	KubeConfigPath            string
	YurtManagerImage          string
	NodeServantImage          string
	YurthubImage              string
	EnableDummyIf             bool
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
	if err := c.deployYurtManager(); err != nil {
		klog.Errorf("failed to deploy yurt-manager with image %s, %s", c.YurtManagerImage, err)
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
		"kubeadm_conf_path":    "",
		"working_mode":         string(util.WorkingModeEdge),
		"enable_dummy_if":      strconv.FormatBool(c.EnableDummyIf),
		"kubernetesServerAddr": "{{.kubernetesServerAddr}}",
	}
	if c.YurthubHealthCheckTimeout != defaultYurthubHealthCheckTimeout {
		convertCtx["yurthub_healthcheck_timeout"] = c.YurthubHealthCheckTimeout.String()
	}

	// create the yurthub-cloud and yurthub yss
	tempDir, err := os.MkdirTemp(c.RootDir, "yurt-hub")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)
	tempFile := filepath.Join(tempDir, "yurthub-cloud-yurtstaticset.yaml")
	yssYurtHubCloud, err := tmplutil.SubsituteTemplate(constants.YurthubCloudYurtStaticSet, convertCtx)
	if err != nil {
		return err
	}
	if err = os.WriteFile(tempFile, []byte(yssYurtHubCloud), 0644); err != nil {
		return err
	}
	if err = c.ComponentsBuilder.InstallComponents(tempFile, false); err != nil {
		return err
	}

	tempFile = filepath.Join(tempDir, "yurthub-yurtstaticset.yaml")
	yssYurtHub, err := tmplutil.SubsituteTemplate(constants.YurthubYurtStaticSet, convertCtx)
	if err != nil {
		return err
	}
	if err = os.WriteFile(tempFile, []byte(yssYurtHub), 0644); err != nil {
		return err
	}
	if err = c.ComponentsBuilder.InstallComponents(tempFile, false); err != nil {
		return err
	}

	npExist, err := nodePoolResourceExists(c.ClientSet)
	if err != nil {
		return err
	}
	convertCtx["enable_node_pool"] = strconv.FormatBool(npExist)
	klog.Infof("convert context for edge nodes(%q): %#+v", c.EdgeNodes, convertCtx)

	if len(c.EdgeNodes) != 0 {
		convertCtx["working_mode"] = string(util.WorkingModeEdge)
		convertCtx["configmap_name"] = yssYurtHubName
		if err = kubeutil.RunServantJobs(c.ClientSet, c.WaitServantJobTimeout, func(nodeName string) (*batchv1.Job, error) {
			return nodeservant.RenderNodeServantJob("convert", convertCtx, nodeName)
		}, c.EdgeNodes, os.Stderr, false); err != nil {
			// print logs of yurthub
			for i := range c.EdgeNodes {
				hubPodName := fmt.Sprintf("yurt-hub-%s", c.EdgeNodes[i])
				pod, logErr := c.ClientSet.CoreV1().Pods("kube-system").Get(context.TODO(), hubPodName, metav1.GetOptions{})
				if logErr == nil {
					kubeutil.PrintPodLog(c.ClientSet, pod, os.Stderr)
				}
			}

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
			if logErr = kubeutil.PrintPodLog(c.ClientSet, &podList.Items[0], os.Stderr); logErr != nil {
				return err
			}
			return err
		}
	}

	// deploy yurt-hub and reset the kubelet service on cloud nodes
	convertCtx["working_mode"] = string(util.WorkingModeCloud)
	convertCtx["configmap_name"] = yssYurtHubCloudName
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

func prepareYurthubStart(cliSet kubeclientset.Interface, kcfg string) (string, error) {
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

func nodePoolResourceExists(client kubeclientset.Interface) (bool, error) {
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

func (c *ClusterConverter) deployYurtManager() error {
	// create auto generated yaml(including clusterrole and webhooks) for yurt-manager
	renderedFile, err := generatedAutoGeneratedTempFile(c.RootDir, "kube-system")
	if err != nil {
		return err
	}

	// get renderedFile parent dir
	renderedFileDir := filepath.Dir(renderedFile)
	defer os.RemoveAll(renderedFileDir)
	if err := c.ComponentsBuilder.InstallComponents(renderedFile, false); err != nil {
		return err
	}

	if err := c.ComponentsBuilder.InstallComponents(filepath.Join(c.RootDir, "charts/yurt-manager/crds"), false); err != nil {
		return err
	}

	// create yurt-manager
	if err := kubeutil.CreateYurtManager(c.ClientSet, c.YurtManagerImage); err != nil {
		return err
	}

	// waiting yurt-manager pod ready
	return wait.PollImmediate(10*time.Second, 2*time.Minute, func() (bool, error) {
		podList, err := c.ClientSet.CoreV1().Pods("kube-system").List(context.TODO(), metav1.ListOptions{
			LabelSelector: labels.SelectorFromSet(map[string]string{"app.kubernetes.io/name": "yurt-manager"}).String(),
		})
		if err != nil {
			klog.Errorf("failed to list yurt-manager pod, %v", err)
			return false, nil
		} else if len(podList.Items) == 0 {
			klog.Infof("no yurt-manager pod: %#v", podList)
			return false, nil
		}

		if podList.Items[0].Status.Phase == corev1.PodRunning {
			for i := range podList.Items[0].Status.Conditions {
				if podList.Items[0].Status.Conditions[i].Type == corev1.PodReady &&
					podList.Items[0].Status.Conditions[i].Status != corev1.ConditionTrue {
					klog.Infof("pod(%s/%s): %#v", podList.Items[0].Namespace, podList.Items[0].Name, podList.Items[0])
					return false, nil
				}
				if podList.Items[0].Status.Conditions[i].Type == corev1.ContainersReady &&
					podList.Items[0].Status.Conditions[i].Status != corev1.ConditionTrue {
					klog.Info("yurt manager's container is not ready")
					return false, nil
				}
			}
		}
		return true, nil
	})
}

// generatedAutoGeneratedTempFile will replace {{ .Release.Namespace }} with ns in webhooks
func generatedAutoGeneratedTempFile(root, ns string) (string, error) {
	tempDir, err := os.MkdirTemp(root, "yurt-manager")
	if err != nil {
		return "", err
	}

	autoGeneratedYaml := filepath.Join(root, "charts/yurt-manager/templates/yurt-manager-auto-generated.yaml")
	contents, err := os.ReadFile(autoGeneratedYaml)
	if err != nil {
		return "", err
	}

	lines := strings.Split(string(contents), "\n")
	for i, line := range lines {
		if strings.Contains(line, "{{ .Release.Namespace }}") {
			lines[i] = strings.ReplaceAll(line, "{{ .Release.Namespace }}", ns)
		}
	}
	newContents := strings.Join(lines, "\n")

	tempFile := filepath.Join(tempDir, "yurt-manager-auto-generated.yaml")
	klog.Infof("rendered yurt-manager-auto-generated.yaml file: \n%s\n", newContents)
	return tempFile, os.WriteFile(tempFile, []byte(newContents), 0644)
}
