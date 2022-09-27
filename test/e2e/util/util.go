/*
Copyright 2021 The OpenYurt Authors.

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

package util

import (
	"context"
	"flag"
	"fmt"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/test/e2e/yurtconfig"
)

const (
	// DefaultNamespaceDeletionTimeout is timeout duration for waiting for a namespace deletion.
	DefaultNamespaceDeletionTimeout = 5 * time.Minute

	// PodStartTimeout is how long to wait for the pod to be started.
	PodStartTimeout = 5 * time.Minute
)

var EnableYurtAutonomy = flag.Bool("enable-yurt-autonomy", false, "switch of yurt node autonomy. If set to true, yurt node autonomy test can be run normally")
var RegionID = flag.String("region-id", "", "aliyun region id for ailunyun:ecs/ens")
var NodeType = flag.String("node-type", "minikube", "node type such as ailunyun:ecs/ens, minikube and user_self")
var AccessKeyID = flag.String("access-key-id", "", "aliyun AccessKeyId  for ailunyun:ecs/ens")
var AccessKeySecret = flag.String("access-key-secret", "", "aliyun AccessKeySecret  for ailunyun:ecs/ens")
var Kubeconfig = flag.String("kubeconfig", "", "kubeconfig file path for OpenYurt cluster")
var ReportDir = flag.String("report-dir", "", "Path to the directory where the JUnit XML reports should be saved. Default is empty, which doesn't generate these reports.")

// LoadRestConfigAndClientset returns rest config and  clientset for connecting to kubernetes clusters.
func LoadRestConfigAndClientset(kubeconfig string) (*restclient.Config, *clientset.Clientset, error) {
	config, err := LoadRESTClientConfigFromEnv(kubeconfig)
	if err != nil {
		return nil, nil, fmt.Errorf("error load rest client config: %w", err)
	}

	client, err := clientset.NewForConfig(config)
	if err != nil {
		return nil, nil, fmt.Errorf("error new clientset: %w", err)
	}

	return config, client, nil
}

// WaitForNamespacesDeleted waits for the namespaces to be deleted.
func WaitForNamespacesDeleted(c clientset.Interface, namespaces []string, timeout time.Duration) error {
	klog.Infof("Waiting for namespaces to vanish")
	nsMap := map[string]bool{}
	for _, ns := range namespaces {
		nsMap[ns] = true
	}
	//Now POLL until all namespaces have been eradicated.
	return wait.Poll(2*time.Second, timeout,
		func() (bool, error) {
			nsList, err := c.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
			if err != nil {
				return false, err
			}
			for _, item := range nsList.Items {
				if _, ok := nsMap[item.Name]; ok {
					return false, nil
				}
			}
			return true, nil
		})
}

//Set up yurt-e2e-config for e2e-tests
func SetYurtE2eCfg() error {
	yurtconfig.YurtE2eCfg.NodeType = strings.ToLower(*NodeType)
	yurtconfig.YurtE2eCfg.RegionID = *RegionID
	yurtconfig.YurtE2eCfg.EnableYurtAutonomy = *EnableYurtAutonomy
	yurtconfig.YurtE2eCfg.AccessKeyID = *AccessKeyID
	yurtconfig.YurtE2eCfg.AccessKeySecret = *AccessKeySecret

	config, client, err := LoadRestConfigAndClientset(*Kubeconfig)
	if err != nil {
		klog.Errorf("pre_check_load_client_set failed errmsg:%v", err)
		return err
	}
	yurtconfig.YurtE2eCfg.KubeClient = client
	yurtconfig.YurtE2eCfg.RestConfig = config
	yurtconfig.YurtE2eCfg.ReportDir = *ReportDir

	return nil
}

// Load restClientConfig from env
func LoadRESTClientConfigFromEnv(kubeconfig string) (*restclient.Config, error) {
	// Load structured kubeconfig data from env path.
	loader := clientcmd.NewDefaultClientConfigLoadingRules()
	loadedConfig, err := loader.Load()
	if err != nil {
		return nil, err
	}
	// Flatten the loaded data to a particular restclient.Config based on the current context.
	return clientcmd.NewNonInteractiveClientConfig(
		*loadedConfig,
		loadedConfig.CurrentContext,
		&clientcmd.ConfigOverrides{},
		loader,
	).ClientConfig()
}
