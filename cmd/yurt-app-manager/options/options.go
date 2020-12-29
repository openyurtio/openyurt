/*
Copyright 2020 The OpenYurt Authors.

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
)

// YurtAppOptions is the main settings for the yurtapp-manger
type YurtAppOptions struct {
	MetricsAddr             string
	PprofAddr               string
	HealthProbeAddr         string
	EnableLeaderElection    bool
	EnablePprof             bool
	LeaderElectionNamespace string
	Namespace               string
	CreateDefaultPool       bool
	Version                 bool
}

// NewYurtAppOptions creates a new YurtAppOptions with a default config.
func NewYurtAppOptions() *YurtAppOptions {
	o := &YurtAppOptions{
		MetricsAddr:             ":8080",
		PprofAddr:               ":8090",
		HealthProbeAddr:         ":8000",
		EnableLeaderElection:    true,
		EnablePprof:             false,
		LeaderElectionNamespace: "kube-system",
		Namespace:               "",
		CreateDefaultPool:       false,
	}

	return o
}

// ValidateOptions validates YurtAppOptions
func ValidateOptions(options *YurtAppOptions) error {
	// TODO
	return nil
}

// AddFlags returns flags for a specific yurthub by section name
func (o *YurtAppOptions) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&o.MetricsAddr, "metrics-addr", o.MetricsAddr, "The address the metric endpoint binds to.")
	fs.StringVar(&o.PprofAddr, "pprof-addr", o.PprofAddr, "The address the pprof binds to.")
	fs.StringVar(&o.HealthProbeAddr, "health-probe-addr", o.HealthProbeAddr, "The address the healthz/readyz endpoint binds to.")
	fs.BoolVar(&o.EnableLeaderElection, "enable-leader-election", o.EnableLeaderElection, "Whether you need to enable leader election.")
	fs.BoolVar(&o.EnablePprof, "enable-pprof", o.EnablePprof, "Enable pprof for controller manager.")
	fs.StringVar(&o.LeaderElectionNamespace, "leader-election-namespace", o.LeaderElectionNamespace, "This determines the namespace in which the leader election configmap will be created, it will use in-cluster namespace if empty.")
	fs.StringVar(&o.Namespace, "namespace", o.Namespace, "Namespace if specified restricts the manager's cache to watch objects in the desired namespace. Defaults to all namespaces.")
	fs.BoolVar(&o.CreateDefaultPool, "create-default-pool", o.CreateDefaultPool, "Create default cloud/edge pools if indicated.")
	fs.BoolVar(&o.Version, "version", o.Version, "print the version information.")
}
