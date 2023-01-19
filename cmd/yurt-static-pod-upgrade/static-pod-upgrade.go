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

package main

import (
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
	upgrade "github.com/openyurtio/openyurt/pkg/static-pod-upgrade"
)

func main() {
	rand.Seed(time.Now().UnixNano())
	version := fmt.Sprintf("%#v", projectinfo.Get())
	cmd := &cobra.Command{
		Use: "yurt-static-pod-upgrade",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("yurt-static-pod-upgrade version: %#v\n", version)

			cmd.Flags().VisitAll(func(flag *pflag.Flag) {
				klog.Infof("FLAG: --%s=%q", flag.Name, flag.Value)
			})

			if err := upgrade.Validate(); err != nil {
				klog.Fatalf("Fail to validate yurt static pod upgrade args, %v", err)
			}

			c, err := upgrade.GetClient()
			if err != nil {
				klog.Fatalf("Fail to get kubernetes client, %v", err)
			}

			ctrl, err := upgrade.New(c)
			if err != nil {
				klog.Fatal("Fail to create static-pod-upgrade controller, %v", err)
			}

			if err := ctrl.Upgrade(); err != nil {
				klog.Fatalf("Fail to upgrade static pod, %v", err)
			}
			klog.Info("Static pod upgrade Success")
		},
		Version: version,
	}

	addFlags(cmd)

	if err := viper.BindPFlags(cmd.Flags()); err != nil {
		os.Exit(1)
	}

	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func addFlags(cmd *cobra.Command) {
	cmd.Flags().String("kubeconfig", "", "The path to the kubeconfig file")
	cmd.Flags().String("name", "", "The name of static pod which needs be upgraded")
	cmd.Flags().String("namespace", "", "The namespace of static pod which needs be upgraded")
	cmd.Flags().String("manifest", "", "The manifest file name of static pod which needs be upgraded")
	cmd.Flags().String("hash", "", "The hash value of new static pod specification")
	cmd.Flags().String("mode", "", "The upgrade mode which is used")
}
