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
	"time"

	"github.com/openyurtio/openyurt/pkg/controller/certificates"
	"github.com/openyurtio/openyurt/pkg/controller/certificates/signer"
	lifecyclecontroller "github.com/openyurtio/openyurt/pkg/controller/nodelifecycle"
)

func startNodeLifecycleController(ctx ControllerContext) (http.Handler, bool, error) {
	lifecycleController, err := lifecyclecontroller.NewNodeLifecycleController(
		ctx.InformerFactory.Coordination().V1().Leases(),
		ctx.InformerFactory.Core().V1().Pods(),
		ctx.InformerFactory.Core().V1().Nodes(),
		ctx.InformerFactory.Apps().V1().DaemonSets(),
		// node lifecycle controller uses existing cluster role from node-controller
		ctx.ClientBuilder.ClientOrDie("node-controller"),
		//ctx.ComponentConfig.KubeCloudShared.NodeMonitorPeriod.Duration,
		5*time.Second,
		ctx.ComponentConfig.NodeLifecycleController.NodeStartupGracePeriod.Duration,
		ctx.ComponentConfig.NodeLifecycleController.NodeMonitorGracePeriod.Duration,
		ctx.ComponentConfig.NodeLifecycleController.PodEvictionTimeout.Duration,
		ctx.ComponentConfig.NodeLifecycleController.NodeEvictionRate,
		ctx.ComponentConfig.NodeLifecycleController.SecondaryNodeEvictionRate,
		ctx.ComponentConfig.NodeLifecycleController.LargeClusterSizeThreshold,
		ctx.ComponentConfig.NodeLifecycleController.UnhealthyZoneThreshold,
		*ctx.ComponentConfig.NodeLifecycleController.EnableTaintManager,
	)
	if err != nil {
		return nil, true, err
	}
	go lifecycleController.Run(ctx.Stop)
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

func startYurtCSRSignerController(ctx ControllerContext) (http.Handler, bool, error) {
	clientSet := ctx.ClientBuilder.ClientOrDie("yurt-signer-controller")

	certTTL := 100 * 365 * 24 * time.Hour
	certFile := "/etc/kubernetes/pki/ca.crt"
	keyFile := "/etc/kubernetes/pki/ca.key"
	yurtSigner, err := signer.NewKubeAPIServerClientCSRSigningController(clientSet, ctx.InformerFactory, certFile, keyFile, certTTL)

	if err != nil {
		return nil, false, err
	}
	go yurtSigner.Run(2, ctx.Stop)

	return nil, true, nil
}
