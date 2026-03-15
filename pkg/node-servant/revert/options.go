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

import "os"

import "github.com/openyurtio/openyurt/pkg/yurtadm/constants"

// Options has the information that required by revert operation.
type Options struct {
	openyurtDir string
}

// NewRevertOptions creates a new Options.
func NewRevertOptions() *Options {
	return &Options{}
}

// Complete completes all the required options.
func (o *Options) Complete() error {
	openyurtDir := os.Getenv("OPENYURT_DIR")
	if openyurtDir == "" {
		openyurtDir = constants.OpenyurtDir
	}
	o.openyurtDir = openyurtDir

	return nil
}
