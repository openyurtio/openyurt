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
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/component-base/cli/globalflag"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/config"
	"github.com/openyurtio/openyurt/cmd/yurthub/app/options"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/yurthub/cachemanager"
	"github.com/openyurtio/openyurt/pkg/yurthub/gc"
	"github.com/openyurtio/openyurt/pkg/yurthub/healthchecker"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/directclient"
	"github.com/openyurtio/openyurt/pkg/yurthub/locallb"
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
			projectinfo.RegisterVersionInfo(nil, projectinfo.GetHubName())

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

			util.SetupDumpStackTrap(yurtHubOptions.RootDir, ctx.Done())
			klog.Infof("start watch SIGUSR1 signal")

			if err := Run(ctx, yurtHubCfg); err != nil {
				klog.Fatalf("run %s failed, %v", projectinfo.GetHubName(), err)
			}
		},
	}

	globalflag.AddGlobalFlags(cmd.Flags(), cmd.Name())
	yurtHubOptions.AddFlags(cmd.Flags())
	return cmd
}

// Run runs the YurtHubConfiguration. This should never exit
func Run(ctx context.Context, cfg *config.YurtHubConfiguration) error {
	if cfg.WorkingMode != util.WorkingModeLocal {
		defer cfg.CertManager.Stop()
		trace := 1
		klog.Infof("%d. new transport manager", trace)
		transportManager, err := transport.NewTransportManager(cfg.CertManager, ctx.Done())
		if err != nil {
			return fmt.Errorf("could not new transport manager, %w", err)
		}
		trace++

		klog.Infof("%d. prepare cloud kube clients", trace)
		cloudClients, err := createClients(cfg.HeartbeatTimeoutSeconds, cfg.RemoteServers, transportManager)
		if err != nil {
			return fmt.Errorf("could not create cloud clients, %w", err)
		}
		trace++

		var cloudHealthChecker healthchecker.MultipleBackendsHealthChecker
		if cfg.WorkingMode == util.WorkingModeEdge {
			klog.Infof("%d. create health checkers for remote servers", trace)
			cloudHealthChecker, err = healthchecker.NewCloudAPIServerHealthChecker(cfg, cloudClients, ctx.Done())
			if err != nil {
				return fmt.Errorf("could not new cloud health checker, %w", err)
			}
		} else {
			klog.Infof("%d. disable health checker for node %s because it is a cloud node", trace, cfg.NodeName)
			// In cloud mode, cloud health checker is not needed.
			// This fake checker will always report that the cloud is healthy is unhealthy.
			cloudHealthChecker = healthchecker.NewFakeChecker(true, make(map[string]int))
		}
		trace++

		klog.Infof("%d. new direct client manager", trace)
		directClientManager, err := directclient.NewRestClientManager(cfg.RemoteServers, transportManager, cloudHealthChecker)
		if err != nil {
			return fmt.Errorf("could not new restConfig manager, %w", err)
		}
		trace++

		var cacheMgr cachemanager.CacheManager
		if cfg.WorkingMode == util.WorkingModeEdge {
			klog.Infof("%d. new cache manager with storage wrapper and serializer manager", trace)
			cacheMgr = cachemanager.NewCacheManager(cfg.StorageWrapper, cfg.SerializerManager, cfg.RESTMapperManager, cfg.ConfigManager)
		} else {
			klog.Infof("%d. disable cache manager for node %s because it is a cloud node", trace, cfg.NodeName)
		}
		trace++

		if cfg.WorkingMode == util.WorkingModeEdge {
			klog.Infof("%d. new gc manager for node %s, and gc frequency is a random time between %d min and %d min", trace, cfg.NodeName, cfg.GCFrequency, 3*cfg.GCFrequency)
			gcMgr, err := gc.NewGCManager(cfg, directClientManager, ctx.Done())
			if err != nil {
				return fmt.Errorf("could not new gc manager, %w", err)
			}
			gcMgr.Run()
		} else {
			klog.Infof("%d. disable gc manager for node %s because it is a cloud node", trace, cfg.NodeName)
		}
		trace++

		klog.Infof("%d. new tenant sa manager", trace)
		tenantMgr := tenant.New(cfg.TenantNs, cfg.SharedFactory, ctx.Done())
		trace++

		// Start the informer factory if all informers have been registered
		cfg.SharedFactory.Start(ctx.Done())
		cfg.NodePoolInformerFactory.Start(ctx.Done())

		klog.Infof("%d. new reverse proxy handler for remote servers", trace)
		yurtProxyHandler, err := proxy.NewYurtReverseProxyHandler(
			cfg,
			cacheMgr,
			directClientManager,
			transportManager,
			cloudHealthChecker,
			tenantMgr,
			ctx.Done())
		if err != nil {
			return fmt.Errorf("could not create reverse proxy handler, %w", err)
		}
		trace++

		if cfg.NetworkMgr != nil {
			cfg.NetworkMgr.Run(ctx.Done())
		}

		klog.Infof("%d. new %s server and begin to serve", trace, projectinfo.GetHubName())
		if err := server.RunYurtHubServers(cfg, yurtProxyHandler, directClientManager, ctx.Done()); err != nil {
			return fmt.Errorf("could not run hub servers, %w", err)
		}
	} else {
		klog.Infof("new locallb manager for node %s ", cfg.NodeName)
		locallbMgr, err := locallb.NewLocalLBManager(cfg, cfg.SharedFactory)
		// when local mode yurthub exits, we need to clean configured iptables
		defer locallbMgr.CleanIptables()
		if err != nil {
			return fmt.Errorf("could not new locallb manager, %w", err)
		}
		// Start the informer factory if all informers have been registered
		cfg.SharedFactory.Start(ctx.Done())
	}
	<-ctx.Done()
	klog.Info("hub agent exited")
	return nil
}

// createClients will create clients for all cloud APIServer
// It will return a map, mapping cloud APIServer URL to its client
func createClients(heartbeatTimeoutSeconds int, remoteServers []*url.URL, tp transport.Interface) (map[string]kubernetes.Interface, error) {
	cloudClients := make(map[string]kubernetes.Interface)
	for i := range remoteServers {
		restConf := &rest.Config{
			Host:      remoteServers[i].String(),
			Transport: tp.CurrentTransport(),
			Timeout:   time.Duration(heartbeatTimeoutSeconds) * time.Second,
		}
		c, err := kubernetes.NewForConfig(restConf)
		if err != nil {
			return cloudClients, err
		}
		cloudClients[remoteServers[i].String()] = c
	}
	return cloudClients, nil
}
