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
	"net/http"
	"time"

	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/openyurtio/openyurt/cmd/yurt-manager/app/config"
	webhookcontroller "github.com/openyurtio/openyurt/pkg/webhook/util/controller"
	"github.com/openyurtio/openyurt/pkg/webhook/util/health"
)

type SetupWebhookWithManager interface {
	admission.CustomDefaulter
	admission.CustomValidator
	// mutate path, validatepath, error
	SetupWebhookWithManager(mgr ctrl.Manager) (string, string, error)
}

var WebhookLists []SetupWebhookWithManager = make([]SetupWebhookWithManager, 0, 5)
var WebhookHandlerPath = make(map[string]struct{})

// Note !!! @kadisi
// Do not change the name of the file or the contents of the file !!!!!!!!!!
// Note !!!

func addWebhook(w SetupWebhookWithManager) {
	WebhookLists = append(WebhookLists, w)
}

func SetupWithManager(mgr manager.Manager) error {
	for _, s := range WebhookLists {
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
	}
	return nil
}

type GateFunc func() (enabled bool)

// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=mutatingwebhookconfigurations,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=validatingwebhookconfigurations,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch;update;patch

func Initialize(ctx context.Context, cfg *rest.Config, cc *config.CompletedConfig) error {
	c, err := webhookcontroller.New(cfg, WebhookHandlerPath, cc)
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
		return fmt.Errorf("failed to start webhook controller for waiting more than 20s")
	}
}

func Checker(req *http.Request) error {
	// Firstly wait webhook controller initialized
	select {
	case <-webhookcontroller.Inited():
	default:
		return fmt.Errorf("webhook controller has not initialized")
	}
	return health.Checker(req)
}

func WaitReady() error {
	startTS := time.Now()
	var err error
	for {
		duration := time.Since(startTS)
		if err = Checker(nil); err == nil {
			return nil
		}

		if duration > time.Second*5 {
			klog.Warningf("Failed to wait webhook ready over %s: %v", duration, err)
		}
		time.Sleep(time.Second * 2)
	}

}
