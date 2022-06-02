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
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/yurtctl/cmd/clusterinfo"
	"github.com/openyurtio/openyurt/pkg/yurtctl/cmd/markautonomous"
	"github.com/openyurtio/openyurt/pkg/yurtctl/cmd/yurttest"
)

// NewYurtctlCommand creates a new yurtctl command
func NewYurtctlCommand() *cobra.Command {
	version := fmt.Sprintf("%#v", projectinfo.Get())
	cmds := &cobra.Command{
		Use:     "yurtctl",
		Short:   "yurtctl controls the yurt cluster",
		Version: version,
	}

	setVersion(cmds)
	// add kubeconfig to persistent flags
	cmds.PersistentFlags().String("kubeconfig", "", "The path to the kubeconfig file")
	cmds.AddCommand(markautonomous.NewMarkAutonomousCmd())
	cmds.AddCommand(clusterinfo.NewClusterInfoCmd())
	cmds.AddCommand(yurttest.NewCmdTest(os.Stdout))

	klog.InitFlags(nil)
	// goflag.Parse()
	flag.CommandLine.AddGoFlagSet(goflag.CommandLine)

	return cmds
}

func setVersion(cmd *cobra.Command) {
	cmd.SetVersionTemplate(`{{with .Name}}{{printf "%s " .}}{{end}}{{printf "version: %s" .Version}}`)
}
