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

	iotconfig "github.com/openyurtio/openyurt/pkg/controller/iot/config"
	nodepoolconfig "github.com/openyurtio/openyurt/pkg/controller/nodepool/config"
	gatewayconfig "github.com/openyurtio/openyurt/pkg/controller/raven/config"
	yurtappdaemonconfig "github.com/openyurtio/openyurt/pkg/controller/yurtappdaemon/config"
	yurtappsetconfig "github.com/openyurtio/openyurt/pkg/controller/yurtappset/config"
	yurtstaticsetconfig "github.com/openyurtio/openyurt/pkg/controller/yurtstaticset/config"
)

// YurtManagerConfiguration contains elements describing yurt-manager.
type YurtManagerConfiguration struct {
	metav1.TypeMeta
	Generic GenericConfiguration
	// NodePoolControllerConfiguration holds configuration for NodePoolController related features.
	NodePoolController nodepoolconfig.NodePoolControllerConfiguration

	// GatewayControllerConfiguration holds configuration for GatewayController related features.
	GatewayController gatewayconfig.GatewayControllerConfiguration

	// YurtAppSetControllerConfiguration holds configuration for YurtAppSetController related features.
	YurtAppSetController yurtappsetconfig.YurtAppSetControllerConfiguration

	// YurtStaticSetControllerConfiguration holds configuration for YurtStaticSetController related features.
	YurtStaticSetController yurtstaticsetconfig.YurtStaticSetControllerConfiguration

	// YurtAppDaemonControllerConfiguration holds configuration for YurtAppDaemonController related features.
	YurtAppDaemonController yurtappdaemonconfig.YurtAppDaemonControllerConfiguration

	// IoTControllerConfiguration holds configuration for IoTController related features.
	IoTController iotconfig.IoTControllerConfiguration
}

type GenericConfiguration struct {
	Version                 bool
	MetricsAddr             string
	HealthProbeAddr         string
	WebhookPort             int
	EnableLeaderElection    bool
	LeaderElectionNamespace string
	RestConfigQPS           int
	RestConfigBurst         int
	WorkingNamespace        string
	// Controllers is the list of controllers to enable or disable
	// '*' means "all enabled by default controllers"
	// 'foo' means "enable 'foo'"
	// '-foo' means "disable 'foo'"
	// first item for a particular name wins
	Controllers []string
	// DisabledWebhooks is used to specify the disabled webhooks
	// Only care about controller-independent webhooks
	DisabledWebhooks []string
}
