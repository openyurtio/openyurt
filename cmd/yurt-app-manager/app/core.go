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
	"flag"
	"fmt"
	"net/http"
	"os"

	appsv1alpha1 "github.com/alibaba/openyurt/pkg/yurtappmanager/apis/apps/v1alpha1"
	extclient "github.com/alibaba/openyurt/pkg/yurtappmanager/client"
	"github.com/alibaba/openyurt/pkg/yurtappmanager/constant"
	"github.com/alibaba/openyurt/pkg/yurtappmanager/controller"
	"github.com/alibaba/openyurt/pkg/yurtappmanager/util/fieldindex"
	"github.com/alibaba/openyurt/pkg/yurtappmanager/webhook"
	"k8s.io/apimachinery/pkg/runtime"

	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/alibaba/openyurt/cmd/yurt-app-manager/options"
	"github.com/alibaba/openyurt/pkg/projectinfo"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/klog"
	"k8s.io/klog/klogr"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")

	restConfigQPS   = flag.Int("rest-config-qps", 30, "QPS of rest config.")
	restConfigBurst = flag.Int("rest-config-burst", 50, "Burst of rest config.")
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = appsv1alpha1.AddToScheme(clientgoscheme.Scheme)

	_ = appsv1alpha1.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

// NewCmdYurtAppManager creates a *cobra.Command object with default parameters
func NewCmdYurtAppManager(stopCh <-chan struct{}) *cobra.Command {
	yurtAppOptions := options.NewYurtAppOptions()

	cmd := &cobra.Command{
		Use:   projectinfo.GetYurtAppManagerName(),
		Short: "Launch " + projectinfo.GetYurtAppManagerName(),
		Long:  "Launch " + projectinfo.GetYurtAppManagerName(),
		Run: func(cmd *cobra.Command, args []string) {
			if yurtAppOptions.Version {
				fmt.Printf("%s: %#v\n", projectinfo.GetYurtAppManagerName(), projectinfo.Get())
				return
			}

			fmt.Printf("%s version: %#v\n", projectinfo.GetYurtAppManagerName(), projectinfo.Get())

			cmd.Flags().VisitAll(func(flag *pflag.Flag) {
				klog.V(1).Infof("FLAG: --%s=%q", flag.Name, flag.Value)
			})
			if err := options.ValidateOptions(yurtAppOptions); err != nil {
				klog.Fatalf("validate options: %v", err)
			}

			Run(yurtAppOptions)
		},
	}

	yurtAppOptions.AddFlags(cmd.Flags())
	return cmd
}

func Run(opts *options.YurtAppOptions) {
	if opts.EnablePprof {
		go func() {
			if err := http.ListenAndServe(opts.PprofAddr, nil); err != nil {
				setupLog.Error(err, "unable to start pprof")
			}
		}()
	}

	//ctrl.SetLogger(zap.New(zap.UseDevMode(true)))
	ctrl.SetLogger(klogr.New())

	cfg := ctrl.GetConfigOrDie()
	setRestConfig(cfg)

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                  scheme,
		MetricsBindAddress:      opts.MetricsAddr,
		HealthProbeBindAddress:  opts.HealthProbeAddr,
		LeaderElection:          opts.EnableLeaderElection,
		LeaderElectionID:        "yurt-app-manager",
		LeaderElectionNamespace: opts.LeaderElectionNamespace,
		Namespace:               opts.Namespace,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	setupLog.Info("register field index")
	if err := fieldindex.RegisterFieldIndexes(mgr.GetCache()); err != nil {
		setupLog.Error(err, "failed to register field index")
		os.Exit(1)
	}

	setupLog.Info("new clientset registry")
	err = extclient.NewRegistry(mgr)
	if err != nil {
		setupLog.Error(err, "unable to init yurtapp clientset and informer")
		os.Exit(1)
	}

	setupLog.Info("setup controllers")

	ctx := genOptCtx(opts.CreateDefaultPool)
	if err = controller.SetupWithManager(mgr, ctx); err != nil {
		setupLog.Error(err, "unable to setup controllers")
		os.Exit(1)
	}

	setupLog.Info("setup webhook")
	if err = webhook.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to setup webhook")
		os.Exit(1)
	}

	// +kubebuilder:scaffold:builder

	stopCh := ctrl.SetupSignalHandler()
	setupLog.Info("initialize webhook")
	if err := webhook.Initialize(mgr, stopCh); err != nil {
		setupLog.Error(err, "unable to initialize webhook")
		os.Exit(1)
	}

	if err := mgr.AddReadyzCheck("webhook-ready", webhook.Checker); err != nil {
		setupLog.Error(err, "unable to add readyz check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(stopCh); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}

}

func genOptCtx(createDefaultPool bool) context.Context {
	return context.WithValue(context.Background(),
		constant.ContextKeyCreateDefaultPool, createDefaultPool)
}

func setRestConfig(c *rest.Config) {
	if *restConfigQPS > 0 {
		c.QPS = float32(*restConfigQPS)
	}
	if *restConfigBurst > 0 {
		c.Burst = *restConfigBurst
	}
}
