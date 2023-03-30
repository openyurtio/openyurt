/*
Copyright 2023 The OpenYurt Authors.

Licensed under the Apache License, Version 2.0 (the License);
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an AS IS BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package poolcoordinatorcert

import (
	"context"
	"crypto"
	"crypto/x509"
	"flag"
	"fmt"
	"net"

	"github.com/pkg/errors"
	certificatesv1 "k8s.io/api/certificates/v1"
	client "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appconfig "github.com/openyurtio/openyurt/cmd/yurt-manager/app/config"
	certfactory "github.com/openyurtio/openyurt/pkg/util/certmanager/factory"
	"github.com/openyurtio/openyurt/pkg/util/ip"
)

func init() {
	flag.IntVar(&concurrentReconciles, "poolcoordinatorcert-workers", concurrentReconciles, "Max concurrent workers for Poolcoordinatorcert controller.")
}

var (
	concurrentReconciles = 3
)

const (
	controllerName = "Poolcoordinatorcert-controller"

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

	PoolcoordinatorAPIServerCN            = "openyurt:pool-coordinator:apiserver"
	PoolcoordinatorNodeLeaseProxyClientCN = "openyurt:pool-coordinator:node-lease-proxy-client"
	PoolcoordinatorETCDCN                 = "openyurt:pool-coordinator:etcd"
	PoolcoordinatorYurthubClientCN        = "openyurt:pool-coordinator:yurthub"
	KubeConfigMonitoringClientCN          = "openyurt:pool-coordinator:monitoring"
	KubeConfigAdminClientCN               = "cluster-admin"
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
		CommonName:   KubeConfigAdminClientCN,
		Organization: []string{PoolcoordinatorAdminOrg},
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
		IPs: []net.IP{
			net.ParseIP("127.0.0.1"),
		},
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

func Format(format string, args ...interface{}) string {
	s := fmt.Sprintf(format, args...)
	return fmt.Sprintf("%s: %s", controllerName, s)
}

// Add creates a new Poolcoordinatorcert Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(c *appconfig.CompletedConfig, mgr manager.Manager) error {
	r := &ReconcilePoolcoordinatorcert{}

	// Create a new controller
	_, err := controller.New(controllerName, mgr, controller.Options{
		Reconciler: r, MaxConcurrentReconciles: concurrentReconciles,
	})
	if err != nil {
		return err
	}

	// init PoolCoordinator
	// prepare some necessary assets (CA, certs, kubeconfigs) for pool-coordinator
	err = initPoolCoordinator(r.Client, nil)
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcilePoolcoordinatorcert{}

// ReconcilePoolcoordinatorcert reconciles a Poolcoordinatorcert object
type ReconcilePoolcoordinatorcert struct {
	Client client.Interface
}

// InjectConfig
func (r *ReconcilePoolcoordinatorcert) InjectConfig(cfg *rest.Config) error {
	client, err := client.NewForConfig(cfg)
	if err != nil {
		klog.Errorf("failed to create kube client, %v", err)
		return err
	}
	r.Client = client
	return nil
}

// +kubebuilder:rbac:groups=certificates.k8s.io,resources=certificatesigningrequests,verbs=create
// +kubebuilder:rbac:groups="",resources=secret,verbs=get;update;patch;create;list
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;watch;list

// todo: make customized certificate for each poolcoordinator pod
func (r *ReconcilePoolcoordinatorcert) Reconcile(_ context.Context, request reconcile.Request) (reconcile.Result, error) {

	// Note !!!!!!!!!!
	// We strongly recommend use Format() to  encapsulation because Format() can print logs by module
	// @kadisi
	klog.Infof(Format("Reconcile Poolcoordinatorcert %s/%s", request.Namespace, request.Name))

	return reconcile.Result{}, nil
}

func initPoolCoordinator(clientSet client.Interface, stopCh <-chan struct{}) error {

	klog.Infof(Format("init poolcoordinator started"))

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
			cert, _, err := loadCertAndKeyFromSecret(clientSet, certConf)
			if err != nil {
				klog.Infof(Format("can not load cert %s from %s secret", certConf.CertName, certConf.SecretName))
				selfSignedCerts = append(selfSignedCerts, certConf)
				continue
			}

			// 1.2 check if cert is autorized by current CA
			if !IsCertFromCA(cert, caCert) {
				klog.Infof(Format("existing cert %s is not authorized by current CA", certConf.CertName))
				selfSignedCerts = append(selfSignedCerts, certConf)
				continue
			}

			// 1.3 check has dynamic attrs changed
			if certConf.certInit != nil {
				// receive dynamic IP addresses
				ips, _, err := certConf.certInit(clientSet, stopCh)
				if err != nil {
					// if cert init failed, skip this cert
					klog.Errorf(Format("fail to init cert %s when checking dynamic attrs: %v", certConf.CertName, err))
					continue
				} else {
					// check if dynamic IP addresses already exist in cert
					changed := ip.SearchAllIP(cert.IPAddresses, ips)
					if changed {
						klog.Infof(Format("cert %s IP has changed", certConf.CertName))
						selfSignedCerts = append(selfSignedCerts, certConf)
						continue
					}
				}
			}

			klog.Infof(Format("cert %s not change, reuse it", certConf.CertName))
		}
	} else {
		// create all certs with new CA
		selfSignedCerts = allSelfSignedCerts
	}

	// create self signed certs
	for _, certConf := range selfSignedCerts {
		if err := initPoolCoordinatorCert(clientSet, certConf, caCert, caKey, stopCh); err != nil {
			klog.Errorf(Format("create cert %s fail: %v", certConf.CertName, err))
			return err
		}
	}

	// 2. prepare apiserver-kubelet-client cert
	if err := initAPIServerClientCert(clientSet, stopCh); err != nil {
		return err
	}

	// 3. prepare node-lease-proxy-client cert
	if err := initNodeLeaseProxyClient(clientSet, stopCh); err != nil {
		return err
	}

	// 4. prepare ca cert in static secret
	if err := WriteCertAndKeyIntoSecret(clientSet, "ca", PoolcoordinatorStaticSecertName, caCert, nil); err != nil {
		return err
	}

	// 5. prepare ca cert in yurthub secret
	if err := WriteCertAndKeyIntoSecret(clientSet, "ca", PoolcoordinatorYurthubClientSecertName, caCert, nil); err != nil {
		return err
	}

	// 6. prepare sa key pairs
	if err := initSAKeyPair(clientSet, "sa", PoolcoordinatorStaticSecertName); err != nil {
		return err
	}

	return nil
}

// Prepare CA certs,
// check if pool-coordinator CA already exist, if not creat one
func initCA(clientSet client.Interface) (caCert *x509.Certificate, caKey crypto.Signer, reuse bool, err error) {
	// try load CA cert&key from secret
	caCert, caKey, err = loadCertAndKeyFromSecret(clientSet, CertConfig{
		SecretName:   PoolCoordinatorCASecretName,
		CertName:     "ca",
		IsKubeConfig: false,
	})

	if err == nil {
		// if CA already exist
		klog.Info(Format("CA already exist in secret, reuse it"))
		return caCert, caKey, true, nil
	} else {
		// if not exist
		// create new CA certs
		klog.Infof(Format("fail to get CA from secret: %v, create new CA", err))
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

func initNodeLeaseProxyClient(clientSet client.Interface, stopCh <-chan struct{}) error {
	certMgr, err := certfactory.NewCertManagerFactory(clientSet).New(&certfactory.CertManagerConfig{
		CertDir:       certDir,
		ComponentName: "yurthub",
		SignerName:    certificatesv1.KubeAPIServerClientSignerName,
		CommonName:    PoolcoordinatorNodeLeaseProxyClientCN,
		Organizations: []string{PoolcoordinatorOrg},
	})
	if err != nil {
		return err
	}

	return WriteCertIntoSecret(clientSet, "node-lease-proxy-client", PoolcoordinatorYurthubClientSecertName, certMgr, stopCh)
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
