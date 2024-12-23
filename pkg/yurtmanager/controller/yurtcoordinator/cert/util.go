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

package yurtcoordinatorcert

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

// get yurtcoordinator apiserver address
func getAPIServerSVCURL(clientSet client.Interface) (string, error) {
	serverSVC, err := clientSet.CoreV1().Services(YurtCoordinatorNS).Get(context.TODO(), YurtCoordinatorAPIServerSVC, metav1.GetOptions{})
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
	if err = wait.PollUntilContextCancel(context.Background(), 1*time.Second, true, func(ctx context.Context) (bool, error) {
		serverSVC, err = clientSet.CoreV1().Services(YurtCoordinatorNS).Get(context.TODO(), serviceName, metav1.GetOptions{})
		if err == nil {
			klog.Info(Format("%s service is ready for yurtcoordinator_cert_manager", serviceName))
			return true, nil
		}
		return false, nil
	}); err != nil {
		return nil, nil, err
	}

	// prepare certmanager
	ips = ip.ParseIPList([]string{serverSVC.Spec.ClusterIP})
	dnsnames = serveraddr.GetDefaultDomainsForSvc(YurtCoordinatorNS, serviceName)

	return ips, dnsnames, nil
}
