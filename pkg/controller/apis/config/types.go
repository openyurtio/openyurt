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

package config

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	cmconfig "k8s.io/controller-manager/config"
	nodelifecycleconfig "k8s.io/kube-controller-manager/config/v1alpha1"

	gatewayconfig "github.com/openyurtio/openyurt/pkg/controller/gateway/config"
	nodepoolconfig "github.com/openyurtio/openyurt/pkg/controller/nodepool/config"
	staticpodconfig "github.com/openyurtio/openyurt/pkg/controller/staticpod/config"
	yurtappdaemonconfig "github.com/openyurtio/openyurt/pkg/controller/yurtappdaemon/config"
)

// YurtControllerManagerConfiguration contains elements describing yurt-controller manager.
type YurtControllerManagerConfiguration struct {
	metav1.TypeMeta

	// Generic holds configuration for a generic controller-manager
	Generic cmconfig.GenericControllerManagerConfiguration

	// NodeLifecycleControllerConfiguration holds configuration for
	// NodeLifecycleController related features.
	NodeLifecycleController nodelifecycleconfig.NodeLifecycleControllerConfiguration
}

// YurtManagerConfiguration contains elements describing yurt-manager.
type YurtManagerConfiguration struct {
	metav1.TypeMeta
	Generic GenericConfiguration
	// NodePoolControllerConfiguration holds configuration for NodePoolController related features.
	NodePoolController nodepoolconfig.NodePoolControllerConfiguration

	// GatewayControllerConfiguration holds configuration for GatewayController related features.
	GatewayController gatewayconfig.GatewayControllerConfiguration

	// StaticPodControllerConfiguration holds configuration for  StaticPodController related features.
	StaticPodController staticpodconfig.StaticPodControllerConfiguration

	// YurtAppDaemonControllerConfiguration holds configuration for YurtAppDaemonController related features.
	YurtAppDaemonController yurtappdaemonconfig.YurtAppDaemonControllerConfiguration
}

type GenericConfiguration struct {
	Version                 bool
	MetricsAddr             string
	HealthProbeAddr         string
	EnableLeaderElection    bool
	LeaderElectionNamespace string
	RestConfigQPS           int
	RestConfigBurst         int
	WorkingNamespace        string
}
