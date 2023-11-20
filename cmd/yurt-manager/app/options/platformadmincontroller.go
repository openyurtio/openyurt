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
	"errors"

	"github.com/spf13/pflag"

	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/platformadmin/config"
)

type PlatformAdminControllerOptions struct {
	*config.PlatformAdminControllerConfiguration
}

func NewPlatformAdminControllerOptions() *PlatformAdminControllerOptions {
	return &PlatformAdminControllerOptions{
		config.NewPlatformAdminControllerConfiguration(),
	}
}

// AddFlags adds flags related to nodePool for yurt-manager to the specified FlagSet.
func (n *PlatformAdminControllerOptions) AddFlags(fs *pflag.FlagSet) {
	if n == nil {
		return
	}
}

// ApplyTo fills up nodePool config with options.
func (o *PlatformAdminControllerOptions) ApplyTo(cfg *config.PlatformAdminControllerConfiguration) error {
	if o == nil {
		return nil
	}
	*cfg = *o.PlatformAdminControllerConfiguration
	return nil
}

// Validate checks validation of IoTControllerOptions.
func (o *PlatformAdminControllerOptions) Validate() []error {
	if o == nil {
		return nil
	}
	errs := []error{}
	if o.PlatformAdminControllerConfiguration == nil {
		errs = append(errs, errors.New("IoTControllerConfiguration can not be empty!"))
	}
	return errs
}
