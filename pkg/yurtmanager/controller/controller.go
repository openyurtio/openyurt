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

package controller

import (
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/openyurtio/openyurt/cmd/yurt-manager/app/config"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/csrapprover"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/daemonpodupdater"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/nodepool"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/platformadmin"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/raven"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/raven/dns"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/raven/gatewaypickup"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/servicetopology"
	servicetopologyendpoints "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/servicetopology/endpoints"
	servicetopologyendpointslice "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/servicetopology/endpointslice"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/yurtappdaemon"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/yurtappoverrider"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/yurtappset"
	yurtcoordinatorcert "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/yurtcoordinator/cert"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/yurtcoordinator/delegatelease"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/yurtcoordinator/podbinding"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/yurtstaticset"
)

// Note !!! @kadisi
// Do not change the name of the file @kadisi
// Note !!!

// Don`t Change this Name !!!!  @kadisi
// TODO support feature gate @kadisi
type AddControllerFn func(*config.CompletedConfig, manager.Manager) error

var controllerAddFuncs = make(map[string][]AddControllerFn)

func init() {
	controllerAddFuncs[csrapprover.ControllerName] = []AddControllerFn{csrapprover.Add}
	controllerAddFuncs[daemonpodupdater.ControllerName] = []AddControllerFn{daemonpodupdater.Add}
	controllerAddFuncs[delegatelease.ControllerName] = []AddControllerFn{delegatelease.Add}
	controllerAddFuncs[podbinding.ControllerName] = []AddControllerFn{podbinding.Add}
	controllerAddFuncs[raven.GatewayPickupControllerName] = []AddControllerFn{gatewaypickup.Add}
	controllerAddFuncs[raven.GatewayDNSControllerName] = []AddControllerFn{dns.Add}
	controllerAddFuncs[nodepool.ControllerName] = []AddControllerFn{nodepool.Add}
	controllerAddFuncs[yurtcoordinatorcert.ControllerName] = []AddControllerFn{yurtcoordinatorcert.Add}
	controllerAddFuncs[servicetopology.ControllerName] = []AddControllerFn{servicetopologyendpoints.Add, servicetopologyendpointslice.Add}
	controllerAddFuncs[yurtstaticset.ControllerName] = []AddControllerFn{yurtstaticset.Add}
	controllerAddFuncs[yurtappset.ControllerName] = []AddControllerFn{yurtappset.Add}
	controllerAddFuncs[yurtappdaemon.ControllerName] = []AddControllerFn{yurtappdaemon.Add}
	controllerAddFuncs[platformadmin.ControllerName] = []AddControllerFn{platformadmin.Add}
	controllerAddFuncs[yurtappoverrider.ControllerName] = []AddControllerFn{yurtappoverrider.Add}
}

// If you want to add additional RBAC, enter it here !!! @kadisi

// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;update;patch;delete

func SetupWithManager(c *config.CompletedConfig, m manager.Manager) error {
	klog.InfoS("SetupWithManager", "len", len(controllerAddFuncs))
	for controllerName, fns := range controllerAddFuncs {
		if !util.IsControllerEnabled(controllerName, c.ComponentConfig.Generic.Controllers) {
			klog.Warningf("Controller %v is disabled", controllerName)
			continue
		}

		for _, f := range fns {
			if err := f(c, m); err != nil {
				if kindMatchErr, ok := err.(*meta.NoKindMatchError); ok {
					klog.Infof("CRD %v is not installed, its controller will perform noops!", kindMatchErr.GroupKind)
					continue
				}
				return err
			}
		}
	}
	return nil
}
