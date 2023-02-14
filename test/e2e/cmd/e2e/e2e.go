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

package e2e

import (
	"io"
	"os"
	"testing"

	"github.com/spf13/cobra"
)

func NewE2ECmd(m *testing.M, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "e2e",
		Short: "Tools for developers to test the OpenYurt Cluster",
		RunE: func(cmd *cobra.Command, args []string) error {
			os.Exit(m.Run())
			return nil
		},
		Args: cobra.NoArgs,
	}
	cmd.SetOut(out)

	return cmd
}
