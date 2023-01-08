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

package certmanager

import (
	"crypto/tls"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/informers"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurthub/poolcoordinator/constants"
	"github.com/openyurtio/openyurt/pkg/yurthub/util/fs"
)

type CertFileType string

var (
	RootCA            CertFileType = "CA"
	YurthubClientCert CertFileType = "YurthubClientCert"
	YurthubClientKey  CertFileType = "YurthubClientKey"
)

var certFileNames = map[CertFileType]string{
	RootCA:            "pool-coordinator-ca.crt",
	YurthubClientCert: "pool-coordinator-yurthub-client.crt",
	YurthubClientKey:  "pool-coordinator-yurthub-client.key",
}

func NewCertManager(pkiDir string, yurtClient kubernetes.Interface, informerFactory informers.SharedInformerFactory) (*CertManager, error) {
	store := fs.FileSystemOperator{}
	if err := store.CreateDir(pkiDir); err != nil && err != fs.ErrExists {
		return nil, fmt.Errorf("failed to create dir %s, %v", pkiDir, err)
	}

	certMgr := &CertManager{
		pkiDir: pkiDir,
		store:  store,
	}

	secretInformerFunc := func(client kubernetes.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
		tweakListOptions := func(options *metav1.ListOptions) {
			options.FieldSelector = fields.Set{"metadata.name": constants.PoolCoordinatorClientSecretName}.String()
		}
		return coreinformers.NewFilteredSecretInformer(yurtClient, constants.PoolCoordinatorClientSecretNamespace, 0, nil, tweakListOptions)
	}
	secretInformer := informerFactory.InformerFor(&corev1.Secret{}, secretInformerFunc)
	secretInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			klog.V(4).Infof("notify secret add event for %s", constants.PoolCoordinatorClientSecretName)
			secret := obj.(*corev1.Secret)
			certMgr.updateCerts(secret)
		},
		UpdateFunc: func(_, newObj interface{}) {
			klog.V(4).Infof("notify secret update event for %s", constants.PoolCoordinatorClientSecretName)
			secret := newObj.(*corev1.Secret)
			certMgr.updateCerts(secret)
		},
		DeleteFunc: func(_ interface{}) {
			klog.V(4).Infof("notify secret delete event for %s", constants.PoolCoordinatorClientSecretName)
			certMgr.deleteCerts()
		},
	})

	return certMgr, nil
}

type CertManager struct {
	sync.Mutex
	pkiDir string
	cert   *tls.Certificate
	store  fs.FileSystemOperator

	// Used for unit test.
	secret *corev1.Secret
}

func (c *CertManager) Current() *tls.Certificate {
	c.Lock()
	defer c.Unlock()
	return c.cert
}

func (c *CertManager) GetCaFile() string {
	return c.GetFilePath(RootCA)
}

func (c *CertManager) GetFilePath(t CertFileType) string {
	return filepath.Join(c.pkiDir, certFileNames[t])
}

func (c *CertManager) updateCerts(secret *corev1.Secret) {
	ca := secret.Data["ca.crt"]
	coordinatorClientCrt := secret.Data["pool-coordinator-yurthub-client.crt"]
	coordinatorClientKey := secret.Data["pool-coordinator-yurthub-client.key"]

	cert, err := tls.X509KeyPair(coordinatorClientCrt, coordinatorClientKey)
	if err != nil {
		klog.Errorf("failed to create tls certificate for coordinator, %v", err)
		return
	}

	caPath := c.GetCaFile()
	certPath := c.GetFilePath(YurthubClientCert)
	keyPath := c.GetFilePath(YurthubClientKey)

	c.Lock()
	defer c.Unlock()
	// TODO: The following updates should rollback on failure,
	// making the certs in-memory and certs on disk consistent.
	if err := c.createOrUpdateFile(caPath, ca); err != nil {
		klog.Errorf("failed to update ca, %v", err)
	}
	if err := c.createOrUpdateFile(keyPath, coordinatorClientKey); err != nil {
		klog.Errorf("failed to update client key, %v", err)
	}
	if err := c.createOrUpdateFile(certPath, coordinatorClientCrt); err != nil {
		klog.Errorf("failed to update client cert, %v", err)
	}
	c.cert = &cert
	c.secret = secret.DeepCopy()
}

func (c *CertManager) deleteCerts() {
	c.cert = nil
}

func (c *CertManager) createOrUpdateFile(path string, data []byte) error {
	if err := c.store.Write(path, data); err == fs.ErrNotExists {
		if err := c.store.CreateFile(path, data); err != nil {
			return fmt.Errorf("failed to create file at %s, %v", path, err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to update file at %s, %v", path, err)
	}
	return nil
}
