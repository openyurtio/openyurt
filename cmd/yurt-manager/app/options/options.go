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
	Generic                      *GenericOptions
	PodBindingController         *PodBindingControllerOptions
	DaemonPodUpdaterController   *DaemonPodUpdaterControllerOptions
	CsrApproverController        *CsrApproverControllerOptions
	NodePoolController           *NodePoolControllerOptions
	YurtStaticSetController      *YurtStaticSetControllerOptions
	YurtAppSetController         *YurtAppSetControllerOptions
	PlatformAdminController      *PlatformAdminControllerOptions
	NodeLifeCycleController      *NodeLifecycleControllerOptions
	NodeBucketController         *NodeBucketControllerOptions
	EndpointsController          *EndpointsControllerOptions
	EndpointSliceController      *EndpointSliceControllerOptions
	LoadBalancerSetController    *LoadBalancerSetControllerOptions
	GatewayPickupController      *GatewayPickupControllerOptions
	GatewayDNSController         *GatewayDNSControllerOptions
	GatewayInternalSvcController *GatewayInternalSvcControllerOptions
	GatewayPublicSvcController   *GatewayPublicSvcControllerOptions
	HubLeaderController          *HubLeaderControllerOptions
	HubLeaderConfigController    *HubLeaderConfigControllerOptions
}

// NewYurtManagerOptions creates a new YurtManagerOptions with a default config.
func NewYurtManagerOptions() (*YurtManagerOptions, error) {
	genericOptions := NewGenericOptions()
	s := YurtManagerOptions{
		Generic:                      genericOptions,
		PodBindingController:         NewPodBindingControllerOptions(),
		DaemonPodUpdaterController:   NewDaemonPodUpdaterControllerOptions(),
		CsrApproverController:        NewCsrApproverControllerOptions(),
		NodePoolController:           NewNodePoolControllerOptions(),
		YurtStaticSetController:      NewYurtStaticSetControllerOptions(),
		YurtAppSetController:         NewYurtAppSetControllerOptions(),
		PlatformAdminController:      NewPlatformAdminControllerOptions(),
		NodeLifeCycleController:      NewNodeLifecycleControllerOptions(),
		NodeBucketController:         NewNodeBucketControllerOptions(),
		EndpointsController:          NewEndpointsControllerOptions(),
		EndpointSliceController:      NewEndpointSliceControllerOptions(),
		LoadBalancerSetController:    NewLoadBalancerSetControllerOptions(),
		GatewayPickupController:      NewGatewayPickupControllerOptions(),
		GatewayDNSController:         NewGatewayDNSControllerOptions(),
		GatewayInternalSvcController: NewGatewayInternalSvcControllerOptions(),
		GatewayPublicSvcController:   NewGatewayPublicSvcControllerOptions(),
		HubLeaderController:          NewHubLeaderControllerOptions(),
		HubLeaderConfigController:    NewHubLeaderConfigControllerOptions(genericOptions.WorkingNamespace),
	}

	return &s, nil
}

func (y *YurtManagerOptions) Flags(allControllers, disabledByDefaultControllers []string) cliflag.NamedFlagSets {
	fss := cliflag.NamedFlagSets{}
	y.Generic.AddFlags(fss.FlagSet("generic"), allControllers, disabledByDefaultControllers)
	y.PodBindingController.AddFlags(fss.FlagSet("podbinding controller"))
	y.DaemonPodUpdaterController.AddFlags(fss.FlagSet("daemonpodupdater controller"))
	y.CsrApproverController.AddFlags(fss.FlagSet("csrapprover controller"))
	y.NodePoolController.AddFlags(fss.FlagSet("nodepool controller"))
	y.YurtAppSetController.AddFlags(fss.FlagSet("yurtappset controller"))
	y.YurtStaticSetController.AddFlags(fss.FlagSet("yurtstaticset controller"))
	y.PlatformAdminController.AddFlags(fss.FlagSet("iot controller"))
	y.NodeLifeCycleController.AddFlags(fss.FlagSet("nodelifecycle controller"))
	y.NodeBucketController.AddFlags(fss.FlagSet("nodebucket controller"))
	y.EndpointsController.AddFlags(fss.FlagSet("endpoints controller"))
	y.EndpointSliceController.AddFlags(fss.FlagSet("endpointslice controller"))
	y.LoadBalancerSetController.AddFlags(fss.FlagSet("loadbalancerset controller"))
	y.GatewayPickupController.AddFlags(fss.FlagSet("gatewaypickup controller"))
	y.GatewayDNSController.AddFlags(fss.FlagSet("gatewaydns controller"))
	y.GatewayInternalSvcController.AddFlags(fss.FlagSet("gatewayinternalsvc controller"))
	y.GatewayPublicSvcController.AddFlags(fss.FlagSet("gatewaypublicsvc controller"))
	y.HubLeaderController.AddFlags(fss.FlagSet("hubleader controller"))
	y.HubLeaderConfigController.AddFlags(fss.FlagSet("hubleaderconfig controller"))
	return fss
}

// Validate is used to validate the options and config before launching the yurt-manager
func (y *YurtManagerOptions) Validate(allControllers []string, controllerAliases map[string]string) error {
	var errs []error
	errs = append(errs, y.Generic.Validate(allControllers, controllerAliases)...)
	errs = append(errs, y.PodBindingController.Validate()...)
	errs = append(errs, y.DaemonPodUpdaterController.Validate()...)
	errs = append(errs, y.CsrApproverController.Validate()...)
	errs = append(errs, y.NodePoolController.Validate()...)
	errs = append(errs, y.YurtAppSetController.Validate()...)
	errs = append(errs, y.YurtStaticSetController.Validate()...)
	errs = append(errs, y.PlatformAdminController.Validate()...)
	errs = append(errs, y.NodeLifeCycleController.Validate()...)
	errs = append(errs, y.NodeBucketController.Validate()...)
	errs = append(errs, y.EndpointsController.Validate()...)
	errs = append(errs, y.EndpointSliceController.Validate()...)
	errs = append(errs, y.LoadBalancerSetController.Validate()...)
	errs = append(errs, y.GatewayPickupController.Validate()...)
	errs = append(errs, y.GatewayDNSController.Validate()...)
	errs = append(errs, y.GatewayInternalSvcController.Validate()...)
	errs = append(errs, y.GatewayPublicSvcController.Validate()...)
	errs = append(errs, y.HubLeaderController.Validate()...)
	errs = append(errs, y.HubLeaderConfigController.Validate()...)
	return utilerrors.NewAggregate(errs)
}

// ApplyTo fills up yurt manager config with options.
func (y *YurtManagerOptions) ApplyTo(c *config.Config, controllerAliases map[string]string) error {
	if err := y.Generic.ApplyTo(&c.ComponentConfig.Generic, controllerAliases); err != nil {
		return err
	}
	if err := y.PodBindingController.ApplyTo(&c.ComponentConfig.PodBindingController); err != nil {
		return err
	}
	if err := y.DaemonPodUpdaterController.ApplyTo(&c.ComponentConfig.DaemonPodUpdaterController); err != nil {
		return err
	}
	if err := y.CsrApproverController.ApplyTo(&c.ComponentConfig.CsrApproverController); err != nil {
		return err
	}
	if err := y.NodePoolController.ApplyTo(&c.ComponentConfig.NodePoolController); err != nil {
		return err
	}
	if err := y.YurtAppSetController.ApplyTo(&c.ComponentConfig.YurtAppSetController); err != nil {
		return err
	}
	if err := y.YurtStaticSetController.ApplyTo(&c.ComponentConfig.YurtStaticSetController); err != nil {
		return err
	}
	if err := y.PlatformAdminController.ApplyTo(&c.ComponentConfig.PlatformAdminController); err != nil {
		return err
	}
	if err := y.NodeLifeCycleController.ApplyTo(&c.ComponentConfig.NodeLifeCycleController); err != nil {
		return err
	}
	if err := y.NodeBucketController.ApplyTo(&c.ComponentConfig.NodeBucketController); err != nil {
		return err
	}
	if err := y.EndpointsController.ApplyTo(&c.ComponentConfig.ServiceTopologyEndpointsController); err != nil {
		return err
	}
	if err := y.EndpointSliceController.ApplyTo(&c.ComponentConfig.ServiceTopologyEndpointSliceController); err != nil {
		return err
	}
	if err := y.LoadBalancerSetController.ApplyTo(&c.ComponentConfig.LoadBalancerSetController); err != nil {
		return err
	}
	if err := y.GatewayPickupController.ApplyTo(&c.ComponentConfig.GatewayPickupController); err != nil {
		return err
	}
	if err := y.GatewayDNSController.ApplyTo(&c.ComponentConfig.GatewayDNSController); err != nil {
		return err
	}
	if err := y.GatewayInternalSvcController.ApplyTo(&c.ComponentConfig.GatewayInternalSvcController); err != nil {
		return err
	}
	if err := y.GatewayPublicSvcController.ApplyTo(&c.ComponentConfig.GatewayPublicSvcController); err != nil {
		return err
	}
	if err := y.HubLeaderController.ApplyTo(&c.ComponentConfig.HubLeaderController); err != nil {
		return err
	}
	if err := y.HubLeaderConfigController.ApplyTo(&c.ComponentConfig.HubLeaderConfigController); err != nil {
		return err
	}
	return nil
}

// Config return a yurt-manager config objective
func (y *YurtManagerOptions) Config(
	allControllers []string,
	controllerAliases map[string]string,
) (*config.Config, error) {
	if err := y.Validate(allControllers, controllerAliases); err != nil {
		return nil, err
	}

	c := &config.Config{}
	if err := y.ApplyTo(c, controllerAliases); err != nil {
		return nil, err
	}

	return c, nil
}
