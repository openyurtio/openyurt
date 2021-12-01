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
	"time"

	"github.com/spf13/cobra"
	"k8s.io/klog/v2"

	nodeconverter "github.com/openyurtio/openyurt/pkg/node-servant/convert"
)

const (
	// defaultYurthubHealthCheckTimeout defines the default timeout for yurthub health check phase
	defaultYurthubHealthCheckTimeout = 2 * time.Minute
)

// NewConvertCmd generates a new convert command
func NewConvertCmd() *cobra.Command {
	o := nodeconverter.NewConvertOptions()
	cmd := &cobra.Command{
		Use:   "convert --working-mode",
		Short: "",
		Run: func(cmd *cobra.Command, args []string) {
			if err := o.Complete(cmd.Flags()); err != nil {
				klog.Fatalf("fail to complete the convert option: %s", err)
			}

			converter := nodeconverter.NewConverterWithOptions(o)
			if err := converter.Do(); err != nil {
				klog.Fatalf("fail to convert the kubernetes node to a yurt node: %s", err)
			}
			klog.Info("convert success")
		},
	}
	setFlags(cmd)

	return cmd
}

// setFlags sets flags.
func setFlags(cmd *cobra.Command) {
	cmd.Flags().String("yurthub-image", "openyurt/yurthub:latest",
		"The yurthub image.")
	cmd.Flags().Duration("yurthub-healthcheck-timeout", defaultYurthubHealthCheckTimeout,
		"The timeout for yurthub health check.")
	cmd.Flags().String("kubeadm-conf-path", "",
		"The path to kubelet service conf that is used by kubelet component to join the cluster on the work node.")
	cmd.Flags().String("join-token", "", "The token used by yurthub for joining the cluster.")
	cmd.Flags().String("working-mode", "edge", "The node type cloud/edge, effect yurthub workingMode.")
}
