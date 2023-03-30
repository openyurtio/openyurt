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

package renew

import (
	"fmt"
	"io"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/openyurtio/openyurt/pkg/yurtadm/cmd/renew/certificate"
	util "github.com/openyurtio/openyurt/pkg/yurtadm/util/error"
)

// NewCmdRenew returns "yurtadm renew" command.
func NewCmdRenew(in io.Reader, out io.Writer, outErr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "renew",
		Short: "Renew something like certificate",
		// Without this callback, if a user runs just the "renew"
		// command without a subcommand, or with an invalid subcommand,
		// cobra will print usage information, but still exit cleanly.
		// We want to return an error code in these cases so that the
		// user knows that their command was invalid.
		Run: subCmdRun(),
	}

	cmd.AddCommand(certificate.NewCmdCertificate())
	return cmd
}

// subCmdRun returns a function that handles a case where a subcommand must be specified
// Without this callback, if a user runs just the command without a subcommand,
// or with an invalid subcommand, cobra will print usage information, but still exit cleanly.
func subCmdRun() func(c *cobra.Command, args []string) {
	return func(c *cobra.Command, args []string) {
		if len(args) > 0 {
			util.CheckErr(usageErrorf(c, "invalid subcommand %q", strings.Join(args, " ")))
		}
		err := c.Help()
		if err != nil {
			return
		}
		util.CheckErr(util.ErrExit)
	}
}

func usageErrorf(c *cobra.Command, format string, args ...interface{}) error {
	msg := fmt.Sprintf(format, args...)
	return errors.Errorf("%s\nSee '%s -h' for help and examples", msg, c.CommandPath())
}
