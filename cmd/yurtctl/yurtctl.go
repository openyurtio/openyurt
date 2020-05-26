package main

import (
	"os"

	"github.com/alibaba/openyurt/pkg/yurtctl/cmd"
)

func main() {
	cmd := cmd.NewYurtctlCommand()
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
