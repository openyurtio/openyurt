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

package markautonomous

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
	kubeutil "github.com/openyurtio/openyurt/pkg/yurtadm/util/kubernetes"
	"github.com/openyurtio/openyurt/pkg/yurtctl/constants"
	"github.com/openyurtio/openyurt/pkg/yurtctl/lock"
)

// MarkAutonomousOptions has the information that required by convert operation
type MarkAutonomousOptions struct {
	*kubernetes.Clientset
	AutonomousNodes  []string
	MarkAllEdgeNodes bool
}

// NewMarkAutonomousOptions creates a new MarkAutonomousOptions
func NewMarkAutonomousOptions() *MarkAutonomousOptions {
	return &MarkAutonomousOptions{}
}

// NewMarkAutonomousCmd generates a new markautonomous command
func NewMarkAutonomousCmd() *cobra.Command {
	co := NewMarkAutonomousOptions()
	cmd := &cobra.Command{
		Use:   "markautonomous -a AUTONOMOUSNODES",
		Short: "mark the nodes as autonomous",
		Run: func(cmd *cobra.Command, _ []string) {
			if err := co.Complete(cmd.Flags()); err != nil {
				klog.Fatalf("fail to complete the markautonomous option: %s", err)
			}
			if err := co.RunMarkAutonomous(); err != nil {
				klog.Fatalf("fail to make nodes autonomous: %s", err)
			}
		},
		Args: cobra.NoArgs,
	}

	cmd.Flags().StringP("autonomous-nodes", "a", "",
		"The list of nodes that will be marked as autonomous. If not set, all edge nodes will be marked as autonomous."+
			"(e.g. -a autonomousnode1,autonomousnode2)")

	return cmd
}

// Complete completes all the required options
func (mao *MarkAutonomousOptions) Complete(flags *pflag.FlagSet) error {
	anStr, err := flags.GetString("autonomous-nodes")
	if err != nil {
		return err
	}
	if anStr == "" {
		mao.AutonomousNodes = []string{}
	} else {
		mao.AutonomousNodes = strings.Split(anStr, ",")
	}

	// set mark-all-edge-node to false, as user has specified autonomous nodes
	if len(mao.AutonomousNodes) == 0 {
		mao.MarkAllEdgeNodes = true
	}

	mao.Clientset, err = kubeutil.GenClientSet(flags)
	if err != nil {
		return err
	}

	return nil
}

// RunMarkAutonomous annotates specified edge nodes as autonomous
func (mao *MarkAutonomousOptions) RunMarkAutonomous() (err error) {
	if err = lock.AcquireLock(mao.Clientset); err != nil {
		return
	}
	defer func() {
		err = lock.ReleaseLock(mao.Clientset)
	}()
	var (
		autonomousNodes []*v1.Node
		edgeNodeList    *v1.NodeList
	)
	if mao.MarkAllEdgeNodes {
		// make all edge nodes autonomous
		labelSelector := fmt.Sprintf("%s=true", projectinfo.GetEdgeWorkerLabelKey())
		edgeNodeList, err = mao.CoreV1().Nodes().
			List(context.Background(), metav1.ListOptions{LabelSelector: labelSelector})
		if err != nil {
			return
		}
		if len(edgeNodeList.Items) == 0 {
			klog.Warning("there is no edge nodes, please label the edge node first")
			return
		}
		for i := range edgeNodeList.Items {
			autonomousNodes = append(autonomousNodes, &edgeNodeList.Items[i])
		}
	} else {
		// make only the specified edge nodes autonomous
		for _, nodeName := range mao.AutonomousNodes {
			var node *v1.Node
			node, err = mao.CoreV1().Nodes().Get(context.Background(), nodeName, metav1.GetOptions{})
			if err != nil {
				return
			}
			if node.Labels[projectinfo.GetEdgeWorkerLabelKey()] == "false" {
				err = fmt.Errorf("can't make cloud node(%s) autonomous",
					node.GetName())
				return
			}
			autonomousNodes = append(autonomousNodes, node)
		}
	}

	for _, anode := range autonomousNodes {
		klog.Infof("mark %s as autonomous", anode.GetName())
		if _, err = kubeutil.AnnotateNode(mao.Clientset,
			anode, constants.AnnotationAutonomy, "true"); err != nil {
			return
		}
	}

	return
}
