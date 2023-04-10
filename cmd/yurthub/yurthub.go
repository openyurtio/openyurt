/*
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

package main

import (
	"flag"
	"math/rand"
	"time"

	"k8s.io/apiserver/pkg/server"

	"github.com/openyurtio/openyurt/cmd/yurthub/app"
)

func main() {
	newRand := rand.New(rand.NewSource(time.Now().UnixNano()))
	newRand.Seed(time.Now().UnixNano())

	cmd := app.NewCmdStartYurtHub(server.SetupSignalContext())
	cmd.Flags().AddGoFlagSet(flag.CommandLine)
	if err := cmd.Execute(); err != nil {
		panic(err)
	}
}
