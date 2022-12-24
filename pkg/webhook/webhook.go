/*
Copyright 2022 The OpenYurt Authors.
Copyright 2017 The Kubernetes Authors.

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
	"fmt"
	"net/http"
	"time"

	"k8s.io/client-go/informers"
	client "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/controller/poolcoordinator/utils"
)

type Handler struct {
	Path        string
	HttpHandler func(http.ResponseWriter, *http.Request)
}

type Webhook interface {
	Handler() []Handler
	Init(*Certs, <-chan struct{})
}

type WebhookManager struct {
	client   client.Interface
	certdir  string
	webhooks []Webhook
}

func webhookCertDir() string {
	return utils.GetEnv("WEBHOOK_CERT_DIR", "/tmp/k8s-webhook-server/serving-certs")
}

func webhookNamespace() string {
	return utils.GetEnv("WEBHOOK_NAMESPACE", "kube-system")
}

func webhookServiceName() string {
	return utils.GetEnv("WEBHOOK_SERVICE_NAME", "yurt-controller-manager-webhook")
}

func webhookServicePort() string {
	return utils.GetEnv("WEBHOOK_SERVICE_PORT", "9443")
}

func NewWebhookManager(kc client.Interface, informerFactory informers.SharedInformerFactory) *WebhookManager {
	m := &WebhookManager{}

	m.certdir = webhookCertDir()

	h := NewPoolcoordinatorWebhook(kc, informerFactory)
	m.addWebhook(h)

	return m
}

func (m *WebhookManager) addWebhook(webhook Webhook) {
	m.webhooks = append(m.webhooks, webhook)
}

func (m *WebhookManager) Run(stopCH <-chan struct{}) {
	err := utils.EnsureDir(m.certdir)
	if err != nil {
		klog.Error(err)
	}
	crt := m.certdir + "/tls.crt"
	key := m.certdir + "/tls.key"

	certs := GenerateCerts(webhookNamespace(), webhookServiceName())
	err = utils.WriteFile(crt, certs.Cert)
	if err != nil {
		klog.Error(err)
	}
	err = utils.WriteFile(key, certs.Key)
	if err != nil {
		klog.Error(err)
	}

	for {
		if utils.FileExists(crt) && utils.FileExists(key) {
			klog.Info("tls key and cert ok.")
			break
		} else {
			klog.Info("Wating for tls key and cert...")
			time.Sleep(time.Second)
		}
	}

	for _, h := range m.webhooks {
		h.Init(certs, stopCH)
		for _, hh := range h.Handler() {
			http.HandleFunc(hh.Path, hh.HttpHandler)
		}
	}

	klog.Infof("Listening on port %s ...", webhookServicePort())
	klog.Fatal(http.ListenAndServeTLS(fmt.Sprintf(":%s", webhookServicePort()), crt, key, nil))
}
