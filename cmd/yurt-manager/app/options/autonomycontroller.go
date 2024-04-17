/*
Copyright 2024 The OpenYurt Authors.

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

	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/autonomy/config"
)

type AutonomyControllerOptions struct {
	*config.AutonomyControllerConfiguration
}

func NewAutonomyControllerOptions() *AutonomyControllerOptions {
	return &AutonomyControllerOptions{
		&config.AutonomyControllerConfiguration{
			ConcurrentAutonomyWorkers: 3,
		},
	}
}

// AddFlags adds flags related to autonomy manager to the specified FlagSet.
func (o *AutonomyControllerOptions) AddFlags(fs *pflag.FlagSet) {
	if o == nil {
		return
	}

	fs.Int32Var(&o.ConcurrentAutonomyWorkers, "concurrent-autonomy-workers", o.ConcurrentAutonomyWorkers, "The number of autonomy controller that are allowed to reconcile concurrently. Larger number = more responsive autonomy controller, but more CPU (and network) load")
}

func (o *AutonomyControllerOptions) ApplyTo(cfg *config.AutonomyControllerConfiguration) error {
	if o == nil {
		return nil
	}
	cfg.ConcurrentAutonomyWorkers = o.ConcurrentAutonomyWorkers
	return nil
}

func (o *AutonomyControllerOptions) Validate() []error {
	if o == nil {
		return nil
	}
	var errs []error
	return errs
}
