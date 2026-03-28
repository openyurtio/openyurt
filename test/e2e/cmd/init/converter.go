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
	"reflect"
	"sort"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	kubeclientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1beta2 "github.com/openyurtio/openyurt/pkg/apis/apps/v1beta2"
	nodeservant "github.com/openyurtio/openyurt/pkg/node-servant"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/test/e2e/cmd/init/lock"
	kubeutil "github.com/openyurtio/openyurt/test/e2e/cmd/init/util/kubernetes"
)

const (
	conversionConditionType corev1.NodeConditionType = "YurtNodeConversionFailed"
)

var (
	converterExecCommand               = exec.Command
	converterInstallRenderedComponents = func(kubeConfigPath, manifestPath string) error {
		return kubeutil.NewBuilder(kubeConfigPath).InstallComponents(manifestPath, false)
	}
)

type ClusterConverter struct {
	RootDir                   string
	ClientSet                 kubeclientset.Interface
	RuntimeClient             client.Client
	CloudNodes                []string
	EdgeNodes                 []string
	WaitNodeConversionTimeout time.Duration
	KubeConfigPath            string
	YurtManagerImage          string
	NodeServantImage          string
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

	klog.Info("Add node autonomy annotation to edge nodes")
	if err := c.configureNodeAutonomyAnnotations(); err != nil {
		klog.Errorf("failed to annotate edge nodes, %v", err)
		return err
	}

	klog.Info("Deploying yurt-manager")
	if err := c.installYurtManagerByHelm(); err != nil {
		klog.Errorf("failed to deploy yurt-manager with image %s, %s", c.YurtManagerImage, err)
		return err
	}

	klog.Info("Deploying yurthub chart resources")
	if err := c.installYurthubByHelm(); err != nil {
		klog.Errorf("failed to deploy yurthub chart resources, %v", err)
		return err
	}

	klog.Infof("Start to initialize default node pools: %+v", DefaultPools)
	if err := c.createDefaultNodePools(); err != nil {
		return err
	}

	targetNodeNames, err := c.assignNodesToNodePools()
	if err != nil {
		return err
	}

	klog.Infof("Waiting for node conversion controller to convert nodes: %v", targetNodeNames)
	if err := c.waitNodesConverted(targetNodeNames); err != nil {
		klog.Errorf("failed waiting for converted nodes, %v", err)
		c.dumpYurtManagerLog()
		c.dumpConversionDiagnostics(targetNodeNames)
		return err
	}
	return nil
}

func (c *ClusterConverter) createDefaultNodePools() error {
	for name, leaderInfo := range DefaultPools {
		np := &appsv1beta2.NodePool{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Spec: appsv1beta2.NodePoolSpec{
				Type:                 leaderInfo.Kind,
				EnableLeaderElection: leaderInfo.EnableLeaderElection,
				LeaderReplicas:       int32(leaderInfo.LeaderReplicas),
				InterConnectivity:    true,
			},
		}
		if err := c.RuntimeClient.Create(context.Background(), np); err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create nodepool %s, %w", name, err)
		}
	}
	return nil
}

func (c *ClusterConverter) assignNodesToNodePools() ([]string, error) {
	assignments, err := c.edgeNodePoolAssignments()
	if err != nil {
		return nil, err
	}

	targetNodeNames := make([]string, 0, len(assignments))
	for _, nodeName := range c.EdgeNodes {
		poolName, ok := assignments[nodeName]
		if !ok {
			continue
		}

		node := &corev1.Node{}
		if err := c.RuntimeClient.Get(context.Background(), client.ObjectKey{Name: nodeName}, node); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return nil, fmt.Errorf("failed to get node %s for nodepool assignment, %w", nodeName, err)
		}

		targetNodeNames = append(targetNodeNames, nodeName)
		newNode := node.DeepCopy()
		nodeLabels := newNode.Labels
		if nodeLabels == nil {
			nodeLabels = map[string]string{}
		}

		nodeLabels[projectinfo.GetNodePoolLabel()] = poolName
		newNode.Labels = nodeLabels
		if reflect.DeepEqual(newNode.Labels, node.Labels) {
			continue
		}

		if err := c.RuntimeClient.Patch(context.Background(), newNode, client.MergeFrom(node)); err != nil {
			return nil, fmt.Errorf("failed to assign nodepool %s to node %s, %w", poolName, nodeName, err)
		}
	}

	sort.Strings(targetNodeNames)
	return targetNodeNames, nil
}

func (c *ClusterConverter) edgeNodePoolAssignments() (map[string]string, error) {
	assignments := make(map[string]string, len(c.EdgeNodes))
	for _, nodeName := range c.EdgeNodes {
		poolName, ok := NodeNameToPool[nodeName]
		if !ok {
			return nil, fmt.Errorf("missing nodepool assignment for edge node %s", nodeName)
		}
		assignments[nodeName] = poolName
	}

	return assignments, nil
}

func (c *ClusterConverter) configureNodeAutonomyAnnotations() error {
	nodeLst, err := c.ClientSet.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list nodes, %w", err)
	}
	for _, node := range nodeLst.Items {
		newNode := node.DeepCopy()
		if newNode.Annotations == nil {
			newNode.Annotations = map[string]string{}
		}
		newNode.Annotations[projectinfo.GetNodeAutonomyDurationAnnotation()] = "0"
		if reflect.DeepEqual(newNode.Annotations, node.Annotations) {
			continue
		}
		if _, err = c.ClientSet.CoreV1().Nodes().Update(context.Background(), newNode, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("failed to set autonomy annotation for node %s, %w", node.Name, err)
		}
	}
	return nil
}

func (c *ClusterConverter) waitNodesConverted(nodeNames []string) error {
	timeout := c.WaitNodeConversionTimeout
	if timeout == 0 {
		timeout = kubeutil.DefaultWaitNodeConversionTimeout
	}

	assignments, err := c.edgeNodePoolAssignments()
	if err != nil {
		return err
	}

	return wait.PollUntilContextTimeout(context.Background(), 5*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		for _, nodeName := range nodeNames {
			node, err := c.ClientSet.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
			if err != nil {
				return false, err
			}

			jobs, err := c.listConversionJobs(ctx, nodeName)
			if err != nil {
				return false, err
			}

			converted, err := isNodeConversionSettled(node, assignments[nodeName], jobs.Items)
			if err != nil {
				return false, err
			}
			if !converted {
				return false, nil
			}
		}

		return true, nil
	})
}

func (c *ClusterConverter) listConversionJobs(ctx context.Context, nodeName string) (*batchv1.JobList, error) {
	return c.ClientSet.BatchV1().Jobs(nodeservant.DefaultConversionJobNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{
			nodeservant.ConversionNodeLabelKey: nodeName,
		}).String(),
	})
}

func isNodeConversionSettled(node *corev1.Node, expectedNodePool string, jobs []batchv1.Job) (bool, error) {
	if node == nil {
		return false, fmt.Errorf("node is nil")
	}

	if cond := getNodeCondition(node, conversionConditionType); cond != nil && cond.Status == corev1.ConditionTrue {
		return false, fmt.Errorf("node %s conversion failed: reason=%s message=%s", node.Name, cond.Reason, cond.Message)
	}

	if node.Labels[projectinfo.GetNodePoolLabel()] != expectedNodePool {
		return false, nil
	}
	if node.Labels[projectinfo.GetEdgeWorkerLabelKey()] != "true" {
		return false, nil
	}
	if node.Spec.Unschedulable {
		return false, nil
	}

	for i := range jobs {
		if isJobFailed(&jobs[i]) {
			return false, fmt.Errorf("conversion job %s for node %s failed", jobs[i].Name, node.Name)
		}
		if !isJobFinished(&jobs[i]) {
			return false, nil
		}
	}

	return true, nil
}

func isJobFinished(job *batchv1.Job) bool {
	return isJobSucceeded(job) || isJobFailed(job)
}

func isJobSucceeded(job *batchv1.Job) bool {
	if job == nil {
		return false
	}
	for _, cond := range job.Status.Conditions {
		if cond.Type == batchv1.JobComplete && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func isJobFailed(job *batchv1.Job) bool {
	if job == nil {
		return false
	}
	for _, cond := range job.Status.Conditions {
		if cond.Type == batchv1.JobFailed && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func getNodeCondition(node *corev1.Node, condType corev1.NodeConditionType) *corev1.NodeCondition {
	if node == nil {
		return nil
	}
	for i := range node.Status.Conditions {
		if node.Status.Conditions[i].Type == condType {
			return &node.Status.Conditions[i]
		}
	}
	return nil
}

func (c *ClusterConverter) dumpConversionDiagnostics(nodeNames []string) {
	for _, nodeName := range nodeNames {
		node, err := c.ClientSet.CoreV1().Nodes().Get(context.Background(), nodeName, metav1.GetOptions{})
		if err != nil {
			klog.Errorf("failed to get node %s while dumping conversion diagnostics, %v", nodeName, err)
			continue
		}

		klog.Infof("node %s labels=%v annotations=%v unschedulable=%t", nodeName, node.Labels, node.Annotations, node.Spec.Unschedulable)
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeReady || cond.Type == conversionConditionType {
				klog.Infof("node %s condition type=%s status=%s reason=%s message=%s", nodeName, cond.Type, cond.Status, cond.Reason, cond.Message)
			}
		}

		eventList, err := c.ClientSet.CoreV1().Events("").List(context.Background(), metav1.ListOptions{
			FieldSelector: fmt.Sprintf("involvedObject.kind=Node,involvedObject.name=%s", nodeName),
		})
		if err == nil {
			for _, event := range eventList.Items {
				klog.Infof("node %s event type=%s reason=%s message=%s", nodeName, event.Type, event.Reason, event.Message)
			}
		}

		jobs, err := c.listConversionJobs(context.Background(), nodeName)
		if err != nil {
			klog.Errorf("failed to list conversion jobs for node %s, %v", nodeName, err)
			continue
		}
		for i := range jobs.Items {
			job := &jobs.Items[i]
			klog.Infof("conversion job %s/%s active=%d succeeded=%d failed=%d", job.Namespace, job.Name, job.Status.Active, job.Status.Succeeded, job.Status.Failed)
			podList, podErr := c.ClientSet.CoreV1().Pods(job.Namespace).List(context.Background(), metav1.ListOptions{
				LabelSelector: labels.SelectorFromSet(map[string]string{"job-name": job.Name}).String(),
			})
			if podErr != nil {
				klog.Errorf("failed to list pods for conversion job %s/%s, %v", job.Namespace, job.Name, podErr)
				continue
			}
			for j := range podList.Items {
				if err := kubeutil.DumpPod(c.ClientSet, &podList.Items[j], os.Stderr); err != nil {
					klog.Warningf("failed to dump pod %s/%s for node %s, %v", podList.Items[j].Namespace, podList.Items[j].Name, nodeName, err)
				}
			}
		}
	}
}

func (c *ClusterConverter) installYurtManagerByHelm() error {
	helmPath := filepath.Join(c.RootDir, "bin", "helm")
	yurtManagerChartPath := filepath.Join(c.RootDir, "charts", "yurt-manager")

	managerTag, err := imageTag(c.YurtManagerImage)
	if err != nil {
		return err
	}
	nodeServantTag, err := imageTag(c.NodeServantImage)
	if err != nil {
		return err
	}

	cmd := exec.Command(
		helmPath,
		"install",
		"yurt-manager",
		yurtManagerChartPath,
		"--namespace",
		"kube-system",
		"--set",
		fmt.Sprintf("image.tag=%s", managerTag),
		"--set",
		fmt.Sprintf("nodeServant.image.tag=%s", nodeServantTag),
		"--set",
		"log.level=5",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		klog.Errorf("couldn't install yurt-manager, %v", err)
		klog.Errorf("Helm install output: %s", string(output))
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
		c.dumpYurtManagerLog()
		return err
	}

	return nil
}

func (c *ClusterConverter) installYurthubByHelm() error {
	helmPath := filepath.Join(c.RootDir, "bin", "helm")
	yurthubChartPath := filepath.Join(c.RootDir, "charts", "yurthub")

	cmd := converterExecCommand(
		helmPath,
		"template",
		"yurthub",
		yurthubChartPath,
		"--namespace",
		"kube-system",
		"--show-only",
		"templates/yurthub-cfg.yaml",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		klog.Errorf("couldn't install yurthub, %v, %s", err, string(output))
		return err
	}

	manifestFile, err := os.CreateTemp("", "openyurt-yurthub-cfg-*.yaml")
	if err != nil {
		return fmt.Errorf("failed to create temp file for yurthub resources: %w", err)
	}
	defer os.Remove(manifestFile.Name())

	if _, err := manifestFile.Write(output); err != nil {
		_ = manifestFile.Close()
		return fmt.Errorf("failed to write rendered yurthub resources: %w", err)
	}
	if err := manifestFile.Close(); err != nil {
		return fmt.Errorf("failed to close rendered yurthub resources file: %w", err)
	}

	if err := converterInstallRenderedComponents(c.KubeConfigPath, manifestFile.Name()); err != nil {
		return fmt.Errorf("failed to install rendered yurthub resources: %w", err)
	}

	klog.Infof("start to install yurthub shared resources, %s", string(output))
	return nil
}

func imageTag(image string) (string, error) {
	lastColon := strings.LastIndex(image, ":")
	lastSlash := strings.LastIndex(image, "/")
	if lastColon == -1 || lastColon < lastSlash {
		return "", fmt.Errorf("image %q does not include a tag", image)
	}
	return image[lastColon+1:], nil
}

// print logs of yurt-manager
func (c *ClusterConverter) dumpYurtManagerLog() {
	// print logs of yurt-manager
	podList, logErr := c.ClientSet.CoreV1().Pods("kube-system").List(context.TODO(), metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{"app.kubernetes.io/name": "yurt-manager"}).String(),
	})
	if logErr != nil {
		klog.Errorf("failed to get yurt-manager pod, %v", logErr)
	}

	if len(podList.Items) == 0 {
		klog.Errorf("yurt-manager pod doesn't exist")
	}
	if logErr = kubeutil.DumpPod(c.ClientSet, &podList.Items[0], os.Stderr); logErr != nil {
		klog.Warning("failed to dump yurtmanager logs")
	}
}
