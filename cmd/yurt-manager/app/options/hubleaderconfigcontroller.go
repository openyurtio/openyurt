/*
Copyright 2025 The OpenYurt Authors.

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

	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/hubleaderconfig/config"
)

type HubLeaderConfigControllerOptions struct {
	*config.HubLeaderConfigControllerConfiguration
}

func NewHubLeaderConfigControllerOptions(hubleaderNamespace string) *HubLeaderConfigControllerOptions {
	return &HubLeaderConfigControllerOptions{
		&config.HubLeaderConfigControllerConfiguration{
			ConcurrentHubLeaderConfigWorkers: 3,
			HubLeaderNamespace:               hubleaderNamespace,
		},
	}
}

// AddFlags adds flags related to hubleader for yurt-manager to the specified FlagSet.
func (h *HubLeaderConfigControllerOptions) AddFlags(fs *pflag.FlagSet) {
	if h == nil {
		return
	}

	fs.Int32Var(
		&h.ConcurrentHubLeaderConfigWorkers,
		"concurrent-hubleaderconfig-workers",
		h.ConcurrentHubLeaderConfigWorkers,
		"The number of nodepool objects that are allowed to reconcile concurrently.",
	)
}

// ApplyTo fills up hubleader config with options.
func (h *HubLeaderConfigControllerOptions) ApplyTo(cfg *config.HubLeaderConfigControllerConfiguration) error {
	if h == nil {
		return nil
	}

	cfg.ConcurrentHubLeaderConfigWorkers = h.ConcurrentHubLeaderConfigWorkers
	cfg.HubLeaderNamespace = h.HubLeaderNamespace

	return nil
}

// Validate checks validation of HubLeaderControllerOptions.
func (h *HubLeaderConfigControllerOptions) Validate() []error {
	if h == nil {
		return nil
	}
	errs := []error{}
	return errs
}
