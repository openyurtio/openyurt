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
	corev1 "k8s.io/api/core/v1"
	client "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	appconfig "github.com/openyurtio/openyurt/cmd/yurt-manager/app/config"
	certfactory "github.com/openyurtio/openyurt/pkg/util/certmanager/factory"
	"github.com/openyurtio/openyurt/pkg/util/ip"
)

func init() {
	flag.IntVar(&concurrentReconciles, "poolcoordinatorcert-workers", concurrentReconciles, "Max concurrent workers for PoolCoordinatorCert controller.")
}

var (
	concurrentReconciles = 1
	PoolcoordinatorNS    = "kube-system"
)

const (
	ControllerName = "poolcoordinatorcert"

	// tmp file directory for certmanager to write cert files
	certDir = "/tmp"

	ComponentName               = "yurt-controller-manager_poolcoordinator"
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
	PoolcoordinatorStaticSecretName = "pool-coordinator-static-certs"
	// Dynamic certs will not be shared among clients or servers, contains:
	// - apiserver.crt
	// - apiserver.key
	// - etcd-server.crt
	// - etcd-server.key
	// todo: currently we only create one copy, this will be refined in the future to assign customized certs for different nodepools
	PoolcoordinatorDynamicSecretName = "pool-coordinator-dynamic-certs"
	// Yurthub certs shared by all yurthub, contains:
	// - ca.crt
	// - pool-coordinator-yurthub-client.crt
	// - pool-coordinator-yurthub-client.key
	PoolcoordinatorYurthubClientSecretName = "pool-coordinator-yurthub-certs"
	// Monitoring kubeconfig contains: monitoring kubeconfig for poolcoordinator
	// - kubeconfig
	PoolcoordinatorMonitoringKubeconfigSecretName = "pool-coordinator-monitoring-kubeconfig"

	PoolcoordinatorOrg      = "openyurt:pool-coordinator"
	PoolcoordinatorAdminOrg = "system:masters"

	PoolcoordinatorAPIServerCN            = "openyurt:pool-coordinator:apiserver"
	PoolcoordinatorNodeLeaseProxyClientCN = "openyurt:pool-coordinator:node-lease-proxy-client"
	PoolcoordinatorETCDCN                 = "openyurt:pool-coordinator:etcd"
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

	// certInit is used for initialize those attrs which has to be determined dynamically
	// e.g. TLS server cert's IP & DNSNames
	certInit certInitFunc
}

var (
	allIndependentCerts = []CertConfig{
		{
			CertName:     "apiserver-etcd-client",
			SecretName:   PoolcoordinatorStaticSecretName,
			IsKubeConfig: false,
			ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
			CommonName:   PoolcoordinatorETCDCN,
			Organization: []string{PoolcoordinatorOrg},
		},
		{
			CertName:     "pool-coordinator-yurthub-client",
			SecretName:   PoolcoordinatorYurthubClientSecretName,
			IsKubeConfig: false,
			ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
			CommonName:   KubeConfigAdminClientCN,
			Organization: []string{PoolcoordinatorAdminOrg},
		},
	}

	certsDependOnETCDSvc = []CertConfig{
		{
			CertName:     "etcd-server",
			SecretName:   PoolcoordinatorDynamicSecretName,
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
	}

	certsDependOnAPIServerSvc = []CertConfig{
		{
			CertName:     "apiserver",
			SecretName:   PoolcoordinatorDynamicSecretName,
			IsKubeConfig: false,
			ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
			CommonName:   PoolcoordinatorAPIServerCN,
			Organization: []string{PoolcoordinatorOrg},
			certInit: func(i client.Interface, c <-chan struct{}) ([]net.IP, []string, error) {
				return waitUntilSVCReady(i, PoolcoordinatorAPIServerSVC, c)
			},
		},
		{
			CertName:     "kubeconfig",
			SecretName:   PoolcoordinatorMonitoringKubeconfigSecretName,
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
			SecretName:   PoolcoordinatorStaticSecretName,
			IsKubeConfig: true,
			ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
			CommonName:   KubeConfigAdminClientCN,
			Organization: []string{PoolcoordinatorAdminOrg},
			certInit: func(i client.Interface, c <-chan struct{}) ([]net.IP, []string, error) {
				return waitUntilSVCReady(i, PoolcoordinatorAPIServerSVC, c)
			},
		},
	}
)

func Format(format string, args ...interface{}) string {
	s := fmt.Sprintf(format, args...)
	return fmt.Sprintf("%s: %s", ControllerName, s)
}

// Add creates a new Poolcoordinatorcert Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(cfg *appconfig.CompletedConfig, mgr manager.Manager) error {
	r := &ReconcilePoolCoordinatorCert{}

	// Create a new controller
	c, err := controller.New(ControllerName, mgr, controller.Options{
		Reconciler: r, MaxConcurrentReconciles: concurrentReconciles,
	})
	if err != nil {
		return err
	}

	// init global variables
	PoolcoordinatorNS = cfg.ComponentConfig.Generic.WorkingNamespace

	// prepare ca certs for pool coordinator
	caCert, caKey, reuseCA, err := initCA(r.kubeClient)
	if err != nil {
		return errors.Wrap(err, "init poolcoordinator failed")
	}
	r.caCert = caCert
	r.caKey = caKey
	r.reuseCA = reuseCA

	// prepare all independent certs
	if err := r.initPoolCoordinator(allIndependentCerts, nil); err != nil {
		return err
	}

	// prepare ca cert in static secret
	if err := WriteCertAndKeyIntoSecret(r.kubeClient, "ca", PoolcoordinatorStaticSecretName, r.caCert, nil); err != nil {
		return err
	}

	// prepare ca cert in yurthub secret
	if err := WriteCertAndKeyIntoSecret(r.kubeClient, "ca", PoolcoordinatorYurthubClientSecretName, r.caCert, nil); err != nil {
		return err
	}

	// prepare sa key pairs
	if err := initSAKeyPair(r.kubeClient, "sa", PoolcoordinatorStaticSecretName); err != nil {
		return err
	}

	// watch pool coordinator service
	svcReadyPredicates := predicate.Funcs{
		CreateFunc: func(evt event.CreateEvent) bool {
			if svc, ok := evt.Object.(*corev1.Service); ok {
				return isPoolCoordinatorSvc(svc)
			}
			return false
		},
		UpdateFunc: func(evt event.UpdateEvent) bool {
			if svc, ok := evt.ObjectNew.(*corev1.Service); ok {
				return isPoolCoordinatorSvc(svc)
			}
			return false
		},
		DeleteFunc: func(evt event.DeleteEvent) bool {
			return false
		},
	}
	return c.Watch(&source.Kind{Type: &corev1.Service{}}, &handler.EnqueueRequestForObject{}, svcReadyPredicates)
}

func isPoolCoordinatorSvc(svc *corev1.Service) bool {
	if svc == nil {
		return false
	}

	if svc.Namespace == PoolcoordinatorNS && (svc.Name == PoolcoordinatorAPIServerSVC || svc.Name == PoolcoordinatorETCDSVC) {
		return true
	}

	return false
}

var _ reconcile.Reconciler = &ReconcilePoolCoordinatorCert{}

// ReconcilePoolCoordinatorCert reconciles a Poolcoordinatorcert object
type ReconcilePoolCoordinatorCert struct {
	kubeClient client.Interface
	caCert     *x509.Certificate
	caKey      crypto.Signer
	reuseCA    bool
}

// InjectConfig will prepare kube client for PoolCoordinatorCert
func (r *ReconcilePoolCoordinatorCert) InjectConfig(cfg *rest.Config) error {
	kubeClient, err := client.NewForConfig(cfg)
	if err != nil {
		klog.Errorf("failed to create kube client, %v", err)
		return err
	}
	r.kubeClient = kubeClient
	return nil
}

// +kubebuilder:rbac:groups=certificates.k8s.io,resources=certificatesigningrequests,verbs=create
// +kubebuilder:rbac:groups="",resources=secret,verbs=get;update;patch;create;list
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;watch;list

// todo: make customized certificate for each poolcoordinator pod
func (r *ReconcilePoolCoordinatorCert) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {

	// Note !!!!!!!!!!
	// We strongly recommend use Format() to  encapsulation because Format() can print logs by module
	// @kadisi
	klog.Infof(Format("Reconcile PoolCoordinatorCert %s/%s", request.Namespace, request.Name))
	// 1. prepare apiserver-kubelet-client cert
	if err := initAPIServerClientCert(r.kubeClient, ctx.Done()); err != nil {
		return reconcile.Result{}, err
	}

	// 2. prepare node-lease-proxy-client cert
	if err := initNodeLeaseProxyClient(r.kubeClient, ctx.Done()); err != nil {
		return reconcile.Result{}, err
	}

	// 3. prepare certs based on service
	if request.NamespacedName.Namespace == PoolcoordinatorNS {
		if request.NamespacedName.Name == PoolcoordinatorAPIServerSVC {
			return reconcile.Result{}, r.initPoolCoordinator(certsDependOnAPIServerSvc, ctx.Done())
		} else if request.NamespacedName.Name == PoolcoordinatorETCDSVC {
			return reconcile.Result{}, r.initPoolCoordinator(certsDependOnETCDSvc, ctx.Done())
		}
	}
	return reconcile.Result{}, nil
}

func (r *ReconcilePoolCoordinatorCert) initPoolCoordinator(allSelfSignedCerts []CertConfig, stopCh <-chan struct{}) error {

	klog.Infof(Format("init poolcoordinator started"))
	// Prepare certs used by poolcoordinators

	// prepare selfsigned certs
	var selfSignedCerts []CertConfig

	if r.reuseCA {
		// if CA is reused
		// then we can check if there are selfsigned certs can be reused too
		for _, certConf := range allSelfSignedCerts {

			// 1.1 check if cert exist
			cert, _, err := loadCertAndKeyFromSecret(r.kubeClient, certConf)
			if err != nil {
				klog.Infof(Format("can not load cert %s from %s secret", certConf.CertName, certConf.SecretName))
				selfSignedCerts = append(selfSignedCerts, certConf)
				continue
			}

			// 1.2 check if cert is authorized by current CA
			if !IsCertFromCA(cert, r.caCert) {
				klog.Infof(Format("existing cert %s is not authorized by current CA", certConf.CertName))
				selfSignedCerts = append(selfSignedCerts, certConf)
				continue
			}

			// 1.3 check has dynamic attrs changed
			if certConf.certInit != nil {
				// receive dynamic IP addresses
				ips, _, err := certConf.certInit(r.kubeClient, stopCh)
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

	// create selfsigned certs
	for _, certConf := range selfSignedCerts {
		if err := initPoolCoordinatorCert(r.kubeClient, certConf, r.caCert, r.caKey, stopCh); err != nil {
			klog.Errorf(Format("create cert %s fail: %v", certConf.CertName, err))
			return err
		}
	}

	return nil
}

// initCA is used for preparing CA certs,
// check if pool-coordinator CA already exist, if not create one
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
		// if ca secret does not exist, create new CA certs
		klog.Infof(Format("secret(%s/%s) is not found, create new CA", PoolcoordinatorNS, PoolCoordinatorCASecretName))
		// write it into the secret
		caCert, caKey, err = NewSelfSignedCA()
		if err != nil {
			return nil, nil, false, errors.Wrap(err, "fail to new self CA assets when initializing poolcoordinator")
		}

		err = WriteCertAndKeyIntoSecret(clientSet, "ca", PoolCoordinatorCASecretName, caCert, caKey)
		if err != nil {
			return nil, nil, false, errors.Wrap(err, "fail to write CA assets into secret when initializing poolcoordinator")
		}
	}

	return caCert, caKey, false, nil
}

func initAPIServerClientCert(clientSet client.Interface, stopCh <-chan struct{}) error {
	if cert, _, err := loadCertAndKeyFromSecret(clientSet, CertConfig{
		SecretName:   PoolcoordinatorStaticSecretName,
		CertName:     "apiserver-kubelet-client",
		IsKubeConfig: false,
	}); cert != nil {
		klog.Infof("apiserver-kubelet-client cert has already existed in secret %s", PoolcoordinatorStaticSecretName)
		return nil
	} else if err != nil {
		klog.Errorf("fail to get apiserver-kubelet-client cert in secret(%s), %v, and new cert will be created", PoolcoordinatorStaticSecretName, err)
	}

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

	return WriteCertIntoSecret(clientSet, "apiserver-kubelet-client", PoolcoordinatorStaticSecretName, certMgr, stopCh)
}

func initNodeLeaseProxyClient(clientSet client.Interface, stopCh <-chan struct{}) error {
	if cert, _, err := loadCertAndKeyFromSecret(clientSet, CertConfig{
		SecretName:   PoolcoordinatorYurthubClientSecretName,
		CertName:     "node-lease-proxy-client",
		IsKubeConfig: false,
	}); cert != nil {
		klog.Infof("node-lease-proxy-client cert has already existed in secret %s", PoolcoordinatorYurthubClientSecretName)
		return nil
	} else if err != nil {
		klog.Errorf("fail to get node-lease-proxy-client cert in secret(%s), %v, and new cert will be created", PoolcoordinatorYurthubClientSecretName, err)
	}

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

	return WriteCertIntoSecret(clientSet, "node-lease-proxy-client", PoolcoordinatorYurthubClientSecretName, certMgr, stopCh)
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
