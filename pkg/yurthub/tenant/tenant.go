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
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

type Interface interface {
	GetTenantNs() string

	GetTenantToken() string

	InformerSynced() cache.InformerSynced

	SetSecret(sec *v1.Secret)
}

type tenantManager struct {
	Informer cache.SharedIndexInformer

	secretSynced cache.InformerSynced

	TenantSecret *v1.Secret

	TenantNs string
}

func New(orgs []string, factory informers.SharedInformerFactory) Interface {

	tenantNs := util.ParseTenantNsFromOrgs(orgs)

	klog.Infof("parse tenant ns: %s", tenantNs)
	if tenantNs == "" {
		return nil
	}

	tenantMgr := &tenantManager{TenantNs: tenantNs}

	if factory != nil {
		tenantMgr.Informer = factory.InformerFor(&v1.Secret{}, nil) //get registered secret informer
		tenantMgr.secretSynced = tenantMgr.Informer.HasSynced

		//add handlers
		tenantMgr.Informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
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

	tokenStr := string(mgr.TenantSecret.Data["token"])

	return tokenStr
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
