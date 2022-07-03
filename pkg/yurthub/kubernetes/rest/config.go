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
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/config"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/yurthub/certificate/interfaces"
	"github.com/openyurtio/openyurt/pkg/yurthub/healthchecker"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

type RestConfigManager struct {
	remoteServers []*url.URL
	checker       healthchecker.HealthChecker
	certManager   interfaces.YurtCertificateManager
}

// NewRestConfigManager creates a *RestConfigManager object
func NewRestConfigManager(cfg *config.YurtHubConfiguration, certMgr interfaces.YurtCertificateManager, healthChecker healthchecker.HealthChecker) (*RestConfigManager, error) {
	mgr := &RestConfigManager{
		remoteServers: cfg.RemoteServers,
		checker:       healthChecker,
		certManager:   certMgr,
	}
	return mgr, nil
}

// GetRestConfig gets rest client config according to the mode of certificateManager
func (rcm *RestConfigManager) GetRestConfig(needHealthyServer bool) *rest.Config {
	return rcm.getHubselfRestConfig(needHealthyServer)
}

// getHubselfRestConfig gets rest client config from hub agent conf file.
func (rcm *RestConfigManager) getHubselfRestConfig(needHealthyServer bool) *rest.Config {
	healthyServer := rcm.remoteServers[0]
	if needHealthyServer {
		healthyServer = rcm.getHealthyServer()
		if healthyServer == nil {
			klog.Infof("all of remote servers are unhealthy, so return nil for rest config")
			return nil
		}
	}

	// certificate expired, rest config can not be used to connect remote server,
	// so return nil for rest config
	if rcm.certManager.Current() == nil {
		klog.Infof("certificate expired, so return nil for rest config")
		return nil
	}

	hubConfFile := rcm.certManager.GetConfFilePath()
	if isExist, _ := util.FileExists(hubConfFile); isExist {
		cfg, err := util.LoadRESTClientConfig(hubConfFile)
		if err != nil {
			klog.Errorf("could not get rest config for %s, %v", hubConfFile, err)
			return nil
		}

		// re-fix host connecting healthy server
		cfg.Host = healthyServer.String()
		klog.Infof("re-fix hub rest config host successfully with server %s", cfg.Host)
		return cfg
	}

	klog.Errorf("%s config file(%s) is not exist", projectinfo.GetHubName(), hubConfFile)
	return nil
}

// getHealthyServer is used to get a healthy server
func (rcm *RestConfigManager) getHealthyServer() *url.URL {
	for _, server := range rcm.remoteServers {
		if rcm.checker.IsHealthy(server) {
			return server
		}
	}
	return nil
}
