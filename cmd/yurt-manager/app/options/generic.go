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
	"strings"

	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	componentbaseconfig "k8s.io/component-base/config"
	"k8s.io/component-base/config/options"

	"github.com/openyurtio/openyurt/pkg/features"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/apis/config"
)

type GenericOptions struct {
	*config.GenericConfiguration
}

func NewGenericOptions() *GenericOptions {
	return &GenericOptions{
		&config.GenericConfiguration{
			Version:         false,
			MetricsAddr:     ":10271",
			HealthProbeAddr: ":10272",
			WebhookPort:     10273,
			LeaderElection: componentbaseconfig.LeaderElectionConfiguration{
				LeaderElect:       true,
				ResourceLock:      resourcelock.LeasesResourceLock,
				ResourceName:      "yurt-manager",
				ResourceNamespace: "kube-system",
			},
			RestConfigQPS:    50,
			RestConfigBurst:  100,
			WorkingNamespace: "kube-system",
			DisabledWebhooks: []string{},
		},
	}
}

// Validate checks validation of GenericOptions.
func (o *GenericOptions) Validate(allControllers []string, controllerAliases map[string]string) []error {
	if o == nil {
		return nil
	}

	errs := []error{}
	if o.WebhookPort == 0 {
		errs = append(errs, fmt.Errorf("webhook server can not be switched off with 0"))
	}

	allControllersSet := sets.NewString(allControllers...)
	for _, initialName := range o.Controllers {
		if initialName == "*" {
			continue
		}
		initialNameWithoutPrefix := strings.TrimPrefix(initialName, "-")
		controllerName := initialNameWithoutPrefix
		if canonicalName, ok := controllerAliases[controllerName]; ok {
			controllerName = canonicalName
		}
		if !allControllersSet.Has(controllerName) {
			errs = append(errs, fmt.Errorf("%q is not in the list of known controllers", initialNameWithoutPrefix))
		}
	}
	return errs
}

// ApplyTo fills up generic config with options.
func (o *GenericOptions) ApplyTo(cfg *config.GenericConfiguration, controllerAliases map[string]string) error {
	if o == nil {
		return nil
	}

	cfg.Version = o.Version
	cfg.MetricsAddr = o.MetricsAddr
	cfg.HealthProbeAddr = o.HealthProbeAddr
	cfg.WebhookPort = o.WebhookPort
	cfg.LeaderElection = o.LeaderElection
	cfg.LeaderElection.ResourceNamespace = o.WorkingNamespace
	cfg.RestConfigQPS = o.RestConfigQPS
	cfg.RestConfigBurst = o.RestConfigBurst
	cfg.WorkingNamespace = o.WorkingNamespace
	cfg.Kubeconfig = o.Kubeconfig

	cfg.Controllers = make([]string, len(o.Controllers))
	for i, initialName := range o.Controllers {
		initialNameWithoutPrefix := strings.TrimPrefix(initialName, "-")
		controllerName := initialNameWithoutPrefix
		if canonicalName, ok := controllerAliases[controllerName]; ok {
			controllerName = canonicalName
		}
		if strings.HasPrefix(initialName, "-") {
			controllerName = fmt.Sprintf("-%s", controllerName)
		}
		cfg.Controllers[i] = controllerName
	}
	cfg.DisabledWebhooks = o.DisabledWebhooks

	return nil
}

// AddFlags adds flags related to generic for yurt-manager to the specified FlagSet.
func (o *GenericOptions) AddFlags(fs *pflag.FlagSet, allControllers, disabledByDefaultControllers []string) {
	if o == nil {
		return
	}

	fs.BoolVar(&o.Version, "version", o.Version, "Print the version information, and then exit")
	fs.StringVar(&o.MetricsAddr, "metrics-addr", o.MetricsAddr, "The address the metric endpoint binds to.")
	fs.StringVar(&o.HealthProbeAddr, "health-probe-addr", o.HealthProbeAddr, "The address the healthz/readyz endpoint binds to.")
	fs.IntVar(&o.WebhookPort, "webhook-port", o.WebhookPort, "The port on which to serve HTTPS for webhook server. It can't be switched off with 0")
	options.BindLeaderElectionFlags(&o.LeaderElection, fs)
	fs.IntVar(&o.RestConfigQPS, "rest-config-qps", o.RestConfigQPS, "rest-config-qps.")
	fs.IntVar(&o.RestConfigBurst, "rest-config-burst", o.RestConfigBurst, "rest-config-burst.")
	fs.StringVar(&o.WorkingNamespace, "working-namespace", o.WorkingNamespace, "The namespace where the yurt-manager is working.")
	fs.StringSliceVar(&o.Controllers, "controllers", o.Controllers, fmt.Sprintf("A list of controllers to enable. '*' enables all on-by-default controllers, 'foo' enables the controller "+
		"named 'foo', '-foo' disables the controller named 'foo'.\nAll controllers: %s\nDisabled-by-default controllers: %s",
		strings.Join(allControllers, ", "), strings.Join(disabledByDefaultControllers, ", ")))
	fs.StringSliceVar(&o.DisabledWebhooks, "disable-independent-webhooks", o.DisabledWebhooks, "A list of webhooks to disable. "+
		"'*' disables all independent webhooks, 'foo' disables the independent webhook named 'foo'.")
	fs.StringVar(&o.Kubeconfig, "kubeconfig", o.Kubeconfig, "Path to kubeconfig file with authorization and master location information")
	features.DefaultMutableFeatureGate.AddFlag(fs)
}
