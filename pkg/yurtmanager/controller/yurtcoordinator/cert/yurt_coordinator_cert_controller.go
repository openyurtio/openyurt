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

package yurtcoordinatorcert

import (
	"context"
	"crypto"
	"crypto/x509"
	"fmt"
	"net"

	"github.com/pkg/errors"
	certificatesv1 "k8s.io/api/certificates/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	yurtClient "github.com/openyurtio/openyurt/cmd/yurt-manager/app/client"
	appconfig "github.com/openyurtio/openyurt/cmd/yurt-manager/app/config"
	"github.com/openyurtio/openyurt/cmd/yurt-manager/names"
	certfactory "github.com/openyurtio/openyurt/pkg/util/certmanager/factory"
	"github.com/openyurtio/openyurt/pkg/util/ip"
)

var (
	YurtCoordinatorNS = "kube-system"
)

const (
	// tmp file directory for certmanager to write cert files
	certDir = "/tmp"

	ComponentName               = "yurt-controller-manager_yurtcoordinator"
	YurtCoordinatorAPIServerSVC = "yurt-coordinator-apiserver"
	YurtCoordinatorETCDSVC      = "yurt-coordinator-etcd"

	// CA certs contains the yurt-coordinator CA certs
	YurtCoordinatorCASecretName = "yurt-coordinator-ca-certs"
	// Static certs is shared among all yurt-coordinator system, which contains:
	// - ca.crt
	// - apiserver-etcd-client.crt
	// - apiserver-etcd-client.key
	// - sa.pub
	// - sa.key
	// - apiserver-kubelet-client.crt  (not self signed)
	// - apiserver-kubelet-client.key （not self signed)
	// - admin.conf (kube-config)
	YurtCoordinatorStaticSecretName = "yurt-coordinator-static-certs"
	// Dynamic certs will not be shared among clients or servers, contains:
	// - apiserver.crt
	// - apiserver.key
	// - etcd-server.crt
	// - etcd-server.key
	// todo: currently we only create one copy, this will be refined in the future to assign customized certs for different nodepools
	YurtCoordinatorDynamicSecretName = "yurt-coordinator-dynamic-certs"
	// Yurthub certs shared by all yurthub, contains:
	// - ca.crt
	// - yurt-coordinator-yurthub-client.crt
	// - yurt-coordinator-yurthub-client.key
	YurtCoordinatorYurthubClientSecretName = "yurt-coordinator-yurthub-certs"
	// Monitoring kubeconfig contains: monitoring kubeconfig for yurtcoordinator
	// - kubeconfig
	YurtCoordinatorMonitoringKubeconfigSecretName = "yurt-coordinator-monitoring-kubeconfig"

	YurtCoordinatorOrg      = "openyurt:yurt-coordinator"
	YurtCoordinatorAdminOrg = "system:masters"

	YurtCoordinatorAPIServerCN            = "openyurt:yurt-coordinator:apiserver"
	YurtCoordinatorNodeLeaseProxyClientCN = "openyurt:yurt-coordinator:node-lease-proxy-client"
	YurtCoordinatorETCDCN                 = "openyurt:yurt-coordinator:etcd"
	KubeConfigMonitoringClientCN          = "openyurt:yurt-coordinator:monitoring"
	KubeConfigAdminClientCN               = "cluster-admin"
)

type certInitFunc = func(kubernetes.Interface, <-chan struct{}) ([]net.IP, []string, error)

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
			SecretName:   YurtCoordinatorStaticSecretName,
			IsKubeConfig: false,
			ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
			CommonName:   YurtCoordinatorETCDCN,
			Organization: []string{YurtCoordinatorOrg},
		},
		{
			CertName:     "yurt-coordinator-yurthub-client",
			SecretName:   YurtCoordinatorYurthubClientSecretName,
			IsKubeConfig: false,
			ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
			CommonName:   KubeConfigAdminClientCN,
			Organization: []string{YurtCoordinatorAdminOrg},
		},
	}

	certsDependOnETCDSvc = []CertConfig{
		{
			CertName:     "etcd-server",
			SecretName:   YurtCoordinatorDynamicSecretName,
			IsKubeConfig: false,
			ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
			IPs: []net.IP{
				net.ParseIP("127.0.0.1"),
			},
			CommonName:   YurtCoordinatorETCDCN,
			Organization: []string{YurtCoordinatorOrg},
			certInit: func(i kubernetes.Interface, c <-chan struct{}) ([]net.IP, []string, error) {
				return waitUntilSVCReady(i, YurtCoordinatorETCDSVC, c)
			},
		},
	}

	certsDependOnAPIServerSvc = []CertConfig{
		{
			CertName:     "apiserver",
			SecretName:   YurtCoordinatorDynamicSecretName,
			IsKubeConfig: false,
			ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
			CommonName:   YurtCoordinatorAPIServerCN,
			Organization: []string{YurtCoordinatorOrg},
			certInit: func(i kubernetes.Interface, c <-chan struct{}) ([]net.IP, []string, error) {
				return waitUntilSVCReady(i, YurtCoordinatorAPIServerSVC, c)
			},
		},
		{
			CertName:     "kubeconfig",
			SecretName:   YurtCoordinatorMonitoringKubeconfigSecretName,
			IsKubeConfig: true,
			ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
			CommonName:   KubeConfigMonitoringClientCN,
			Organization: []string{YurtCoordinatorOrg},
			// As a clientAuth cert, kubeconfig cert don't need IP&DNS to work,
			// but kubeconfig need this extra information to verify if it's out of date
			certInit: func(i kubernetes.Interface, c <-chan struct{}) ([]net.IP, []string, error) {
				return waitUntilSVCReady(i, YurtCoordinatorAPIServerSVC, c)
			},
		},
		{
			CertName:     "admin.conf",
			SecretName:   YurtCoordinatorStaticSecretName,
			IsKubeConfig: true,
			ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
			CommonName:   KubeConfigAdminClientCN,
			Organization: []string{YurtCoordinatorAdminOrg},
			certInit: func(i kubernetes.Interface, c <-chan struct{}) ([]net.IP, []string, error) {
				return waitUntilSVCReady(i, YurtCoordinatorAPIServerSVC, c)
			},
		},
	}
)

func Format(format string, args ...interface{}) string {
	s := fmt.Sprintf(format, args...)
	return fmt.Sprintf("%s: %s", names.YurtCoordinatorCertController, s)
}

// Add creates a new YurtCoordinatorcert Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(ctx context.Context, cfg *appconfig.CompletedConfig, mgr manager.Manager) error {
	kubeClient, err := kubernetes.NewForConfig(yurtClient.GetConfigByControllerNameOrDie(mgr, names.YurtCoordinatorCertController))
	if err != nil {
		klog.Errorf("could not create kube client, %v", err)
		return err
	}

	r := &ReconcileYurtCoordinatorCert{
		kubeClient: kubeClient,
	}

	// Create a new controller
	c, err := controller.New(names.YurtCoordinatorCertController, mgr, controller.Options{
		Reconciler: r, MaxConcurrentReconciles: int(cfg.ComponentConfig.YurtCoordinatorCertController.ConcurrentCoordinatorCertWorkers),
	})
	if err != nil {
		return err
	}

	// init global variables
	YurtCoordinatorNS = cfg.ComponentConfig.Generic.WorkingNamespace

	// prepare ca certs for yurt coordinator
	caCert, caKey, reuseCA, err := initCA(r.kubeClient)
	if err != nil {
		return errors.Wrap(err, "init yurtcoordinator failed")
	}
	r.caCert = caCert
	r.caKey = caKey
	r.reuseCA = reuseCA

	// prepare all independent certs
	if err := r.initYurtCoordinator(allIndependentCerts, nil); err != nil {
		return err
	}

	// prepare ca cert in static secret
	if err := WriteCertAndKeyIntoSecret(r.kubeClient, "ca", YurtCoordinatorStaticSecretName, r.caCert, nil); err != nil {
		return err
	}

	// prepare ca cert in yurthub secret
	if err := WriteCertAndKeyIntoSecret(r.kubeClient, "ca", YurtCoordinatorYurthubClientSecretName, r.caCert, nil); err != nil {
		return err
	}

	// prepare sa key pairs
	if err := initSAKeyPair(r.kubeClient, "sa", YurtCoordinatorStaticSecretName); err != nil {
		return err
	}

	// watch yurt coordinator service
	svcReadyPredicates := predicate.Funcs{
		CreateFunc: func(evt event.CreateEvent) bool {
			if svc, ok := evt.Object.(*corev1.Service); ok {
				return isYurtCoordinatorSvc(svc)
			}
			return false
		},
		UpdateFunc: func(evt event.UpdateEvent) bool {
			if svc, ok := evt.ObjectNew.(*corev1.Service); ok {
				return isYurtCoordinatorSvc(svc)
			}
			return false
		},
		DeleteFunc: func(evt event.DeleteEvent) bool {
			return false
		},
	}
	return c.Watch(source.Kind[client.Object](mgr.GetCache(), &corev1.Service{}, &handler.EnqueueRequestForObject{}, svcReadyPredicates))
}

func isYurtCoordinatorSvc(svc *corev1.Service) bool {
	if svc == nil {
		return false
	}

	if svc.Namespace == YurtCoordinatorNS && (svc.Name == YurtCoordinatorAPIServerSVC || svc.Name == YurtCoordinatorETCDSVC) {
		return true
	}

	return false
}

var _ reconcile.Reconciler = &ReconcileYurtCoordinatorCert{}

// ReconcileYurtCoordinatorCert reconciles a YurtCoordinatorcert object
type ReconcileYurtCoordinatorCert struct {
	kubeClient kubernetes.Interface
	caCert     *x509.Certificate
	caKey      crypto.Signer
	reuseCA    bool
}

// +kubebuilder:rbac:groups=certificates.k8s.io,resources=certificatesigningrequests,verbs=create
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;update;create;patch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get
// +kubebuilder:rbac:groups="",resources=services,verbs=get

// todo: make customized certificate for each yurtcoordinator pod
func (r *ReconcileYurtCoordinatorCert) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {

	// Note !!!!!!!!!!
	// We strongly recommend use Format() to  encapsulation because Format() can print logs by module
	// @kadisi
	klog.Info(Format("Reconcile YurtCoordinatorCert %s/%s", request.Namespace, request.Name))
	// 1. prepare apiserver-kubelet-client cert
	if err := initAPIServerClientCert(r.kubeClient, ctx.Done()); err != nil {
		return reconcile.Result{}, err
	}

	// 2. prepare node-lease-proxy-client cert
	if err := initNodeLeaseProxyClient(r.kubeClient, ctx.Done()); err != nil {
		return reconcile.Result{}, err
	}

	// 3. prepare certs based on service
	if request.NamespacedName.Namespace == YurtCoordinatorNS {
		if request.NamespacedName.Name == YurtCoordinatorAPIServerSVC {
			return reconcile.Result{}, r.initYurtCoordinator(certsDependOnAPIServerSvc, ctx.Done())
		} else if request.NamespacedName.Name == YurtCoordinatorETCDSVC {
			return reconcile.Result{}, r.initYurtCoordinator(certsDependOnETCDSvc, ctx.Done())
		}
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileYurtCoordinatorCert) initYurtCoordinator(allSelfSignedCerts []CertConfig, stopCh <-chan struct{}) error {

	klog.Info(Format("init yurtcoordinator started"))
	// Prepare certs used by yurtcoordinators

	// prepare selfsigned certs
	var selfSignedCerts []CertConfig

	if r.reuseCA {
		// if CA is reused
		// then we can check if there are selfsigned certs can be reused too
		for _, certConf := range allSelfSignedCerts {

			// 1.1 check if cert exist
			cert, _, err := loadCertAndKeyFromSecret(r.kubeClient, certConf)
			if err != nil {
				klog.Info(Format("can not load cert %s from %s secret", certConf.CertName, certConf.SecretName))
				selfSignedCerts = append(selfSignedCerts, certConf)
				continue
			}

			// 1.2 check if cert is authorized by current CA
			if !IsCertFromCA(cert, r.caCert) {
				klog.Info(Format("existing cert %s is not authorized by current CA", certConf.CertName))
				selfSignedCerts = append(selfSignedCerts, certConf)
				continue
			}

			// 1.3 check has dynamic attrs changed
			if certConf.certInit != nil {
				// receive dynamic IP addresses
				ips, _, err := certConf.certInit(r.kubeClient, stopCh)
				if err != nil {
					// if cert init failed, skip this cert
					klog.Error(Format("could not init cert %s when checking dynamic attrs: %v", certConf.CertName, err))
					continue
				} else {
					// check if dynamic IP addresses already exist in cert
					changed := ip.SearchAllIP(cert.IPAddresses, ips)
					if changed {
						klog.Info(Format("cert %s IP has changed", certConf.CertName))
						selfSignedCerts = append(selfSignedCerts, certConf)
						continue
					}
				}
			}

			klog.Info(Format("cert %s not change, reuse it", certConf.CertName))
		}
	} else {
		// create all certs with new CA
		selfSignedCerts = allSelfSignedCerts
	}

	// create selfsigned certs
	for _, certConf := range selfSignedCerts {
		if err := initYurtCoordinatorCert(r.kubeClient, certConf, r.caCert, r.caKey, stopCh); err != nil {
			klog.Error(Format("create cert %s fail: %v", certConf.CertName, err))
			return err
		}
	}

	return nil
}

// initCA is used for preparing CA certs,
// check if yurt-coordinator CA already exist, if not create one
func initCA(clientSet kubernetes.Interface) (caCert *x509.Certificate, caKey crypto.Signer, reuse bool, err error) {
	// try load CA cert&key from secret
	caCert, caKey, err = loadCertAndKeyFromSecret(clientSet, CertConfig{
		SecretName:   YurtCoordinatorCASecretName,
		CertName:     "ca",
		IsKubeConfig: false,
	})

	if err == nil {
		// if CA already exist
		klog.Info(Format("CA already exist in secret, reuse it"))
		return caCert, caKey, true, nil
	} else {
		// if ca secret does not exist, create new CA certs
		klog.Info(Format("secret(%s/%s) is not found, create new CA", YurtCoordinatorNS, YurtCoordinatorCASecretName))
		// write it into the secret
		caCert, caKey, err = NewSelfSignedCA()
		if err != nil {
			return nil, nil, false, errors.Wrap(err, "could not new self CA assets when initializing yurtcoordinator")
		}

		err = WriteCertAndKeyIntoSecret(clientSet, "ca", YurtCoordinatorCASecretName, caCert, caKey)
		if err != nil {
			return nil, nil, false, errors.Wrap(err, "could not write CA assets into secret when initializing yurtcoordinator")
		}
	}

	return caCert, caKey, false, nil
}

func initAPIServerClientCert(clientSet kubernetes.Interface, stopCh <-chan struct{}) error {
	if cert, _, err := loadCertAndKeyFromSecret(clientSet, CertConfig{
		SecretName:   YurtCoordinatorStaticSecretName,
		CertName:     "apiserver-kubelet-client",
		IsKubeConfig: false,
	}); cert != nil {
		klog.Infof("apiserver-kubelet-client cert has already existed in secret %s", YurtCoordinatorStaticSecretName)
		return nil
	} else if err != nil {
		klog.Errorf("could not get apiserver-kubelet-client cert in secret(%s), %v, and new cert will be created", YurtCoordinatorStaticSecretName, err)
	}

	certMgr, err := certfactory.NewCertManagerFactory(clientSet).New(&certfactory.CertManagerConfig{
		CertDir:        certDir,
		ComponentName:  fmt.Sprintf("%s-%s", ComponentName, "apiserver-client"),
		SignerName:     certificatesv1.KubeAPIServerClientSignerName,
		ForServerUsage: false,
		CommonName:     YurtCoordinatorAPIServerCN,
		Organizations:  []string{YurtCoordinatorOrg},
	})
	if err != nil {
		return err
	}

	return WriteCertIntoSecret(clientSet, "apiserver-kubelet-client", YurtCoordinatorStaticSecretName, certMgr, stopCh)
}

func initNodeLeaseProxyClient(clientSet kubernetes.Interface, stopCh <-chan struct{}) error {
	if cert, _, err := loadCertAndKeyFromSecret(clientSet, CertConfig{
		SecretName:   YurtCoordinatorYurthubClientSecretName,
		CertName:     "node-lease-proxy-client",
		IsKubeConfig: false,
	}); cert != nil {
		klog.Infof("node-lease-proxy-client cert has already existed in secret %s", YurtCoordinatorYurthubClientSecretName)
		return nil
	} else if err != nil {
		klog.Errorf("could not get node-lease-proxy-client cert in secret(%s), %v, and new cert will be created", YurtCoordinatorYurthubClientSecretName, err)
	}

	certMgr, err := certfactory.NewCertManagerFactory(clientSet).New(&certfactory.CertManagerConfig{
		CertDir:       certDir,
		ComponentName: "yurthub",
		SignerName:    certificatesv1.KubeAPIServerClientSignerName,
		CommonName:    YurtCoordinatorNodeLeaseProxyClientCN,
		Organizations: []string{YurtCoordinatorOrg},
	})
	if err != nil {
		return err
	}

	return WriteCertIntoSecret(clientSet, "node-lease-proxy-client", YurtCoordinatorYurthubClientSecretName, certMgr, stopCh)
}

// create new public/private key pair for signing service account users
// and write them into secret
func initSAKeyPair(clientSet kubernetes.Interface, keyName, secretName string) (err error) {
	key, err := NewPrivateKey()
	if err != nil {
		return errors.Wrap(err, "could not create sa key pair")
	}

	return WriteKeyPairIntoSecret(clientSet, secretName, keyName, key)
}
