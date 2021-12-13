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

package preflight_convert

import (
	"os"

	"github.com/spf13/cobra"
	"k8s.io/klog/v2"

	preflightconvert "github.com/openyurtio/openyurt/pkg/node-servant/preflight-convert"
)

const (
	latestYurtHubImage         = "openyurt/yurthub:latest"
	latestYurtTunnelAgentImage = "openyurt/yurt-tunnel-agent:latest"
)

// NewxPreflightConvertCmd generates a new preflight-convert check command
func NewxPreflightConvertCmd() *cobra.Command {
	o := preflightconvert.NewPreflightConvertOptions()
	cmd := &cobra.Command{
		Use:   "preflight-convert",
		Short: "",
		Run: func(cmd *cobra.Command, args []string) {
			if err := o.Complete(cmd.Flags()); err != nil {
				klog.Errorf("Fail to complete the preflight-convert option: %s", err)
				os.Exit(1)
			}
			preflighter := preflightconvert.NewPreflighterWithOptions(o)
			if err := preflighter.Do(); err != nil {
				klog.Errorf("Fail to run pre-flight checks: %s", err)
				os.Exit(1)
			}
			klog.Info("convert pre-flight checks success")
		},
	}
	setFlags(cmd)

	return cmd
}

func setFlags(cmd *cobra.Command) {
	cmd.Flags().StringP("kubeadm-conf-path", "k", "",
		"The path to kubelet service conf that is used by kubelet component to join the cluster on the work node."+
			"Support multiple values, will search in order until get the file.(e.g -k kbcfg1,kbcfg2)",
	)
	cmd.Flags().String("yurthub-image", latestYurtHubImage, "The yurthub image.")
	cmd.Flags().String("yurt-tunnel-agent-image", latestYurtTunnelAgentImage, "The yurt-tunnel-agent image.")
	cmd.Flags().BoolP("deploy-yurttunnel", "t", false, "If set, yurt-tunnel-agent will be deployed.")
	cmd.Flags().String("ignore-preflight-errors", "", "A list of checks whose errors will be shown as warnings. "+
		"Example: 'isprivilegeduser,imagepull'.Value 'all' ignores errors from all checks.",
	)
}
