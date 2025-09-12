/*
Copyright 2023 The OpenYurt Authors.

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

package yurthub

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/node-servant/static-pod-upgrade/util"
	kubeconfigutil "github.com/openyurtio/openyurt/pkg/util/kubeconfig"
	"github.com/openyurtio/openyurt/pkg/util/templates"
	"github.com/openyurtio/openyurt/pkg/util/token"
	"github.com/openyurtio/openyurt/pkg/yurtadm/cmd/join/joindata"
	"github.com/openyurtio/openyurt/pkg/yurtadm/constants"

	yurtadmutil "github.com/openyurtio/openyurt/pkg/yurtadm/util"
	"github.com/openyurtio/openyurt/pkg/yurtadm/util/edgenode"
)

// CheckAndInstallYurthub install yurthub binary, skip install if it exists.
// func CheckAndInstallYurthub(yurthubResourceServer, yurthubVersion string) error {
func CheckAndInstallYurthub() error {
	// klog.Infof("Check and install yurthub %s", yurthubVersion)
	// if yurthubVersion == "" {
	// 	return ErrClusterVersionEmpty
	// }

	yurthubExist := false
	if _, err := exec.LookPath(constants.YurthubExecStart); err == nil {
		// yurthub binary already exists
		klog.Infof("Yurthub binary already exists, skip install.")
		yurthubExist = true
	}

	if !yurthubExist {
		// Download and install yurthub
		// packageUrl := fmt.Sprintf("%s/yurthub/%s/%s/yurthub", yurthubResourceServer, yurthubVersion, runtime.GOARCH)
		packageUrl := constants.YurtHubExecInstallUrlFormat
		savePath := fmt.Sprintf("%s/yurthub", constants.TmpDownloadDir)
		klog.V(1).Infof("Download yurthub from: %s", packageUrl)
		if err := yurtadmutil.DownloadFile(packageUrl, savePath, 3); err != nil {
			return fmt.Errorf("download yurthub fail: %w", err)
		}
		// copy to /usr/bin/yurthub and set executable permission
		if err := edgenode.CopyFile(savePath, constants.YurthubExecStart, 0755); err != nil {
			return err
		}
	}

	return nil
}

// creates the systemd service configuration of YurtHub
func CreateYurthubSystemdService(data joindata.YurtJoinData) error {
	nodePoolName := data.NodeRegistration().NodePoolName
	// If nodePoolName is empty, use the default value "default"
	if nodePoolName == "" {
		nodePoolName = "default"
	}
	// generate systemd service content
	ctx := map[string]string{
		"execStart":     constants.YurthubExecStart,
		"bindAddress":   "127.0.0.1",
		"serverAddr":    fmt.Sprintf("https://%s", data.ServerAddr()),
		"nodeName":      data.NodeRegistration().Name,
		"nodePoolName":  nodePoolName,
		"bootstrapFile": constants.YurtHubBootstrapConfig,
		"workingMode":   data.NodeRegistration().WorkingMode,
		"namespace":     data.Namespace(),
	}

	serviceContent, err := templates.SubstituteTemplate(constants.YurtHubSystemServiceTemplate, ctx)
	if err != nil {
		return err
	}

	serviceFile := "/etc/systemd/system/yurthub.service"
	if err := os.WriteFile(serviceFile, []byte(serviceContent), 0644); err != nil {
		return err
	}

	cmd := exec.Command("systemctl", "daemon-reload")
	if err := cmd.Run(); err != nil {
		return err
	}

	cmd = exec.Command("systemctl", "enable", "yurthub.service")
	if err := cmd.Run(); err != nil {
		return err
	}

	cmd = exec.Command("systemctl", "start", "yurthub.service")
	if err := cmd.Run(); err != nil {
		return err
	}

	return nil
}


// AddYurthubStaticYaml generate YurtHub static yaml for worker node.
func AddYurthubStaticYaml(data joindata.YurtJoinData, podManifestPath string) error {
	klog.Info("[join-node] Adding edge hub static yaml")
	if _, err := os.Stat(podManifestPath); err != nil {
		if os.IsNotExist(err) {
			err = os.MkdirAll(podManifestPath, os.ModePerm)
			if err != nil {
				return err
			}
		} else {
			klog.Errorf("Describe dir %s fail: %v", podManifestPath, err)
			return err
		}
	}

	// There can be multiple master IP addresses
	serverAddrs := strings.Split(data.ServerAddr(), ",")
	for i := 0; i < len(serverAddrs); i++ {
		serverAddrs[i] = fmt.Sprintf("https://%s", serverAddrs[i])
	}

	kubernetesServerAddrs := strings.Join(serverAddrs, ",")

	ctx := map[string]string{
		"yurthubBindingAddr":   data.YurtHubServer(),
		"kubernetesServerAddr": kubernetesServerAddrs,
		"workingMode":          data.NodeRegistration().WorkingMode,
		"organizations":        data.NodeRegistration().Organizations,
		"namespace":            data.Namespace(),
		"image":                data.YurtHubImage(),
	}
	if len(data.NodeRegistration().NodePoolName) != 0 {
		ctx["nodePoolName"] = data.NodeRegistration().NodePoolName
	}

	yurthubTemplate, err := templates.SubstituteTemplate(data.YurtHubTemplate(), ctx)
	if err != nil {
		return err
	}

	yurthubTemplate, err = useRealServerAddr(yurthubTemplate, kubernetesServerAddrs)
	if err != nil {
		return err
	}

	yurthubManifestFile := filepath.Join(podManifestPath, util.WithYamlSuffix(data.YurtHubManifest()))
	klog.Infof("yurthub template: %s\n%s", yurthubManifestFile, yurthubTemplate)

	if err := os.WriteFile(yurthubManifestFile, []byte(yurthubTemplate), 0600); err != nil {
		return err
	}
	klog.Info("[join-node] Add hub agent static yaml is ok")
	return nil
}

func SetHubBootstrapConfig(serverAddr string, joinToken string, caCertHashes []string) error {
	if cfg, err := token.RetrieveValidatedConfigInfo(nil, &token.BootstrapData{
		ServerAddr:   serverAddr,
		JoinToken:    joinToken,
		CaCertHashes: caCertHashes,
	}); err != nil {
		return errors.Wrap(err, "couldn't retrieve bootstrap config info")
	} else {
		clusterInfo := kubeconfigutil.GetClusterFromKubeConfig(cfg)
		tlsBootstrapCfg := kubeconfigutil.CreateWithToken(
			fmt.Sprintf("https://%s", serverAddr),
			"kubernetes",
			"token-bootstrap-client",
			clusterInfo.CertificateAuthorityData,
			joinToken,
		)

		// make sure the parent directory of YurtHubBootstrapConfig exists
		if err := os.MkdirAll(filepath.Dir(constants.YurtHubBootstrapConfig), os.ModePerm); err != nil {
			return err
		}

		if err = kubeconfigutil.WriteToDisk(constants.YurtHubBootstrapConfig, tlsBootstrapCfg); err != nil {
			return errors.Wrap(err, "couldn't save bootstrap-hub.conf to disk")
		}
	}

	return nil
}

// Check the health status of the YurtHub service
func CheckYurthubServiceHealth(yurthubServer string) error {
	// Checking the systemd service status
	cmd := exec.Command("systemctl", "is-active", "yurthub.service")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("yurthub service is not active: %v", err)
	}

	// Check the YurtHub health endpoint
	if err := CheckYurthubHealthz(yurthubServer); err != nil { // Here is the previous CheckYurthubHealthz, called in postcheck.go
		return err
	}

	return nil
}

// CheckYurthubHealthz check if YurtHub is healthy.
func CheckYurthubHealthz(yurthubServer string) error {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s%s", fmt.Sprintf("%s:10267", yurthubServer), constants.ServerHealthzURLPath), nil)
	if err != nil {
		return err
	}
	client := &http.Client{}
	return wait.PollUntilContextTimeout(context.Background(), time.Second*5, 300*time.Second, true, func(ctx context.Context) (bool, error) {
		resp, err := client.Do(req)
		if err != nil {
			return false, nil
		}
		ok, err := io.ReadAll(resp.Body)
		if err != nil {
			return false, nil
		}
		return string(ok) == "OK", nil
	})
}

// CheckYurthubReadyz check if YurtHub's certificates are ready or not
func CheckYurthubReadyz(yurthubServer string) error {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s%s", fmt.Sprintf("%s:10267", yurthubServer), constants.ServerReadyzURLPath), nil)
	if err != nil {
		return err
	}
	client := &http.Client{}
	return wait.PollUntilContextTimeout(context.Background(), time.Second*5, 300*time.Second, true, func(ctx context.Context) (bool, error) {
		resp, err := client.Do(req)
		if err != nil {
			return false, nil
		}
		ok, err := io.ReadAll(resp.Body)
		if err != nil {
			return false, nil
		}
		return string(ok) == "OK", nil
	})
}

func CheckYurthubReadyzOnce(yurthubServer string) bool {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s%s", fmt.Sprintf("%s:10267", yurthubServer), constants.ServerReadyzURLPath), nil)
	if err != nil {
		return false
	}
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	ok, err := io.ReadAll(resp.Body)
	if err != nil {
		return false
	}
	return string(ok) == "OK"
}

func CleanHubBootstrapConfig() error {
	if err := os.RemoveAll(constants.YurtHubBootstrapConfig); err != nil {
		klog.Warningf("Clean file %s fail: %v, please clean it manually.", constants.YurtHubBootstrapConfig, err)
	}
	return nil
}

// useRealServerAddr check if the server-addr from yurthubTemplate is default value: 127.0.0.1:6443
// if yes, we should use the real server addr
func useRealServerAddr(yurthubTemplate string, kubernetesServerAddrs string) (string, error) {
	scanner := bufio.NewScanner(bytes.NewReader([]byte(yurthubTemplate)))
	var buffer bytes.Buffer
	// compile ipv4 regex
	ipRegex := regexp.MustCompile(`https?://(?:[0-9]{1,3}\.){3}[0-9]{1,3}:\d+`)

	// scan template and replace setAddr
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, fmt.Sprintf("- --%s=", constants.ServerAddr)) {
			// replace kubernetesServerAddrs by new addr
			line = ipRegex.ReplaceAllString(line, kubernetesServerAddrs)
		}
		buffer.WriteString(line + "\n")
	}

	if err := scanner.Err(); err != nil {
		klog.Infof("Error scanning file: %v\n", err)
		return "", err
	}
	return buffer.String(), nil
}

func CheckYurtHubItself(ns, name string) bool {
	if ns == constants.YurthubNamespace &&
		(name == constants.YurthubYurtStaticSetName || name == constants.YurthubCloudYurtStaticSetName) {
		return true
	}
	return false
}
