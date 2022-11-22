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

package app

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/config"
	"github.com/openyurtio/openyurt/cmd/yurthub/app/options"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/yurthub/cachemanager"
	"github.com/openyurtio/openyurt/pkg/yurthub/certificate/hubself"
	"github.com/openyurtio/openyurt/pkg/yurthub/gc"
	"github.com/openyurtio/openyurt/pkg/yurthub/healthchecker"
	hubrest "github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/rest"
	"github.com/openyurtio/openyurt/pkg/yurthub/network"
	"github.com/openyurtio/openyurt/pkg/yurthub/poolcoordinator"
	"github.com/openyurtio/openyurt/pkg/yurthub/proxy"
	"github.com/openyurtio/openyurt/pkg/yurthub/server"
	"github.com/openyurtio/openyurt/pkg/yurthub/tenant"
	"github.com/openyurtio/openyurt/pkg/yurthub/transport"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

// NewCmdStartYurtHub creates a *cobra.Command object with default parameters
func NewCmdStartYurtHub(ctx context.Context) *cobra.Command {
	yurtHubOptions := options.NewYurtHubOptions()

	cmd := &cobra.Command{
		Use:   projectinfo.GetHubName(),
		Short: "Launch " + projectinfo.GetHubName(),
		Long:  "Launch " + projectinfo.GetHubName(),
		Run: func(cmd *cobra.Command, args []string) {
			if yurtHubOptions.Version {
				fmt.Printf("%s: %#v\n", projectinfo.GetHubName(), projectinfo.Get())
				return
			}
			fmt.Printf("%s version: %#v\n", projectinfo.GetHubName(), projectinfo.Get())

			cmd.Flags().VisitAll(func(flag *pflag.Flag) {
				klog.V(1).Infof("FLAG: --%s=%q", flag.Name, flag.Value)
			})
			if err := yurtHubOptions.Validate(); err != nil {
				klog.Fatalf("validate options: %v", err)
			}

			yurtHubCfg, err := config.Complete(yurtHubOptions)
			if err != nil {
				klog.Fatalf("complete %s configuration error, %v", projectinfo.GetHubName(), err)
			}
			klog.Infof("%s cfg: %#+v", projectinfo.GetHubName(), yurtHubCfg)

			if err := Run(ctx, yurtHubCfg); err != nil {
				klog.Fatalf("run %s failed, %v", projectinfo.GetHubName(), err)
			}
		},
	}

	yurtHubOptions.AddFlags(cmd.Flags())
	return cmd
}

// Run runs the YurtHubConfiguration. This should never exit
func Run(ctx context.Context, cfg *config.YurtHubConfiguration) error {
	trace := 1
	klog.Infof("%d. register cert managers", trace)
	certManager, err := hubself.NewYurtHubCertManager(cfg)
	if err != nil {
		return fmt.Errorf("failed to create cert manager for yurthub, %v", err)
	}
	trace++

	certManager.Start()
	err = wait.PollImmediate(5*time.Second, 4*time.Minute, func() (bool, error) {
		curr := certManager.Current()
		if curr != nil {
			return true, nil
		}

		klog.Infof("waiting for preparing client certificate")
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("client certificate preparation failed, %v", err)
	}
	trace++

	klog.Infof("%d. new transport manager", trace)
	transportManager, err := transport.NewTransportManager(certManager, ctx.Done())
	if err != nil {
		return fmt.Errorf("could not new transport manager, %w", err)
	}
	trace++

	klog.Infof("%d. prepare for health checker clients", trace)
	cloudClients, coordinatorClient, err := createClients(cfg.HeartbeatTimeoutSeconds, cfg.RemoteServers, cfg.CoordinatorServer, transportManager)
	if err != nil {
		return fmt.Errorf("failed to create health checker clients, %w", err)
	}
	trace++

	var cloudHealthChecker healthchecker.MultipleBackendsHealthChecker
	var coordinatorHealthChecker healthchecker.HealthChecker
	if cfg.WorkingMode == util.WorkingModeEdge {
		klog.Infof("%d. create health checkers for remote servers and pool coordinator", trace)
		cloudHealthChecker, err = healthchecker.NewCloudAPIServerHealthChecker(cfg, cloudClients, ctx.Done())
		if err != nil {
			return fmt.Errorf("could not new cloud health checker, %w", err)
		}
		coordinatorHealthChecker, err = healthchecker.NewCoordinatorHealthChecker(cfg, coordinatorClient, cloudHealthChecker, ctx.Done())
		if err != nil {
			return fmt.Errorf("failed to create coordinator health checker, %v", err)
		}

	} else {
		klog.Infof("%d. disable health checker for node %s because it is a cloud node", trace, cfg.NodeName)
		// In cloud mode, cloud health checker and pool coordinator health checker are not needed.
		// This fake checker will always report that the cloud is healthy and pool coordinator is unhealthy.
		cloudHealthChecker = healthchecker.NewFakeChecker(true, make(map[string]int))
		coordinatorHealthChecker = healthchecker.NewFakeChecker(false, make(map[string]int))
	}
	trace++

	klog.Infof("%d. new restConfig manager for %s mode", trace, cfg.CertMgrMode)
	restConfigMgr, err := hubrest.NewRestConfigManager(cfg, certManager, cloudHealthChecker)
	if err != nil {
		return fmt.Errorf("could not new restConfig manager, %w", err)
	}
	trace++

	klog.Infof("%d. create tls config for secure servers ", trace)
	cfg.TLSConfig, err = server.GenUseCertMgrAndTLSConfig(
		restConfigMgr, certManager, filepath.Join(cfg.RootDir, "pki"), cfg.NodeName, cfg.CertIPs, ctx.Done())
	if err != nil {
		return fmt.Errorf("could not create tls config, %w", err)
	}
	trace++

	var cacheMgr cachemanager.CacheManager
	if cfg.WorkingMode == util.WorkingModeEdge {
		klog.Infof("%d. new cache manager with storage wrapper and serializer manager", trace)
		cacheMgr = cachemanager.NewCacheManager(cfg.StorageWrapper, cfg.SerializerManager, cfg.RESTMapperManager, cfg.SharedFactory)
	} else {
		klog.Infof("%d. disable cache manager for node %s because it is a cloud node", trace, cfg.NodeName)
	}
	trace++

	if cfg.WorkingMode == util.WorkingModeEdge {
		klog.Infof("%d. new gc manager for node %s, and gc frequency is a random time between %d min and %d min", trace, cfg.NodeName, cfg.GCFrequency, 3*cfg.GCFrequency)
		gcMgr, err := gc.NewGCManager(cfg, restConfigMgr, ctx.Done())
		if err != nil {
			return fmt.Errorf("could not new gc manager, %w", err)
		}
		gcMgr.Run()
	} else {
		klog.Infof("%d. disable gc manager for node %s because it is a cloud node", trace, cfg.NodeName)
	}
	trace++

	klog.Infof("%d. new tenant sa manager", trace)
	tenantMgr := tenant.New(cfg.YurtHubCertOrganizations, cfg.SharedFactory, ctx.Done())
	trace++

	klog.Infof("%d. create yurthub elector", trace)
	elector, err := poolcoordinator.NewHubElector(cfg, coordinatorClient, coordinatorHealthChecker, cloudHealthChecker, ctx.Done())
	if err != nil {
		klog.Errorf("failed to create hub elector, %v", err)
	}
	elector.Run(ctx.Done())
	trace++

	// TODO: cloud client load balance
	klog.Infof("%d. create coordinator", trace)
	coordinator, err := poolcoordinator.NewCoordinator(ctx, cfg, cloudClients[cfg.RemoteServers[0].String()], transportManager, elector)
	if err != nil {
		klog.Errorf("failed to create coordinator, %v", err)
	}
	coordinator.Run()
	trace++

	klog.Infof("%d. new reverse proxy handler for remote servers", trace)
	yurtProxyHandler, err := proxy.NewYurtReverseProxyHandler(cfg, cacheMgr, transportManager, coordinator, cloudHealthChecker, coordinatorHealthChecker, tenantMgr, ctx.Done())
	if err != nil {
		return fmt.Errorf("could not create reverse proxy handler, %w", err)
	}
	trace++

	if cfg.EnableDummyIf {
		klog.Infof("%d. create dummy network interface %s and init iptables manager", trace, cfg.HubAgentDummyIfName)
		networkMgr, err := network.NewNetworkManager(cfg)
		if err != nil {
			return fmt.Errorf("could not create network manager, %w", err)
		}
		networkMgr.Run(ctx.Done())
		trace++
		klog.Infof("%d. new %s server and begin to serve, dummy proxy server: %s, secure dummy proxy server: %s", trace, projectinfo.GetHubName(), cfg.YurtHubProxyServerDummyAddr, cfg.YurtHubProxyServerSecureDummyAddr)
	}

	// start shared informers before start hub server
	cfg.SharedFactory.Start(ctx.Done())
	cfg.YurtSharedFactory.Start(ctx.Done())

	klog.Infof("%d. new %s server and begin to serve, proxy server: %s, secure proxy server: %s, hub server: %s", trace, projectinfo.GetHubName(), cfg.YurtHubProxyServerAddr, cfg.YurtHubProxyServerSecureAddr, cfg.YurtHubServerAddr)
	s, err := server.NewYurtHubServer(cfg, certManager, yurtProxyHandler, restConfigMgr)
	if err != nil {
		return fmt.Errorf("could not create hub server, %w", err)
	}
	s.Run()
	klog.Infof("hub agent exited")
	return nil
}

// createClients will create clients for all cloud APIServer and client for pool coordinator
// It will return a map, mapping cloud APIServer URL to its client, and a pool coordinator client
func createClients(heartbeatTimeoutSeconds int, remoteServers []*url.URL, coordinatorServer *url.URL, tp transport.Interface) (map[string]kubernetes.Interface, kubernetes.Interface, error) {
	var coordinatorClient kubernetes.Interface
	cloudClients := make(map[string]kubernetes.Interface)
	for i := range remoteServers {
		restConf := &rest.Config{
			Host:      remoteServers[i].String(),
			Transport: tp.CurrentTransport(),
			Timeout:   time.Duration(heartbeatTimeoutSeconds) * time.Second,
		}
		c, err := kubernetes.NewForConfig(restConf)
		if err != nil {
			return cloudClients, coordinatorClient, err
		}
		cloudClients[remoteServers[i].String()] = c
	}

	cfg := &rest.Config{
		Host:      coordinatorServer.String(),
		Transport: tp.CurrentTransport(),
		Timeout:   time.Duration(heartbeatTimeoutSeconds) * time.Second,
	}
	c, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return cloudClients, coordinatorClient, err
	}
	coordinatorClient = c

	return cloudClients, coordinatorClient, nil
}
