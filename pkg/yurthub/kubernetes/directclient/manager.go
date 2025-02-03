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

package directclient

import (
	"fmt"
	"net/url"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurthub/healthchecker"
	"github.com/openyurtio/openyurt/pkg/yurthub/transport"
)

// DirectClientManager is a holder for clientsets which are used to connecting cloud kube-apiserver directly.
// All clientsets are prepared when yurthub startup, so it is efficient to get a clientset by this manager
// for accessing cloud kube-apiserver.
type DirectClientManager struct {
	checker           healthchecker.MultipleBackendsHealthChecker
	serverToClientset map[string]*kubernetes.Clientset
}

func NewRestClientManager(servers []*url.URL, tansportManager transport.Interface, healthChecker healthchecker.MultipleBackendsHealthChecker) (*DirectClientManager, error) {
	mgr := &DirectClientManager{
		checker:           healthChecker,
		serverToClientset: make(map[string]*kubernetes.Clientset),
	}

	for i := range servers {
		config := &rest.Config{
			Host:      servers[i].String(),
			Transport: tansportManager.CurrentTransport(),
		}

		clientset, err := kubernetes.NewForConfig(config)
		if err != nil {
			return nil, err
		}

		if len(servers[i].String()) != 0 {
			mgr.serverToClientset[servers[i].String()] = clientset
		}
	}

	if len(mgr.serverToClientset) == 0 {
		return nil, fmt.Errorf("clientset should not be empty")
	}

	return mgr, nil
}

// GetDirectClientset gets kube clientset according to the healthy status of server
func (rcm *DirectClientManager) GetDirectClientset(needHealthyServer bool) *kubernetes.Clientset {
	var serverHost string
	if needHealthyServer {
		healthyServer, _ := rcm.checker.PickHealthyServer()
		if healthyServer == nil {
			klog.Infof("all of remote servers are unhealthy, so return nil for clientset")
			return nil
		}
		serverHost = healthyServer.String()
	} else {
		for host := range rcm.serverToClientset {
			serverHost = host
		}
	}

	return rcm.serverToClientset[serverHost]
}
