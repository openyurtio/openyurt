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

	"github.com/openyurtio/openyurt/pkg/controller/iot/config"
)

type IoTControllerOptions struct {
	*config.IoTControllerConfiguration
}

func NewIoTControllerOptions() *IoTControllerOptions {
	return &IoTControllerOptions{
		config.NewIoTControllerConfiguration(),
	}
}

// AddFlags adds flags related to nodepool for yurt-manager to the specified FlagSet.
func (n *IoTControllerOptions) AddFlags(fs *pflag.FlagSet) {
	if n == nil {
		return
	}
}

// ApplyTo fills up nodepool config with options.
func (o *IoTControllerOptions) ApplyTo(cfg *config.IoTControllerConfiguration) error {
	if o == nil {
		return nil
	}
	*cfg = *o.IoTControllerConfiguration
	return nil
}

// Validate checks validation of IoTControllerOptions.
func (o *IoTControllerOptions) Validate() []error {
	if o == nil {
		return nil
	}
	errs := []error{}
	if o.IoTControllerConfiguration == nil {
		errs = append(errs, errors.New("IoTControllerConfiguration can not be empty!"))
	}
	return errs
}
