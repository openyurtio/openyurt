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
	"math/rand"
	_ "net/http/pprof"
	"os"
	"time"

	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/component-base/logs"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/cmd/yurt-manager/app"
)

func main() {
	newRand := rand.New(rand.NewSource(time.Now().UnixNano()))
	newRand.Seed(time.Now().UnixNano())

	command := app.NewYurtManagerCommand()

	// TODO: once we switch everything over to Cobra commands, we can go back to calling
	// utilflag.InitFlags() (by removing its pflag.Parse() call). For now, we have to set the
	// normalize func and add the go flag set by hand.
	// utilflag.InitFlags()
	logs.InitLogs()
	defer logs.FlushLogs()
	defer klog.Flush()

	if err := command.Execute(); err != nil {
		os.Exit(1)
	}
}
