/*
Copyright 2020 The OpenYurt Authors.
Copyright 2014 The Kubernetes Authors.

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

// Package options provides the flags used for the controller manager.
//
package options

import (
	"time"

	yurtcontrollerconfig "github.com/alibaba/openyurt/cmd/yurt-controller-manager/app/config"
	v1 "k8s.io/api/core/v1"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	clientset "k8s.io/client-go/kubernetes"
	clientgokubescheme "k8s.io/client-go/kubernetes/scheme"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/record"
	cliflag "k8s.io/component-base/cli/flag"
	kubectrlmgrconfig "k8s.io/kubernetes/pkg/controller/apis/config"
	nodelifecycleconfig "k8s.io/kubernetes/pkg/controller/nodelifecycle/config"

	// add the kubernetes feature gates
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	componentbaseconfig "k8s.io/component-base/config"
	"k8s.io/klog"
)

const (
	// YurtControllerManagerUserAgent is the userAgent name when starting yurt-controller managers.
	YurtControllerManagerUserAgent = "yurt-controller-manager"
)

// YurtControllerManagerOptions is the main context object for the kube-controller manager.
type YurtControllerManagerOptions struct {
	Generic                 *GenericControllerManagerConfigurationOptions
	NodeLifecycleController *NodeLifecycleControllerOptions
	Master                  string
	Kubeconfig              string
}

// NewYurtControllerManagerOptions creates a new YurtControllerManagerOptions with a default config.
func NewYurtControllerManagerOptions() (*YurtControllerManagerOptions, error) {
	generic := kubectrlmgrconfig.GenericControllerManagerConfiguration{
		Address:                 "0.0.0.0",
		Port:                    10266,
		MinResyncPeriod:         metav1.Duration{Duration: 12 * time.Hour},
		ControllerStartInterval: metav1.Duration{Duration: 0 * time.Second},
		Controllers:             []string{"*"},
		ClientConnection: componentbaseconfig.ClientConnectionConfiguration{
			ContentType: "application/vnd.kubernetes.protobuf",
			QPS:         50.0,
			Burst:       100,
		},
		LeaderElection: componentbaseconfig.LeaderElectionConfiguration{
			LeaseDuration: metav1.Duration{Duration: 15 * time.Second},
			RenewDeadline: metav1.Duration{Duration: 10 * time.Second},
			RetryPeriod:   metav1.Duration{Duration: 2 * time.Second},
			ResourceLock:  resourcelock.LeasesResourceLock,
			LeaderElect:   true,
		},
	}

	s := YurtControllerManagerOptions{
		Generic: NewGenericControllerManagerConfigurationOptions(&generic),
		NodeLifecycleController: &NodeLifecycleControllerOptions{
			NodeLifecycleControllerConfiguration: &nodelifecycleconfig.NodeLifecycleControllerConfiguration{
				EnableTaintManager:     true,
				PodEvictionTimeout:     metav1.Duration{Duration: 5 * time.Minute},
				NodeMonitorGracePeriod: metav1.Duration{Duration: 40 * time.Second},
				NodeStartupGracePeriod: metav1.Duration{Duration: 60 * time.Second},
			},
		},
	}

	return &s, nil
}

// Flags returns flags for a specific APIServer by section name
func (s *YurtControllerManagerOptions) Flags(allControllers []string, disabledByDefaultControllers []string) cliflag.NamedFlagSets {
	fss := cliflag.NamedFlagSets{}
	s.Generic.AddFlags(&fss, allControllers, disabledByDefaultControllers)
	s.NodeLifecycleController.AddFlags(fss.FlagSet("nodelifecycle controller"))

	fs := fss.FlagSet("misc")
	fs.StringVar(&s.Master, "master", s.Master, "The address of the Kubernetes API server (overrides any value in kubeconfig).")
	fs.StringVar(&s.Kubeconfig, "kubeconfig", s.Kubeconfig, "Path to kubeconfig file with authorization and master location information.")
	//utilfeature.DefaultMutableFeatureGate.AddFlag(fss.FlagSet("generic"))

	return fss
}

// ApplyTo fills up controller manager config with options.
func (s *YurtControllerManagerOptions) ApplyTo(c *yurtcontrollerconfig.Config) error {
	if err := s.Generic.ApplyTo(&c.ComponentConfig.Generic); err != nil {
		return err
	}

	if err := s.NodeLifecycleController.ApplyTo(&c.ComponentConfig.NodeLifecycleController); err != nil {
		return err
	}

	return nil
}

// Validate is used to validate the options and config before launching the controller manager
func (s *YurtControllerManagerOptions) Validate(allControllers []string, disabledByDefaultControllers []string) error {
	var errs []error

	errs = append(errs, s.Generic.Validate(allControllers, disabledByDefaultControllers)...)
	errs = append(errs, s.NodeLifecycleController.Validate()...)

	// TODO: validate component config, master and kubeconfig

	return utilerrors.NewAggregate(errs)
}

// Config return a controller manager config objective
func (s YurtControllerManagerOptions) Config(allControllers []string, disabledByDefaultControllers []string) (*yurtcontrollerconfig.Config, error) {
	if err := s.Validate(allControllers, disabledByDefaultControllers); err != nil {
		return nil, err
	}

	kubeconfig, err := clientcmd.BuildConfigFromFlags(s.Master, s.Kubeconfig)
	if err != nil {
		return nil, err
	}
	kubeconfig.ContentConfig.ContentType = s.Generic.ClientConnection.ContentType
	kubeconfig.QPS = s.Generic.ClientConnection.QPS
	kubeconfig.Burst = int(s.Generic.ClientConnection.Burst)

	client, err := clientset.NewForConfig(restclient.AddUserAgent(kubeconfig, YurtControllerManagerUserAgent))
	if err != nil {
		return nil, err
	}

	// shallow copy, do not modify the kubeconfig.Timeout.
	config := *kubeconfig
	config.Timeout = s.Generic.LeaderElection.RenewDeadline.Duration
	leaderElectionClient := clientset.NewForConfigOrDie(restclient.AddUserAgent(&config, "leader-election"))

	eventRecorder := createRecorder(client, YurtControllerManagerUserAgent)

	c := &yurtcontrollerconfig.Config{
		Client:               client,
		Kubeconfig:           kubeconfig,
		EventRecorder:        eventRecorder,
		LeaderElectionClient: leaderElectionClient,
	}
	if err := s.ApplyTo(c); err != nil {
		return nil, err
	}

	return c, nil
}

func createRecorder(kubeClient clientset.Interface, userAgent string) record.EventRecorder {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.Infof)
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: kubeClient.CoreV1().Events("")})
	return eventBroadcaster.NewRecorder(clientgokubescheme.Scheme, v1.EventSource{Component: userAgent})
}
