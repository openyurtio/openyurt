package main

import (
	"flag"
	"math/rand"
	"time"

	"github.com/openyurtio/openyurt/cmd/yurthub/app"
	"k8s.io/apimachinery/pkg/util/wait"
)

func main() {
	rand.Seed(time.Now().UnixNano())
	cmd := app.NewCmdStartYurtHub(wait.NeverStop)
	cmd.Flags().AddGoFlagSet(flag.CommandLine)
	if err := cmd.Execute(); err != nil {
		panic(err)
	}
}
