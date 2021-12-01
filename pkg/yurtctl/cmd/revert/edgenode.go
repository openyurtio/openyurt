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
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

// RevertEdgeNodeOptions has the information required by sub command revert edgenode
type RevertEdgeNodeOptions struct {
	RevertNodeOptions
}

// NewRevertEdgeNodeOptions creates a new RevertEdgeNodeOptions
func NewRevertEdgeNodeOptions() *RevertEdgeNodeOptions {
	return &RevertEdgeNodeOptions{}
}

// NewRevertEdgeNodeCmd generates a new sub command revert edgenode
func NewRevertEdgeNodeCmd() *cobra.Command {
	r := NewRevertEdgeNodeOptions()
	cmd := &cobra.Command{
		Use:   "edgenode",
		Short: "reverts the yurt edge node to a kubernetes node",
		Run: func(cmd *cobra.Command, _ []string) {
			if err := r.Complete(cmd.Flags()); err != nil {
				klog.Fatalf("fail to complete the revert edgenode option: %s", err)
			}
			if err := r.RunRevertNode(util.WorkingModeEdge); err != nil {
				klog.Fatalf("fail to revert the yurt node to a kubernetes node: %s", err)
			}
		},
	}
	cmd.Flags().StringP("edge-nodes", "e", "",
		"The list of edge nodes wanted to be revert.(e.g. -e edgenode1,edgenode2)")
	commonFlags(cmd)
	return cmd
}

// Complete completes all the required options
func (r *RevertEdgeNodeOptions) Complete(flags *pflag.FlagSet) (err error) {
	enStr, err := flags.GetString("edge-nodes")
	if err != nil {
		return err
	}
	if enStr != "" {
		r.Nodes = strings.Split(enStr, ",")
	}
	return r.RevertNodeOptions.Complete(flags)
}
