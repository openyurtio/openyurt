/*
Copyright 2021 The OpenYurt Authors.

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

package revert

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog"

	nodeutil "github.com/openyurtio/openyurt/pkg/controller/util/node"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/yurtctl/constants"
	enutil "github.com/openyurtio/openyurt/pkg/yurtctl/util/edgenode"
	kubeutil "github.com/openyurtio/openyurt/pkg/yurtctl/util/kubernetes"
	strutil "github.com/openyurtio/openyurt/pkg/yurtctl/util/strings"
	"github.com/openyurtio/openyurt/pkg/yurthub/certificate/hubself"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

// RevertNodeOptions has the information required by sub command revert edgenode and revert cloudnode
type RevertNodeOptions struct {
	clientSet           *kubernetes.Clientset
	Nodes               []string
	YurtctlServantImage string
	KubeadmConfPath     string
	openyurtDir         string
}

// commonFlags sets all common flags.
func commonFlags(cmd *cobra.Command) {
	cmd.Flags().String("yurtctl-servant-image", "openyurt/yurtctl-servant:latest",
		"The yurtctl-servant image.")
	cmd.Flags().String("kubeadm-conf-path", "",
		"The path to kubelet service conf that is used by kubelet component to join the cluster on the edge node.")
}

func isNodeReady(status *v1.NodeStatus) bool {
	_, condition := nodeutil.GetNodeCondition(status, v1.NodeReady)
	return condition != nil && condition.Status == v1.ConditionTrue
}

// Complete completes all the required options
func (r *RevertNodeOptions) Complete(flags *pflag.FlagSet) (err error) {
	ycsi, err := flags.GetString("yurtctl-servant-image")
	if err != nil {
		return err
	}
	r.YurtctlServantImage = ycsi

	kubeadmConfPath, err := flags.GetString("kubeadm-conf-path")
	if err != nil {
		return err
	}
	if kubeadmConfPath == "" {
		kubeadmConfPath = os.Getenv("KUBELET_SVC")
	}
	if kubeadmConfPath == "" {
		kubeadmConfPath = enutil.KubeletSvcPath
	}
	r.KubeadmConfPath = kubeadmConfPath

	r.clientSet, err = enutil.GenClientSet(flags)
	if err != nil {
		return err
	}

	openyurtDir := os.Getenv("OPENYURT_DIR")
	if openyurtDir == "" {
		openyurtDir = enutil.OpenyurtDir
	}
	r.openyurtDir = openyurtDir

	return
}

// RunRevertNode reverts the target Yurt node back to a standard Kubernetes node
func (r *RevertNodeOptions) RunRevertNode(workingMode util.WorkingMode) (err error) {
	// 1. check the server version
	if err = kubeutil.ValidateServerVersion(r.clientSet); err != nil {
		return
	}
	klog.V(4).Info("the server version is valid")

	nodeName, err := enutil.GetNodeName(r.KubeadmConfPath)
	if err != nil {
		nodeName = ""
	}
	if len(r.Nodes) > 1 || (len(r.Nodes) == 1 && r.Nodes[0] != nodeName) {
		// dispatch servant job to remote nodes
		err = r.dispatchServantJobs(workingMode)
	} else if (len(r.Nodes) == 0 && nodeName != "") || (len(r.Nodes) == 1 && r.Nodes[0] == nodeName) {
		// convert local node
		err = r.convertLocalNode(nodeName, workingMode)
	} else {
		return fmt.Errorf("fail to revert node, flag --edge-nodes or --cloud-nodes %s err", r.Nodes)
	}

	return err
}

// dispatchServantJobs dispatches servant job to remote nodes.
func (r *RevertNodeOptions) dispatchServantJobs(workingMode util.WorkingMode) error {
	nodeLst, err := r.clientSet.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return err
	}

	// 1. check the Nodes and its label
	var nodeNames []string
	for _, node := range nodeLst.Items {
		isEdgeNode, ok := node.Labels[projectinfo.GetEdgeWorkerLabelKey()]
		if workingMode == util.WorkingModeEdge && isEdgeNode == "true" {
			nodeNames = append(nodeNames, node.GetName())
		}
		if workingMode == util.WorkingModeCloud && (!ok || isEdgeNode == "false") {
			nodeNames = append(nodeNames, node.GetName())
		}
	}
	for _, node := range r.Nodes {
		if !strutil.IsInStringLst(nodeNames, node) {
			klog.Errorf("Cannot do the revert, the worker node: %s is not a Yurt edge node.", node)
			return err
		}
	}

	// 2. check the state of Nodes
	for _, node := range nodeLst.Items {
		if strutil.IsInStringLst(r.Nodes, node.GetName()) {
			if !isNodeReady(&node.Status) {
				klog.Errorf("Cannot do the revert, the status of worker node: %s is not 'Ready'.", node.Name)
				return err
			}
		}
	}

	// 3. remove yurt-hub and revert kubelet service
	ctx := map[string]string{
		"action":                "revert",
		"yurtctl_servant_image": r.YurtctlServantImage,
		"kubeadm_conf_path":     r.KubeadmConfPath,
	}

	switch workingMode {
	case util.WorkingModeCloud:
		ctx["sub_command"] = "cloudnode"
	case util.WorkingModeEdge:
		ctx["sub_command"] = "edgenode"
	}

	if err = kubeutil.RunServantJobs(r.clientSet, ctx, r.Nodes); err != nil {
		klog.Errorf("fail to revert node: %s", err)
		return err
	}
	return nil
}

// convertLocalNode converts the local node
func (r *RevertNodeOptions) convertLocalNode(nodeName string, workingMode util.WorkingMode) error {
	// 1. check if critical files exist
	yurtKubeletConf := r.getYurthubKubeletConf()
	if ok, err := enutil.FileExists(yurtKubeletConf); !ok {
		return err
	}
	kubeletSvcBk := r.getKubeletSvcBackup()
	if _, err := enutil.FileExists(kubeletSvcBk); err != nil {
		klog.Errorf("fail to get file %s, should revise the %s directly", kubeletSvcBk, r.KubeadmConfPath)
		return err
	}
	podManifestPath := enutil.GetPodManifestPath()
	yurthubYamlPath := getYurthubYaml(podManifestPath)
	if ok, err := enutil.FileExists(yurthubYamlPath); !ok {
		return err
	}
	// 2. check the state of Nodes
	node, err := r.clientSet.CoreV1().Nodes().Get(context.Background(), nodeName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if !isNodeReady(&node.Status) {
		klog.Errorf("Cannot do the revert, the status of worker node: %s is not 'Ready'.", node.Name)
		return err
	}

	// 3. check the label of Nodes
	isEdgeNode := node.Labels[projectinfo.GetEdgeWorkerLabelKey()]
	if workingMode == util.WorkingModeEdge && isEdgeNode != "true" {
		return fmt.Errorf("Cannot do the revert, the node: %s is not a Yurt edge node.", node.Name)
	}
	if workingMode == util.WorkingModeCloud && isEdgeNode == "true" {
		return fmt.Errorf("Cannot do the revert, the node: %s is not a Yurt cloud node.", node.Name)
	}

	// 3.4. remove yurt-hub and revert kubelet service
	if err := r.RevertKubelet(); err != nil {
		return fmt.Errorf("fail to revert kubelet: %v", err)
	}
	if err := r.RemoveYurthub(yurthubYamlPath); err != nil {
		return err
	}

	// 5. remove label of Nodes
	node, err = r.clientSet.CoreV1().Nodes().Get(context.Background(), nodeName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	_, foundAutonomy := node.Annotations[constants.AnnotationAutonomy]
	if foundAutonomy {
		delete(node.Annotations, constants.AnnotationAutonomy)
	}
	delete(node.Labels, projectinfo.GetEdgeWorkerLabelKey())
	if _, err = r.clientSet.CoreV1().Nodes().Update(context.Background(), node, metav1.UpdateOptions{}); err != nil {
		return err
	}
	klog.Infof("label %s is removed", projectinfo.GetEdgeWorkerLabelKey())
	if foundAutonomy {
		klog.Infof("annotation %s is removed", constants.AnnotationAutonomy)
	}

	return nil
}

// RevertKubelet resets the kubelet service
func (r *RevertNodeOptions) RevertKubelet() error {
	// 1. remove openyurt's kubelet.conf if exist
	yurtKubeletConf := r.getYurthubKubeletConf()
	if err := os.Remove(yurtKubeletConf); err != nil {
		return err
	}
	kubeletSvcBk := r.getKubeletSvcBackup()
	klog.Infof("found backup file %s, will use it to revert the node", kubeletSvcBk)
	err := os.Rename(kubeletSvcBk, r.KubeadmConfPath)
	if err != nil {
		return err
	}

	// 2. reset the kubelet.service
	klog.Info(enutil.DaemonReload)
	cmd := exec.Command("bash", "-c", enutil.DaemonReload)
	if err := enutil.Exec(cmd); err != nil {
		return err
	}

	klog.Info(enutil.RestartKubeletSvc)
	cmd = exec.Command("bash", "-c", enutil.RestartKubeletSvc)
	if err := enutil.Exec(cmd); err != nil {
		return err
	}
	klog.Infof("kubelet has been reset back to default")
	return nil
}

// RemoveYurthub deletes the yurt-hub pod
func (r *RevertNodeOptions) RemoveYurthub(yurthubYamlPath string) error {
	// 1. remove the yurt-hub.yaml to delete the yurt-hub
	err := os.Remove(yurthubYamlPath)
	if err != nil {
		return err
	}

	// 2. remove yurt-hub config directory and certificates in it
	yurthubConf := getYurthubConf()
	err = os.RemoveAll(yurthubConf)
	if err != nil {
		return err
	}
	klog.Infof("yurt-hub has been removed")
	return nil
}

func (r *RevertNodeOptions) getYurthubKubeletConf() string {
	return filepath.Join(r.openyurtDir, enutil.KubeletConfName)
}

func (r *RevertNodeOptions) getKubeletSvcBackup() string {
	return fmt.Sprintf(enutil.KubeletSvcBackup, r.KubeadmConfPath)
}

func getYurthubYaml(podManifestPath string) string {
	return filepath.Join(podManifestPath, enutil.YurthubYamlName)
}

func getYurthubConf() string {
	return filepath.Join(hubself.HubRootDir, hubself.HubName)
}
