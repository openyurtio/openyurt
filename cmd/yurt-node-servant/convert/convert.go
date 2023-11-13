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
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/klog/v2"

	nodeconverter "github.com/openyurtio/openyurt/pkg/node-servant/convert"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
)

// NewConvertCmd generates a new convert command
func NewConvertCmd() *cobra.Command {
	o := nodeconverter.NewConvertOptions()
	cmd := &cobra.Command{
		Use:   "convert --working-mode",
		Short: "",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("node-servant version: %#v\n", projectinfo.Get())
			if o.Version {
				return
			}

			cmd.Flags().VisitAll(func(flag *pflag.Flag) {
				klog.Infof("FLAG: --%s=%q", flag.Name, flag.Value)
			})

			if err := o.Validate(); err != nil {
				klog.Fatalf("validate options: %v", err)
			}

			converter := nodeconverter.NewConverterWithOptions(o)
			if err := converter.Do(); err != nil {
				klog.Fatalf("could not convert the kubernetes node to a yurt node: %s", err)
			}
			klog.Info("convert success")
		},
		Args: cobra.NoArgs,
	}
	o.AddFlags(cmd.Flags())

	return cmd
}
