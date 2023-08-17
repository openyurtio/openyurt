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
	"fmt"

	"github.com/spf13/pflag"

	"github.com/openyurtio/openyurt/pkg/features"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/apis/config"
)

const enableAll = "*"

type GenericOptions struct {
	*config.GenericConfiguration
}

func NewGenericOptions() *GenericOptions {
	return &GenericOptions{
		&config.GenericConfiguration{
			Version:                 false,
			MetricsAddr:             ":10271",
			HealthProbeAddr:         ":10272",
			WebhookPort:             10273,
			EnableLeaderElection:    true,
			LeaderElectionNamespace: "kube-system",
			RestConfigQPS:           30,
			RestConfigBurst:         50,
			WorkingNamespace:        "kube-system",
			Controllers:             []string{enableAll},
			DisabledWebhooks:        []string{},
		},
	}
}

// Validate checks validation of GenericOptions.
func (o *GenericOptions) Validate() []error {
	if o == nil {
		return nil
	}

	errs := []error{}
	if o.WebhookPort == 0 {
		errs = append(errs, fmt.Errorf("webhook server can not be switched off with 0"))
	}
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
	cfg.WebhookPort = o.WebhookPort
	cfg.EnableLeaderElection = o.EnableLeaderElection
	cfg.LeaderElectionNamespace = o.WorkingNamespace
	cfg.RestConfigQPS = o.RestConfigQPS
	cfg.RestConfigBurst = o.RestConfigBurst
	cfg.WorkingNamespace = o.WorkingNamespace
	cfg.Controllers = o.Controllers
	cfg.DisabledWebhooks = o.DisabledWebhooks

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
	fs.IntVar(&o.WebhookPort, "webhook-port", o.WebhookPort, "The port on which to serve HTTPS for webhook server. It can't be switched off with 0")
	fs.BoolVar(&o.EnableLeaderElection, "enable-leader-election", o.EnableLeaderElection, "Whether you need to enable leader election.")

	fs.IntVar(&o.RestConfigQPS, "rest-config-qps", o.RestConfigQPS, "rest-config-qps.")
	fs.IntVar(&o.RestConfigBurst, "rest-config-burst", o.RestConfigBurst, "rest-config-burst.")
	fs.StringVar(&o.WorkingNamespace, "working-namespace", o.WorkingNamespace, "The namespace where the yurt-manager is working.")
	fs.StringSliceVar(&o.Controllers, "controllers", o.Controllers, "A list of controllers to enable. "+
		"'*' enables all on-by-default controllers, 'foo' enables the controller named 'foo', '-foo' disables the controller named 'foo'.")
	fs.StringSliceVar(&o.DisabledWebhooks, "disable-independent-webhooks", o.DisabledWebhooks, "A list of webhooks to disable. "+
		"'*' disables all webhooks, 'foo' disables the webhook named 'foo'.")

	features.DefaultMutableFeatureGate.AddFlag(fs)
}
