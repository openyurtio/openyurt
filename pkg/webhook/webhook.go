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
	"net/http"
	"os"
	"time"

	"k8s.io/client-go/informers"
	client "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/controller/poolcoordinator/utils"
)

const (
	CertDirEnvName = "WEBHOOK_CERT_DIR"
	DefaultCertDir = "/tmp/k8s-webhook-server/serving-certs"
)

type Handler struct {
	Path        string
	HttpHandler func(http.ResponseWriter, *http.Request)
}

type Webhook interface {
	Handler() []Handler
	Init(<-chan struct{})
}

type WebhookManager struct {
	client   client.Interface
	certdir  string
	webhooks []Webhook
}

func NewWebhookManager(kc client.Interface, informerFactory informers.SharedInformerFactory) *WebhookManager {
	m := &WebhookManager{}

	m.certdir = os.Getenv(CertDirEnvName)
	if m.certdir == "" {
		m.certdir = DefaultCertDir
	}

	h := NewPoolcoordinatorWebhook(kc, informerFactory)
	m.addWebhook(h)

	return m
}

func (m *WebhookManager) addWebhook(webhook Webhook) {
	m.webhooks = append(m.webhooks, webhook)
}

func (m *WebhookManager) Run(stopCH <-chan struct{}) {
	for _, h := range m.webhooks {
		h.Init(stopCH)
		for _, hh := range h.Handler() {
			http.HandleFunc(hh.Path, hh.HttpHandler)
		}
	}

	err := utils.EnsureDir(m.certdir)
	if err != nil {
		klog.Error(err)
	}
	cert := m.certdir + "/tls.crt"
	key := m.certdir + "/tls.key"

	for {
		if utils.FileExists(cert) && utils.FileExists(key) {
			klog.Info("tls key and cert ok.")
			break
		} else {
			klog.Info("Wating for tls key and cert...")
			time.Sleep(time.Second)
		}
	}

	klog.Info("Listening on port 443...")
	klog.Fatal(http.ListenAndServeTLS(":443", cert, key, nil))
}
