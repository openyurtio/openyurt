/*
Copyright 2023 The OpenYurt Authors.

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
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/klogr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"

	"github.com/openyurtio/openyurt/cmd/yurt-iot-dock/app/options"
	"github.com/openyurtio/openyurt/pkg/apis"
	edgexclients "github.com/openyurtio/openyurt/pkg/yurtiotdock/clients/edgex-foundry"
	"github.com/openyurtio/openyurt/pkg/yurtiotdock/controllers"
	"github.com/openyurtio/openyurt/pkg/yurtiotdock/controllers/util"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)

	_ = apis.AddToScheme(clientgoscheme.Scheme)
	_ = apis.AddToScheme(scheme)

	// +kubebuilder:scaffold:scheme
}

func NewCmdYurtIoTDock(stopCh <-chan struct{}) *cobra.Command {
	yurtIoTDockOptions := options.NewYurtIoTDockOptions()
	cmd := &cobra.Command{
		Use:   "yurt-iot-dock",
		Short: "Launch yurt-iot-dock",
		Long:  "Launch yurt-iot-dock",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Flags().VisitAll(func(flag *pflag.Flag) {
				klog.V(1).Infof("FLAG: --%s=%q", flag.Name, flag.Value)
			})
			if err := options.ValidateOptions(yurtIoTDockOptions); err != nil {
				klog.Fatalf("validate options: %v", err)
			}
			Run(yurtIoTDockOptions, stopCh)
		},
	}

	yurtIoTDockOptions.AddFlags(cmd.Flags())
	return cmd
}

func Run(opts *options.YurtIoTDockOptions, stopCh <-chan struct{}) {
	ctrl.SetLogger(klogr.New())
	cfg := ctrl.GetConfigOrDie()

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     opts.MetricsAddr,
		HealthProbeBindAddress: opts.ProbeAddr,
		LeaderElection:         opts.EnableLeaderElection,
		LeaderElectionID:       "yurt-iot-dock",
		Namespace:              opts.Namespace,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// perform preflight check
	setupLog.Info("[preflight] Running pre-flight checks")
	if err := preflightCheck(mgr, opts); err != nil {
		setupLog.Error(err, "could not run pre-flight checks")
		os.Exit(1)
	}

	// register the field indexers
	setupLog.Info("[preflight] Registering the field indexers")
	if err := util.RegisterFieldIndexers(mgr.GetFieldIndexer()); err != nil {
		setupLog.Error(err, "could not register field indexers")
		os.Exit(1)
	}
	// get nodepool where yurt-iot-dock run
	if opts.Nodepool == "" {
		opts.Nodepool, err = util.GetNodePool(mgr.GetConfig())
		if err != nil {
			setupLog.Error(err, "could not get the nodepool where yurt-iot-dock run")
			os.Exit(1)
		}
	}

	edgexdock := edgexclients.NewEdgexDock(opts.Version, opts.CoreMetadataAddr, opts.CoreCommandAddr)

	// setup the DeviceProfile Reconciler and Syncer
	if err = (&controllers.DeviceProfileReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr, opts, edgexdock); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DeviceProfile")
		os.Exit(1)
	}
	dfs, err := controllers.NewDeviceProfileSyncer(mgr.GetClient(), opts, edgexdock)
	if err != nil {
		setupLog.Error(err, "unable to create syncer", "syncer", "DeviceProfile")
		os.Exit(1)
	}
	err = mgr.Add(dfs.NewDeviceProfileSyncerRunnable())
	if err != nil {
		setupLog.Error(err, "unable to create syncer runnable", "syncer", "DeviceProfile")
		os.Exit(1)
	}

	// setup the Device Reconciler and Syncer
	if err = (&controllers.DeviceReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr, opts, edgexdock); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Device")
		os.Exit(1)
	}
	ds, err := controllers.NewDeviceSyncer(mgr.GetClient(), opts, edgexdock)
	if err != nil {
		setupLog.Error(err, "unable to create syncer", "controller", "Device")
		os.Exit(1)
	}
	err = mgr.Add(ds.NewDeviceSyncerRunnable())
	if err != nil {
		setupLog.Error(err, "unable to create syncer runnable", "syncer", "Device")
		os.Exit(1)
	}

	// setup the DeviceService Reconciler and Syncer
	if err = (&controllers.DeviceServiceReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr, opts, edgexdock); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DeviceService")
		os.Exit(1)
	}
	dss, err := controllers.NewDeviceServiceSyncer(mgr.GetClient(), opts, edgexdock)
	if err != nil {
		setupLog.Error(err, "unable to create syncer", "syncer", "DeviceService")
		os.Exit(1)
	}
	err = mgr.Add(dss.NewDeviceServiceSyncerRunnable())
	if err != nil {
		setupLog.Error(err, "unable to create syncer runnable", "syncer", "DeviceService")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("health", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("check", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("[run controllers] Starting manager, acting on " + fmt.Sprintf("[NodePool: %s, Namespace: %s]", opts.Nodepool, opts.Namespace))
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "could not running manager")
		os.Exit(1)
	}
}

func deleteCRsOnControllerShutdown(ctx context.Context, cli client.Client, opts *options.YurtIoTDockOptions) error {
	setupLog.Info("[deleteCRsOnControllerShutdown] start delete device crd")
	if err := controllers.DeleteDevicesOnControllerShutdown(ctx, cli, opts); err != nil {
		setupLog.Error(err, "could not shutdown device cr")
		return err
	}

	setupLog.Info("[deleteCRsOnControllerShutdown] start delete deviceprofile crd")
	if err := controllers.DeleteDeviceProfilesOnControllerShutdown(ctx, cli, opts); err != nil {
		setupLog.Error(err, "could not shutdown deviceprofile cr")
		return err
	}

	setupLog.Info("[deleteCRsOnControllerShutdown] start delete deviceservice crd")
	if err := controllers.DeleteDeviceServicesOnControllerShutdown(ctx, cli, opts); err != nil {
		setupLog.Error(err, "could not shutdown deviceservice cr")
		return err
	}

	return nil
}

var onlyOneSignalHandler = make(chan struct{})
var shutdownSignals = []os.Signal{syscall.SIGTERM}

func SetupSignalHandler(client client.Client, opts *options.YurtIoTDockOptions) context.Context {
	close(onlyOneSignalHandler) // panics when called twice

	ctx, cancel := context.WithCancel(context.Background())
	setupLog.Info("[SetupSignalHandler] shutdown controller with crd")
	c := make(chan os.Signal, 2)
	signal.Notify(c, shutdownSignals...)
	go func() {
		<-c
		setupLog.Info("[SetupSignalHandler] shutdown signal concur")
		deleteCRsOnControllerShutdown(ctx, client, opts)
		cancel()
		<-c
		os.Exit(1) // second signal. Exit directly.
	}()

	return ctx
}

func preflightCheck(mgr ctrl.Manager, opts *options.YurtIoTDockOptions) error {
	client, err := kubernetes.NewForConfig(mgr.GetConfig())
	if err != nil {
		return err
	}
	if _, err := client.CoreV1().Namespaces().Get(context.TODO(), opts.Namespace, metav1.GetOptions{}); err != nil {
		return err
	}
	return nil
}
