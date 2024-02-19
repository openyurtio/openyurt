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

package webhook

import (
	"context"
	"fmt"
	"time"

	"k8s.io/client-go/rest"
	"k8s.io/controller-manager/app"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/openyurtio/openyurt/cmd/yurt-manager/app/config"
	"github.com/openyurtio/openyurt/cmd/yurt-manager/names"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller"
	v1alpha1deploymentrender "github.com/openyurtio/openyurt/pkg/yurtmanager/webhook/deploymentrender/v1alpha1"
	v1beta1gateway "github.com/openyurtio/openyurt/pkg/yurtmanager/webhook/gateway/v1beta1"
	v1node "github.com/openyurtio/openyurt/pkg/yurtmanager/webhook/node/v1"
	v1beta1nodepool "github.com/openyurtio/openyurt/pkg/yurtmanager/webhook/nodepool/v1beta1"
	v1alpha1platformadmin "github.com/openyurtio/openyurt/pkg/yurtmanager/webhook/platformadmin/v1alpha1"
	v1alpha2platformadmin "github.com/openyurtio/openyurt/pkg/yurtmanager/webhook/platformadmin/v1alpha2"
	v1alpha1pod "github.com/openyurtio/openyurt/pkg/yurtmanager/webhook/pod/v1alpha1"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/webhook/util"
	webhookcontroller "github.com/openyurtio/openyurt/pkg/yurtmanager/webhook/util/controller"
	v1alpha1yurtappdaemon "github.com/openyurtio/openyurt/pkg/yurtmanager/webhook/yurtappdaemon/v1alpha1"
	v1alpha1yurtappoverrider "github.com/openyurtio/openyurt/pkg/yurtmanager/webhook/yurtappoverrider/v1alpha1"
	v1beta1yurtappset "github.com/openyurtio/openyurt/pkg/yurtmanager/webhook/yurtappset/v1beta1"
	v1alpha1yurtstaticset "github.com/openyurtio/openyurt/pkg/yurtmanager/webhook/yurtstaticset/v1alpha1"
)

type SetupWebhookWithManager interface {
	// mutate path, validatepath, error
	SetupWebhookWithManager(mgr ctrl.Manager) (string, string, error)
}

// controllerWebhooks is used to control whether enable or disable controller-webhooks
var controllerWebhooks map[string][]SetupWebhookWithManager

// independentWebhooks is used to control whether disable independent-webhooks
var independentWebhooks = make(map[string]SetupWebhookWithManager)

var WebhookHandlerPath = make(map[string]struct{})

func addControllerWebhook(name string, handler SetupWebhookWithManager) {
	if controllerWebhooks == nil {
		controllerWebhooks = make(map[string][]SetupWebhookWithManager)
	}

	if controllerWebhooks[name] == nil {
		controllerWebhooks[name] = make([]SetupWebhookWithManager, 0)
	}

	controllerWebhooks[name] = append(controllerWebhooks[name], handler)
}

func init() {
	addControllerWebhook(names.GatewayPickupController, &v1beta1gateway.GatewayHandler{})
	addControllerWebhook(names.NodePoolController, &v1beta1nodepool.NodePoolHandler{})
	addControllerWebhook(names.YurtStaticSetController, &v1alpha1yurtstaticset.YurtStaticSetHandler{})
	addControllerWebhook(names.YurtAppSetController, &v1beta1yurtappset.YurtAppSetHandler{})
	addControllerWebhook(names.YurtAppDaemonController, &v1alpha1yurtappdaemon.YurtAppDaemonHandler{})
	addControllerWebhook(names.PlatformAdminController, &v1alpha1platformadmin.PlatformAdminHandler{})
	addControllerWebhook(names.PlatformAdminController, &v1alpha2platformadmin.PlatformAdminHandler{})
	addControllerWebhook(names.YurtAppOverriderController, &v1alpha1yurtappoverrider.YurtAppOverriderHandler{})
	addControllerWebhook(names.YurtAppOverriderController, &v1alpha1deploymentrender.DeploymentRenderHandler{})

	independentWebhooks[v1node.WebhookName] = &v1node.NodeHandler{}
	independentWebhooks[v1alpha1pod.WebhookName] = &v1alpha1pod.PodHandler{}
}

// Note !!! @kadisi
// Do not change the name of the file or the contents of the file !!!!!!!!!!
// Note !!!

func SetupWithManager(c *config.CompletedConfig, mgr manager.Manager) error {
	setup := func(s SetupWebhookWithManager) error {
		m, v, err := s.SetupWebhookWithManager(mgr)
		if err != nil {
			return fmt.Errorf("unable to create webhook %v", err)
		}
		if _, ok := WebhookHandlerPath[m]; ok {
			panic(fmt.Errorf("webhook handler path %s duplicated", m))
		}
		WebhookHandlerPath[m] = struct{}{}
		klog.Infof("Add webhook mutate path %s", m)
		if _, ok := WebhookHandlerPath[v]; ok {
			panic(fmt.Errorf("webhook handler path %s duplicated", v))
		}
		WebhookHandlerPath[v] = struct{}{}
		klog.Infof("Add webhook validate path %s", v)

		return nil
	}

	// set up webhook namespace
	util.SetNamespace(c.ComponentConfig.Generic.WorkingNamespace)

	// set up independent webhooks
	for name, s := range independentWebhooks {
		if util.IsWebhookDisabled(name, c.ComponentConfig.Generic.DisabledWebhooks) {
			klog.Warningf("Webhook %v is disabled", name)
			continue
		}
		if err := setup(s); err != nil {
			return err
		}
	}

	// set up controller webhooks
	for controllerName, list := range controllerWebhooks {
		if !app.IsControllerEnabled(controllerName, controller.ControllersDisabledByDefault, c.ComponentConfig.Generic.Controllers) {
			klog.Warningf("Webhook for %v is disabled", controllerName)
			continue
		}
		for _, s := range list {
			if err := setup(s); err != nil {
				return err
			}
		}
	}
	return nil
}

type GateFunc func() (enabled bool)

// +kubebuilder:rbac:groups=core,namespace=kube-system,resources=secrets,verbs=get;update
// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=mutatingwebhookconfigurations,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=validatingwebhookconfigurations,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch;update;patch

func Initialize(ctx context.Context, cc *config.CompletedConfig, restCfg *rest.Config) error {
	c, err := webhookcontroller.New(WebhookHandlerPath, cc, restCfg)
	if err != nil {
		return err
	}
	go func() {
		c.Start(ctx)
	}()

	timer := time.NewTimer(time.Second * 20)
	defer timer.Stop()
	select {
	case <-webhookcontroller.Inited():
		return nil
	case <-timer.C:
		return fmt.Errorf("could not prepare certificate for webhook within 20s")
	}
}
