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

package clusterinfo

import (
	"context"
	"fmt"
	"io"
	"os"

	ct "github.com/daviddengcn/go-colortext"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
	kubeutil "github.com/openyurtio/openyurt/pkg/yurtctl/util/kubernetes"
)

// ClusterInfoOptions has the information that required by cluster-info operation
type ClusterInfoOptions struct {
	clientSet    *kubernetes.Clientset
	CloudNodes   []string
	EdgeNodes    []string
	ClusterNodes []string
	OtherNodes   []string
}

// NewClusterInfoOptions creates a new ClusterInfoOptions
func NewClusterInfoOptions() *ClusterInfoOptions {
	return &ClusterInfoOptions{
		CloudNodes:   []string{},
		EdgeNodes:    []string{},
		ClusterNodes: []string{},
		OtherNodes:   []string{},
	}
}

// NewClusterInfoCmd generates a new cluster-info command
func NewClusterInfoCmd() *cobra.Command {
	o := NewClusterInfoOptions()
	cmd := &cobra.Command{
		Use:   "cluster-info",
		Short: "list cloud nodes and edge nodes in cluster",
		Run: func(cmd *cobra.Command, _ []string) {
			if err := o.Complete(cmd.Flags()); err != nil {
				klog.Fatalf("fail to complete the cluster-info option: %s", err)
			}
			if err := o.Run(); err != nil {
				klog.Fatalf("fail to run cluster-info cmd: %s", err)
			}
		},
	}

	return cmd
}

// Complete completes all the required options
func (o *ClusterInfoOptions) Complete(flags *pflag.FlagSet) error {
	var err error
	o.clientSet, err = kubeutil.GenClientSet(flags)
	if err != nil {
		return err
	}
	return nil
}

// Validate makes sure provided values for ClusterInfoOptions are valid
func (o *ClusterInfoOptions) Validate() error {
	return nil
}

func (o *ClusterInfoOptions) Run() (err error) {
	key := projectinfo.GetEdgeWorkerLabelKey()
	Nodes, err := o.clientSet.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return
	}
	for _, node := range Nodes.Items {
		o.ClusterNodes = append(o.ClusterNodes, node.Name)
		if node.Labels[key] == "false" {
			o.CloudNodes = append(o.CloudNodes, node.Name)
		} else if node.Labels[key] == "true" {
			o.EdgeNodes = append(o.EdgeNodes, node.Name)
		} else {
			o.OtherNodes = append(o.OtherNodes, node.Name)
		}

	}
	printClusterInfo(os.Stdout, "openyurt cluster", o.ClusterNodes)
	printClusterInfo(os.Stdout, "openyurt cloud", o.CloudNodes)
	printClusterInfo(os.Stdout, "openyurt edge", o.EdgeNodes)
	printClusterInfo(os.Stdout, "other", o.OtherNodes)
	return
}

func printClusterInfo(out io.Writer, name string, nodes []string) {
	ct.ChangeColor(ct.Green, false, ct.None, false)
	fmt.Fprint(out, name)
	ct.ResetColor()
	fmt.Fprint(out, " nodes list ")
	ct.ChangeColor(ct.Yellow, false, ct.None, false)
	fmt.Fprint(out, nodes)
	ct.ResetColor()
	fmt.Fprintln(out, "")
}
