/*
Copyright 2016 The Kubernetes Authors.
Copyright 2020 The OpenYurt Authors.

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

package reset

import (
	"io"

	"github.com/spf13/cobra"
	flag "github.com/spf13/pflag"

	yurtphases "github.com/openyurtio/openyurt/pkg/yurtadm/cmd/reset/phases"
	"github.com/openyurtio/openyurt/pkg/yurtadm/constants"
)

// resetOptions defines all the options exposed via flags by kubeadm reset.
type resetOptions struct {
	certificatesDir       string
	criSocketPath         string
	forceReset            bool
	ignorePreflightErrors []string
}

// resetData defines all the runtime information used when running the kubeadm reset workflow;
// this data is shared across all the phases that are included in the workflow.
type resetData struct {
	certificatesDir       string
	criSocketPath         string
	forceReset            bool
	ignorePreflightErrors []string
}

// newResetOptions returns a struct ready for being used for creating cmd join flags.
func newResetOptions() *resetOptions {
	return &resetOptions{
		certificatesDir: constants.DefaultCertificatesDir,
		forceReset:      false,
	}
}

// newResetData returns a new resetData struct to be used for the execution of the kubeadm reset workflow.
func newResetData(options *resetOptions) (*resetData, error) {
	return &resetData{
		certificatesDir:       options.certificatesDir,
		criSocketPath:         options.criSocketPath,
		forceReset:            options.forceReset,
		ignorePreflightErrors: options.ignorePreflightErrors,
	}, nil
}

// AddResetFlags adds reset flags
func AddResetFlags(flagSet *flag.FlagSet, resetOptions *resetOptions) {
	flagSet.StringVar(
		&resetOptions.certificatesDir, constants.CertificatesDir, resetOptions.certificatesDir,
		"The path to the directory where the certificates are stored. If specified, clean this directory. (default \"/etc/kubernetes/pki\")",
	)
	flagSet.BoolVarP(
		&resetOptions.forceReset, constants.ForceReset, "f", false,
		"Reset the node without prompting for confirmation.",
	)
	flagSet.StringVar(
		&resetOptions.criSocketPath, constants.NodeCRISocket, resetOptions.criSocketPath,
		"Path to the CRI socket to connect",
	)
	flagSet.StringSliceVar(
		&resetOptions.ignorePreflightErrors, constants.IgnorePreflightErrors, resetOptions.ignorePreflightErrors,
		"A list of checks whose errors will be shown as warnings. Example: 'IsPrivilegedUser,Swap'. Value 'all' ignores errors from all checks.",
	)
}

type nodeReseter struct {
	*resetData
	inReader     io.Reader
	outWriter    io.Writer
	outErrWriter io.Writer
}

func newReseterWithResetData(o *resetData, in io.Reader, out io.Writer, outErr io.Writer) *nodeReseter {
	return &nodeReseter{
		o,
		in,
		out,
		outErr,
	}
}

// Run use kubeadm to reset the node.
func (nodeReseter *nodeReseter) Run() error {
	resetData := nodeReseter.resetData

	if err := yurtphases.RunResetNode(resetData, nodeReseter.inReader, nodeReseter.outWriter, nodeReseter.outErrWriter); err != nil {
		return err
	}

	if err := yurtphases.RunCleanYurtFile(); err != nil {
		return err
	}

	return nil
}

// NewCmdReset returns the "yurtadm reset" command
func NewCmdReset(in io.Reader, out io.Writer, outErr io.Writer) *cobra.Command {
	resetOptions := newResetOptions()

	cmd := &cobra.Command{
		Use:   "reset",
		Short: "Performs a best effort revert of changes made to this host by 'yurtadm join'",
		RunE: func(cmd *cobra.Command, args []string) error {
			o, err := newResetData(resetOptions)
			if err != nil {
				return err
			}

			reseter := newReseterWithResetData(o, in, out, outErr)
			if err := reseter.Run(); err != nil {
				return err
			}

			return nil
		},
	}

	AddResetFlags(cmd.Flags(), resetOptions)

	return cmd
}

// CertificatesDir returns the certificatesDir flag.
func (r *resetData) CertificatesDir() string {
	return r.certificatesDir
}

// ForceReset returns the forceReset flag.
func (r *resetData) ForceReset() bool {
	return r.forceReset
}

// IgnorePreflightErrors returns the list of preflight errors to ignore.
func (r *resetData) IgnorePreflightErrors() []string {
	return r.ignorePreflightErrors
}

// CRISocketPath returns the criSocketPath.
func (r *resetData) CRISocketPath() string {
	return r.criSocketPath
}
