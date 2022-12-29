package cert

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	client "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/util/ip"
	"github.com/openyurtio/openyurt/pkg/yurttunnel/server/serveraddr"
)

// get poolcoordinator apiserver address
func getAPIServerSVCURL(clientSet client.Interface) (string, error) {
	serverSVC, err := clientSet.CoreV1().Services(PoolcoordinatorNS).Get(context.TODO(), PoolcoordinatorAPIServerSVC, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	apiServerURL, _ := GetURLFromSVC(serverSVC)
	return apiServerURL, nil
}

func GetURLFromSVC(svc *corev1.Service) (string, error) {
	hostName := svc.Spec.ClusterIP
	if svc.Spec.Ports == nil || len(svc.Spec.Ports) == 0 {
		return "", errors.New("Service port list cannot be empty")
	}
	port := svc.Spec.Ports[0].Port
	return fmt.Sprintf("https://%s:%d", hostName, port), nil
}

func waitUntilSVCReady(clientSet client.Interface, serviceName string, stopCh <-chan struct{}) (ips []net.IP, dnsnames []string, err error) {
	var serverSVC *corev1.Service

	// wait until get tls server Service
	if err = wait.PollUntil(1*time.Second, func() (bool, error) {
		serverSVC, err = clientSet.CoreV1().Services(PoolcoordinatorNS).Get(context.TODO(), serviceName, metav1.GetOptions{})
		if err == nil {
			klog.Infof("%s service is ready for poolcoordinator_cert_manager", serviceName)
			return true, nil
		}
		klog.Infof("waiting for the poolcoordinator %s service", serviceName)
		return false, nil
	}, stopCh); err != nil {
		return nil, nil, err
	}

	// prepare certmanager
	ips = ip.ParseIPList(serverSVC.Spec.ClusterIPs)
	dnsnames = serveraddr.GetDefaultDomainsForSvc(PoolcoordinatorNS, serviceName)

	return ips, dnsnames, nil
}
