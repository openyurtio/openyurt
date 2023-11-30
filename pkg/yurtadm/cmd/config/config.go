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

package config

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/lithammer/dedent"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/util/templates"
	"github.com/openyurtio/openyurt/pkg/yurtadm/constants"
	"github.com/openyurtio/openyurt/pkg/yurtadm/util"
	"github.com/openyurtio/openyurt/pkg/yurtadm/util/edgenode"
)

// NewCmdConfig returns cobra.Command for "yurtadm config" command
func NewCmdConfig(in io.Reader, out io.Writer, outErr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage configuration for a openyurt cluster",
		Run:   util.SubCmdRun(),
	}

	cmd.AddCommand(newCmdConfigPrint(out))
	return cmd
}

// newCmdConfigPrint returns cobra.Command for "yurtadm config print" command
func newCmdConfigPrint(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "print",
		Short: "Print configuration",
		Run:   util.SubCmdRun(),
	}
	cmd.AddCommand(newCmdConfigPrintJoinDefaults(out))
	return cmd
}

// newCmdConfigPrintJoinDefaults returns cobra.Command for "yurtadm config print join-defaults" command
func newCmdConfigPrintJoinDefaults(out io.Writer) *cobra.Command {
	return newCmdConfigPrintActionDefaults(out, "join", getDefaultNodeConfigBytes)
}

func newCmdConfigPrintActionDefaults(out io.Writer, action string, configBytesProc func() (string, error)) *cobra.Command {
	cmd := &cobra.Command{
		Use:   fmt.Sprintf("%s-defaults", action),
		Short: fmt.Sprintf("Print default %s configuration, that can be used for 'yurtadm %s'", action, action),
		Long: fmt.Sprintf(dedent.Dedent(`
			This command prints objects such as the default %s configuration that is used for 'yurtadm %s'.

			Note that sensitive values like the Bootstrap Token fields are replaced with placeholder values like %q in order to pass validation but
			not perform the real computation for creating a token.
		`), action, action, constants.PlaceholderToken),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigPrintActionDefaults(out, configBytesProc)
		},
		Args: cobra.NoArgs,
	}
	return cmd
}

func runConfigPrintActionDefaults(out io.Writer, configBytesProc func() (string, error)) error {
	initialConfig, err := configBytesProc()
	if err != nil {
		return err
	}

	fmt.Fprint(out, initialConfig)
	return nil
}

func getDefaultNodeConfigBytes() (string, error) {
	KubeadmJoinDiscoveryFilePath := filepath.Join(constants.KubeletWorkdir, constants.KubeadmJoinDiscoveryFileName)
	ignoreErrors := sets.NewString(constants.KubeletConfFileAvailableError, constants.ManifestsDirAvailableError)
	name, err := edgenode.GetHostname("")
	if err != nil {
		return "", err
	}
	ctx := map[string]interface{}{
		"kubeConfigPath":         KubeadmJoinDiscoveryFilePath,
		"tlsBootstrapToken":      constants.PlaceholderToken,
		"ignorePreflightErrors":  ignoreErrors.List(),
		"podInfraContainerImage": constants.PauseImagePath,
		"nodeLabels":             fmt.Sprintf("%s=true", projectinfo.GetEdgeWorkerLabelKey()),
		"criSocket":              constants.DefaultDockerCRISocket,
		"name":                   name,
		"networkPlugin":          "cni",
		"apiVersion":             "kubeadm.k8s.io/v1beta3",
	}

	kubeadmJoinTemplate, err := templates.SubsituteTemplate(constants.KubeadmJoinConf, ctx)
	if err != nil {
		return "", err
	}

	return kubeadmJoinTemplate, nil
}
