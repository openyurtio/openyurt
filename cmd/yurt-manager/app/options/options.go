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

package options

import (
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	cliflag "k8s.io/component-base/cli/flag"

	"github.com/openyurtio/openyurt/cmd/yurt-manager/app/config"
)

// YurtManagerOptions is the main context object for the yurt-manager.
type YurtManagerOptions struct {
	Generic                 *GenericOptions
	NodePoolController      *NodePoolControllerOptions
	GatewayController       *GatewayControllerOptions
	YurtStaticSetController *YurtStaticSetControllerOptions
	YurtAppSetController    *YurtAppSetControllerOptions
	YurtAppDaemonController *YurtAppDaemonControllerOptions
	PlatformAdminController *PlatformAdminControllerOptions
}

// NewYurtManagerOptions creates a new YurtManagerOptions with a default config.
func NewYurtManagerOptions() (*YurtManagerOptions, error) {

	s := YurtManagerOptions{
		Generic:                 NewGenericOptions(),
		NodePoolController:      NewNodePoolControllerOptions(),
		GatewayController:       NewGatewayControllerOptions(),
		YurtStaticSetController: NewYurtStaticSetControllerOptions(),
		YurtAppSetController:    NewYurtAppSetControllerOptions(),
		YurtAppDaemonController: NewYurtAppDaemonControllerOptions(),
		PlatformAdminController: NewPlatformAdminControllerOptions(),
	}

	return &s, nil
}

func (y *YurtManagerOptions) Flags() cliflag.NamedFlagSets {
	fss := cliflag.NamedFlagSets{}
	y.Generic.AddFlags(fss.FlagSet("generic"))
	y.NodePoolController.AddFlags(fss.FlagSet("nodepool controller"))
	y.GatewayController.AddFlags(fss.FlagSet("gateway controller"))
	y.YurtStaticSetController.AddFlags(fss.FlagSet("yurtstaticset controller"))
	y.YurtAppDaemonController.AddFlags(fss.FlagSet("yurtappdaemon controller"))
	y.PlatformAdminController.AddFlags(fss.FlagSet("iot controller"))
	// Please Add Other controller flags @kadisi

	return fss
}

// Validate is used to validate the options and config before launching the yurt-manager
func (y *YurtManagerOptions) Validate() error {
	var errs []error
	errs = append(errs, y.Generic.Validate()...)
	errs = append(errs, y.NodePoolController.Validate()...)
	errs = append(errs, y.GatewayController.Validate()...)
	errs = append(errs, y.YurtStaticSetController.Validate()...)
	errs = append(errs, y.YurtAppDaemonController.Validate()...)
	errs = append(errs, y.PlatformAdminController.Validate()...)
	return utilerrors.NewAggregate(errs)
}

// ApplyTo fills up yurt manager config with options.
func (y *YurtManagerOptions) ApplyTo(c *config.Config) error {
	if err := y.Generic.ApplyTo(&c.ComponentConfig.Generic); err != nil {
		return err
	}
	if err := y.NodePoolController.ApplyTo(&c.ComponentConfig.NodePoolController); err != nil {
		return err
	}
	if err := y.YurtStaticSetController.ApplyTo(&c.ComponentConfig.YurtStaticSetController); err != nil {
		return err
	}
	if err := y.YurtAppDaemonController.ApplyTo(&c.ComponentConfig.YurtAppDaemonController); err != nil {
		return err
	}
	if err := y.PlatformAdminController.ApplyTo(&c.ComponentConfig.PlatformAdminController); err != nil {
		return err
	}
	return nil
}

// Config return a yurt-manager config objective
func (y *YurtManagerOptions) Config() (*config.Config, error) {
	if err := y.Validate(); err != nil {
		return nil, err
	}

	c := &config.Config{}
	if err := y.ApplyTo(c); err != nil {
		return nil, err
	}

	return c, nil
}
