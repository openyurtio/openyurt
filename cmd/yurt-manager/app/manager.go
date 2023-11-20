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
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	cliflag "k8s.io/component-base/cli/flag"
	"k8s.io/component-base/cli/globalflag"
	"k8s.io/component-base/term"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/klogr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"

	"github.com/openyurtio/openyurt/cmd/yurt-manager/app/config"
	"github.com/openyurtio/openyurt/cmd/yurt-manager/app/options"
	"github.com/openyurtio/openyurt/cmd/yurt-manager/names"
	"github.com/openyurtio/openyurt/pkg/apis"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/util/profile"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/webhook"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/webhook/util"
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

const (
	YurtManager = "yurt-manager"
)

// NewYurtManagerCommand creates a *cobra.Command object with default parameters
func NewYurtManagerCommand() *cobra.Command {
	s, err := options.NewYurtManagerOptions()
	if err != nil {
		klog.Fatalf("unable to initialize command options: %v", err)
	}

	cmd := &cobra.Command{
		Use: YurtManager,
		Long: `The yurt manager is a daemon that embeds
the all control loops shipped with openyurt. In applications of robotics and
automation, a control loop is a non-terminating loop that regulates the state of
the system. In openyurt, a controller is a control loop that watches the shared
state of the cluster through the apiserver and makes changes attempting to move the
current state towards the desired state.`,
		PersistentPreRunE: func(*cobra.Command, []string) error {
			// silence client-go warnings.
			// yurt-manager generically watches APIs (including deprecated ones),
			// and CI ensures it works properly against matching kube-apiserver versions.
			rest.SetDefaultWarningHandler(rest.NoWarnings{})
			return nil
		},
		Run: func(cmd *cobra.Command, args []string) {
			// verflag.PrintAndExitIfRequested()
			fmt.Printf("%s version: %#v\n", projectinfo.GetYurtManagerName(), projectinfo.Get())
			if s.Generic.Version {
				return
			}

			PrintFlags(cmd.Flags())

			c, err := s.Config(controller.KnownControllers(), names.YurtManagerControllerAliases())
			if err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				os.Exit(1)
			}

			if err := Run(c.Complete(), wait.NeverStop); err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				os.Exit(1)
			}
		},
		Args: func(cmd *cobra.Command, args []string) error {
			for _, arg := range args {
				if len(arg) > 0 {
					return fmt.Errorf("%q does not take any arguments, got %q", cmd.CommandPath(), args)
				}
			}
			return nil
		},
	}

	fs := cmd.Flags()
	namedFlagSets := s.Flags(controller.KnownControllers(), controller.ControllersDisabledByDefault.List())
	// verflag.AddFlags(namedFlagSets.FlagSet("global"))
	globalflag.AddGlobalFlags(namedFlagSets.FlagSet("global"), cmd.Name())
	for _, f := range namedFlagSets.FlagSets {
		fs.AddFlagSet(f)
	}
	usageFmt := "Usage:\n  %s\n"
	cols, _, _ := term.TerminalSize(cmd.OutOrStdout())
	cmd.SetUsageFunc(func(cmd *cobra.Command) error {
		fmt.Fprintf(cmd.OutOrStderr(), usageFmt, cmd.UseLine())
		cliflag.PrintSections(cmd.OutOrStderr(), namedFlagSets, cols)
		return nil
	})
	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		fmt.Fprintf(cmd.OutOrStdout(), "%s\n\n"+usageFmt, cmd.Long, cmd.UseLine())
		cliflag.PrintSections(cmd.OutOrStdout(), namedFlagSets, cols)
	})

	return cmd
}

// PrintFlags logs the flags in the flagSet
func PrintFlags(flags *pflag.FlagSet) {
	flags.VisitAll(func(flag *pflag.Flag) {
		klog.V(1).Infof("FLAG: --%s=%q", flag.Name, flag.Value)
	})
}

// Run runs the KubeControllerManagerOptions.  This should never exit.
func Run(c *config.CompletedConfig, stopCh <-chan struct{}) error {
	ctrl.SetLogger(klogr.New())
	ctx := ctrl.SetupSignalHandler()
	cfg := ctrl.GetConfigOrDie()
	if len(c.ComponentConfig.Generic.Kubeconfig) != 0 {
		config, err := clientcmd.BuildConfigFromFlags("", c.ComponentConfig.Generic.Kubeconfig)
		if err != nil {
			klog.Infof("could not build rest config, %v", err)
			return err
		}
		cfg = config
	}
	setRestConfig(cfg, c)

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:                     scheme,
		MetricsBindAddress:         c.ComponentConfig.Generic.MetricsAddr,
		HealthProbeBindAddress:     c.ComponentConfig.Generic.HealthProbeAddr,
		LeaderElection:             c.ComponentConfig.Generic.LeaderElection.LeaderElect,
		LeaderElectionID:           c.ComponentConfig.Generic.LeaderElection.ResourceName,
		LeaderElectionNamespace:    c.ComponentConfig.Generic.LeaderElection.ResourceNamespace,
		LeaderElectionResourceLock: c.ComponentConfig.Generic.LeaderElection.ResourceLock,
		Port:                       util.GetWebHookPort(),
		Namespace:                  "",
		Logger:                     setupLog,
		CertDir:                    util.GetCertDir(),
		Host:                       "0.0.0.0",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	setupLog.Info("setup controllers")
	if err = controller.SetupWithManager(ctx, c, mgr); err != nil {
		setupLog.Error(err, "unable to setup controllers")
		os.Exit(1)
	}

	setupLog.Info("setup webhook")
	if err = webhook.SetupWithManager(c, mgr); err != nil {
		setupLog.Error(err, "unable to setup webhook")
		os.Exit(1)
	}

	if len(webhook.WebhookHandlerPath) != 0 {
		// +kubebuilder:scaffold:builder
		setupLog.Info("initialize webhook")
		if err := webhook.Initialize(ctx, c, mgr.GetConfig()); err != nil {
			setupLog.Error(err, "unable to initialize webhook")
			os.Exit(1)
		}

		if err := mgr.AddReadyzCheck("webhook-ready", mgr.GetWebhookServer().StartedChecker()); err != nil {
			setupLog.Error(err, "unable to add readyz check")
			os.Exit(1)
		}
	} else {
		klog.Infof("no webhook is registered, so skip webhook setup")
	}

	if err := mgr.AddHealthzCheck("health", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("check", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	for path, handler := range profile.GetPprofHandlers() {
		if err := mgr.AddMetricsExtraHandler(path, handler); err != nil {
			setupLog.Error(err, "unable to add pprof handler")
			os.Exit(1)
		}
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}

	return nil
}

func setRestConfig(c *rest.Config, config *config.CompletedConfig) {
	if config.ComponentConfig.Generic.RestConfigQPS > 0 {
		c.QPS = float32(config.ComponentConfig.Generic.RestConfigQPS)
	}
	if config.ComponentConfig.Generic.RestConfigBurst > 0 {
		c.Burst = config.ComponentConfig.Generic.RestConfigBurst
	}

	c.UserAgent = YurtManager
}
