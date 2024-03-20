/*
Copyright 2023 The OpenYurt Authors.

Licensed under the Apache License, Version 2.0 (the License);
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an AS IS BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package options

import (
	"github.com/spf13/pflag"

	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/yurtappdaemon/config"
)

type YurtAppDaemonControllerOptions struct {
	*config.YurtAppDaemonControllerConfiguration
}

func NewYurtAppDaemonControllerOptions() *YurtAppDaemonControllerOptions {
	return &YurtAppDaemonControllerOptions{
		&config.YurtAppDaemonControllerConfiguration{
			ConcurrentYurtAppDaemonWorkers: 3,
		},
	}
}

// AddFlags adds flags related to YurtAppDaemon for yurt-manager to the specified FlagSet.
func (o *YurtAppDaemonControllerOptions) AddFlags(fs *pflag.FlagSet) {
	if o == nil {
		return
	}

	//fs.BoolVar(&n.CreateDefaultPool, "create-default-pool", n.CreateDefaultPool, "Create default cloud/edge pools if indicated.")
	fs.Int32Var(&o.ConcurrentYurtAppDaemonWorkers, "concurrent-yurtappdaemon-workers", o.ConcurrentYurtAppDaemonWorkers, "The number of yurtappdaemon objects that are allowed to reconcile concurrently. Larger number = more responsive yurtappdaemons, but more CPU (and network) load")
}

// ApplyTo fills up YurtAppDaemon config with options.
func (o *YurtAppDaemonControllerOptions) ApplyTo(cfg *config.YurtAppDaemonControllerConfiguration) error {
	if o == nil {
		return nil
	}
	cfg.ConcurrentYurtAppDaemonWorkers = o.ConcurrentYurtAppDaemonWorkers
	return nil
}

// Validate checks validation of YurtAppDaemonControllerOptions.
func (o *YurtAppDaemonControllerOptions) Validate() []error {
	if o == nil {
		return nil
	}
	var errs []error
	return errs
}
