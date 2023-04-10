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

	"github.com/openyurtio/openyurt/pkg/controller/gateway/config"
)

type GatewayControllerOptions struct {
	*config.GatewayControllerConfiguration
}

func NewGatewayControllerOptions() *GatewayControllerOptions {
	return &GatewayControllerOptions{
		&config.GatewayControllerConfiguration{},
	}
}

// AddFlags adds flags related to nodepool for yurt-manager to the specified FlagSet.
func (g *GatewayControllerOptions) AddFlags(fs *pflag.FlagSet) {
	if g == nil {
		return
	}

}

// ApplyTo fills up nodepool config with options.
func (g *GatewayControllerOptions) ApplyTo(cfg *config.GatewayControllerConfiguration) error {
	if g == nil {
		return nil
	}

	return nil
}

// Validate checks validation of GatewayControllerOptions.
func (g *GatewayControllerOptions) Validate() []error {
	if g == nil {
		return nil
	}
	var errs []error
	return errs
}
