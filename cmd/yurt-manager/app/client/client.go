/*
Copyright 2024 The OpenYurt Authors.

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
	"net/http"
	"sync"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/transport"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/openyurtio/openyurt/cmd/yurt-manager/app/config"
)

const serviceAccountPrefix = "yurt-manager-"

var clientStore = &struct {
	clientByName map[string]client.Client
	lock         sync.Mutex
}{
	clientByName: make(map[string]client.Client),
	lock:         sync.Mutex{},
}

var configStore = &struct {
	configByName map[string]*rest.Config
	lock         sync.Mutex
	// baseClient is used to get service account token for each controller client
	baseClient *kubernetes.Clientset
}{
	configByName: make(map[string]*rest.Config),
	lock:         sync.Mutex{},
}

func GetConfigByControllerNameOrDie(mgr manager.Manager, controllerName string) *rest.Config {

	namespace := config.WorkingNamespace

	// if controllerName is empty, return the base config of manager
	if controllerName == "" {
		return mgr.GetConfig()
	}

	configStore.lock.Lock()
	defer configStore.lock.Unlock()

	if cfg, ok := configStore.configByName[controllerName]; ok {
		return cfg
	}

	// get base config
	baseCfg := mgr.GetConfig()

	// get base client
	var err error
	if configStore.baseClient == nil {
		configStore.baseClient, err = kubernetes.NewForConfig(baseCfg)
		if err != nil {
			klog.Fatalf("failed to create base client: %v", err)
		}
	}

	// rename cfg user-agent
	cfg := rest.CopyConfig(baseCfg)
	rest.AddUserAgent(cfg, controllerName)

	// clean cert/key info in tls config for ensuring service account will be used.
	cfg.TLSClientConfig.KeyFile = ""
	cfg.TLSClientConfig.CertFile = ""
	cfg.TLSClientConfig.CertData = []byte{}
	cfg.TLSClientConfig.KeyData = []byte{}

	// add controller-specific token wrapper to cfg
	cachedTokenSource := transport.NewCachedTokenSource(&tokenSourceImpl{
		namespace:          namespace,
		serviceAccountName: serviceAccountPrefix + controllerName,
		cli:                *configStore.baseClient,
		expirationSeconds:  defaultExpirationSeconds,
		leewayPercent:      defaultLeewayPercent,
	})

	// Notice: The execution order is the opposite of the display order
	// EmptyIfHasAuthorization -> Reset Authorization -> PostCheck
	cfg.Wrap(CheckAuthorization)
	cfg.Wrap(transport.ResettableTokenSourceWrapTransport(cachedTokenSource))
	cfg.Wrap(EmptyIfHasAuthorization)

	configStore.configByName[controllerName] = cfg
	klog.V(5).Infof("create new client config for controller %s", controllerName)

	return cfg
}

func GetClientByControllerNameOrDie(mgr manager.Manager, controllerName string) client.Client {
	// if controllerName is empty, return the base client of manager
	if controllerName == "" {
		return mgr.GetClient()
	}

	clientStore.lock.Lock()
	defer clientStore.lock.Unlock()

	if cli, ok := clientStore.clientByName[controllerName]; ok {
		return cli
	}

	// construct client options
	clientOptions := client.Options{
		Scheme: mgr.GetScheme(),
		Mapper: mgr.GetRESTMapper(),
		// controller client should get/list unstructured resource from cache instead of kube-apiserver,
		// because only base client has the right privilege to list/watch unstructured resources.
		Cache: &client.CacheOptions{
			Unstructured: true,
			Reader:       mgr.GetCache(),
		},
	}

	cfg := GetConfigByControllerNameOrDie(mgr, controllerName)
	cli, err := client.New(cfg, clientOptions)
	if err != nil {
		panic(err)
	}
	clientStore.clientByName[controllerName] = cli

	return cli
}

func EmptyIfHasAuthorization(rt http.RoundTripper) http.RoundTripper {
	return TokenResetter{
		defaultRT: rt,
	}
}

type TokenResetter struct {
	defaultRT http.RoundTripper
}

func (tr TokenResetter) RoundTrip(req *http.Request) (*http.Response, error) {
	if len(req.Header.Get("Authorization")) > 0 {
		klog.V(5).Info("[before] check request credential: already set, reset it")
		req.Header.Set("Authorization", "")
	}
	return tr.defaultRT.RoundTrip(req)
}

func CheckAuthorization(rt http.RoundTripper) http.RoundTripper {
	return RequestInspector{
		defaultRT: rt,
	}
}

type RequestInspector struct {
	defaultRT http.RoundTripper
}

func (r RequestInspector) RoundTrip(req *http.Request) (*http.Response, error) {
	if len(req.Header.Get("Authorization")) > 0 {
		klog.V(5).Infof("[after] check request credential: %s", req.Header.Get("Authorization"))
	}
	return r.defaultRT.RoundTrip(req)
}
