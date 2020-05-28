package cmd

import (
	"github.com/spf13/cobra"

	"github.com/alibaba/openyurt/pkg/yurtctl/cmd/convert"
)

// NewYurtctlCommand creates a new yurtctl command
func NewYurtctlCommand() *cobra.Command {
	cmds := &cobra.Command{
		Use:   "yurtctl",
		Short: "yurtctl controls the yurt cluster",
		Run:   showHelp,
	}

	// add kubeconfig to persistent flags
	cmds.PersistentFlags().String("kubeconfig", "", "The path to the kubeconfig file")
	cmds.AddCommand(convert.NewConvertCmd())

	return cmds
}

// showHelp shows the help message
func showHelp(cmd *cobra.Command, _ []string) {
	cmd.Help()
}
