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
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/util/templates"
	"github.com/openyurtio/openyurt/pkg/yurtadm/constants"
	enutil "github.com/openyurtio/openyurt/pkg/yurtadm/util/edgenode"
	"github.com/openyurtio/openyurt/pkg/yurthub/certificate/token"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage/disk"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

const (
	hubHealthzCheckFrequency = 10 * time.Second
	fileMode                 = 0666
)

type yurtHubOperator struct {
	apiServerAddr             string
	yurthubImage              string
	joinToken                 string
	workingMode               util.WorkingMode
	yurthubHealthCheckTimeout time.Duration
	enableDummyIf             bool
	enableNodePool            bool
}

// NewYurthubOperator new yurtHubOperator struct
func NewYurthubOperator(apiServerAddr string, yurthubImage string, joinToken string,
	workingMode util.WorkingMode, yurthubHealthCheckTimeout time.Duration, enableDummyIf, enableNodePool bool) *yurtHubOperator {
	return &yurtHubOperator{
		apiServerAddr:             apiServerAddr,
		yurthubImage:              yurthubImage,
		joinToken:                 joinToken,
		workingMode:               workingMode,
		yurthubHealthCheckTimeout: yurthubHealthCheckTimeout,
		enableDummyIf:             enableDummyIf,
		enableNodePool:            enableNodePool,
	}
}

// Install set yurthub yaml to static path to start pod
func (op *yurtHubOperator) Install() error {

	// 1. put yurt-hub yaml into /etc/kubernetes/manifests
	klog.Infof("setting up yurthub on node")
	// 1-1. replace variables in yaml file
	klog.Infof("setting up yurthub apiServer addr")
	yurthubTemplate, err := templates.SubsituteTemplate(constants.YurthubTemplate, map[string]string{
		"yurthubServerAddr":    constants.DefaultYurtHubServerAddr,
		"kubernetesServerAddr": op.apiServerAddr,
		"image":                op.yurthubImage,
		"joinToken":            op.joinToken,
		"workingMode":          string(op.workingMode),
		"enableDummyIf":        strconv.FormatBool(op.enableDummyIf),
		"enableNodePool":       strconv.FormatBool(op.enableNodePool),
	})
	if err != nil {
		return err
	}

	// 1-2. create yurthub.yaml
	podManifestPath := enutil.GetPodManifestPath()
	if err := enutil.EnsureDir(podManifestPath); err != nil {
		return err
	}
	if err := os.WriteFile(getYurthubYaml(podManifestPath), []byte(yurthubTemplate), fileMode); err != nil {
		return err
	}
	klog.Infof("create the %s/yurt-hub.yaml", podManifestPath)

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
	return filepath.Join(token.DefaultRootDir, projectinfo.GetHubName())
}

func getYurthubCacheDir() string {
	// get default dir
	return disk.CacheBaseDir
}

func waitUntilYurthubExit(timeout time.Duration, period time.Duration) error {
	klog.Info("wait for yurt-hub exit")
	serverHealthzURL, _ := url.Parse(fmt.Sprintf("http://%s", constants.ServerHealthzServer))
	serverHealthzURL.Path = constants.ServerHealthzURLPath

	return wait.PollImmediate(period, timeout, func() (bool, error) {
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
	return wait.PollImmediate(hubHealthzCheckFrequency, timeout, func() (bool, error) {
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
		return false, fmt.Errorf("failed to read response of cluster healthz, %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("response status code is %d", resp.StatusCode)
	}

	if strings.ToLower(string(b)) != "ok" {
		return false, fmt.Errorf("cluster healthz is %s", string(b))
	}

	return true, nil
}
