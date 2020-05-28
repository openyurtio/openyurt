package app

import (
	"github.com/alibaba/openyurt/cmd/yurthub/app/config"
	"github.com/alibaba/openyurt/cmd/yurthub/app/options"
	"github.com/alibaba/openyurt/pkg/yurthub/cachemanager"
	"github.com/alibaba/openyurt/pkg/yurthub/certificate"
	"github.com/alibaba/openyurt/pkg/yurthub/certificate/initializer"
	"github.com/alibaba/openyurt/pkg/yurthub/certificate/kubelet"
	"github.com/alibaba/openyurt/pkg/yurthub/gc"
	"github.com/alibaba/openyurt/pkg/yurthub/healthchecker"
	"github.com/alibaba/openyurt/pkg/yurthub/kubernetes/serializer"
	"github.com/alibaba/openyurt/pkg/yurthub/proxy"
	"github.com/alibaba/openyurt/pkg/yurthub/server"
	"github.com/alibaba/openyurt/pkg/yurthub/storage/factory"
	"github.com/alibaba/openyurt/pkg/yurthub/transport"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/klog"
)

const (
	// yurthub component name
	componentYurtHub = "yurthub"
)

// NewCmdStartYurtHub creates a *cobra.Command object with default parameters
func NewCmdStartYurtHub(stopCh <-chan struct{}) *cobra.Command {
	yurtHubOptions := options.NewYurtHubOptions()

	cmd := &cobra.Command{
		Use:   componentYurtHub,
		Short: "Launch yurthub",
		Long:  "Launch yurthub",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Flags().VisitAll(func(flag *pflag.Flag) {
				klog.V(1).Infof("FLAG: --%s=%q", flag.Name, flag.Value)
			})
			if err := options.ValidateOptions(yurtHubOptions); err != nil {
				klog.Fatalf("validate options: %v", err)
			}

			yurtHubCfg, err := config.Complete(yurtHubOptions)
			if err != nil {
				klog.Fatalf("complete yurthub configuration error, %v", err)
			}
			klog.Infof("yurthub cfg: %#+v", yurtHubCfg)

			if err := Run(yurtHubCfg, stopCh); err != nil {
				klog.Fatalf("run yurthub failed, %v", err)
			}
		},
	}

	yurtHubOptions.AddFlags(cmd.Flags())
	return cmd
}

// Run runs the YurtHubConfiguration. This should never exit
func Run(cfg *config.YurtHubConfiguration, stopCh <-chan struct{}) error {
	trace := 1
	klog.Infof("%d. new transport manager for healthz client", trace)
	transportManager, err := transport.NewTransportManager(cfg.HeartbeatTimeoutSeconds, stopCh)
	if err != nil {
		klog.Errorf("could not new transport manager, %v", err)
		return err
	}
	trace++

	klog.Infof("%d. create health checker for remote servers ", trace)
	healthChecker, err := healthchecker.NewHealthChecker(cfg.RemoteServers, transportManager, cfg.HeartbeatFailedRetry, cfg.HeartbeatHealthyThreshold, stopCh)
	if err != nil {
		klog.Errorf("could not new health checker, %v", err)
		return err
	}
	trace++

	klog.Infof("%d. init cert initializer", trace)
	cmInitializer := initializer.NewCMInitializer(healthChecker)
	trace++

	klog.Infof("%d. register cert managers", trace)
	cmr := certificate.NewCertificateManagerRegistry()
	kubelet.Register(cmr)
	trace++

	klog.Infof("%d. create cert manager with %s mode", trace, cfg.CertMgrMode)
	certManager, err := cmr.New(cfg.CertMgrMode, cfg, cmInitializer)
	if err != nil {
		klog.Errorf("could not create certificate manager, %v", err)
		return err
	}
	trace++

	klog.Infof("%d. update transport manager", trace)
	err = transportManager.UpdateTransport(certManager)
	if err != nil {
		klog.Errorf("could not update transport manager, %v", err)
		return err
	}
	trace++

	klog.Infof("%d. create storage manager", trace)
	storageManager, err := factory.CreateStorage()
	if err != nil {
		klog.Errorf("could not create storage manager, %v", err)
		return err
	}
	storageWrapper := cachemanager.NewStorageWrapper(storageManager)
	trace++

	klog.Infof("%d. new serializer manager", trace)
	serializerManager := serializer.NewSerializerManager()
	trace++

	klog.Infof("%d. new yurt cache manager with storage wrapper and serializer manager", trace)
	cacheMgr, err := cachemanager.NewCacheManager(storageWrapper, serializerManager)
	if err != nil {
		klog.Errorf("could not new cache manager, %v", err)
		return err
	}
	trace++

	klog.Infof("%d. new gc manager for node %s, and gc frequency is a random time between %d min and %d min", trace, cfg.NodeName, cfg.GCFrequency, 3*cfg.GCFrequency)
	gcMgr, err := gc.NewGCManager(cfg, storageManager, transportManager, stopCh)
	if err != nil {
		klog.Errorf("could not new gc manager, %v", err)
		return err
	}
	gcMgr.Run()
	trace++

	klog.Infof("%d. new yurt reverse proxy handler for remote servers", trace)
	yurtProxyHandler, err := proxy.NewYurtReverseProxyHandler(cfg, cacheMgr, transportManager, healthChecker, certManager, stopCh)
	if err != nil {
		klog.Errorf("could not create yurt reverse proxy handler, %v", err)
		return err
	}
	trace++

	klog.Infof("%d. new yurthub server and begin to serve", trace)
	s := server.NewYurtHubServer(cfg, certManager, yurtProxyHandler)
	s.Run()
	<-stopCh
	return nil
}
