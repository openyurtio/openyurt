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
	"strings"
	"time"

	"github.com/spf13/pflag"

	"github.com/openyurtio/openyurt/pkg/node-servant/components"
	"github.com/openyurtio/openyurt/pkg/yurtadm/constants"
	hubutil "github.com/openyurtio/openyurt/pkg/yurthub/util"
)

const (
	// defaultYurthubHealthCheckTimeout defines the default timeout for yurthub health check phase
	defaultYurthubHealthCheckTimeout = 2 * time.Minute
)

// Options has the information that required by convert operation
type Options struct {
	yurthubImage              string
	yurthubHealthCheckTimeout time.Duration
	workingMode               string
	joinToken                 string
	kubeadmConfPaths          string
	openyurtDir               string
	enableDummyIf             bool
	enableNodePool            bool
	Version                   bool
}

// NewConvertOptions creates a new Options
func NewConvertOptions() *Options {
	return &Options{
		yurthubImage:              "openyurt/yurthub:latest",
		yurthubHealthCheckTimeout: defaultYurthubHealthCheckTimeout,
		workingMode:               string(hubutil.WorkingModeEdge),
		kubeadmConfPaths:          strings.Join(components.GetDefaultKubeadmConfPath(), ","),
		openyurtDir:               constants.OpenyurtDir,
		enableDummyIf:             true,
		enableNodePool:            true,
	}
}

// Validate validates Options
func (o *Options) Validate() error {
	if len(o.joinToken) == 0 {
		return fmt.Errorf("join token(bootstrap token) is empty")
	}

	if !hubutil.IsSupportedWorkingMode(hubutil.WorkingMode(o.workingMode)) {
		return fmt.Errorf("workingMode must be pointed out as cloud or edge. got %s", o.workingMode)
	}

	return nil
}

// AddFlags sets flags.
func (o *Options) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&o.yurthubImage, "yurthub-image", o.yurthubImage, "The yurthub image.")
	fs.DurationVar(&o.yurthubHealthCheckTimeout, "yurthub-healthcheck-timeout", o.yurthubHealthCheckTimeout, "The timeout for yurthub health check.")
	fs.StringVarP(&o.kubeadmConfPaths, "kubeadm-conf-path", "k", o.kubeadmConfPaths, "The path to kubelet service conf that is used by kubelet component to join the cluster on the work node. Support multiple values, will search in order until get the file.(e.g -k kbcfg1,kbcfg2)")
	fs.StringVar(&o.joinToken, "join-token", o.joinToken, "The token used by yurthub for joining the cluster.")
	fs.StringVar(&o.workingMode, "working-mode", o.workingMode, "The node type cloud/edge, effect yurthub workingMode.")
	fs.BoolVar(&o.enableDummyIf, "enable-dummy-if", o.enableDummyIf, "Enable dummy interface for yurthub or not.")
	fs.BoolVar(&o.enableNodePool, "enable-node-pool", o.enableNodePool, "Enable list/watch nodepools for yurthub or not.")
	fs.BoolVar(&o.Version, "version", o.Version, "print the version information.")
}
