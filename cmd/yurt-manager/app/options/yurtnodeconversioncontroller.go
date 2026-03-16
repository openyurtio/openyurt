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
	"fmt"

	"github.com/spf13/pflag"

	"github.com/openyurtio/openyurt/pkg/yurtadm/constants"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/yurtnodeconversion/config"
)

type YurtNodeConversionControllerOptions struct {
	*config.YurtNodeConversionControllerConfiguration
}

func NewYurtNodeConversionControllerOptions() *YurtNodeConversionControllerOptions {
	return &YurtNodeConversionControllerOptions{
		&config.YurtNodeConversionControllerConfiguration{
			ConcurrentYurtNodeConversionWorkers: 3,
			YurthubVersion:                      constants.YurthubVersion,
		},
	}
}

// AddFlags adds flags related to YurtNodeConversion for yurt-manager to the specified FlagSet.
func (o *YurtNodeConversionControllerOptions) AddFlags(fs *pflag.FlagSet) {
	if o == nil {
		return
	}

	fs.Int32Var(&o.ConcurrentYurtNodeConversionWorkers, "concurrent-yurtnodeconversion-workers", o.ConcurrentYurtNodeConversionWorkers, "The number of node conversion objects that are allowed to reconcile concurrently. Larger number = more responsive conversion, but more CPU (and network) load")
	fs.StringVar(&o.YurthubVersion, "yurthub-version", o.YurthubVersion, "The yurthub binary version to install when custom binary URL is not set.")
	fs.StringVar(&o.YurthubBinaryURL, constants.YurtHubBinaryUrl, o.YurthubBinaryURL, "The yurthub binary download URL.")
}

// ApplyTo fills up YurtNodeConversion config with options.
func (o *YurtNodeConversionControllerOptions) ApplyTo(cfg *config.YurtNodeConversionControllerConfiguration) error {
	if o == nil {
		return nil
	}

	cfg.ConcurrentYurtNodeConversionWorkers = o.ConcurrentYurtNodeConversionWorkers
	cfg.YurthubVersion = o.YurthubVersion
	cfg.YurthubBinaryURL = o.YurthubBinaryURL

	return nil
}

// Validate checks validation of YurtNodeConversionControllerOptions.
func (o *YurtNodeConversionControllerOptions) Validate() []error {
	if o == nil {
		return nil
	}

	errs := []error{}
	if o.ConcurrentYurtNodeConversionWorkers <= 0 {
		errs = append(errs, fmt.Errorf("concurrent-yurtnodeconversion-workers(%d) is invalid, should greater than 0", o.ConcurrentYurtNodeConversionWorkers))
	}
	if len(o.YurthubBinaryURL) == 0 && len(o.YurthubVersion) == 0 {
		errs = append(errs, fmt.Errorf("yurthub version and binary URL are both empty"))
	}
	return errs
}
