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

	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/raven/dns/config"
)

type GatewayDNSControllerOptions struct {
	*config.GatewayDNSControllerConfiguration
}

func NewGatewayDNSControllerOptions() *GatewayDNSControllerOptions {
	return &GatewayDNSControllerOptions{
		&config.GatewayDNSControllerConfiguration{
			ConcurrentGatewayDNSWorkers: 1,
		},
	}
}

// AddFlags adds flags related to gateway for yurt-manager to the specified FlagSet.
func (g *GatewayDNSControllerOptions) AddFlags(fs *pflag.FlagSet) {
	if g == nil {
		return
	}

	fs.Int32Var(&g.ConcurrentGatewayDNSWorkers, "concurrent-gateway-dns-workers", g.ConcurrentGatewayDNSWorkers, "The number of gateway objects that are allowed to reconcile concurrently. Larger number = more responsive gateway dns, but more CPU (and network) load")

}

// ApplyTo fills up gateway config with options.
func (g *GatewayDNSControllerOptions) ApplyTo(cfg *config.GatewayDNSControllerConfiguration) error {
	if g == nil {
		return nil
	}

	cfg.ConcurrentGatewayDNSWorkers = g.ConcurrentGatewayDNSWorkers
	return nil
}

// Validate checks validation of GatewayControllerOptions.
func (g *GatewayDNSControllerOptions) Validate() []error {
	if g == nil {
		return nil
	}
	var errs []error
	return errs
}
