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
	"context"
	"fmt"
	"time"

	pkgerrors "github.com/pkg/errors"
	certificatesv1 "k8s.io/api/certificates/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apiserver/pkg/authentication/user"
	coreinformers "k8s.io/client-go/informers/core/v1"
	client "k8s.io/client-go/kubernetes"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdAPI "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/util/workqueue"
	bootstrapapi "k8s.io/cluster-bootstrap/token/api"
	"k8s.io/klog/v2"

	certfactory "github.com/openyurtio/openyurt/pkg/util/certmanager/factory"
	"github.com/openyurtio/openyurt/pkg/util/ip"
	"github.com/openyurtio/openyurt/pkg/util/kubeconfig"
	"github.com/openyurtio/openyurt/pkg/yurttunnel/server/serveraddr"
)

const (
	commonNamePrefix = "system:node:poolcoordinator"
	certDir          = "/tmp" // tmp file directory for certmanager to write cert files

	ComponentName                      = "yurt-controller-manager_poolcoordinator"
	PoolcoordinatorNS                  = "kube-system"
	PoolcoordinatorAPIServerSVC        = "pool-coordinator-apiserver"
	PoolcoordinatorETCDSVC             = "pool-coordinator-etcd"
	PoolcoordinatorAPIServerSecertName = "pool-coordinator-apiserver-certs"
	PoolcoordinatorETCDSecertName      = "pool-coordinator-etcd-certs"
	PoolcoordinatorAPIServerClientCN   = "openyurt:pool-coordinator:apiserver"

	PoolcoordinatorOrg      = "openyurt:pool-coordinator"
	PoolcoordinatorAdminOrg = "system:masters"

	KubeConfigMonitoringClientCN = "openyurt:pool-coordinator:monitoring"
	KubeConfigAdminClientCN      = "cluster-admin"
)

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

	// Prepare certs shared among all poolcoordinators
	if err := initPoolCoordinatorCerts(c.kubeclientset, stopCh); err != nil {
		klog.Errorf("poolcoordinatorCertManager init certs fail, %v", err)
	}

	// Prepare kubeconfigs
	if err := initPoolCoordinatorKubeConfigs(c.kubeclientset, stopCh); err != nil {
		klog.Errorf("poolcoordinatorCertManager init kubeconfigs fail, %v", err)
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

func initPoolCoordinatorCerts(clientSet client.Interface, stopCh <-chan struct{}) error {
	if err := initAPIServerTLSCert(clientSet, stopCh); err != nil {
		return err
	}
	if err := initAPIServerClientCert(clientSet, stopCh); err != nil {
		return err
	}
	return initETCDServerTLSCert(clientSet, stopCh)
}

func initPoolCoordinatorKubeConfigs(clientSet client.Interface, stopCh <-chan struct{}) error {
	return initPoolCoordinatorMonitoringKubeConfig(clientSet, stopCh)
}

// init tls server certificates for poolcoordinators
func initTLSCert(clientSet client.Interface, serviceName, certName, secretName string, stopCh <-chan struct{}) (err error) {
	var serverSVC *corev1.Service

	// wait until get tls server Service
	err = wait.PollUntil(1*time.Second, func() (bool, error) {
		serverSVC, err = clientSet.CoreV1().Services(PoolcoordinatorNS).Get(context.TODO(), serviceName, metav1.GetOptions{})
		if err == nil {
			klog.Infof("poolcoordinator %s service get successfully", serviceName)
			return true, nil
		}
		klog.Infof("waiting for the poolcoordinator %s service", serviceName)
		return false, nil
	}, stopCh)

	// prepare certmanager
	ips := ip.ParseIPList(serverSVC.Spec.ClusterIPs)
	dnsnames := serveraddr.GetDefaultDomainsForSvc(PoolcoordinatorNS, serviceName)
	serverCertMgr, err := certfactory.NewCertManagerFactory(clientSet).New(&certfactory.CertManagerConfig{
		CertDir:        certDir,
		ComponentName:  fmt.Sprintf("%s-%s", ComponentName, certName),
		SignerName:     certificatesv1.KubeletServingSignerName,
		ForServerUsage: true,
		CommonName:     fmt.Sprintf("%s-%s", commonNamePrefix, certName),
		Organizations:  []string{user.NodesGroup},
		IPs:            ips,
		DNSNames:       dnsnames,
	})

	if err != nil {
		klog.Errorf("create %s serverCertMgr fail", certName)
		return err
	}

	return WriteCertIntoSecret(clientSet, certName, secretName, serverCertMgr, stopCh)
}

func initETCDServerTLSCert(clientSet client.Interface, stopCh <-chan struct{}) (err error) {
	return initTLSCert(clientSet, PoolcoordinatorETCDSVC, "server", PoolcoordinatorETCDSecertName, stopCh)
}

func initAPIServerTLSCert(clientSet client.Interface, stopCh <-chan struct{}) (err error) {
	return initTLSCert(clientSet, PoolcoordinatorAPIServerSVC, "apiserver", PoolcoordinatorAPIServerSecertName, stopCh)
}

func initAPIServerClientCert(clientSet client.Interface, stopCh <-chan struct{}) (err error) {
	certMgr, err := certfactory.NewCertManagerFactory(clientSet).New(&certfactory.CertManagerConfig{
		CertDir:        certDir,
		ComponentName:  fmt.Sprintf("%s-%s", ComponentName, "apiserver-client"),
		SignerName:     certificatesv1.KubeAPIServerClientSignerName,
		ForServerUsage: false,
		CommonName:     PoolcoordinatorAPIServerClientCN,
		Organizations:  []string{PoolcoordinatorOrg},
	})
	if err != nil {
		return err
	}

	return WriteCertIntoSecret(clientSet, "apiserver-kubelet-client", PoolcoordinatorAPIServerSecertName, certMgr, stopCh)
}

// get poolcoordinator apiserver address
func getAPIServerSVCURL(clientSet client.Interface) (string, error) {
	serverSVC, err := clientSet.CoreV1().Services(PoolcoordinatorNS).Get(context.TODO(), PoolcoordinatorAPIServerSVC, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	apiServerURL, _ := GetURLFromSVC(serverSVC)
	return apiServerURL, nil
}

// get CA certs from cluster-info in kube-public namespace
func getCACerts(clientSet client.Interface) ([]byte, error) {
	// get cluster-info
	clusterinfo, err := clientSet.CoreV1().ConfigMaps(metav1.NamespacePublic).Get(context.TODO(), bootstrapapi.ConfigMapClusterInfo, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	// parse kubeconfig
	kubeConfigString, ok := clusterinfo.Data[bootstrapapi.KubeConfigKey]
	if !ok || len(kubeConfigString) == 0 {
		return nil, pkgerrors.Errorf("there is no %s key in the %s ConfigMap.",
			bootstrapapi.KubeConfigKey, bootstrapapi.ConfigMapClusterInfo)
	}

	inSecureConfig, err := clientcmd.Load([]byte(kubeConfigString))
	if err != nil {
		return nil, pkgerrors.Wrapf(err, "couldn't parse the kubeconfig file in the %s ConfigMap", bootstrapapi.ConfigMapClusterInfo)
	}

	var cluster *clientcmdAPI.Cluster
	for _, obj := range inSecureConfig.Clusters {
		cluster = obj
	}

	// get ca cert
	if cluster == nil {
		return nil, pkgerrors.Errorf("there is no item in the clusters.")
	}

	return cluster.CertificateAuthorityData, nil
}

func initPoolCoordinatorKubeConfig(clientSet client.Interface, commonName, orgName, secretName string, stopCh <-chan struct{}) error {

	caCert, err := getCACerts(clientSet)
	if err != nil {
		return pkgerrors.Wrapf(err, "couldn't get CA certs")
	}

	apiServerURL, err := getAPIServerSVCURL(clientSet)
	if err != nil {
		return pkgerrors.Wrapf(err, "couldn't get PoolCoordinator APIServer service url")
	}

	certMgr, err := certfactory.NewCertManagerFactory(clientSet).New(&certfactory.CertManagerConfig{
		CertDir:        certDir,
		ComponentName:  fmt.Sprintf("%s-%s", ComponentName, "kubeconfig-monitoring-client"),
		SignerName:     certificatesv1.KubeAPIServerClientSignerName,
		ForServerUsage: false,
		CommonName:     commonName,
		Organizations:  []string{orgName},
	})
	if err != nil {
		return err
	}

	key, cert, err := GetCertAndKeyFromCertMgr(certMgr, stopCh)
	if err != nil {
		return pkgerrors.Wrapf(err, "couldn't get cert&key from %s certmanager", commonName)
	}

	kubeConfig := kubeconfig.CreateWithCerts(apiServerURL, "cluster", commonName, caCert, key, cert)
	kubeConfigByte, err := clientcmd.Write(*kubeConfig)
	if err != nil {
		return err
	}

	// write certificate data into secret
	secretClient, err := NewSecretClient(clientSet, PoolcoordinatorNS, secretName)
	if err != nil {
		return err
	}
	err = secretClient.AddData("kubeconfig", kubeConfigByte)
	if err != nil {
		return err
	}

	klog.Infof("successfully write %s kubeconfig into secret %s", commonName, secretName)

	return nil
}

// func initPoolCoordinatorAdminKubeConfig(clientSet client.Interface, stopCh <-chan struct{}) error {
// 	return initPoolCoordinatorKubeConfig(clientSet, KubeConfigAdminClientCN, PoolcoordinatorAdminOrg, "poolcoordinator-admin-kubeconfig", stopCh)
// }

func initPoolCoordinatorMonitoringKubeConfig(clientSet client.Interface, stopCh <-chan struct{}) error {
	return initPoolCoordinatorKubeConfig(clientSet, KubeConfigMonitoringClientCN, PoolcoordinatorOrg, "poolcoordinator-monitoring-kubeconfig", stopCh)
}
