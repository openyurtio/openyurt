/*
Copyright 2022 The OpenYurt Authors.

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
package tenant

import (
	"sync"

	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

type Interface interface {
	GetTenantNs() string

	GetTenantToken() string

	WaitForCacheSync() bool

	SetSecret(sec *v1.Secret)
}

type tenantManager struct {
	secretSynced cache.InformerSynced

	TenantSecret *v1.Secret

	TenantNs string

	StopCh <-chan struct{}

	IsSynced bool

	mutex sync.Mutex
}

func (mgr *tenantManager) WaitForCacheSync() bool {

	if mgr.IsSynced || mgr.TenantSecret != nil { //try to do sync for just one time， fast return
		return true
	}

	mgr.mutex.Lock()
	defer mgr.mutex.Unlock()

	mgr.IsSynced = cache.WaitForCacheSync(mgr.StopCh, mgr.secretSynced)

	return mgr.IsSynced
}

func New(tenantNs string, factory informers.SharedInformerFactory, stopCh <-chan struct{}) Interface {
	klog.Infof("parse tenant ns: %s", tenantNs)
	if tenantNs == "" {
		return nil
	}

	tenantMgr := &tenantManager{TenantNs: tenantNs, StopCh: stopCh}

	if factory != nil {
		informer := factory.InformerFor(&v1.Secret{}, nil) //get registered secret informer

		tenantMgr.secretSynced = informer.HasSynced
		//add handlers
		informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc:    tenantMgr.addSecret,
			UpdateFunc: tenantMgr.updateSecret,
			DeleteFunc: tenantMgr.deleteSecret})
	}

	return tenantMgr

}

func (mgr *tenantManager) GetTenantToken() string {

	if mgr.TenantSecret == nil {

		return ""
	}

	return string(mgr.TenantSecret.Data["token"])
}

func (mgr *tenantManager) GetTenantNs() string {

	return mgr.TenantNs
}

func (mgr *tenantManager) InformerSynced() cache.InformerSynced {

	return mgr.secretSynced
}

func (mgr *tenantManager) SetSecret(sec *v1.Secret) {
	mgr.TenantSecret = sec
}

func (mgr *tenantManager) addSecret(sec interface{}) {

	secret, ok := sec.(*v1.Secret)
	if !ok {
		klog.Errorf("failed to convert to *v1.Secret")
		return
	}

	klog.Infof("secret added %s", secret.GetName())

	if IsDefaultSASecret(secret) { //found it

		mgr.TenantSecret = secret
	}

}

func (mgr *tenantManager) deleteSecret(sec interface{}) {
	secret, ok := sec.(*v1.Secret)
	if !ok {
		klog.Errorf("failed to convert to *v1.Secret")
		return
	}

	klog.Infof("secret deleted %s", secret.GetName())

	if IsDefaultSASecret(secret) { //found it
		mgr.TenantSecret = nil
	}
}

func (mgr *tenantManager) updateSecret(oldSec interface{}, newSec interface{}) {

	secret, ok := newSec.(*v1.Secret)
	if !ok {
		klog.Errorf("failed to convert to *v1.Secret")
		return
	}

	klog.Infof("secret updated %s", secret.GetName())
	if IsDefaultSASecret(secret) { //found it

		mgr.TenantSecret = secret
	}
}

func IsDefaultSASecret(secret *v1.Secret) bool {

	return secret.Type == v1.SecretTypeServiceAccountToken &&
		(len(secret.Annotations) != 0 && secret.Annotations[v1.ServiceAccountNameKey] == "default")
}
