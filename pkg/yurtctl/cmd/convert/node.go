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

package convert

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog"

	nodeutil "github.com/openyurtio/openyurt/pkg/controller/util/node"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/yurtctl/constants"
	enutil "github.com/openyurtio/openyurt/pkg/yurtctl/util/edgenode"
	kubeutil "github.com/openyurtio/openyurt/pkg/yurtctl/util/kubernetes"
	strutil "github.com/openyurtio/openyurt/pkg/yurtctl/util/strings"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

const (
	kubeletConfigRegularExpression = "\\-\\-kubeconfig=.*kubelet.conf"
	apiserverAddrRegularExpression = "server: (http(s)?:\\/\\/)?[\\w][-\\w]{0,62}(\\.[\\w][-\\w]{0,62})*(:[\\d]{1,5})?"
	hubHealthzCheckFrequency       = 10 * time.Second
	filemode                       = 0666
	dirmode                        = 0755
)

// ConvertNodeOptions has the information required by sub command convert edgenode and convert cloudnode
type ConvertNodeOptions struct {
	clientSet                 *kubernetes.Clientset
	Nodes                     []string
	YurthubImage              string
	YurthubHealthCheckTimeout time.Duration
	YurctlServantImage        string
	JoinToken                 string
	KubeadmConfPath           string
	openyurtDir               string
}

// commonFlags sets all common flags.
func commonFlags(cmd *cobra.Command) {
	cmd.Flags().String("yurthub-image", "openyurt/yurthub:latest",
		"The yurthub image.")
	cmd.Flags().Duration("yurthub-healthcheck-timeout", defaultYurthubHealthCheckTimeout,
		"The timeout for yurthub health check.")
	cmd.Flags().String("yurtctl-servant-image", "openyurt/yurtctl-servant:latest",
		"The yurtctl-servant image.")
	cmd.Flags().String("kubeadm-conf-path", "",
		"The path to kubelet service conf that is used by kubelet component to join the cluster on the edge node.")
	cmd.Flags().String("join-token", "", "The token used by yurthub for joining the cluster.")
}

// Complete completes all the required options.
func (c *ConvertNodeOptions) Complete(flags *pflag.FlagSet) error {
	yurthubImage, err := flags.GetString("yurthub-image")
	if err != nil {
		return err
	}
	c.YurthubImage = yurthubImage

	yurthubHealthCheckTimeout, err := flags.GetDuration("yurthub-healthcheck-timeout")
	if err != nil {
		return err
	}
	c.YurthubHealthCheckTimeout = yurthubHealthCheckTimeout

	ycsi, err := flags.GetString("yurtctl-servant-image")
	if err != nil {
		return err
	}
	c.YurctlServantImage = ycsi

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
	c.KubeadmConfPath = kubeadmConfPath

	c.clientSet, err = enutil.GenClientSet(flags)
	if err != nil {
		return err
	}

	joinToken, err := flags.GetString("join-token")
	if err != nil {
		return err
	}
	if joinToken == "" {
		joinToken, err = kubeutil.GetOrCreateJoinTokenString(c.clientSet)
		if err != nil {
			return err
		}
	}
	c.JoinToken = joinToken

	openyurtDir := os.Getenv("OPENYURT_DIR")
	if openyurtDir == "" {
		openyurtDir = enutil.OpenyurtDir
	}
	c.openyurtDir = openyurtDir

	return nil
}

// SetupYurthub sets up the yurthub pod and wait for the its status to be Running.
func (c *ConvertNodeOptions) SetupYurthub(workingMode util.WorkingMode) error {
	// 1. put yurt-hub yaml into /etc/kubernetes/manifests
	klog.Infof("setting up yurthub on node")

	// 1-1. get apiserver address
	kubeletConfPath, err := enutil.GetSingleContentFromFile(c.KubeadmConfPath, kubeletConfigRegularExpression)
	if err != nil {
		return err
	}
	kubeletConfPathArr := strings.Split(kubeletConfPath, "=")
	if len(kubeletConfPathArr) < 2 {
		return fmt.Errorf("--kubeconfig-path format error")
	}
	apiserverAddr, err := enutil.GetSingleContentFromFile(kubeletConfPathArr[1], apiserverAddrRegularExpression)
	if err != nil {
		return err
	}
	apiserverAddrArr := strings.Split(apiserverAddr, " ")
	if len(apiserverAddrArr) < 2 {
		return fmt.Errorf("--kubeconfig-path format error")
	}

	// 1-2. replace variables in yaml file
	klog.Infof("setting up yurthub apiserver addr")
	yurthubTemplate := enutil.ReplaceRegularExpression(enutil.YurthubTemplate,
		map[string]string{
			"__kubernetes_service_addr__": apiserverAddrArr[1],
			"__yurthub_image__":           c.YurthubImage,
			"__join_token__":              c.JoinToken,
			"__working_mode__":            string(workingMode),
		})

	// 1-3. create yurthub.yaml
	podManifestPath := enutil.GetPodManifestPath()
	if err != nil {
		return err
	}
	if err = enutil.EnsureDir(podManifestPath); err != nil {
		return err
	}
	err = ioutil.WriteFile(getYurthubYaml(podManifestPath), []byte(yurthubTemplate), filemode)
	if err != nil {
		return err
	}
	klog.Infof("create the %s/yurt-hub.yaml", podManifestPath)

	// 2. wait yurthub pod to be ready
	err = hubHealthcheck(c.YurthubHealthCheckTimeout)
	return err
}

// ResetKubelet changes the configuration of the kubelet service and restart it
func (c *ConvertNodeOptions) ResetKubelet() error {
	// 1. create a working dir to store revised kubelet.conf
	err := os.MkdirAll(c.openyurtDir, dirmode)
	if err != nil {
		return err
	}
	fullpath := c.getYurthubKubeletConf()
	err = ioutil.WriteFile(fullpath, []byte(enutil.OpenyurtKubeletConf), filemode)
	if err != nil {
		return err
	}
	klog.Infof("revised kubeconfig %s is generated", fullpath)

	// 2. revise the kubelet.service drop-in
	// 2.1 make a backup for the origin kubelet.service
	bkfile := c.getKubeletSvcBackup()
	err = enutil.CopyFile(c.KubeadmConfPath, bkfile, 0666)
	if err != nil {
		return err
	}

	// 2.2 revise the drop-in, point it to the $OPENYURT_DIR/kubelet.conf
	contentbyte, err := ioutil.ReadFile(c.KubeadmConfPath)
	if err != nil {
		return err
	}
	kubeConfigSetup := fmt.Sprintf("--kubeconfig=%s/kubelet.conf", c.openyurtDir)
	content := enutil.ReplaceRegularExpression(string(contentbyte), map[string]string{
		"--bootstrap.*bootstrap-kubelet.conf": "",
		"--kubeconfig=.*kubelet.conf":         kubeConfigSetup,
	})
	err = ioutil.WriteFile(c.KubeadmConfPath, []byte(content), filemode)
	if err != nil {
		return err
	}
	klog.Info("kubelet.service drop-in file is revised")

	// 3. reset the kubelet.service
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
	klog.Infof("kubelet has been restarted")
	return nil
}

func (c *ConvertNodeOptions) getYurthubKubeletConf() string {
	return filepath.Join(c.openyurtDir, enutil.KubeletConfName)
}

func (c *ConvertNodeOptions) getKubeletSvcBackup() string {
	return fmt.Sprintf(enutil.KubeletSvcBackup, c.KubeadmConfPath)
}

func getYurthubYaml(podManifestPath string) string {
	return filepath.Join(podManifestPath, enutil.YurthubYamlName)
}

// hubHealthcheck will check the status of yurthub pod
func hubHealthcheck(timeout time.Duration) error {
	serverHealthzURL, err := url.Parse(fmt.Sprintf("http://%s", enutil.ServerHealthzServer))
	if err != nil {
		return err
	}
	serverHealthzURL.Path = enutil.ServerHealthzURLPath

	start := time.Now()
	return wait.PollImmediate(hubHealthzCheckFrequency, timeout, func() (bool, error) {
		_, err := pingClusterHealthz(http.DefaultClient, serverHealthzURL.String())
		if err != nil {
			klog.Infof("yurt-hub is not ready, ping cluster healthz with result: %v", err)
			return false, nil
		}
		klog.Infof("yurt-hub healthz is OK after %f seconds", time.Since(start).Seconds())
		return true, nil
	})
}

func pingClusterHealthz(client *http.Client, addr string) (bool, error) {
	if client == nil {
		return false, fmt.Errorf("http client is invalid")
	}

	resp, err := client.Get(addr)
	if err != nil {
		return false, err
	}

	b, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		return false, fmt.Errorf("failed to read response of cluster healthz, %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("response status code is %d", resp.StatusCode)
	}

	if strings.ToLower(string(b)) != "ok" {
		return false, fmt.Errorf("cluster healthz is %s", string(b))
	}

	return true, nil
}

func isNodeReady(status *v1.NodeStatus) bool {
	_, condition := nodeutil.GetNodeCondition(status, v1.NodeReady)
	return condition != nil && condition.Status == v1.ConditionTrue
}

// RunConvertNode converts a standard Kubernetes node to a Yurt node
func (c *ConvertNodeOptions) RunConvertNode(workingMode util.WorkingMode) (err error) {
	// 1. check the server version
	if err = kubeutil.ValidateServerVersion(c.clientSet); err != nil {
		return
	}
	klog.V(4).Info("the server version is valid")

	nodeName, err := enutil.GetNodeName(c.KubeadmConfPath)
	if err != nil {
		nodeName = ""
	}
	if len(c.Nodes) > 1 || (len(c.Nodes) == 1 && c.Nodes[0] != nodeName) {
		// dispatch servant job to remote nodes
		err = c.dispatchServantJobs(workingMode)
	} else if (len(c.Nodes) == 0 && nodeName != "") || (len(c.Nodes) == 1 && c.Nodes[0] == nodeName) {
		// convert local node
		err = c.convertLocalNode(nodeName, workingMode)
	} else {
		return fmt.Errorf("fail to convert node, flag --edge-nodes or --cloud-nodes %s err", c.Nodes)
	}
	return err
}

// dispatchServantJobs dispatches servant job to remote nodes.
func (c *ConvertNodeOptions) dispatchServantJobs(workingMode util.WorkingMode) error {
	nodeLst, err := c.clientSet.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return err
	}

	// 1. check the nodes and its label
	var nodeNames []string
	for _, node := range nodeLst.Items {
		labelValue, ok := node.Labels[projectinfo.GetEdgeWorkerLabelKey()]
		// The label of edge nodes will be set by the servant job that we are going to dispatch here,
		// so we assume that when dispatching servant job to edge nodes, the label should not be set.
		if workingMode == util.WorkingModeEdge && !ok {
			nodeNames = append(nodeNames, node.GetName())
		}
		// The label of cloud nodes is usually already set by yurtctl convert before dispatching the servant job.
		if workingMode == util.WorkingModeCloud && labelValue == "false" {
			nodeNames = append(nodeNames, node.GetName())
		}
	}

	for _, node := range c.Nodes {
		if !strutil.IsInStringLst(nodeNames, node) {
			return fmt.Errorf("Cannot do the convert, the node: %s is not a Kubernetes node.", node)
		}
	}

	// 2. check the state of nodes
	for _, node := range nodeLst.Items {
		if strutil.IsInStringLst(c.Nodes, node.GetName()) {
			if !isNodeReady(&node.Status) {
				return fmt.Errorf("Cannot do the convert, the status of node: %s is not 'Ready'.", node.Name)
			}
		}
	}

	// 3. deploy yurt-hub and reset the kubelet service
	ctx := map[string]string{
		"action":                "convert",
		"yurtctl_servant_image": c.YurctlServantImage,
		"yurthub_image":         c.YurthubImage,
		"joinToken":             c.JoinToken,
		"kubeadm_conf_path":     c.KubeadmConfPath,
	}

	switch workingMode {
	case util.WorkingModeCloud:
		ctx["sub_command"] = "cloudnode"
	case util.WorkingModeEdge:
		ctx["sub_command"] = "edgenode"
	}
	if c.YurthubHealthCheckTimeout != defaultYurthubHealthCheckTimeout {
		ctx["yurthub_healthcheck_timeout"] = c.YurthubHealthCheckTimeout.String()
	}

	if err = kubeutil.RunServantJobs(c.clientSet, ctx, c.Nodes); err != nil {
		klog.Errorf("fail to run ServantJobs: %s", err)
		return err
	}
	return nil
}

// convertLocalNode converts the local node
func (c *ConvertNodeOptions) convertLocalNode(nodeName string, workingMode util.WorkingMode) error {
	// 1. check if critical files exist
	if _, err := enutil.FileExists(c.KubeadmConfPath); err != nil {
		return err
	}

	// 2. check the state of node
	node, err := c.clientSet.CoreV1().Nodes().Get(context.Background(), nodeName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if !isNodeReady(&node.Status) {
		return fmt.Errorf("Cannot do the convert, the status of node: %s is not 'Ready'.", node.Name)
	}

	// 3. check the label of node
	labelVal, ok := node.Labels[projectinfo.GetEdgeWorkerLabelKey()]
	if workingMode == util.WorkingModeEdge && ok {
		return fmt.Errorf("Cannot do the convert, the edge node: %s is not a Kubernetes node.", node.Name)
	}
	if workingMode == util.WorkingModeCloud && labelVal != "false" {
		return fmt.Errorf("Cannot do the convert, the cloud node: %s is not a Kubernetes node.", node.Name)
	}

	// 4. deploy yurt-hub and reset the kubelet service
	err = c.SetupYurthub(workingMode)
	if err != nil {
		return fmt.Errorf("fail to set up the yurthub pod: %v", err)
	}
	err = c.ResetKubelet()
	if err != nil {
		return fmt.Errorf("fail to reset the kubelet service: %v", err)
	}

	if workingMode == util.WorkingModeEdge {
		// 5. label node as edge node
		klog.Infof("mark %s as the edge-node", nodeName)
		node, err = c.clientSet.CoreV1().Nodes().Get(context.Background(), nodeName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		node, err = kubeutil.LabelNode(c.clientSet, node, projectinfo.GetEdgeWorkerLabelKey(), "true")
		if err != nil {
			return err
		}
		// 6. open the autonomous
		klog.Infof("open the %s autonomous", nodeName)
		_, err = kubeutil.AnnotateNode(c.clientSet, node, constants.AnnotationAutonomy, "true")
		if err != nil {
			return err
		}
	}
	return nil
}
