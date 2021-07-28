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
	"fmt"
	"os"

	"github.com/spf13/cobra"
	flag "github.com/spf13/pflag"
	"k8s.io/klog"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/yurtctl/cmd/clusterinfo"
	"github.com/openyurtio/openyurt/pkg/yurtctl/cmd/convert"
	"github.com/openyurtio/openyurt/pkg/yurtctl/cmd/join"
	"github.com/openyurtio/openyurt/pkg/yurtctl/cmd/markautonomous"
	"github.com/openyurtio/openyurt/pkg/yurtctl/cmd/revert"
)

// NewYurtctlCommand creates a new yurtctl command
func NewYurtctlCommand() *cobra.Command {
	cmds := &cobra.Command{
		Use:   "yurtctl",
		Short: "yurtctl controls the yurt cluster",
		Run: func(cmd *cobra.Command, args []string) {
			printV, _ := cmd.Flags().GetBool("version")
			if printV {
				fmt.Printf("yurtctl: %#v\n", projectinfo.Get())
				return
			}
			fmt.Printf("yurtctl version: %#v\n", projectinfo.Get())

			showHelp(cmd, args)
		},
	}

	// add kubeconfig to persistent flags
	cmds.PersistentFlags().String("kubeconfig", "", "The path to the kubeconfig file")
	cmds.PersistentFlags().Bool("version", false, "print  the version information.")
	cmds.AddCommand(convert.NewConvertCmd())
	cmds.AddCommand(revert.NewRevertCmd())
	cmds.AddCommand(markautonomous.NewMarkAutonomousCmd())
	cmds.AddCommand(clusterinfo.NewClusterInfoCmd())
	cmds.AddCommand(join.NewCmdJoin(os.Stdout, nil))

	klog.InitFlags(nil)
	// goflag.Parse()
	flag.CommandLine.AddGoFlagSet(goflag.CommandLine)

	return cmds
}

// showHelp shows the help message
func showHelp(cmd *cobra.Command, _ []string) {
	cmd.Help()
}
