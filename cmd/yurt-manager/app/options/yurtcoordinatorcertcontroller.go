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

	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/yurtcoordinator/cert/config"
)

type YurtCoordinatorCertControllerOptions struct {
	*config.YurtCoordinatorCertControllerConfiguration
}

func NewYurtCoordinatorCertControllerOptions() *YurtCoordinatorCertControllerOptions {
	return &YurtCoordinatorCertControllerOptions{
		&config.YurtCoordinatorCertControllerConfiguration{
			ConcurrentCoordinatorCertWorkers: 1,
		},
	}
}

// AddFlags adds flags related to nodePool for yurt-manager to the specified FlagSet.
func (n *YurtCoordinatorCertControllerOptions) AddFlags(fs *pflag.FlagSet) {
	if n == nil {
		return
	}

	fs.Int32Var(&n.ConcurrentCoordinatorCertWorkers, "concurrent-coordinator-cert-workers", n.ConcurrentCoordinatorCertWorkers, "The number of service objects that are allowed to reconcile concurrently. Larger number = more responsive service, but more CPU (and network) load")
}

// ApplyTo fills up nodePool config with options.
func (o *YurtCoordinatorCertControllerOptions) ApplyTo(cfg *config.YurtCoordinatorCertControllerConfiguration) error {
	if o == nil {
		return nil
	}

	cfg.ConcurrentCoordinatorCertWorkers = o.ConcurrentCoordinatorCertWorkers
	return nil
}

// Validate checks validation of YurtAppSetControllerOptions.
func (o *YurtCoordinatorCertControllerOptions) Validate() []error {
	if o == nil {
		return nil
	}
	errs := []error{}
	return errs
}
