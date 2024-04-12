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

	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/loadbalancerset/loadbalancerset/config"
)

type LoadBalancerSetControllerOptions struct {
	*config.LoadBalancerSetControllerConfiguration
}

func NewLoadBalancerSetControllerOptions() *LoadBalancerSetControllerOptions {
	return &LoadBalancerSetControllerOptions{
		&config.LoadBalancerSetControllerConfiguration{
			ConcurrentLoadBalancerSetWorkers: 3,
		},
	}
}

// AddFlags adds flags related to poolservice for yurt-manager to the specified FlagSet.
func (n *LoadBalancerSetControllerOptions) AddFlags(fs *pflag.FlagSet) {
	if n == nil {
		return
	}

	fs.Int32Var(&n.ConcurrentLoadBalancerSetWorkers, "concurrent-load-balancer-set-workers", n.ConcurrentLoadBalancerSetWorkers, "The number of load-balancer-set service objects that are allowed to reconcile concurrently. Larger number = more responsive load-balancer-set services, but more CPU (and network) load")
}

// ApplyTo fills up poolservice config with options.
func (o *LoadBalancerSetControllerOptions) ApplyTo(cfg *config.LoadBalancerSetControllerConfiguration) error {
	if o == nil {
		return nil
	}

	cfg.ConcurrentLoadBalancerSetWorkers = o.ConcurrentLoadBalancerSetWorkers

	return nil
}

// Validate checks validation of LoadBalancerSetControllerOptions.
func (o *LoadBalancerSetControllerOptions) Validate() []error {
	if o == nil {
		return nil
	}
	errs := []error{}
	return errs
}
