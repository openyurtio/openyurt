/*
Copyright 2022 The OpenYurt Authors.

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

package config

import (
	"fmt"
	"os"

	"github.com/spf13/pflag"
)

const (
	RunAsStaticPod     = "pod"
	RunAsControllerPod = "controller"
)

// ControlPlaneOptions has the information that required by node-servant config control-plane
type ControlPlaneOptions struct {
	RunMode          string
	PodManifestsPath string
	Version          bool
}

// NewControlPlaneOptions creates a new Options
func NewControlPlaneOptions() *ControlPlaneOptions {
	return &ControlPlaneOptions{
		RunMode:          RunAsStaticPod,
		PodManifestsPath: "/etc/kubernetes/manifests",
	}
}

// Validate validates Options
func (o *ControlPlaneOptions) Validate() error {
	switch o.RunMode {
	case RunAsStaticPod, RunAsControllerPod:
	default:
		return fmt.Errorf("run mode(%s) is not supported, only pod and controller are supported", o.RunMode)
	}

	if info, err := os.Stat(o.PodManifestsPath); err != nil {
		return err
	} else if !info.IsDir() {
		return fmt.Errorf("pod mainifests path(%s) should be a directory", o.PodManifestsPath)
	}

	return nil
}

// AddFlags sets flags.
func (o *ControlPlaneOptions) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&o.RunMode, "run-mode", o.RunMode, "The run mode of control-plane components, only pod and controller modes are supported")
	fs.StringVar(&o.PodManifestsPath, "pod-manifests-path", o.PodManifestsPath, "The path of pod manifests on the worker node.")
	fs.BoolVar(&o.Version, "version", o.Version, "print the version information.")
}
