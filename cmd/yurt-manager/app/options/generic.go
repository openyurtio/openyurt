/*
Copyright 2023 The OpenYurt Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package options

import (
	"github.com/spf13/pflag"

	"github.com/openyurtio/openyurt/pkg/controller/apis/config"
	"github.com/openyurtio/openyurt/pkg/features"
)

const enableAll = "*"

type GenericOptions struct {
	*config.GenericConfiguration
}

func NewGenericOptions() *GenericOptions {
	return &GenericOptions{
		&config.GenericConfiguration{
			Version:                 false,
			MetricsAddr:             ":10250",
			HealthProbeAddr:         ":8000",
			EnableLeaderElection:    true,
			LeaderElectionNamespace: "kube-system",
			RestConfigQPS:           30,
			RestConfigBurst:         50,
			WorkingNamespace:        "kube-system",
			Controllers:             []string{enableAll},
			Webhooks:                []string{enableAll},
		},
	}
}

// Validate checks validation of GenericOptions.
func (o *GenericOptions) Validate() []error {
	if o == nil {
		return nil
	}

	errs := []error{}
	return errs
}

// ApplyTo fills up generic config with options.
func (o *GenericOptions) ApplyTo(cfg *config.GenericConfiguration) error {
	if o == nil {
		return nil
	}

	cfg.Version = o.Version
	cfg.MetricsAddr = o.MetricsAddr

	cfg.HealthProbeAddr = o.HealthProbeAddr
	cfg.EnableLeaderElection = o.EnableLeaderElection
	cfg.LeaderElectionNamespace = o.WorkingNamespace
	cfg.RestConfigQPS = o.RestConfigQPS
	cfg.RestConfigBurst = o.RestConfigBurst
	cfg.WorkingNamespace = o.WorkingNamespace
	cfg.Controllers = o.Controllers
	cfg.Webhooks = o.Webhooks

	return nil
}

// AddFlags adds flags related to generic for yurt-manager to the specified FlagSet.
func (o *GenericOptions) AddFlags(fs *pflag.FlagSet) {
	if o == nil {
		return
	}

	fs.BoolVar(&o.Version, "version", o.Version, "Print the version information, and then exit")
	fs.StringVar(&o.MetricsAddr, "metrics-addr", o.MetricsAddr, "The address the metric endpoint binds to.")
	fs.StringVar(&o.HealthProbeAddr, "health-probe-addr", o.HealthProbeAddr, "The address the healthz/readyz endpoint binds to.")
	fs.BoolVar(&o.EnableLeaderElection, "enable-leader-election", o.EnableLeaderElection, "Whether you need to enable leader election.")

	fs.IntVar(&o.RestConfigQPS, "rest-config-qps", o.RestConfigQPS, "rest-config-qps.")
	fs.IntVar(&o.RestConfigBurst, "rest-config-burst", o.RestConfigBurst, "rest-config-burst.")
	fs.StringVar(&o.WorkingNamespace, "working-namespace", o.WorkingNamespace, "The namespace where the yurt-manager is working.")
	fs.StringSliceVar(&o.Controllers, "controllers", o.Controllers, "A list of controllers to enable. "+
		"'*' enables all on-by-default controllers, 'foo' enables the controller named 'foo', '-foo' disables the controller named 'foo'.")
	fs.StringSliceVar(&o.Webhooks, "webhooks", o.Webhooks, "A list of webhooks to enable. "+
		"'*' enables all on-by-default webhooks, 'foo' enables the webhook named 'foo', '-foo' disables the webhook named 'foo'.")

	features.DefaultMutableFeatureGate.AddFlag(fs)
}
