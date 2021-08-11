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
	nodeutil "k8s.io/kubernetes/pkg/controller/util/node"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
	enutil "github.com/openyurtio/openyurt/pkg/yurtctl/util/edgenode"
	kubeutil "github.com/openyurtio/openyurt/pkg/yurtctl/util/kubernetes"
	strutil "github.com/openyurtio/openyurt/pkg/yurtctl/util/strings"
)

const (
	kubeletConfigRegularExpression = "\\-\\-kubeconfig=.*kubelet.conf"
	apiserverAddrRegularExpression = "server: (http(s)?:\\/\\/)?[\\w][-\\w]{0,62}(\\.[\\w][-\\w]{0,62})*(:[\\d]{1,5})?"
	hubHealthzCheckFrequency       = 10 * time.Second
	filemode                       = 0666
	dirmode                        = 0755
)

// ConvertEdgeNodeOptions has the information required by sub command convert edgenode
type ConvertEdgeNodeOptions struct {
	clientSet                 *kubernetes.Clientset
	EdgeNodes                 []string
	YurthubImage              string
	YurthubHealthCheckTimeout time.Duration
	YurctlServantImage        string
	PodMainfestPath           string
	JoinToken                 string
	KubeadmConfPath           string
	openyurtDir               string
}

// NewConvertEdgeNodeOptions creates a new ConvertEdgeNodeOptions
func NewConvertEdgeNodeOptions() *ConvertEdgeNodeOptions {
	return &ConvertEdgeNodeOptions{}
}

// NewConvertEdgeNodeCmd generates a new sub command convert edgenode
func NewConvertEdgeNodeCmd() *cobra.Command {
	c := NewConvertEdgeNodeOptions()
	cmd := &cobra.Command{
		Use:   "edgenode",
		Short: "Converts the kubernetes node to a yurt node",
		Run: func(cmd *cobra.Command, _ []string) {
			if err := c.Complete(cmd.Flags()); err != nil {
				klog.Fatalf("fail to complete the convert edgenode option: %s", err)
			}
			if err := c.RunConvertEdgeNode(); err != nil {
				klog.Fatalf("fail to covert the kubernetes node to a yurt node: %s", err)
			}
		},
	}

	cmd.Flags().StringP("edge-nodes", "e", "",
		"The list of edge nodes wanted to be convert.(e.g. -e edgenode1,edgenode2)")
	cmd.Flags().String("yurthub-image", "openyurt/yurthub:latest",
		"The yurthub image.")
	cmd.Flags().Duration("yurthub-healthcheck-timeout", defaultYurthubHealthCheckTimeout,
		"The timeout for yurthub health check.")
	cmd.Flags().String("yurtctl-servant-image", "openyurt/yurtctl-servant:latest",
		"The yurtctl-servant image.")
	cmd.Flags().String("pod-manifest-path", "",
		"Path to the directory on edge node containing static pod files.")
	cmd.Flags().String("kubeadm-conf-path", "",
		"The path to kubelet service conf that is used by kubelet component to join the cluster on the edge node.")
	cmd.Flags().String("join-token", "", "The token used by yurthub for joining the cluster.")

	return cmd
}

// Complete completes all the required options
func (c *ConvertEdgeNodeOptions) Complete(flags *pflag.FlagSet) error {
	enStr, err := flags.GetString("edge-nodes")
	if err != nil {
		return err
	}
	if enStr != "" {
		c.EdgeNodes = strings.Split(enStr, ",")
	}

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

	podMainfestPath, err := flags.GetString("pod-manifest-path")
	if err != nil {
		return err
	}
	if podMainfestPath == "" {
		podMainfestPath = os.Getenv("STATIC_POD_PATH")
	}
	if podMainfestPath == "" {
		podMainfestPath = enutil.StaticPodPath
	}
	c.PodMainfestPath = podMainfestPath

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

// RunConvertEdgeNode converts a standard Kubernetes node to a Yurt node
func (c *ConvertEdgeNodeOptions) RunConvertEdgeNode() (err error) {
	// 1. check the server version
	if err = kubeutil.ValidateServerVersion(c.clientSet); err != nil {
		return
	}
	klog.V(4).Info("the server version is valid")

	nodeName, err := enutil.GetNodeName(c.KubeadmConfPath)
	if err != nil {
		nodeName = ""
	}
	if len(c.EdgeNodes) > 1 || (len(c.EdgeNodes) == 1 && c.EdgeNodes[0] != nodeName) {
		// 2 remote edgenode convert
		nodeLst, err := c.clientSet.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
		if err != nil {
			return err
		}

		// 2.1. check the EdgeNodes and its label
		var edgeNodeNames []string
		for _, node := range nodeLst.Items {
			_, ok := node.Labels[projectinfo.GetEdgeWorkerLabelKey()]
			if !ok {
				edgeNodeNames = append(edgeNodeNames, node.GetName())
			}
		}
		for _, edgeNode := range c.EdgeNodes {
			if !strutil.IsInStringLst(edgeNodeNames, edgeNode) {
				return fmt.Errorf("Cannot do the convert, the worker node: %s is not a Kubernetes node.", edgeNode)
			}
		}

		// 2.2. check the state of EdgeNodes
		for _, node := range nodeLst.Items {
			if strutil.IsInStringLst(c.EdgeNodes, node.GetName()) {
				_, condition := nodeutil.GetNodeCondition(&node.Status, v1.NodeReady)
				if condition == nil || condition.Status != v1.ConditionTrue {
					return fmt.Errorf("Cannot do the convert, the status of worker node: %s is not 'Ready'.", node.Name)
				}
			}
		}

		// 2.3. deploy yurt-hub and reset the kubelet service
		ctx := map[string]string{
			"action":                "convert",
			"yurtctl_servant_image": c.YurctlServantImage,
			"yurthub_image":         c.YurthubImage,
			"joinToken":             c.JoinToken,
			"pod_manifest_path":     c.PodMainfestPath,
			"kubeadm_conf_path":     c.KubeadmConfPath,
		}

		if c.YurthubHealthCheckTimeout != defaultYurthubHealthCheckTimeout {
			ctx["yurthub_healthcheck_timeout"] = c.YurthubHealthCheckTimeout.String()
		}

		if err = kubeutil.RunServantJobs(c.clientSet, ctx, c.EdgeNodes); err != nil {
			klog.Errorf("fail to run ServantJobs: %s", err)
			return err
		}
	} else if (len(c.EdgeNodes) == 0 && nodeName != "") || (len(c.EdgeNodes) == 1 && c.EdgeNodes[0] == nodeName) {
		// 3. local edgenode convert
		// 3.1. check if critical files exist
		if _, err := enutil.FileExists(c.KubeadmConfPath); err != nil {
			return err
		}
		if ok, err := enutil.DirExists(c.PodMainfestPath); !ok {
			return err
		}

		// 3.2. check the state of EdgeNodes
		node, err := c.clientSet.CoreV1().Nodes().Get(context.Background(), nodeName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		_, condition := nodeutil.GetNodeCondition(&node.Status, v1.NodeReady)
		if condition == nil || condition.Status != v1.ConditionTrue {
			return fmt.Errorf("Cannot do the convert, the status of worker node: %s is not 'Ready'.", node.Name)
		}

		// 3.3. check the label of EdgeNodes
		_, ok := node.Labels[projectinfo.GetEdgeWorkerLabelKey()]
		if ok {
			return fmt.Errorf("Cannot do the convert, the worker node: %s is not a Kubernetes node.", node.Name)
		}

		// 3.4. deploy yurt-hub and reset the kubelet service
		err = c.SetupYurthub()
		if err != nil {
			return fmt.Errorf("fail to set up the yurthub pod: %v", err)
		}
		err = c.ResetKubelet()
		if err != nil {
			return fmt.Errorf("fail to reset the kubelet service: %v", err)
		}

		// 3.5. label node as edge node
		klog.Infof("mark %s as the edge-node", nodeName)
		node, err = c.clientSet.CoreV1().Nodes().Get(context.Background(), nodeName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		_, err = kubeutil.LabelNode(c.clientSet, node, projectinfo.GetEdgeWorkerLabelKey(), "true")
		if err != nil {
			return err
		}
	} else {
		return fmt.Errorf("fail to revert edge node, flag --edge-nodes %s err", c.EdgeNodes)
	}
	return nil
}

// SetupYurthub sets up the yurthub pod and wait for the its status to be Running
func (c *ConvertEdgeNodeOptions) SetupYurthub() error {
	// 1. put yurt-hub yaml into /etc/kubernetes/manifests
	klog.Infof("setting up yurthub on node")

	// 1-1. get apiserver address
	kubeletConfPath, err := enutil.GetSingleContentFromFile(c.KubeadmConfPath, kubeletConfigRegularExpression)
	if err != nil {
		return err
	}
	kubeletConfPath = strings.Split(kubeletConfPath, "=")[1]
	apiserverAddr, err := enutil.GetSingleContentFromFile(kubeletConfPath, apiserverAddrRegularExpression)
	if err != nil {
		return err
	}
	apiserverAddr = strings.Split(apiserverAddr, " ")[1]

	// 1-2. replace variables in yaml file
	klog.Infof("setting up yurthub apiserver addr")
	yurthubTemplate := enutil.ReplaceRegularExpression(enutil.YurthubTemplate,
		map[string]string{
			"__kubernetes_service_addr__": apiserverAddr,
			"__yurthub_image__":           c.YurthubImage,
			"__join_token__":              c.JoinToken,
		})

	// 1-3. create yurthub.yaml
	_, err = enutil.DirExists(c.PodMainfestPath)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(c.getYurthubYaml(), []byte(yurthubTemplate), filemode)
	if err != nil {
		return err
	}
	klog.Infof("create the %s/yurt-hub.yaml", c.PodMainfestPath)

	// 2. wait yurthub pod to be ready
	err = hubHealthcheck(c.YurthubHealthCheckTimeout)
	return err
}

// ResetKubelet changes the configuration of the kubelet service and restart it
func (c *ConvertEdgeNodeOptions) ResetKubelet() error {
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

func (c *ConvertEdgeNodeOptions) getYurthubYaml() string {
	return filepath.Join(c.PodMainfestPath, enutil.YurthubYamlName)
}

func (c *ConvertEdgeNodeOptions) getYurthubKubeletConf() string {
	return filepath.Join(c.openyurtDir, enutil.KubeletConfName)
}

func (c *ConvertEdgeNodeOptions) getKubeletSvcBackup() string {
	return fmt.Sprintf(enutil.KubeletSvcBackup, c.KubeadmConfPath)
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
