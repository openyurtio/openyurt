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
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
)

type StorageProvider interface {
	ResourceStorage(gvr *schema.GroupVersionResource) (storage.Interface, error)
}

type apiServerStorageProvider struct {
	config       *rest.Config
	gvrToStorage map[string]storage.Interface
}

func NewStorageProvider(config *rest.Config) StorageProvider {
	config.NegotiatedSerializer = scheme.Codecs.WithoutConversion()
	return &apiServerStorageProvider{
		config:       config,
		gvrToStorage: make(map[string]storage.Interface),
	}
}

func (sm *apiServerStorageProvider) ResourceStorage(gvr *schema.GroupVersionResource) (storage.Interface, error) {
	if rs, ok := sm.gvrToStorage[gvr.String()]; ok {
		return rs, nil
	}

	restClient, err := sm.restClient(gvr)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get rest client for %v", gvr)
	}

	rs := NewStorage(restClient, gvr.Resource)
	sm.gvrToStorage[gvr.String()] = rs

	return rs, nil
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
