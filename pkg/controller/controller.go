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
	"github.com/openyurtio/openyurt/pkg/controller/csrapprover"
	"github.com/openyurtio/openyurt/pkg/controller/daemonpodupdater"
	"github.com/openyurtio/openyurt/pkg/controller/nodepool"
	poolcoordinatorcert "github.com/openyurtio/openyurt/pkg/controller/poolcoordinator/cert"
	"github.com/openyurtio/openyurt/pkg/controller/poolcoordinator/delegatelease"
	"github.com/openyurtio/openyurt/pkg/controller/poolcoordinator/podbinding"
	"github.com/openyurtio/openyurt/pkg/controller/raven"
	"github.com/openyurtio/openyurt/pkg/controller/raven/gateway"
	"github.com/openyurtio/openyurt/pkg/controller/raven/service"
	"github.com/openyurtio/openyurt/pkg/controller/servicetopology"
	servicetopologyendpoints "github.com/openyurtio/openyurt/pkg/controller/servicetopology/endpoints"
	servicetopologyendpointslice "github.com/openyurtio/openyurt/pkg/controller/servicetopology/endpointslice"
	"github.com/openyurtio/openyurt/pkg/controller/util"
	"github.com/openyurtio/openyurt/pkg/controller/yurtappdaemon"
	"github.com/openyurtio/openyurt/pkg/controller/yurtappset"
	"github.com/openyurtio/openyurt/pkg/controller/yurtstaticset"
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
	controllerAddFuncs[raven.ControllerName] = []AddControllerFn{gateway.Add, service.Add}
	controllerAddFuncs[nodepool.ControllerName] = []AddControllerFn{nodepool.Add}
	controllerAddFuncs[poolcoordinatorcert.ControllerName] = []AddControllerFn{poolcoordinatorcert.Add}
	controllerAddFuncs[servicetopology.ControllerName] = []AddControllerFn{servicetopologyendpoints.Add, servicetopologyendpointslice.Add}
	controllerAddFuncs[yurtstaticset.ControllerName] = []AddControllerFn{yurtstaticset.Add}
	controllerAddFuncs[yurtappset.ControllerName] = []AddControllerFn{yurtappset.Add}
	controllerAddFuncs[yurtappdaemon.ControllerName] = []AddControllerFn{yurtappdaemon.Add}
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
