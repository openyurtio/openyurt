/*
Copyright 2020 The OpenYurt Authors.
Copyright 2016 The Kubernetes Authors.

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

// Package app implements a server that runs a set of active
// components.  This includes replication controllers, service endpoints and
// nodes.

package app

import (
	"net/http"

	poolcoordinator "github.com/openyurtio/openyurt/pkg/controller/poolcoordinator/delegatelease"
	"github.com/openyurtio/openyurt/pkg/controller/poolcoordinator/podbinding"
	"github.com/openyurtio/openyurt/pkg/controller/servicetopology"
)

func startPoolCoordinatorController(ctx ControllerContext) (http.Handler, bool, error) {
	poolcoordinatorController := poolcoordinator.NewController(
		ctx.ClientBuilder.ClientOrDie("poolcoordinator-delegate-lease"),
		ctx.InformerFactory,
	)
	go poolcoordinatorController.Run(ctx.Stop)
	return nil, true, nil
}

func startServiceTopologyController(ctx ControllerContext) (http.Handler, bool, error) {
	clientSet := ctx.ClientBuilder.ClientOrDie("yurt-servicetopology-controller")

	svcTopologyController, err := servicetopology.NewServiceTopologyController(
		clientSet,
		ctx.InformerFactory,
		ctx.YurtInformerFactory,
	)
	if err != nil {
		return nil, false, err
	}
	go svcTopologyController.Run(ctx.Stop)
	return nil, true, nil
}

func startPodBindingController(ctx ControllerContext) (http.Handler, bool, error) {
	podBindingController := podbinding.NewController(
		ctx.ClientBuilder.ClientOrDie("poolcoordinator-pod-binding"),
		ctx.InformerFactory,
	)
	go podBindingController.Run(ctx.Stop)
	return nil, true, nil
}
