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

package upgrade

import (
	"fmt"
	"time"

	"github.com/spf13/pflag"
)

const (
	DefaultStaticPodRunningCheckTimeout = 2 * time.Minute
)

// Options has the information that required by static-pod-upgrade operation
type Options struct {
	name      string
	namespace string
	manifest  string
	hash      string
	mode      string
	timeout   time.Duration
}

// NewUpgradeOptions creates a new Options
func NewUpgradeOptions() *Options {
	return &Options{
		timeout: DefaultStaticPodRunningCheckTimeout,
	}
}

// AddFlags sets flags.
func (o *Options) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&o.name, "name", o.name, "The name of static pod which needs be upgraded")
	fs.StringVar(&o.namespace, "namespace", o.namespace, "The namespace of static pod which needs be upgraded")
	fs.StringVar(&o.manifest, "manifest", o.manifest, "The manifest file name of static pod which needs be upgraded")
	fs.StringVar(&o.hash, "hash", o.hash, "The hash value of new static pod specification")
	fs.StringVar(&o.mode, "mode", o.mode, "The upgrade mode which is used")
	fs.DurationVar(&o.timeout, "timeout", o.timeout, "The timeout for upgrade success check.")
}

// Validate validates Options
func (o *Options) Validate() error {
	if len(o.name) == 0 || len(o.namespace) == 0 || len(o.manifest) == 0 || len(o.hash) == 0 || len(o.mode) == 0 {
		return fmt.Errorf("args can not be empty, name is %s, namespace is %s,manifest is %s, hash is %s,mode is %s",
			o.name, o.namespace, o.manifest, o.hash, o.mode)
	}

	return nil
}
