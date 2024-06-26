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

	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/raven/gatewaypublicservice/config"
)

type GatewayPublicSvcControllerOptions struct {
	*config.GatewayPublicSvcControllerConfiguration
}

func NewGatewayPublicSvcControllerOptions() *GatewayPublicSvcControllerOptions {
	return &GatewayPublicSvcControllerOptions{
		&config.GatewayPublicSvcControllerConfiguration{
			ConcurrentGatewayPublicSvcWorkers: 1,
		},
	}
}

// AddFlags adds flags related to gateway for yurt-manager to the specified FlagSet.
func (g *GatewayPublicSvcControllerOptions) AddFlags(fs *pflag.FlagSet) {
	if g == nil {
		return
	}

	fs.Int32Var(&g.ConcurrentGatewayPublicSvcWorkers, "concurrent-gateway-public-svc-workers", g.ConcurrentGatewayPublicSvcWorkers, "The number of gateway objects that are allowed to reconcile concurrently. Larger number = more responsive gateway public svc, but more CPU (and network) load")

}

// ApplyTo fills up gateway config with options.
func (g *GatewayPublicSvcControllerOptions) ApplyTo(cfg *config.GatewayPublicSvcControllerConfiguration) error {
	if g == nil {
		return nil
	}

	cfg.ConcurrentGatewayPublicSvcWorkers = g.ConcurrentGatewayPublicSvcWorkers
	return nil
}

// Validate checks validation of GatewayControllerOptions.
func (g *GatewayPublicSvcControllerOptions) Validate() []error {
	if g == nil {
		return nil
	}
	var errs []error
	return errs
}
