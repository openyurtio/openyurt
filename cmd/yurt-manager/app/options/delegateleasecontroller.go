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

	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/yurtcoordinator/delegatelease/config"
)

type DelegateLeaseControllerOptions struct {
	*config.DelegateLeaseControllerConfiguration
}

func NewDelegateLeaseControllerOptions() *DelegateLeaseControllerOptions {
	return &DelegateLeaseControllerOptions{
		&config.DelegateLeaseControllerConfiguration{
			ConcurrentDelegateLeaseWorkers: 5,
		},
	}
}

// AddFlags adds flags related to nodePool for yurt-manager to the specified FlagSet.
func (n *DelegateLeaseControllerOptions) AddFlags(fs *pflag.FlagSet) {
	if n == nil {
		return
	}

	fs.Int32Var(&n.ConcurrentDelegateLeaseWorkers, "concurrent-delegatelease-workers", n.ConcurrentDelegateLeaseWorkers, "The number of delegatelease objects that are allowed to reconcile concurrently. Larger number = more responsive delegateleases, but more CPU (and network) load")
}

// ApplyTo fills up nodePool config with options.
func (o *DelegateLeaseControllerOptions) ApplyTo(cfg *config.DelegateLeaseControllerConfiguration) error {
	if o == nil {
		return nil
	}

	cfg.ConcurrentDelegateLeaseWorkers = o.ConcurrentDelegateLeaseWorkers
	return nil
}

// Validate checks validation of DelegateLeaseControllerOptions.
func (o *DelegateLeaseControllerOptions) Validate() []error {
	if o == nil {
		return nil
	}
	errs := []error{}
	return errs
}
