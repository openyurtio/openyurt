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

package cmd

import (
	goflag "flag"

	"github.com/spf13/cobra"
	flag "github.com/spf13/pflag"
	"k8s.io/klog"

	"github.com/alibaba/openyurt/pkg/yurtctl/cmd/convert"
	"github.com/alibaba/openyurt/pkg/yurtctl/cmd/markautonomous"
	"github.com/alibaba/openyurt/pkg/yurtctl/cmd/revert"
)

// NewYurtctlCommand creates a new yurtctl command
func NewYurtctlCommand() *cobra.Command {
	cmds := &cobra.Command{
		Use:   "yurtctl",
		Short: "yurtctl controls the yurt cluster",
		Run:   showHelp,
	}

	// add kubeconfig to persistent flags
	cmds.PersistentFlags().String("kubeconfig", "", "The path to the kubeconfig file")
	cmds.AddCommand(convert.NewConvertCmd())
	cmds.AddCommand(revert.NewRevertCmd())
	cmds.AddCommand(markautonomous.NewMarkAutonomousCmd())

	klog.InitFlags(nil)
	// goflag.Parse()
	flag.CommandLine.AddGoFlagSet(goflag.CommandLine)

	return cmds
}

// showHelp shows the help message
func showHelp(cmd *cobra.Command, _ []string) {
	cmd.Help()
}
