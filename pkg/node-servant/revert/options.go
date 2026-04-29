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

package revert

import (
	"fmt"
	"os"

	"github.com/spf13/pflag"

	"github.com/openyurtio/openyurt/pkg/yurtadm/constants"
	enutil "github.com/openyurtio/openyurt/pkg/yurtadm/util/edgenode"
)

// Options has the information that required by revert operation.
type Options struct {
	nodeName    string
	openyurtDir string
}

// NewRevertOptions creates a new Options.
func NewRevertOptions() *Options {
	return &Options{}
}

// Complete completes all the required options.
func (o *Options) Complete() error {
	if o.nodeName == "" {
		o.nodeName = os.Getenv(enutil.NodeName)
	}

	openyurtDir := os.Getenv("OPENYURT_DIR")
	if openyurtDir == "" {
		openyurtDir = constants.OpenyurtDir
	}
	o.openyurtDir = openyurtDir

	return nil
}

// Validate validates Options.
func (o *Options) Validate() error {
	if o.nodeName == "" {
		return fmt.Errorf("node name is empty")
	}
	return nil
}

// AddFlags sets flags.
func (o *Options) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&o.nodeName, constants.NodeName, o.nodeName, "The node name where revert is executed.")
}
