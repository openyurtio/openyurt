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

package components

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
	kubeconfigutil "github.com/openyurtio/openyurt/pkg/util/kubeconfig"
	"github.com/openyurtio/openyurt/pkg/yurtadm/constants"
	enutil "github.com/openyurtio/openyurt/pkg/yurtadm/util/edgenode"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage/disk"
)

const (
	hubHealthzCheckFrequency = 10 * time.Second
	fileMode                 = 0666
	DefaultRootDir           = "/var/lib"
	DefaultCaPath            = "/etc/kubernetes/pki/ca.crt"
	yurthubYurtStaticSetName = "yurthub"
	defaultConfigmapPath     = "/data"
)

type yurtHubOperator struct {
	apiServerAddr             string
	joinToken                 string
	yurthubHealthCheckTimeout time.Duration
}

// NewYurthubOperator new yurtHubOperator struct
func NewYurthubOperator(apiServerAddr string, joinToken string, yurthubHealthCheckTimeout time.Duration) *yurtHubOperator {
	return &yurtHubOperator{
		apiServerAddr:             apiServerAddr,
		joinToken:                 joinToken,
		yurthubHealthCheckTimeout: yurthubHealthCheckTimeout,
	}
}

// Install set yurthub yaml to static path to start pod
func (op *yurtHubOperator) Install() error {

	// 1. put yurt-hub yaml into /etc/kubernetes/manifests
	klog.Infof("setting up yurthub on node")
	// 1-1. get configmap data path
	configMapDataPath := filepath.Join(defaultConfigmapPath, yurthubYurtStaticSetName)

	// 1-2. create /var/lib/yurthub/bootstrap-hub.conf
	if err := enutil.EnsureDir(constants.YurtHubWorkdir); err != nil {
		return err
	}
	if err := setHubBootstrapConfig(op.apiServerAddr, op.joinToken); err != nil {
		return err
	}
	klog.Infof("create the %s", constants.YurtHubBootstrapConfig)

	// 1-3. create yurthub.yaml
	podManifestPath := enutil.GetPodManifestPath()
	if err := enutil.EnsureDir(podManifestPath); err != nil {
		return err
	}
	content, err := os.ReadFile(configMapDataPath)
	if err != nil {
		return fmt.Errorf("could not read source file %s: %w", configMapDataPath, err)
	}
	klog.Infof("yurt-hub.yaml apiServerAddr: %+v", op.apiServerAddr)
	yssYurtHub := strings.ReplaceAll(string(content), "KUBERNETES_SERVER_ADDRESS", op.apiServerAddr)
	if err = os.WriteFile(getYurthubYaml(podManifestPath), []byte(yssYurtHub), fileMode); err != nil {
		return err
	}

	klog.Infof("create the %s/yurt-hub.yaml", podManifestPath)
	klog.Infof("yurt-hub.yaml: %+v", configMapDataPath)
	klog.Infof("yurt-hub.yaml content: %+v", yssYurtHub)

	// 2. wait yurthub pod to be ready
	return hubHealthcheck(op.yurthubHealthCheckTimeout)
}

// UnInstall remove yaml and configs of yurthub
func (op *yurtHubOperator) UnInstall() error {
	// 1. remove the yurt-hub.yaml to delete the yurt-hub
	podManifestPath := enutil.GetPodManifestPath()
	yurthubYamlPath := getYurthubYaml(podManifestPath)
	if _, err := enutil.FileExists(yurthubYamlPath); os.IsNotExist(err) {
		klog.Infof("UnInstallYurthub: %s is not exists, skip delete", yurthubYamlPath)
	} else {
		err := os.Remove(yurthubYamlPath)
		if err != nil {
			return err
		}
		klog.Infof("UnInstallYurthub: %s has been removed", yurthubYamlPath)
	}

	// 2. remove yurt-hub config directory and certificates in it
	yurthubConf := getYurthubConf()
	if _, err := enutil.FileExists(yurthubConf); os.IsNotExist(err) {
		klog.Infof("UnInstallYurthub: dir %s is not exists, skip delete", yurthubConf)
		return nil
	}
	err := os.RemoveAll(yurthubConf)
	if err != nil {
		return err
	}
	klog.Infof("UnInstallYurthub: config dir %s  has been removed", yurthubConf)

	// 3. remove yurthub cache dir
	// since k8s may takes a while to notice and remove yurthub pod, we have to wait for that.
	// because, if we delete dir before yurthub exit, yurthub may recreate cache/kubelet dir before exit.
	err = waitUntilYurthubExit(time.Duration(60)*time.Second, time.Duration(1)*time.Second)
	if err != nil {
		return err
	}
	cacheDir := getYurthubCacheDir()
	err = os.RemoveAll(cacheDir)
	if err != nil {
		return err
	}
	klog.Infof("UnInstallYurthub: cache dir %s  has been removed", cacheDir)

	return nil
}

func getYurthubYaml(podManifestPath string) string {
	return filepath.Join(podManifestPath, constants.YurthubYamlName)
}

func getYurthubConf() string {
	return filepath.Join(DefaultRootDir, projectinfo.GetHubName())
}

func getYurthubCacheDir() string {
	// get default dir
	return disk.CacheBaseDir
}

func waitUntilYurthubExit(timeout time.Duration, period time.Duration) error {
	klog.Info("wait for yurt-hub exit")
	serverHealthzURL, _ := url.Parse(fmt.Sprintf("http://%s", constants.ServerHealthzServer))
	serverHealthzURL.Path = constants.ServerHealthzURLPath

	return wait.PollUntilContextTimeout(context.Background(), period, timeout, true, func(ctx context.Context) (bool, error) {
		_, err := pingClusterHealthz(http.DefaultClient, serverHealthzURL.String())
		if err != nil { // means yurthub has exited
			klog.Infof("yurt-hub is not running, with ping result: %v", err)
			return true, nil
		}
		klog.Infof("yurt-hub is still running")
		return false, nil
	})
}

// hubHealthcheck will check the status of yurthub pod
func hubHealthcheck(timeout time.Duration) error {
	serverHealthzURL, err := url.Parse(fmt.Sprintf("http://%s", constants.ServerHealthzServer))
	if err != nil {
		return err
	}
	serverHealthzURL.Path = constants.ServerHealthzURLPath

	start := time.Now()
	return wait.PollUntilContextTimeout(context.Background(), hubHealthzCheckFrequency, timeout, true, func(ctx context.Context) (bool, error) {
		_, err := pingClusterHealthz(http.DefaultClient, serverHealthzURL.String())
		if err != nil {
			klog.Infof("yurt-hub is not ready, ping cluster healthz with result: %v", err)
			return false, nil
		}
		klog.Infof("yurt-hub healthz is OK after %f seconds", time.Since(start).Seconds())
		return true, nil
	})
}

func pingClusterHealthz(client *http.Client, addr string) (bool, error) {
	if client == nil {
		return false, fmt.Errorf("http client is invalid")
	}

	resp, err := client.Get(addr)
	if err != nil {
		return false, err
	}

	b, err := io.ReadAll(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		return false, fmt.Errorf("could not read response of cluster healthz, %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("response status code is %d", resp.StatusCode)
	}

	if strings.ToLower(string(b)) != "ok" {
		return false, fmt.Errorf("cluster healthz is %s", string(b))
	}

	return true, nil
}

func setHubBootstrapConfig(serverAddr string, joinToken string) error {
	caData, err := os.ReadFile(DefaultCaPath)
	if err != nil {
		return err
	}
	tlsBootstrapCfg := kubeconfigutil.CreateWithToken(
		serverAddr,
		"openyurt-e2e-test",
		"token-bootstrap-client",
		caData,
		joinToken,
	)
	content, err := clientcmd.Write(*tlsBootstrapCfg)
	if err != nil {
		return err
	}
	if err := os.WriteFile(constants.YurtHubBootstrapConfig, content, fileMode); err != nil {
		return errors.Wrap(err, "couldn't save bootstrap-hub.conf to disk")
	}

	return nil
}
