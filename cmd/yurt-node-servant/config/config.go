/*
Copyright 2022 The OpenYurt Authors.

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

package config

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/node-servant/config"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
)

// NewConfigCmd generates a new config command
func NewConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:       "config",
		Short:     "manage configuration of OpenYurt cluster",
		RunE:      cobra.OnlyValidArgs,
		ValidArgs: []string{"control-plane"},
		Args:      cobra.MaximumNArgs(1),
	}
	cmd.AddCommand(newCmdConfigControlPlane())

	return cmd
}

func newCmdConfigControlPlane() *cobra.Command {
	o := config.NewControlPlaneOptions()
	cmd := &cobra.Command{
		Use:   "control-plane",
		Short: "configure control-plane components like kube-apiserver and kube-controller-manager",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("node-servant version: %#v\n", projectinfo.Get())
			if o.Version {
				return nil
			}

			cmd.Flags().VisitAll(func(flag *pflag.Flag) {
				klog.Infof("FLAG: --%s=%q", flag.Name, flag.Value)
			})

			if err := o.Validate(); err != nil {
				klog.Fatalf("validate options: %v", err)
			}

			runner, err := config.NewControlPlaneRunner(o)
			if err != nil {
				return err
			}
			if err := runner.Do(); err != nil {
				return fmt.Errorf("could not config control-plane, %v", err)
			}

			klog.Info("node-servant config control-plane success")
			return nil
		},
		Args: cobra.NoArgs,
	}
	o.AddFlags(cmd.Flags())
	return cmd
}
