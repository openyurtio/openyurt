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

package convert

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/pflag"

	"github.com/openyurtio/openyurt/pkg/node-servant/components"
	enutil "github.com/openyurtio/openyurt/pkg/yurtctl/util/edgenode"
	hubutil "github.com/openyurtio/openyurt/pkg/yurthub/util"
)

// Options has the information that required by convert operation
type Options struct {
	yurthubImage              string
	yurthubHealthCheckTimeout time.Duration
	workingMode               hubutil.WorkingMode

	joinToken        string
	kubeadmConfPaths []string
	openyurtDir      string
}

// NewConvertOptions creates a new Options
func NewConvertOptions() *Options {
	return &Options{
		kubeadmConfPaths: components.GetDefaultKubeadmConfPath(),
	}
}

// Complete completes all the required options.
func (o *Options) Complete(flags *pflag.FlagSet) error {
	yurthubImage, err := flags.GetString("yurthub-image")
	if err != nil {
		return err
	}
	o.yurthubImage = yurthubImage

	yurthubHealthCheckTimeout, err := flags.GetDuration("yurthub-healthcheck-timeout")
	if err != nil {
		return err
	}
	o.yurthubHealthCheckTimeout = yurthubHealthCheckTimeout

	kubeadmConfPaths, err := flags.GetString("kubeadm-conf-path")
	if err != nil {
		return err
	}
	if kubeadmConfPaths != "" {
		o.kubeadmConfPaths = strings.Split(kubeadmConfPaths, ",")
	}

	joinToken, err := flags.GetString("join-token")
	if err != nil {
		return err
	}
	if joinToken == "" {
		return fmt.Errorf("get joinToken empty")
	}
	o.joinToken = joinToken

	openyurtDir := os.Getenv("OPENYURT_DIR")
	if openyurtDir == "" {
		openyurtDir = enutil.OpenyurtDir
	}
	o.openyurtDir = openyurtDir

	workingMode, err := flags.GetString("working-mode")
	if err != nil {
		return err
	}

	wm := hubutil.WorkingMode(workingMode)
	if !hubutil.IsSupportedWorkingMode(wm) {
		return fmt.Errorf("invalid working mode: %s", workingMode)
	}
	o.workingMode = wm

	return nil
}
