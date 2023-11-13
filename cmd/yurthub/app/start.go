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
	"net"
	"net/url"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/config"
	"github.com/openyurtio/openyurt/cmd/yurthub/app/options"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/yurthub/cachemanager"
	"github.com/openyurtio/openyurt/pkg/yurthub/gc"
	"github.com/openyurtio/openyurt/pkg/yurthub/healthchecker"
	hubrest "github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/rest"
	"github.com/openyurtio/openyurt/pkg/yurthub/proxy"
	"github.com/openyurtio/openyurt/pkg/yurthub/server"
	"github.com/openyurtio/openyurt/pkg/yurthub/tenant"
	"github.com/openyurtio/openyurt/pkg/yurthub/transport"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
	"github.com/openyurtio/openyurt/pkg/yurthub/yurtcoordinator"
	coordinatorcertmgr "github.com/openyurtio/openyurt/pkg/yurthub/yurtcoordinator/certmanager"
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

			util.SetupDumpStackTrap(yurtHubOptions.RootDir, ctx.Done())
			klog.Infof("start watch SIGUSR1 signal")

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
		klog.Infof("%d. create health checkers for remote servers and yurt coordinator", trace)
		cloudHealthChecker, err = healthchecker.NewCloudAPIServerHealthChecker(cfg, cloudClients, ctx.Done())
		if err != nil {
			return fmt.Errorf("could not new cloud health checker, %w", err)
		}
	} else {
		klog.Infof("%d. disable health checker for node %s because it is a cloud node", trace, cfg.NodeName)
		// In cloud mode, cloud health checker is not needed.
		// This fake checker will always report that the cloud is healthy and yurt coordinator is unhealthy.
		cloudHealthChecker = healthchecker.NewFakeChecker(true, make(map[string]int))
	}
	trace++

	klog.Infof("%d. new restConfig manager", trace)
	restConfigMgr, err := hubrest.NewRestConfigManager(cfg.CertManager, cloudHealthChecker)
	if err != nil {
		return fmt.Errorf("could not new restConfig manager, %w", err)
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
	tenantMgr := tenant.New(cfg.TenantNs, cfg.SharedFactory, ctx.Done())
	trace++

	var coordinatorHealthCheckerGetter func() healthchecker.HealthChecker = getFakeCoordinatorHealthChecker
	var coordinatorTransportManagerGetter func() transport.Interface = getFakeCoordinatorTransportManager
	var coordinatorGetter func() yurtcoordinator.Coordinator = getFakeCoordinator
	var coordinatorServerURLGetter func() *url.URL = getFakeCoordinatorServerURL

	if cfg.EnableCoordinator {
		klog.Infof("%d. start to run coordinator", trace)
		trace++

		coordinatorInformerRegistryChan := make(chan struct{})
		// coordinatorRun will register secret informer into sharedInformerFactory, and start a new goroutine to periodically check
		// if certs has been got from cloud APIServer. It will close the coordinatorInformerRegistryChan if the secret channel has
		// been registered into informer factory.
		coordinatorHealthCheckerGetter, coordinatorTransportManagerGetter, coordinatorGetter, coordinatorServerURLGetter =
			coordinatorRun(ctx, cfg, restConfigMgr, cloudHealthChecker, coordinatorInformerRegistryChan)
		// wait for coordinator informer registry
		klog.Info("waiting for coordinator informer registry")
		<-coordinatorInformerRegistryChan
		klog.Info("coordinator informer registry finished")
	}

	// Start the informer factory if all informers have been registered
	cfg.SharedFactory.Start(ctx.Done())
	cfg.NodePoolInformerFactory.Start(ctx.Done())

	klog.Infof("%d. new reverse proxy handler for remote servers", trace)
	yurtProxyHandler, err := proxy.NewYurtReverseProxyHandler(
		cfg,
		cacheMgr,
		transportManager,
		cloudHealthChecker,
		tenantMgr,
		coordinatorGetter,
		coordinatorTransportManagerGetter,
		coordinatorHealthCheckerGetter,
		coordinatorServerURLGetter,
		ctx.Done())
	if err != nil {
		return fmt.Errorf("could not create reverse proxy handler, %w", err)
	}
	trace++

	if cfg.NetworkMgr != nil {
		cfg.NetworkMgr.Run(ctx.Done())
	}

	klog.Infof("%d. new %s server and begin to serve", trace, projectinfo.GetHubName())
	if err := server.RunYurtHubServers(cfg, yurtProxyHandler, restConfigMgr, ctx.Done()); err != nil {
		return fmt.Errorf("could not run hub servers, %w", err)
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

// coordinatorRun will initialize and start all coordinator-related components in an async way.
// It returns Getter function for coordinator, coordinator health checker, coordinator transport manager and coordinator service url,
// which will return the relative component if it has been initialized, otherwise it will return nil.
func coordinatorRun(ctx context.Context,
	cfg *config.YurtHubConfiguration,
	restConfigMgr *hubrest.RestConfigManager,
	cloudHealthChecker healthchecker.MultipleBackendsHealthChecker,
	coordinatorInformerRegistryChan chan struct{}) (
	func() healthchecker.HealthChecker,
	func() transport.Interface,
	func() yurtcoordinator.Coordinator,
	func() *url.URL) {

	var coordinatorHealthChecker healthchecker.HealthChecker
	var coordinatorTransportMgr transport.Interface
	var coordinator yurtcoordinator.Coordinator
	var coordinatorServiceUrl *url.URL

	go func() {
		coorCertManager, err := coordinatorcertmgr.NewCertManager(cfg.CoordinatorPKIDir, cfg.YurtHubNamespace, cfg.ProxiedClient, cfg.SharedFactory)
		close(coordinatorInformerRegistryChan) // notify the coordinator secret informer registry event
		if err != nil {
			klog.Errorf("coordinator could not create coordinator cert manager, %v", err)
			return
		}
		klog.Info("coordinator new certManager success")

		// waiting for service sync complete
		if !cache.WaitForCacheSync(ctx.Done(), cfg.SharedFactory.Core().V1().Services().Informer().HasSynced) {
			klog.Error("coordinatorRun sync service shutdown")
			return
		}
		klog.Info("coordinatorRun sync service complete")

		// resolve yurt-coordinator-apiserver and etcd from domain to ips
		serviceList := cfg.SharedFactory.Core().V1().Services().Lister()
		// if yurt-coordinator-apiserver and yurt-coordinator-etcd address is ip, don't need to resolve
		apiServerIP := net.ParseIP(cfg.CoordinatorServerURL.Hostname())
		etcdUrl, err := url.Parse(cfg.CoordinatorStorageAddr)
		if err != nil {
			klog.Errorf("coordinator parse etcd address failed: %+v", err)
			return
		}
		etcdIP := net.ParseIP(etcdUrl.Hostname())
		if apiServerIP == nil {
			apiServerService, err := serviceList.Services(util.YurtHubNamespace).Get(cfg.CoordinatorServerURL.Hostname())
			if err != nil {
				klog.Errorf("coordinator could not get apiServer service, %v", err)
				return
			}
			// rewrite coordinator service info for cfg
			coordinatorServerURL, err :=
				url.Parse(fmt.Sprintf("https://%s:%s", apiServerService.Spec.ClusterIP, cfg.CoordinatorServerURL.Port()))
			if err != nil {
				klog.Errorf("coordinator could not parse apiServer service, %v", err)
				return
			}
			cfg.CoordinatorServerURL = coordinatorServerURL
		}
		if etcdIP == nil {
			etcdService, err := serviceList.Services(util.YurtHubNamespace).Get(etcdUrl.Hostname())
			if err != nil {
				klog.Errorf("coordinator could not get etcd service, %v", err)
				return
			}
			cfg.CoordinatorStorageAddr = fmt.Sprintf("https://%s:%s", etcdService.Spec.ClusterIP, etcdUrl.Port())
		}

		coorTransportMgr, err := yurtCoordinatorTransportMgrGetter(coorCertManager, ctx.Done())
		if err != nil {
			klog.Errorf("coordinator could not create coordinator transport manager, %v", err)
			return
		}

		coordinatorClient, err := kubernetes.NewForConfig(&rest.Config{
			Host:      cfg.CoordinatorServerURL.String(),
			Transport: coorTransportMgr.CurrentTransport(),
			Timeout:   time.Duration(cfg.HeartbeatTimeoutSeconds) * time.Second,
		})
		if err != nil {
			klog.Errorf("coordinator could not get coordinator client for yurt coordinator, %v", err)
			return
		}

		coorHealthChecker, err := healthchecker.NewCoordinatorHealthChecker(cfg, coordinatorClient, cloudHealthChecker, ctx.Done())
		if err != nil {
			klog.Errorf("coordinator could not create coordinator health checker, %v", err)
			return
		}

		var elector *yurtcoordinator.HubElector
		elector, err = yurtcoordinator.NewHubElector(cfg, coordinatorClient, coorHealthChecker, cloudHealthChecker, ctx.Done())
		if err != nil {
			klog.Errorf("coordinator could not create hub elector, %v", err)
			return
		}
		go elector.Run(ctx.Done())

		coor, err := yurtcoordinator.NewCoordinator(ctx, cfg, cloudHealthChecker, restConfigMgr, coorCertManager, coorTransportMgr, elector)
		if err != nil {
			klog.Errorf("coordinator could not create coordinator, %v", err)
			return
		}
		go coor.Run()

		coordinatorTransportMgr = coorTransportMgr
		coordinatorHealthChecker = coorHealthChecker
		coordinator = coor
		coordinatorServiceUrl = cfg.CoordinatorServerURL
	}()

	return func() healthchecker.HealthChecker {
			return coordinatorHealthChecker
		}, func() transport.Interface {
			return coordinatorTransportMgr
		}, func() yurtcoordinator.Coordinator {
			return coordinator
		}, func() *url.URL {
			return coordinatorServiceUrl
		}
}

func yurtCoordinatorTransportMgrGetter(coordinatorCertMgr *coordinatorcertmgr.CertManager, stopCh <-chan struct{}) (transport.Interface, error) {
	err := wait.PollImmediate(5*time.Second, 4*time.Minute, func() (done bool, err error) {
		klog.Info("waiting for preparing certificates for coordinator client and node lease proxy client")
		if coordinatorCertMgr.GetAPIServerClientCert() == nil {
			return false, nil
		}
		if coordinatorCertMgr.GetNodeLeaseProxyClientCert() == nil {
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		klog.Errorf("timeout when waiting for coordinator client certificate")
	}

	coordinatorTransportMgr, err := transport.NewTransportManager(coordinatorCertMgr, stopCh)
	if err != nil {
		return nil, fmt.Errorf("could not create transport manager for yurt coordinator, %v", err)
	}
	return coordinatorTransportMgr, nil
}

func getFakeCoordinator() yurtcoordinator.Coordinator {
	return &yurtcoordinator.FakeCoordinator{}
}

func getFakeCoordinatorHealthChecker() healthchecker.HealthChecker {
	return healthchecker.NewFakeChecker(false, make(map[string]int))
}

func getFakeCoordinatorTransportManager() transport.Interface {
	return nil
}

func getFakeCoordinatorServerURL() *url.URL {
	return nil
}
