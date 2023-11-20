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

	"github.com/openyurtio/openyurt/pkg/yurthub/util/fs"
	"github.com/openyurtio/openyurt/pkg/yurthub/yurtcoordinator/constants"
)

type CertFileType int

const (
	RootCA CertFileType = iota
	YurthubClientCert
	YurthubClientKey
	NodeLeaseProxyClientCert
	NodeLeaseProxyClientKey
)

var certFileNames = map[CertFileType]string{
	RootCA:                   "yurt-coordinator-ca.crt",
	YurthubClientCert:        "yurt-coordinator-yurthub-client.crt",
	YurthubClientKey:         "yurt-coordinator-yurthub-client.key",
	NodeLeaseProxyClientCert: "node-lease-proxy-client.crt",
	NodeLeaseProxyClientKey:  "node-lease-proxy-client.key",
}

func NewCertManager(pkiDir, yurtHubNs string, yurtClient kubernetes.Interface, informerFactory informers.SharedInformerFactory) (*CertManager, error) {
	store := fs.FileSystemOperator{}
	if err := store.CreateDir(pkiDir); err != nil && err != fs.ErrExists {
		return nil, fmt.Errorf("could not create dir %s, %v", pkiDir, err)
	}

	certMgr := &CertManager{
		pkiDir: pkiDir,
		store:  store,
	}

	secretInformerFunc := func(client kubernetes.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
		tweakListOptions := func(options *metav1.ListOptions) {
			options.FieldSelector = fields.Set{"metadata.name": constants.YurtCoordinatorClientSecretName}.String()
		}
		return coreinformers.NewFilteredSecretInformer(yurtClient, yurtHubNs, 0, nil, tweakListOptions)
	}
	secretInformer := informerFactory.InformerFor(&corev1.Secret{}, secretInformerFunc)
	secretInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			klog.V(4).Infof("notify secret add event for %s", constants.YurtCoordinatorClientSecretName)
			secret := obj.(*corev1.Secret)
			certMgr.updateCerts(secret)
		},
		UpdateFunc: func(_, newObj interface{}) {
			klog.V(4).Infof("notify secret update event for %s", constants.YurtCoordinatorClientSecretName)
			secret := newObj.(*corev1.Secret)
			certMgr.updateCerts(secret)
		},
		DeleteFunc: func(_ interface{}) {
			klog.V(4).Infof("notify secret delete event for %s", constants.YurtCoordinatorClientSecretName)
			certMgr.deleteCerts()
		},
	})

	return certMgr, nil
}

type CertManager struct {
	sync.Mutex
	pkiDir             string
	coordinatorCert    *tls.Certificate
	nodeLeaseProxyCert *tls.Certificate
	store              fs.FileSystemOperator
	caData             []byte

	// Used for unit test.
	secret *corev1.Secret
}

func (c *CertManager) GetAPIServerClientCert() *tls.Certificate {
	c.Lock()
	defer c.Unlock()
	return c.coordinatorCert
}

func (c *CertManager) GetNodeLeaseProxyClientCert() *tls.Certificate {
	c.Lock()
	defer c.Unlock()
	return c.nodeLeaseProxyCert
}

func (c *CertManager) GetCAData() []byte {
	return c.caData
}

func (c *CertManager) GetCaFile() string {
	return c.GetFilePath(RootCA)
}

func (c *CertManager) GetFilePath(t CertFileType) string {
	return filepath.Join(c.pkiDir, certFileNames[t])
}

func (c *CertManager) updateCerts(secret *corev1.Secret) {
	ca, caok := secret.Data["ca.crt"]

	// yurt-coordinator-yurthub-client.crt should appear with yurt-coordinator-yurthub-client.key. So we
	// only check the existence once.
	coordinatorClientCrt, cook := secret.Data["yurt-coordinator-yurthub-client.crt"]
	coordinatorClientKey := secret.Data["yurt-coordinator-yurthub-client.key"]

	// node-lease-proxy-client.crt should appear with node-lease-proxy-client.key. So we
	// only check the existence once.
	nodeLeaseProxyClientCrt, nook := secret.Data["node-lease-proxy-client.crt"]
	nodeLeaseProxyClientKey := secret.Data["node-lease-proxy-client.key"]

	var coordinatorCert, nodeLeaseProxyCert *tls.Certificate
	if cook {
		if cert, err := tls.X509KeyPair(coordinatorClientCrt, coordinatorClientKey); err != nil {
			klog.Errorf("could not create tls certificate for coordinator, %v", err)
		} else {
			coordinatorCert = &cert
		}
	}

	if nook {
		if cert, err := tls.X509KeyPair(nodeLeaseProxyClientCrt, nodeLeaseProxyClientKey); err != nil {
			klog.Errorf("could not create tls certificate for node lease proxy, %v", err)
		} else {
			nodeLeaseProxyCert = &cert
		}
	}

	c.Lock()
	defer c.Unlock()
	// TODO: The following updates should rollback on failure,
	// making the certs in-memory and certs on disk consistent.
	if caok {
		klog.Infof("updating coordinator ca cert")
		if err := c.createOrUpdateFile(c.GetFilePath(RootCA), ca); err != nil {
			klog.Errorf("could not update ca, %v", err)
		}
		c.caData = ca
	}

	if cook {
		klog.Infof("updating yurt-coordinator-yurthub client cert and key")
		if err := c.createOrUpdateFile(c.GetFilePath(YurthubClientKey), coordinatorClientKey); err != nil {
			klog.Errorf("could not update coordinator client key, %v", err)
		}
		if err := c.createOrUpdateFile(c.GetFilePath(YurthubClientCert), coordinatorClientCrt); err != nil {
			klog.Errorf("could not update coordinator client cert, %v", err)
		}
	}

	if nook {
		klog.Infof("updating node-lease-proxy-client cert and key")
		if err := c.createOrUpdateFile(c.GetFilePath(NodeLeaseProxyClientKey), nodeLeaseProxyClientKey); err != nil {
			klog.Errorf("could not update node lease proxy client key, %v", err)
		}
		if err := c.createOrUpdateFile(c.GetFilePath(NodeLeaseProxyClientCert), nodeLeaseProxyClientCrt); err != nil {
			klog.Errorf("could not update node lease proxy client cert, %v", err)
		}
	}

	c.coordinatorCert = coordinatorCert
	c.nodeLeaseProxyCert = nodeLeaseProxyCert
	c.secret = secret.DeepCopy()
}

func (c *CertManager) deleteCerts() {
	c.Lock()
	defer c.Unlock()
	c.coordinatorCert = nil
	c.nodeLeaseProxyCert = nil
}

func (c *CertManager) createOrUpdateFile(path string, data []byte) error {
	if err := c.store.Write(path, data); err == fs.ErrNotExists {
		if err := c.store.CreateFile(path, data); err != nil {
			return fmt.Errorf("could not create file at %s, %v", path, err)
		}
	} else if err != nil {
		return fmt.Errorf("could not update file at %s, %v", path, err)
	}
	return nil
}
