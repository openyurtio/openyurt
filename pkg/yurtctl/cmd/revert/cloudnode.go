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
	"github.com/spf13/pflag"

	"github.com/openyurtio/openyurt/pkg/yurthub/util"

	"strings"

	"github.com/spf13/cobra"
	"k8s.io/klog"
)

// RevertCloudNodeOptions has the information required by sub command revert cloudnode
type RevertCloudNodeOptions struct {
	RevertNodeOptions
}

// NewRevertCloudNodeOptions creates a new RevertCloudNodeOptions
func NewRevertCloudNodeOptions() *RevertCloudNodeOptions {
	return &RevertCloudNodeOptions{}
}

// NewRevertCloudNodeCmd generates a new sub command revert edgenode and cloudnode
func NewRevertCloudNodeCmd() *cobra.Command {
	r := NewRevertCloudNodeOptions()
	cmd := &cobra.Command{
		Use:   "cloudnode",
		Short: "reverts the yurt cloud node to a kubernetes node",
		Run: func(cmd *cobra.Command, _ []string) {
			if err := r.Complete(cmd.Flags()); err != nil {
				klog.Fatalf("fail to complete the revert cloudnode option: %s", err)
			}
			if err := r.RunRevertNode(util.WorkingModeCloud); err != nil {
				klog.Fatalf("fail to revert the yurt node to a kubernetes node: %s", err)
			}
		},
	}
	cmd.Flags().StringP("cloud-nodes", "e", "",
		"The list of edge nodes wanted to be revert.(e.g. -e cloudnode1,cloudnode2)")
	commonFlags(cmd)
	return cmd
}

// Complete completes all the required options
func (r *RevertCloudNodeOptions) Complete(flags *pflag.FlagSet) error {
	enStr, err := flags.GetString("cloud-nodes")
	if err != nil {
		return err
	}
	if enStr != "" {
		r.Nodes = strings.Split(enStr, ",")
	}
	return r.RevertNodeOptions.Complete(flags)
}
