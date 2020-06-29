/*
Copyright 2020 The OpenYurt Authors.

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

package healthchecker

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/alibaba/openyurt/pkg/yurthub/transport"

	"k8s.io/klog"
)

const (
	heartbeatFrequency     = 10 * time.Second
	heartbeatRetry         = 3
	continuousHealthyCount = 2
)

// HealthChecker is an interface for checking healthy stats of server
type HealthChecker interface {
	IsHealthy(server *url.URL) bool
}

type healthCheckerManager struct {
	sync.RWMutex
	checkers map[string]*checker
}

// NewHealthChecker create an HealthChecker for servers
func NewHealthChecker(remoteServers []*url.URL, tp transport.Interface, failedRetry, healthyThreshold int, stopCh <-chan struct{}) (HealthChecker, error) {
	if len(remoteServers) == 0 {
		return nil, fmt.Errorf("no remote servers")
	}

	hcm := &healthCheckerManager{
		checkers: make(map[string]*checker),
	}

	for _, server := range remoteServers {
		checker, err := newChecker(server, tp, failedRetry, healthyThreshold, stopCh)
		if err != nil {
			klog.Errorf("new health checker for %s err, %v", server.String(), err)
			return nil, err
		}
		hcm.checkers[server.String()] = checker
	}

	return hcm, nil
}

// IsHealthy returns the healthy stats of specified server
func (hcm *healthCheckerManager) IsHealthy(server *url.URL) bool {
	hcm.RLock()
	defer hcm.RUnlock()
	if checker, ok := hcm.checkers[server.String()]; ok {
		return checker.isHealthy()
	}

	return false
}

type checker struct {
	sync.RWMutex
	serverHealthzAddr string
	healthzClient     *http.Client
	clusterHealthy    bool
	lastTime          time.Time
	onFailureFunc     func(string)
	netAddress        string
	failedRetry       int
	healthyThreshold  int
}

func newChecker(url *url.URL, tp transport.Interface, failedRetry, healthyThreshold int, stopCh <-chan struct{}) (*checker, error) {
	serverHealthzUrl := *url
	if serverHealthzUrl.Path == "" || serverHealthzUrl.Path == "/" {
		serverHealthzUrl.Path = "/healthz"
	}

	if failedRetry == 0 {
		failedRetry = heartbeatRetry
	}

	if healthyThreshold == 0 {
		healthyThreshold = continuousHealthyCount
	}

	c := &checker{
		serverHealthzAddr: serverHealthzUrl.String(),
		healthzClient:     tp.HealthzHttpClient(),
		clusterHealthy:    false,
		lastTime:          time.Now(),
		onFailureFunc:     tp.Close,
		netAddress:        url.Host,
		failedRetry:       failedRetry,
		healthyThreshold:  healthyThreshold,
	}

	initHealthyStatus, err := pingClusterHealthz(c.healthzClient, c.serverHealthzAddr)
	if err != nil {
		klog.Errorf("cluster(%s) init status: unhealthy, %v", c.serverHealthzAddr, err)
	}
	c.clusterHealthy = initHealthyStatus

	go c.healthyCheckLoop(stopCh)
	return c, nil
}

func (c *checker) isHealthy() bool {
	c.RLock()
	defer c.RUnlock()
	return c.clusterHealthy
}

func (c *checker) healthyCheckLoop(stopCh <-chan struct{}) {
	intervalTicker := time.NewTicker(heartbeatFrequency)
	defer intervalTicker.Stop()
	healthyCnt := 0
	isHealthy := false
	var err error

	for {
		select {
		case <-stopCh:
			klog.Infof("exit normally in health check loop for %s", c.netAddress)
			return
		case <-intervalTicker.C:
			for i := 0; i < c.failedRetry; i++ {
				isHealthy, err = pingClusterHealthz(c.healthzClient, c.serverHealthzAddr)
				if err != nil {
					klog.V(2).Infof("ping cluster healthz with result, %v", err)
					if !c.clusterHealthy {
						// cluster is unhealthy, no need ping more times to check unhealthy
						break
					}
				} else {
					if c.clusterHealthy {
						// cluster is healthy, no need ping more times to check healthy
						break
					}
				}
			}

			if !isHealthy {
				// cluster is unhealthy
				healthyCnt = 0
				if c.clusterHealthy {
					c.Lock()
					c.clusterHealthy = false
					c.Unlock()
					now := time.Now()
					klog.Infof("cluster becomes unhealthy from %v, healthy status lasts %v", now, now.Sub(c.lastTime))
					c.onFailureFunc(c.netAddress)
					c.lastTime = now
				}
			} else {
				//  with continuous 2 times cluster healthy, unhealthy will changed to healthy
				healthyCnt++
				if !c.clusterHealthy && healthyCnt >= c.healthyThreshold {
					c.Lock()
					c.clusterHealthy = true
					c.Unlock()
					now := time.Now()
					klog.Infof("cluster becomes healthy from %v, unhealthy status lasts %v", now, now.Sub(c.lastTime))
					c.lastTime = now
				}
			}
		}
	}
}

func pingClusterHealthz(client *http.Client, addr string) (bool, error) {
	if client == nil {
		return false, fmt.Errorf("http client is invalid")
	}

	resp, err := client.Get(addr)
	if err != nil {
		return false, err
	}

	b, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		return false, fmt.Errorf("failed to read response of cluster healthz, %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("response status code is %d", resp.StatusCode)
	}

	if strings.ToLower(string(b)) != "ok" {
		return false, fmt.Errorf("cluster healthz is %s", string(b))
	}

	return true, nil
}
