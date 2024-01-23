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

	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/yurtcoordinator/podbinding/config"
)

type PodBindingControllerOptions struct {
	*config.PodBindingControllerConfiguration
}

func NewPodBindingControllerOptions() *PodBindingControllerOptions {
	return &PodBindingControllerOptions{
		&config.PodBindingControllerConfiguration{
			ConcurrentPodBindingWorkers: 5,
		},
	}
}

// AddFlags adds flags related to nodePool for yurt-manager to the specified FlagSet.
func (n *PodBindingControllerOptions) AddFlags(fs *pflag.FlagSet) {
	if n == nil {
		return
	}

	fs.Int32Var(&n.ConcurrentPodBindingWorkers, "concurrent-podbinding-workers", n.ConcurrentPodBindingWorkers, "The number of podbinding objects that are allowed to reconcile concurrently. Larger number = more responsive podbindings, but more CPU (and network) load")
}

// ApplyTo fills up nodePool config with options.
func (o *PodBindingControllerOptions) ApplyTo(cfg *config.PodBindingControllerConfiguration) error {
	if o == nil {
		return nil
	}

	cfg.ConcurrentPodBindingWorkers = o.ConcurrentPodBindingWorkers
	return nil
}

// Validate checks validation of PodBindingControllerOptions.
func (o *PodBindingControllerOptions) Validate() []error {
	if o == nil {
		return nil
	}
	errs := []error{}
	return errs
}
