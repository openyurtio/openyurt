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

	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/raven/gatewayinternalservice/config"
)

type GatewayInternalSvcControllerOptions struct {
	*config.GatewayInternalSvcControllerConfiguration
}

func NewGatewayInternalSvcControllerOptions() *GatewayInternalSvcControllerOptions {
	return &GatewayInternalSvcControllerOptions{
		&config.GatewayInternalSvcControllerConfiguration{
			ConcurrentGatewayInternalSvcWorkers: 1,
		},
	}
}

// AddFlags adds flags related to gateway for yurt-manager to the specified FlagSet.
func (g *GatewayInternalSvcControllerOptions) AddFlags(fs *pflag.FlagSet) {
	if g == nil {
		return
	}

	fs.Int32Var(&g.ConcurrentGatewayInternalSvcWorkers, "concurrent-gateway-internal-svc-workers", g.ConcurrentGatewayInternalSvcWorkers, "The number of gateway objects that are allowed to reconcile concurrently. Larger number = more responsive gateway internal svc, but more CPU (and network) load")

}

// ApplyTo fills up gateway config with options.
func (g *GatewayInternalSvcControllerOptions) ApplyTo(cfg *config.GatewayInternalSvcControllerConfiguration) error {
	if g == nil {
		return nil
	}

	cfg.ConcurrentGatewayInternalSvcWorkers = g.ConcurrentGatewayInternalSvcWorkers
	return nil
}

// Validate checks validation of GatewayControllerOptions.
func (g *GatewayInternalSvcControllerOptions) Validate() []error {
	if g == nil {
		return nil
	}
	var errs []error
	return errs
}
