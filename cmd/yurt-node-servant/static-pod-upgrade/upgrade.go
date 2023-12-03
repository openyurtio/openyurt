/*
Copyright 2023 The OpenYurt Authors.

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

package upgrade

import (
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/klog/v2"

	upgrade "github.com/openyurtio/openyurt/pkg/node-servant/static-pod-upgrade"
)

// NewUpgradeCmd generates a new upgrade command
func NewUpgradeCmd() *cobra.Command {
	o := upgrade.NewUpgradeOptions()
	cmd := &cobra.Command{
		Use:   "static-pod-upgrade",
		Short: "",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Flags().VisitAll(func(flag *pflag.Flag) {
				klog.Infof("FLAG: --%s=%q", flag.Name, flag.Value)
			})

			if err := o.Validate(); err != nil {
				klog.Fatalf("could not validate static pod upgrade args, %v", err)
			}

			ctrl, err := upgrade.NewWithOptions(o)
			if err != nil {
				klog.Fatalf("could not create static-pod-upgrade controller, %v", err)
			}

			if err = ctrl.Upgrade(); err != nil {
				klog.Fatalf("could not upgrade static pod, %v", err)
			}

			klog.Info("Static pod upgrade Success")
		},
		Args: cobra.NoArgs,
	}
	o.AddFlags(cmd.Flags())

	return cmd
}
