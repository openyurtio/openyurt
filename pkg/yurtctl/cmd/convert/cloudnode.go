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

package convert

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/klog"

	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

// ConvertCloudNodeOptions has the information required by sub command convert cloudnode
type ConvertCloudNodeOptions struct {
	ConvertNodeOptions
}

// NewConvertCloudNodeOptions creates a new ConvertCloudNodeOptions
func NewConvertCloudNodeOptions() *ConvertCloudNodeOptions {
	return &ConvertCloudNodeOptions{}
}

// NewConvertCloudNodeCmd generates a new sub command convert cloudnode
func NewConvertCloudNodeCmd() *cobra.Command {
	c := NewConvertCloudNodeOptions()
	cmd := &cobra.Command{
		Use:   "cloudnode",
		Short: "Converts the kubernetes node to a yurt cloud node",
		Run: func(cmd *cobra.Command, _ []string) {
			if err := c.Complete(cmd.Flags()); err != nil {
				klog.Fatalf("fail to complete the convert cloudnode option: %s", err)
			}
			if err := c.RunConvertNode(util.WorkingModeCloud); err != nil {
				klog.Fatalf("fail to convert the kubernetes node to a yurt node: %s", err)
			}
		},
	}
	cmd.Flags().StringP("cloud-nodes", "c", "",
		"The list of cloud nodes wanted to be convert.(e.g. -e cloudnode1,cloudnode2)")
	commonFlags(cmd)
	return cmd
}

// Complete completes all the required options.
func (c *ConvertCloudNodeOptions) Complete(flags *pflag.FlagSet) error {
	enStr, err := flags.GetString("cloud-nodes")
	if err != nil {
		return err
	}
	if enStr != "" {
		c.Nodes = strings.Split(enStr, ",")
	}
	return c.ConvertNodeOptions.Complete(flags)
}
