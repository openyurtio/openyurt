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

package revert

import (
	"github.com/spf13/cobra"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/node-servant/revert"
)

// NewRevertCmd generates a new revert command
func NewRevertCmd() *cobra.Command {
	o := revert.NewRevertOptions()
	cmd := &cobra.Command{
		Use:   "revert",
		Short: "",
		Run: func(cmd *cobra.Command, args []string) {
			if err := o.Complete(cmd.Flags()); err != nil {
				klog.Fatalf("could not complete the revert option: %s", err)
			}

			r := revert.NewReverterWithOptions(o)
			if err := r.Do(); err != nil {
				klog.Fatalf("could not revert the yurt node to a kubernetes node: %s", err)
			}
			klog.Info("revert success")
		},
		Args: cobra.NoArgs,
	}
	setFlags(cmd)

	return cmd
}

// setFlags sets flags.
func setFlags(cmd *cobra.Command) {
	cmd.Flags().String("kubeadm-conf-path", "",
		"The path to kubelet service conf that is used by kubelet component to join the cluster on the edge node.")
}
