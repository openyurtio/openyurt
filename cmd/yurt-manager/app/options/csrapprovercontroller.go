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

	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/csrapprover/config"
)

type CsrApproverControllerOptions struct {
	*config.CsrApproverControllerConfiguration
}

func NewCsrApproverControllerOptions() *CsrApproverControllerOptions {
	return &CsrApproverControllerOptions{
		&config.CsrApproverControllerConfiguration{
			ConcurrentCsrApproverWorkers: 3,
		},
	}
}

// AddFlags adds flags related to nodePool for yurt-manager to the specified FlagSet.
func (n *CsrApproverControllerOptions) AddFlags(fs *pflag.FlagSet) {
	if n == nil {
		return
	}

	fs.Int32Var(&n.ConcurrentCsrApproverWorkers, "concurrent-csr-approver-workers", n.ConcurrentCsrApproverWorkers, "The number of csr objects that are allowed to reconcile concurrently. Larger number = more responsive csrs, but more CPU (and network) load")
}

// ApplyTo fills up nodePool config with options.
func (o *CsrApproverControllerOptions) ApplyTo(cfg *config.CsrApproverControllerConfiguration) error {
	if o == nil {
		return nil
	}

	cfg.ConcurrentCsrApproverWorkers = o.ConcurrentCsrApproverWorkers
	return nil
}

// Validate checks validation of CsrApproverControllerOptions.
func (o *CsrApproverControllerOptions) Validate() []error {
	if o == nil {
		return nil
	}
	errs := []error{}
	return errs
}
