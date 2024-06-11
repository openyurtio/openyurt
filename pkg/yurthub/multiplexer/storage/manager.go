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

type StorageManager interface {
	ResourceStorage(gvr *schema.GroupVersionResource) (storage.Interface, error)
}

type storageManager struct {
	config     *rest.Config
	storageMap map[string]storage.Interface
}

func NewStorageManager(config *rest.Config) StorageManager {
	config.NegotiatedSerializer = scheme.Codecs.WithoutConversion()
	return &storageManager{
		config:     config,
		storageMap: make(map[string]storage.Interface),
	}
}

func (sm *storageManager) ResourceStorage(gvr *schema.GroupVersionResource) (storage.Interface, error) {
	if rs, ok := sm.storageMap[gvr.String()]; ok {
		return rs, nil
	}

	restClient, err := sm.restClient(gvr)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get rest client for %v", gvr)
	}

	rs := &store{
		resource:   gvr.Resource,
		restClient: restClient,
	}

	sm.storageMap[gvr.String()] = rs

	return rs, nil
}

func (sm *storageManager) restClient(gvr *schema.GroupVersionResource) (rest.Interface, error) {
	httpClient, _ := rest.HTTPClientFor(sm.config)
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
