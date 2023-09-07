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

package main

import (
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/openyurtio/openyurt/cmd/yurt-node-servant/config"
	"github.com/openyurtio/openyurt/cmd/yurt-node-servant/convert"
	"github.com/openyurtio/openyurt/cmd/yurt-node-servant/revert"
	upgrade "github.com/openyurtio/openyurt/cmd/yurt-node-servant/static-pod-upgrade"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
)

// node-servant
// running on specific node, do convert/revert job
// node-servant convert/revert join/reset, yurtcluster operator shall start a k8s job to run this.
func main() {
	newRand := rand.New(rand.NewSource(time.Now().UnixNano()))
	newRand.Seed(time.Now().UnixNano())

	version := fmt.Sprintf("%#v", projectinfo.Get())
	rootCmd := &cobra.Command{
		Use:     "node-servant",
		Short:   "node-servant do convert/revert specific node",
		Version: version,
	}
	rootCmd.PersistentFlags().String("kubeconfig", "", "The path to the kubeconfig file")
	rootCmd.AddCommand(convert.NewConvertCmd())
	rootCmd.AddCommand(revert.NewRevertCmd())
	rootCmd.AddCommand(config.NewConfigCmd())
	rootCmd.AddCommand(upgrade.NewUpgradeCmd())

	if err := rootCmd.Execute(); err != nil { // run command
		os.Exit(1)
	}
}
