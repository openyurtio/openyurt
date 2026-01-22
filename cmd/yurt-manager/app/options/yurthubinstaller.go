/*
Copyright 2026 The OpenYurt Authors.

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

	yurthubinstallerconfig "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/yurthubinstaller/config"
)

// YurtHubInstallerControllerOptions holds the YurtHubInstallerController options
type YurtHubInstallerControllerOptions struct {
	*yurthubinstallerconfig.YurtHubInstallerControllerConfiguration
}

// NewYurtHubInstallerControllerOptions creates a new YurtHubInstallerControllerOptions
func NewYurtHubInstallerControllerOptions() *YurtHubInstallerControllerOptions {
	return &YurtHubInstallerControllerOptions{
		&yurthubinstallerconfig.YurtHubInstallerControllerConfiguration{
			ConcurrentYurtHubInstallerWorkers: 3,
			NodeServantImage:                  "openyurt/node-servant:latest",
			YurtHubVersion:                    "latest",
			EnableYurtHubInstaller:            false, // disabled by default
		},
	}
}

// AddFlags adds flags related to YurtHubInstallerController for yurt-manager to the specified FlagSet
func (y *YurtHubInstallerControllerOptions) AddFlags(fs *pflag.FlagSet) {
	if y == nil {
		return
	}

	fs.Int32Var(&y.ConcurrentYurtHubInstallerWorkers, "concurrent-yurthub-installer-workers", y.ConcurrentYurtHubInstallerWorkers, "The number of yurthub installer controller workers that are allowed to sync concurrently. Larger number = more responsive yurthub installation, but more CPU (and network) load")
	fs.StringVar(&y.NodeServantImage, "node-servant-image", y.NodeServantImage, "The node-servant image used for YurtHub installation jobs")
	fs.StringVar(&y.YurtHubVersion, "yurthub-version", y.YurtHubVersion, "The default YurtHub version to install")
	fs.BoolVar(&y.EnableYurtHubInstaller, "enable-yurthub-installer", y.EnableYurtHubInstaller, "Enable the YurtHub installer controller")
}

// ApplyTo fills up YurtHubInstallerController config with options.
func (y *YurtHubInstallerControllerOptions) ApplyTo(cfg *yurthubinstallerconfig.YurtHubInstallerControllerConfiguration) error {
	if y == nil {
		return nil
	}

	cfg.ConcurrentYurtHubInstallerWorkers = y.ConcurrentYurtHubInstallerWorkers
	cfg.NodeServantImage = y.NodeServantImage
	cfg.YurtHubVersion = y.YurtHubVersion
	cfg.EnableYurtHubInstaller = y.EnableYurtHubInstaller
	return nil
}

// Validate checks validation of YurtHubInstallerControllerOptions.
func (y *YurtHubInstallerControllerOptions) Validate() []error {
	if y == nil {
		return nil
	}
	errs := []error{}
	return errs
}
