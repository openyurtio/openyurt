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

package cert

import (
	"crypto"
	"crypto/x509"
	"fmt"
	"net"
	"reflect"
	"time"

	"github.com/pkg/errors"
	certificatesv1 "k8s.io/api/certificates/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	coreinformers "k8s.io/client-go/informers/core/v1"
	client "k8s.io/client-go/kubernetes"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	certfactory "github.com/openyurtio/openyurt/pkg/util/certmanager/factory"
)

const (
	// tmp file directory for certmanager to write cert files
	certDir = "/tmp"

	ComponentName               = "yurt-controller-manager_poolcoordinator"
	PoolcoordinatorNS           = "kube-system"
	PoolcoordinatorAPIServerSVC = "pool-coordinator-apiserver"
	PoolcoordinatorETCDSVC      = "pool-coordinator-etcd"

	// CA certs contains the pool-coordinator CA certs
	PoolCoordinatorCASecretName = "pool-coordinator-ca-certs"
	// Static certs is shared among all pool-coordinator system, which contains:
	// - ca.crt
	// - apiserver-etcd-client.crt
	// - apiserver-etcd-client.key
	// - sa.pub
	// - sa.key
	// - apiserver-kubelet-client.crt  (not self signed)
	// - apiserver-kubelet-client.key （not self signed)
	// - admin.conf (kube-config)
	PoolcoordinatorStaticSecertName = "pool-coordinator-static-certs"
	// Dynamic certs will not be shared among clients or servers, contains:
	// - apiserver.crt
	// - apiserver.key
	// - etcd-server.crt
	// - etcd-server.key
	// todo: currently we only create one copy, this will be refined in the future to assign customized certs for differnet nodepools
	PoolcoordinatorDynamicSecertName = "pool-coordinator-dynamic-certs"
	// Yurthub certs shared by all yurthub, contains:
	// - ca.crt
	// - pool-coordinator-yurthub-client.crt
	// - pool-coordinator-yurthub-client.key
	PoolcoordinatorYurthubClientSecertName = "pool-coordinator-yurthub-certs"
	// Monitoring kubeconfig contains: monitoring kubeconfig for poolcoordinator
	// - kubeconfig
	PoolcoordinatorMonitoringKubeconfigSecertName = "pool-coordinator-monitoring-kubeconfig"

	PoolcoordinatorOrg      = "openyurt:pool-coordinator"
	PoolcoordinatorAdminOrg = "system:masters"

	PoolcoordinatorAPIServerCN     = "openyurt:pool-coordinator:apiserver"
	PoolcoordinatorETCDCN          = "openyurt:pool-coordinator:etcd"
	PoolcoordinatorYurthubClientCN = "openyurt:pool-coordinator:yurthub"
	KubeConfigMonitoringClientCN   = "openyurt:pool-coordinator:monitoring"
	KubeConfigAdminClientCN        = "cluster-admin"
)

type certInitFunc = func(client.Interface, <-chan struct{}) ([]net.IP, []string, error)

type CertConfig struct {
	// certName should be unique,  will be used as output name ${certName}.crt
	CertName string
	// secretName is where the certs should be stored
	SecretName string
	// used as kubeconfig
	IsKubeConfig bool

	ExtKeyUsage  []x509.ExtKeyUsage
	CommonName   string
	Organization []string
	DNSNames     []string
	IPs          []net.IP

	// certInit is used for initilize those attrs which has to be determined dynamically
	// e.g. TLS server cert's IP & DNSNames
	certInit certInitFunc
}

func (c *CertConfig) init(clientSet client.Interface, stopCh <-chan struct{}) (err error) {
	if c.certInit != nil {
		c.IPs, c.DNSNames, err = c.certInit(clientSet, stopCh)
		if err != nil {
			return errors.Wrapf(err, "fail to init cert %s", c.CertName)
		}
	}
	return nil
}

var allSelfSignedCerts []CertConfig = []CertConfig{
	{
		CertName:     "apiserver-etcd-client",
		SecretName:   PoolcoordinatorStaticSecertName,
		IsKubeConfig: false,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		CommonName:   PoolcoordinatorETCDCN,
		Organization: []string{PoolcoordinatorOrg},
	},
	{
		CertName:     "pool-coordinator-yurthub-client",
		SecretName:   PoolcoordinatorYurthubClientSecertName,
		IsKubeConfig: false,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		CommonName:   PoolcoordinatorYurthubClientCN,
		Organization: []string{PoolcoordinatorOrg},
	},
	{
		CertName:     "apiserver",
		SecretName:   PoolcoordinatorDynamicSecertName,
		IsKubeConfig: false,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		CommonName:   PoolcoordinatorAPIServerCN,
		Organization: []string{PoolcoordinatorOrg},
		certInit: func(i client.Interface, c <-chan struct{}) ([]net.IP, []string, error) {
			return waitUntilSVCReady(i, PoolcoordinatorAPIServerSVC, c)
		},
	},
	{
		CertName:     "etcd-server",
		SecretName:   PoolcoordinatorDynamicSecertName,
		IsKubeConfig: false,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		CommonName:   PoolcoordinatorETCDCN,
		Organization: []string{PoolcoordinatorOrg},
		certInit: func(i client.Interface, c <-chan struct{}) ([]net.IP, []string, error) {
			return waitUntilSVCReady(i, PoolcoordinatorETCDSVC, c)
		},
	},
	{
		CertName:     "kubeconfig",
		SecretName:   PoolcoordinatorMonitoringKubeconfigSecertName,
		IsKubeConfig: true,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		CommonName:   KubeConfigMonitoringClientCN,
		Organization: []string{PoolcoordinatorOrg},
		// As a clientAuth cert, kubeconfig cert don't need IP&DNS to work,
		// but kubeconfig need this extra information to verify if it's out of date
		certInit: func(i client.Interface, c <-chan struct{}) ([]net.IP, []string, error) {
			return waitUntilSVCReady(i, PoolcoordinatorAPIServerSVC, c)
		},
	},
	{
		CertName:     "admin.conf",
		SecretName:   PoolcoordinatorStaticSecertName,
		IsKubeConfig: true,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		CommonName:   KubeConfigAdminClientCN,
		Organization: []string{PoolcoordinatorAdminOrg},
		certInit: func(i client.Interface, c <-chan struct{}) ([]net.IP, []string, error) {
			return waitUntilSVCReady(i, PoolcoordinatorAPIServerSVC, c)
		},
	},
}

// PoolCoordinatorCertManager manages certificates releted with poolcoordinator pod
type PoolCoordinatorCertManager struct {
	kubeclientset client.Interface
	podLister     corelisters.PodLister
	podSynced     cache.InformerSynced
	podWorkQueue  workqueue.RateLimitingInterface
}

func NewPoolCoordinatorCertManager(kc client.Interface, podInformer coreinformers.PodInformer) *PoolCoordinatorCertManager {

	certManager := PoolCoordinatorCertManager{
		kubeclientset: kc,

		podLister: podInformer.Lister(),
		podSynced: podInformer.Informer().HasSynced,

		podWorkQueue: workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter()),
	}

	// Watch for poolcoordinator pod changes to manage related certs (including kubeconfig)
	podInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) {},
		UpdateFunc: func(oldObj, newObj interface{}) {},
		DeleteFunc: func(obj interface{}) {},
	})

	return &certManager
}

func (c *PoolCoordinatorCertManager) Run(threadiness int, stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()

	klog.Info("Starting poolcoordinatorCertManager controller")
	defer klog.Info("Shutting down poolcoordinatorCertManager controller")
	defer c.podWorkQueue.ShutDown()

	// prepare some necessary assets (CA, certs, kubeconfigs) for pool-coordinator
	err := initPoolCoordinator(c.kubeclientset, stopCh)
	if err != nil {
		klog.Errorf("fail to init poolcoordinator %v", err)
	}

	// Synchronize the cache before starting to process events
	if !cache.WaitForCacheSync(stopCh, c.podSynced) {
		klog.Error("sync poolcoordinatorCertManager controller timeout")
	}

	// The main Controller loop
	for i := 0; i < threadiness; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	<-stopCh
}

func (c *PoolCoordinatorCertManager) runWorker() {
	for {
		obj, shutdown := c.podWorkQueue.Get()
		if shutdown {
			return
		}

		if err := c.syncHandler(obj.(string)); err != nil {
			utilruntime.HandleError(err)
		}

		c.podWorkQueue.Forget(obj)
		c.podWorkQueue.Done(obj)
	}
}

func (c *PoolCoordinatorCertManager) syncHandler(key string) error {
	// todo: make customized certificate for each poolcoordinator pod
	return nil
}

func initPoolCoordinator(clientSet client.Interface, stopCh <-chan struct{}) error {

	// Prepare CA certs
	caCert, caKey, reuseCA, err := initCA(clientSet)
	if err != nil {
		return errors.Wrap(err, "init poolcoordinator failed")
	}

	// Prepare certs used by poolcoordinators

	// 1. prepare selfsigned certs
	var selfSignedCerts []CertConfig

	if reuseCA {
		// if CA is reused
		// then we can check if there are selfsigned certs can be reused too
		for _, certConf := range allSelfSignedCerts {

			// 1.1 check if cert exist
			cert, _, err := LoadCertAndKeyFromSecret(clientSet, certConf)
			if err != nil {
				klog.Infof("can not load cert %s from %s secret", certConf.CertName, certConf.SecretName)
				selfSignedCerts = append(selfSignedCerts, certConf)
				continue
			}

			// 1.2 check if cert is autorized by current CA
			if !IsCertFromCA(cert, caCert) {
				klog.Infof("existing cert %s is not authorized by current CA", certConf.CertName)
				selfSignedCerts = append(selfSignedCerts, certConf)
				continue
			}

			// 1.3 check has dynamic attrs changed
			if certConf.certInit != nil {
				if err := certConf.init(clientSet, stopCh); err != nil {
					// if cert init failed, skip this cert
					klog.Errorf("fail to init cert when checking dynamic attrs: %v", err)
					continue
				} else {
					// check if dynamic IP address has changed
					if !reflect.DeepEqual(certConf.IPs, cert.IPAddresses) {
						klog.Infof("cert %s IP has changed", certConf.CertName)
						selfSignedCerts = append(selfSignedCerts, certConf)
						continue
					}
				}
			}

			klog.Infof("cert %s not change, reuse it", certConf.CertName)
		}
	} else {
		// create all certs with new CA
		selfSignedCerts = allSelfSignedCerts
	}

	// create self signed certs
	for _, certConf := range selfSignedCerts {
		if err := initPoolCoordinatorCert(clientSet, certConf, caCert, caKey, stopCh); err != nil {
			klog.Errorf("create cert %s fail: %v", certConf.CertName, err)
			return err
		}
	}

	// 2. prepare not self signed certs ( apiserver-kubelet-client cert)
	if err := initAPIServerClientCert(clientSet, stopCh); err != nil {
		return err
	}

	// 3. prepare ca cert in static secret
	if err := WriteCertAndKeyIntoSecret(clientSet, "ca", PoolcoordinatorStaticSecertName, caCert, nil); err != nil {
		return err
	}

	// 4. prepare sa key pairs
	if err := initSAKeyPair(clientSet, "sa", PoolcoordinatorStaticSecertName); err != nil {
		return err
	}

	return nil
}

// Prepare CA certs,
// check if pool-coordinator CA already exist, if not creat one
func initCA(clientSet client.Interface) (caCert *x509.Certificate, caKey crypto.Signer, reuse bool, err error) {
	// try load CA cert&key from secret
	caCert, caKey, err = LoadCertAndKeyFromSecret(clientSet, CertConfig{
		SecretName:   PoolCoordinatorCASecretName,
		CertName:     "ca",
		IsKubeConfig: false,
	})

	if err == nil {
		// if CA already exist
		klog.Info("CA already exist in secret, reuse it")
		return caCert, caKey, true, nil
	} else {
		// if not exist
		// create new CA certs
		klog.Infof("fail to get CA from secret: %v, create new CA", err)
		// write it into the secret
		caCert, caKey, err = NewSelfSignedCA()
		if err != nil {
			return nil, nil, false, errors.Wrap(err, "fail to write CA assets into secret when initializing poolcoordinator")
		}

		err = WriteCertAndKeyIntoSecret(clientSet, "ca", PoolCoordinatorCASecretName, caCert, caKey)
		if err != nil {
			return nil, nil, false, errors.Wrap(err, "fail to write CA assets into secret when initializing poolcoordinator")
		}
	}
	return caCert, caKey, false, nil
}

func initAPIServerClientCert(clientSet client.Interface, stopCh <-chan struct{}) (err error) {
	certMgr, err := certfactory.NewCertManagerFactory(clientSet).New(&certfactory.CertManagerConfig{
		CertDir:        certDir,
		ComponentName:  fmt.Sprintf("%s-%s", ComponentName, "apiserver-client"),
		SignerName:     certificatesv1.KubeAPIServerClientSignerName,
		ForServerUsage: false,
		CommonName:     PoolcoordinatorAPIServerCN,
		Organizations:  []string{PoolcoordinatorOrg},
	})
	if err != nil {
		return err
	}

	return WriteCertIntoSecret(clientSet, "apiserver-kubelet-client", PoolcoordinatorStaticSecertName, certMgr, stopCh)
}

// create new public/private key pair for signing service account users
// and write them into secret
func initSAKeyPair(clientSet client.Interface, keyName, secretName string) (err error) {
	key, err := NewPrivateKey()
	if err != nil {
		return errors.Wrap(err, "fail to create sa key pair")
	}

	return WriteKeyPairIntoSecret(clientSet, secretName, keyName, key)
}
