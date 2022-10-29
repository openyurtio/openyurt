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
//
package app

import (
	"net/http"

	"github.com/openyurtio/openyurt/pkg/controller/certificates"
	daemonpodupdater "github.com/openyurtio/openyurt/pkg/controller/daemonpodupdater"
	poolcoordinator "github.com/openyurtio/openyurt/pkg/controller/poolcoordinator"
	"github.com/openyurtio/openyurt/pkg/controller/servicetopology"
	"github.com/openyurtio/openyurt/pkg/webhook"
)

func startPoolCoordinatorController(ctx ControllerContext) (http.Handler, bool, error) {
	poolcoordinatorController := poolcoordinator.NewController(
		ctx.ClientBuilder.ClientOrDie("poolcoordinator-controller"),
		ctx.InformerFactory,
	)
	go poolcoordinatorController.Run(ctx.Stop)
	return nil, true, nil
}

func startYurtCSRApproverController(ctx ControllerContext) (http.Handler, bool, error) {
	clientSet := ctx.ClientBuilder.ClientOrDie("yurt-csr-controller")
	csrApprover, err := certificates.NewCSRApprover(clientSet, ctx.InformerFactory)
	if err != nil {
		return nil, false, err
	}
	go csrApprover.Run(2, ctx.Stop)

	return nil, true, nil
}

func startDaemonPodUpdaterController(ctx ControllerContext) (http.Handler, bool, error) {
	daemonPodUpdaterCtrl := daemonpodupdater.NewController(
		ctx.ClientBuilder.ClientOrDie("daemonPodUpdater-controller"),
		ctx.InformerFactory.Apps().V1().DaemonSets(),
		ctx.InformerFactory.Core().V1().Nodes(),
		ctx.InformerFactory.Core().V1().Pods(),
	)

	go daemonPodUpdaterCtrl.Run(2, ctx.Stop)
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

func startWebhookManager(ctx ControllerContext) (http.Handler, bool, error) {
	webhookManager := webhook.NewWebhookManager(
		ctx.ClientBuilder.ClientOrDie("webhook manager"),
		ctx.InformerFactory,
	)
	go webhookManager.Run(ctx.Stop)
	return nil, true, nil
}
