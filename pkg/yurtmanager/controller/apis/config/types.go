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
	componentbaseconfig "k8s.io/component-base/config"
	"k8s.io/kube-controller-manager/config/v1alpha1"

	csrapproverconfig "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/csrapprover/config"
	daemonpodupdaterconfig "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/daemonpodupdater/config"
	nodebucketconfig "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/nodebucket/config"
	nodepoolconfig "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/nodepool/config"
	platformadminconfig "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/platformadmin/config"
	poolserviceconfig "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/poolservice/config"
	gatewaypickupconfig "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/raven/gatewaypickup/config"
	yurtappdaemonconfig "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/yurtappdaemon/config"
	yurtappoverriderconfig "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/yurtappoverrider/config"
	yurtappsetconfig "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/yurtappset/config"
	delegateleaseconfig "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/yurtcoordinator/delegatelease/config"
	podbindingconfig "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/yurtcoordinator/podbinding/config"
	yurtstaticsetconfig "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/yurtstaticset/config"
)

// YurtManagerConfiguration contains elements describing yurt-manager.
type YurtManagerConfiguration struct {
	metav1.TypeMeta
	Generic GenericConfiguration

	// DelegateLeaseControllerConfiguration holds configuration for DelegateLeaseController related features.
	DelegateLeaseController delegateleaseconfig.DelegateLeaseControllerConfiguration

	// PodBindingControllerConfiguration holds configuration for PodBindingController related features.
	PodBindingController podbindingconfig.PodBindingControllerConfiguration

	// DaemonPodUpdaterControllerConfiguration holds configuration for DaemonPodUpdaterController related features.
	DaemonPodUpdaterController daemonpodupdaterconfig.DaemonPodUpdaterControllerConfiguration

	// CsrApproverControllerConfiguration holds configuration for CsrApproverController related features.
	CsrApproverController csrapproverconfig.CsrApproverControllerConfiguration

	// NodePoolControllerConfiguration holds configuration for NodePoolController related features.
	NodePoolController nodepoolconfig.NodePoolControllerConfiguration

	// GatewayPickupControllerConfiguration holds configuration for GatewayController related features.
	GatewayPickupController gatewaypickupconfig.GatewayPickupControllerConfiguration

	// YurtAppSetControllerConfiguration holds configuration for YurtAppSetController related features.
	YurtAppSetController yurtappsetconfig.YurtAppSetControllerConfiguration

	// YurtStaticSetControllerConfiguration holds configuration for YurtStaticSetController related features.
	YurtStaticSetController yurtstaticsetconfig.YurtStaticSetControllerConfiguration

	// YurtAppDaemonControllerConfiguration holds configuration for YurtAppDaemonController related features.
	YurtAppDaemonController yurtappdaemonconfig.YurtAppDaemonControllerConfiguration

	// PlatformAdminControllerConfiguration holds configuration for PlatformAdminController related features.
	PlatformAdminController platformadminconfig.PlatformAdminControllerConfiguration

	// YurtAppOverriderControllerConfiguration holds configuration for YurtAppOverriderController related features.
	YurtAppOverriderController yurtappoverriderconfig.YurtAppOverriderControllerConfiguration

	NodeLifeCycleController v1alpha1.NodeLifecycleControllerConfiguration

	//  NodeBucketController holds configuration for NodeBucketController related features.
	NodeBucketController nodebucketconfig.NodeBucketControllerConfiguration

	//  PoolServiceController holds configuration for PoolServiceController related features.
	PoolServiceController poolserviceconfig.PoolServiceControllerConfiguration
}

type GenericConfiguration struct {
	Version          bool
	MetricsAddr      string
	HealthProbeAddr  string
	WebhookPort      int
	LeaderElection   componentbaseconfig.LeaderElectionConfiguration
	RestConfigQPS    int
	RestConfigBurst  int
	WorkingNamespace string
	Kubeconfig       string
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
