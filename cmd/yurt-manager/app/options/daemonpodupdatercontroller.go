/*
Copyright 2024 The OpenYurt Authors.

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

	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/daemonpodupdater/config"
)

type DaemonPodUpdaterControllerOptions struct {
	*config.DaemonPodUpdaterControllerConfiguration
}

func NewDaemonPodUpdaterControllerOptions() *DaemonPodUpdaterControllerOptions {
	return &DaemonPodUpdaterControllerOptions{
		&config.DaemonPodUpdaterControllerConfiguration{
			ConcurrentDaemonPodUpdaterWorkers: 3,
		},
	}
}

// AddFlags adds flags related to nodePool for yurt-manager to the specified FlagSet.
func (n *DaemonPodUpdaterControllerOptions) AddFlags(fs *pflag.FlagSet) {
	if n == nil {
		return
	}

	fs.Int32Var(&n.ConcurrentDaemonPodUpdaterWorkers, "concurrent-daemonpod-updater-workers", n.ConcurrentDaemonPodUpdaterWorkers, "The number of daemonpodupdater objects that are allowed to reconcile concurrently. Larger number = more responsive daemonpodupdaters, but more CPU (and network) load")
}

// ApplyTo fills up nodePool config with options.
func (o *DaemonPodUpdaterControllerOptions) ApplyTo(cfg *config.DaemonPodUpdaterControllerConfiguration) error {
	if o == nil {
		return nil
	}

	cfg.ConcurrentDaemonPodUpdaterWorkers = o.ConcurrentDaemonPodUpdaterWorkers
	return nil
}

// Validate checks validation of DaemonPodUpdaterControllerOptions.
func (o *DaemonPodUpdaterControllerOptions) Validate() []error {
	if o == nil {
		return nil
	}
	errs := []error{}
	return errs
}
