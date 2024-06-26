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

	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/servicetopology/endpointslice/config"
)

type EndpointSliceControllerOptions struct {
	*config.ServiceTopologyEndpointSliceControllerConfiguration
}

func NewEndpointSliceControllerOptions() *EndpointSliceControllerOptions {
	return &EndpointSliceControllerOptions{
		&config.ServiceTopologyEndpointSliceControllerConfiguration{
			ConcurrentEndpointSliceWorkers: 3,
		},
	}
}

// AddFlags adds flags related to servicetopology endpointslice for yurt-manager to the specified FlagSet.
func (n *EndpointSliceControllerOptions) AddFlags(fs *pflag.FlagSet) {
	if n == nil {
		return
	}

	fs.Int32Var(&n.ConcurrentEndpointSliceWorkers, "concurrent-endpointslice-workers", n.ConcurrentEndpointSliceWorkers, "Max concurrent workers for servicetopology-endpointslice controller.")
}

// ApplyTo fills up servicetopolgy endpointslice config with options.
func (o *EndpointSliceControllerOptions) ApplyTo(cfg *config.ServiceTopologyEndpointSliceControllerConfiguration) error {
	if o == nil {
		return nil
	}

	cfg.ConcurrentEndpointSliceWorkers = o.ConcurrentEndpointSliceWorkers
	return nil
}

// Validate checks validation of EndpointSliceControllerOptions.
func (o *EndpointSliceControllerOptions) Validate() []error {
	if o == nil {
		return nil
	}
	errs := []error{}
	return errs
}
