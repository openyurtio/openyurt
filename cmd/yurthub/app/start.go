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
	"k8s.io/client-go/rest"
	"k8s.io/component-base/cli/globalflag"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/config"
	"github.com/openyurtio/openyurt/cmd/yurthub/app/options"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/yurthub/cachemanager"
	"github.com/openyurtio/openyurt/pkg/yurthub/gc"
	"github.com/openyurtio/openyurt/pkg/yurthub/healthchecker"
	"github.com/openyurtio/openyurt/pkg/yurthub/healthchecker/cloudapiserver"
	"github.com/openyurtio/openyurt/pkg/yurthub/healthchecker/leaderhub"
	"github.com/openyurtio/openyurt/pkg/yurthub/locallb"
	"github.com/openyurtio/openyurt/pkg/yurthub/multiplexer"
	"github.com/openyurtio/openyurt/pkg/yurthub/multiplexer/storage"
	"github.com/openyurtio/openyurt/pkg/yurthub/proxy"
	"github.com/openyurtio/openyurt/pkg/yurthub/proxy/remote"
	"github.com/openyurtio/openyurt/pkg/yurthub/server"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage/disk"
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

			yurtHubCfg, err := config.Complete(ctx, yurtHubOptions)
			if err != nil {
				klog.Fatalf("complete %s configuration error, %v", projectinfo.GetHubName(), err)
			}
			klog.Infof("%s cfg: %#+v", projectinfo.GetHubName(), yurtHubCfg)

			util.SetupDumpStackTrap(ctx, yurtHubOptions.RootDir)
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
	klog.Infof("%s works in %s mode", projectinfo.GetHubName(), string(cfg.WorkingMode))

	switch cfg.WorkingMode {
	case util.WorkingModeLocal:
		klog.Infof("new locallb manager for node %s ", cfg.NodeName)
		locallbMgr, err := locallb.NewLocalLBManager(cfg.TenantKasService, cfg.SharedFactory)
		// when local mode yurthub exits, we need to clean configured iptables
		defer locallbMgr.CleanIptables()
		if err != nil {
			return fmt.Errorf("could not new locallb manager, %w", err)
		}
		// Start the informer factory if all informers have been registered
		cfg.SharedFactory.Start(ctx.Done())

	case util.WorkingModeCloud, util.WorkingModeEdge:
		defer cfg.CertManager.Stop()
		trace := 1
		// compare cloud working mode, edge working mode need following preparations:
		// 1. cache manager: used for caching response on local disk.
		// 2. health checker: periodically check the health status of cloud kube-apiserver
		// 3. gc: used for garbaging collect unused cache on local disk.
		var cloudHealthChecker healthchecker.Interface
		var storageWrapper cachemanager.StorageWrapper
		var cacheManager cachemanager.CacheManager
		var err error
		if cfg.WorkingMode == util.WorkingModeEdge {
			klog.Infof("%d. new cache manager with storage wrapper and serializer manager", trace)
			storageManager, err := disk.NewDiskStorage(cfg.DiskCachePath)
			if err != nil {
				klog.Errorf("could not create storage manager, %v", err)
				return err
			}
			storageWrapper = cachemanager.NewStorageWrapper(storageManager)
			cacheManager = cachemanager.NewCacheManager(storageWrapper, cfg.SerializerManager, cfg.RESTMapperManager, cfg.ConfigManager)
			cfg.StorageWrapper = storageWrapper
			trace++

			klog.Infof("%d. create health checkers for remote servers", trace)
			cloudHealthChecker, err = cloudapiserver.NewCloudAPIServerHealthChecker(ctx, cfg)
			if err != nil {
				return fmt.Errorf("could not new health checker for cloud kube-apiserver, %w", err)
			}
			trace++

			klog.Infof("%d. new gc manager for node %s, and gc frequency is a random time between %d min and %d min", trace, cfg.NodeName, cfg.GCFrequency, 3*cfg.GCFrequency)
			gcMgr, err := gc.NewGCManager(cfg, cloudHealthChecker)
			if err != nil {
				return fmt.Errorf("could not new gc manager, %w", err)
			}
			gcMgr.Run(ctx)
			trace++
		}

		// no leader hub servers for transport manager at startup time.
		// and don't filter response of request for pool scope metadata from leader hub.
		transportManagerForLeaderHub, err := transport.NewTransportAndClientManager([]*url.URL{}, 2, cfg.CertManager)
		if err != nil {
			return fmt.Errorf("could not new transport manager for leader hub, %w", err)
		}
		transportManagerForLeaderHub.Start(ctx)

		healthCheckerForLeaderHub := leaderhub.NewLeaderHubHealthChecker(ctx, 20*time.Second, nil)
		loadBalancerForLeaderHub := remote.NewLoadBalancer("round-robin", []*url.URL{}, cacheManager, transportManagerForLeaderHub, healthCheckerForLeaderHub, nil)

		cfg.LoadBalancerForLeaderHub = loadBalancerForLeaderHub
		requestMultiplexerManager := newRequestMultiplexerManager(cfg, healthCheckerForLeaderHub)

		if cfg.NetworkMgr != nil {
			klog.Infof("%d. start network manager for ensuing dummy interface", trace)
			cfg.NetworkMgr.Run(ctx)
			trace++
		}

		// Start the informer factory if all informers have been registered
		cfg.SharedFactory.Start(ctx.Done())
		cfg.DynamicSharedFactory.Start(ctx.Done())

		// Start to prepare proxy handler and start server serving.
		klog.Infof("%d. new reverse proxy handler for forwarding requests", trace)
		yurtProxyHandler, err := proxy.NewYurtReverseProxyHandler(
			cfg,
			cacheManager,
			cloudHealthChecker,
			requestMultiplexerManager)
		if err != nil {
			return fmt.Errorf("could not create reverse proxy handler, %w", err)
		}
		trace++

		klog.Infof("%d. new %s server and begin to serve", trace, projectinfo.GetHubName())
		if err := server.RunYurtHubServers(ctx, cfg, yurtProxyHandler, cloudHealthChecker); err != nil {
			return fmt.Errorf("could not run hub servers, %w", err)
		}
	default:

	}
	<-ctx.Done()
	klog.Info("hub agent exited")
	return nil
}

func newRequestMultiplexerManager(cfg *config.YurtHubConfiguration, healthCheckerForLeaderHub healthchecker.Interface) *multiplexer.MultiplexerManager {
	insecureHubProxyAddress := cfg.YurtHubProxyServerServing.Listener.Addr().String()
	klog.Infof("hub insecure proxy address: %s", insecureHubProxyAddress)
	config := &rest.Config{
		Host:      fmt.Sprintf("http://%s", insecureHubProxyAddress),
		UserAgent: util.MultiplexerProxyClientUserAgentPrefix + cfg.NodeName,
	}
	storageProvider := storage.NewStorageProvider(config)

	return multiplexer.NewRequestMultiplexerManager(cfg, storageProvider, healthCheckerForLeaderHub)
}
