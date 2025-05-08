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
	client, err = sm.createRESTClient(gvr, isCRD)

	if err != nil {
		return nil, errors.Wrapf(err, "failed to create client for %v", gvr)
	}

	var rs storage.Interface
	rs = NewStorage(client.(rest.Interface), gvr.Resource, isCRD)
	sm.gvrToStorage[cacheKey] = rs
	return rs, nil
}
func (sm *apiServerStorageProvider) createRESTClient(gvr *schema.GroupVersionResource, useDynamicConfig bool) (rest.Interface, error) {
	configCopy := *sm.config

	if useDynamicConfig {
		dynamicConfig := dynamic.ConfigFor(&configCopy)
		configCopy = *dynamicConfig
	}

	gv := gvr.GroupVersion()
	configCopy.GroupVersion = &gv
	configCopy.APIPath = getAPIPath(gvr)

	httpClient, err := rest.HTTPClientFor(sm.config)
	if err != nil {
		if useDynamicConfig {
			klog.Errorf("failed to get http client for %v", gvr)
		} else {
			err = errors.Wrapf(err, "failed to get rest http client")
		}
		return nil, err
	}

	return rest.RESTClientForConfigAndClient(&configCopy, httpClient)
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
