/*
Copyright 2021 The OpenYurt Authors.

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

package rest

import (
	"net/url"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurthub/certificate"
	"github.com/openyurtio/openyurt/pkg/yurthub/healthchecker"
)

type RestConfigManager struct {
	checker     healthchecker.MultipleBackendsHealthChecker
	certManager certificate.YurtCertificateManager
}

// NewRestConfigManager creates a *RestConfigManager object
func NewRestConfigManager(certManager certificate.YurtCertificateManager, healthChecker healthchecker.MultipleBackendsHealthChecker) (*RestConfigManager, error) {
	mgr := &RestConfigManager{
		checker:     healthChecker,
		certManager: certManager,
	}
	return mgr, nil
}

// GetRestConfig gets rest client config according to the mode of certificateManager
func (rcm *RestConfigManager) GetRestConfig(needHealthyServer bool) *rest.Config {
	var healthyServer *url.URL
	if needHealthyServer {
		healthyServer, _ = rcm.checker.PickHealthyServer()
		if healthyServer == nil {
			klog.Infof("all of remote servers are unhealthy, so return nil for rest config")
			return nil
		}
	}

	kubeconfig, err := clientcmd.BuildConfigFromFlags("", rcm.certManager.GetHubConfFile())
	if err != nil {
		klog.Errorf("could not load kube config(%s), %v", rcm.certManager.GetHubConfFile(), err)
		return nil
	}

	if healthyServer != nil {
		// re-fix host connecting healthy server
		kubeconfig.Host = healthyServer.String()
		klog.Infof("re-fix hub rest config host successfully with server %s", kubeconfig.Host)
	}
	return kubeconfig
}
