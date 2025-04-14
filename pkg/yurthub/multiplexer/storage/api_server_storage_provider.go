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

package storage

import (
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/storage"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/serializer"
)

type StorageProvider interface {
	ResourceStorage(gvr *schema.GroupVersionResource, isCRD bool) (storage.Interface, error)
}

type apiServerStorageProvider struct {
	config         *rest.Config
	gvrToStorage   map[string]storage.Interface
	dynamicStorage map[string]storage.Interface
}

func NewStorageProvider(config *rest.Config) StorageProvider {
	config.NegotiatedSerializer = scheme.Codecs.WithoutConversion()
	return &apiServerStorageProvider{
		config:         config,
		gvrToStorage:   make(map[string]storage.Interface),
		dynamicStorage: make(map[string]storage.Interface),
	}
}

func (sm *apiServerStorageProvider) ResourceStorage(gvr *schema.GroupVersionResource, isCRD bool) (storage.Interface, error) {
	cacheKey := gvr.String()
	if rs, ok := sm.gvrToStorage[gvr.String()]; ok {
		return rs, nil
	}

	var err error
	var client rest.Interface
	if isCRD {
		client, err = sm.getDynamicClient(gvr)
	} else {
		client, err = sm.getRESTClient(gvr)
	}

	if err != nil {
		return nil, errors.Wrapf(err, "failed to create client for %v", gvr)
	}

	var rs storage.Interface
	if isCRD {
		rs = newDynamicStorage(client, gvr.Resource)
	} else {
		rs = NewStorage(client.(rest.Interface), gvr.Resource)
	}

	sm.gvrToStorage[cacheKey] = rs
	if isCRD {
		sm.dynamicStorage[cacheKey] = rs
	}
	return rs, nil
}
func (sm *apiServerStorageProvider) getRESTClient(gvr *schema.GroupVersionResource) (rest.Interface, error) {
	return sm.restClient(gvr)
}

func (sm *apiServerStorageProvider) getDynamicClient(gvr *schema.GroupVersionResource) (rest.Interface, error) {
	configCopy := *sm.config
	// config := ConfigFor(&configCopy)
	config := dynamic.ConfigFor(&configCopy)
	gv := gvr.GroupVersion()
	config.GroupVersion = &gv
	config.APIPath = getAPIPath(gvr)
	h, err := rest.HTTPClientFor(sm.config)
	if err != nil {
		klog.Errorf("failed to get http client for %v", gvr)
		return nil, err
	}
	restClient, err := rest.RESTClientForConfigAndClient(config, h)
	return restClient, err
}
func (sm *apiServerStorageProvider) restClient(gvr *schema.GroupVersionResource) (rest.Interface, error) {
	httpClient, err := rest.HTTPClientFor(sm.config)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get reset http client")
	}

	configShallowCopy := *sm.config
	configShallowCopy.APIPath = getAPIPath(gvr)

	gv := gvr.GroupVersion()
	configShallowCopy.GroupVersion = &gv

	return rest.RESTClientForConfigAndClient(&configShallowCopy, httpClient)
}

func getAPIPath(gvr *schema.GroupVersionResource) string {
	if gvr.Group == "" {
		return "/api"
	}
	return "/apis"
}
func ConfigFor(inConfig *rest.Config) *rest.Config {
	config := rest.CopyConfig(inConfig)
	config.AcceptContentTypes = "application/json"
	config.ContentType = "application/json"
	config.NegotiatedSerializer = serializer.NewUnstructuredNegotiatedSerializer()

	if config.UserAgent == "" {
		config.UserAgent = rest.DefaultKubernetesUserAgent()
	}
	return config
}
