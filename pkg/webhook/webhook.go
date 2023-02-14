/*
Copyright 2023 The OpenYurt Authors.
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
	"os"
	"time"

	yurtinformers "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/client/informers/externalversions"
	"k8s.io/client-go/informers"
	client "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/util/file"
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
	certDir  string
	webhooks []Webhook
}

func NewWebhookManager(kc client.Interface,
	yurtInformers yurtinformers.SharedInformerFactory,
	informerFactory informers.SharedInformerFactory) *WebhookManager {

	m := &WebhookManager{}
	m.certDir = GetEnv(EnvCertDirName, DefaultCertDir)

	h := NewPoolcoordinatorWebhook(kc, yurtInformers, informerFactory)
	m.addWebhook(h)

	return m
}

func (m *WebhookManager) addWebhook(webhook Webhook) {
	m.webhooks = append(m.webhooks, webhook)
}

func (m *WebhookManager) Run(stopCH <-chan struct{}) {
	err := file.EnsureDir(m.certDir)
	if err != nil {
		klog.Error(err)
	}
	crt := m.certDir + "/tls.crt"
	key := m.certDir + "/tls.key"

	certs := GenerateCerts(GetEnv(EnvNamespace, DefaultNamespace), GetEnv(EnvServiceName, DefaultServiceName))
	err = os.WriteFile(crt, certs.Cert, 0660)
	if err != nil {
		klog.Error(err)
	}
	err = os.WriteFile(key, certs.Key, 0660)
	if err != nil {
		klog.Error(err)
	}

	for {
		crtExist, _ := file.FileExists(crt)
		keyExist, _ := file.FileExists(key)
		if crtExist && keyExist {
			klog.Info("tls key and cert is ok")
			break
		} else {
			klog.Info("Waiting for tls key and cert...")
			time.Sleep(time.Second)
		}
	}

	for _, h := range m.webhooks {
		h.Init(certs, stopCH)
		for _, hh := range h.Handler() {
			http.HandleFunc(hh.Path, hh.HttpHandler)
		}
	}

	servicePort := GetEnv(EnvServicePort, DefaultServicePort)
	klog.Infof("Listening on port %s ...", servicePort)
	klog.Fatal(http.ListenAndServeTLS(fmt.Sprintf(":%s", servicePort), crt, key, nil))
}
