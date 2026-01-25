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
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
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

var (
	execCommand             = exec.Command
	lookPath                = exec.LookPath
	checkYurthubHealthzFunc = CheckYurthubHealthz
)

func CheckAndInstallYurthub(yurthubVersion string) error {

	klog.Infof("Check and install yurthub %s", yurthubVersion)
	if yurthubVersion == "" {
		return errors.New("yurthub version should not be empty")
	}

	if _, err := lookPath(constants.YurthubExecStart); err == nil {
		klog.Infof("Yurthub binary already exists, skip install.")
		return nil
	}

	packageUrl := fmt.Sprintf(constants.YurthubExecUrlFormat, constants.YurthubExecResourceServer, yurthubVersion, runtime.GOARCH)
	savePath := fmt.Sprintf("%s/yurthub", constants.TmpDownloadDir)
	klog.V(1).Infof("Download yurthub from: %s", packageUrl)
	if err := yurtadmutil.DownloadFile(packageUrl, savePath, 3); err != nil {
		return fmt.Errorf("download yurthub fail: %w", err)
	}
	if err := edgenode.CopyFile(savePath, constants.YurthubExecStart, 0755); err != nil {
		return err
	}

	return nil
}

func setYurthubMainService() error {
	klog.Info("Setting yurthub main service.")

	serviceFile := constants.YurthubServicePath
	serviceDir := filepath.Dir(serviceFile)

	if _, err := os.Stat(serviceDir); err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(serviceDir, os.ModePerm); err != nil {
				klog.Errorf("Create dir %s fail: %v", serviceDir, err)
				return err
			}
		} else {
			klog.Errorf("Describe dir %s fail: %v", serviceDir, err)
			return err
		}
	}

	if err := os.WriteFile(serviceFile, []byte(constants.YurtHubServiceContent), 0644); err != nil {
		klog.Errorf("Write file %s fail: %v", serviceFile, err)
		return err
	}

	return nil
}

func setYurthubUnitService(data joindata.YurtJoinData) error {
	klog.Info("Setting yurthub unit service.")

	ctx := map[string]string{
		"bindAddress":   "127.0.0.1",
		"serverAddr":    fmt.Sprintf("https://%s", data.ServerAddr()),
		"nodeName":      data.NodeRegistration().Name,
		"nodeIP":        data.NodeIP(),
		"bootstrapFile": constants.YurtHubBootstrapConfig,
		"workingMode":   data.NodeRegistration().WorkingMode,
		"namespace":     data.Namespace(),
	}

	if len(data.NodeRegistration().NodePoolName) != 0 {
		ctx["nodePoolName"] = data.NodeRegistration().NodePoolName
	}

	unitContent, err := templates.SubstituteTemplate(constants.YurtHubUnitConfig, ctx)
	if err != nil {
		return err
	}

	unitDir := filepath.Dir(constants.YurthubServiceConfPath)
	if _, err := os.Stat(unitDir); err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(unitDir, os.ModePerm); err != nil {
				klog.Errorf("Create dir %s fail: %v", unitDir, err)
				return err
			}
		} else {
			klog.Errorf("Describe dir %s fail: %v", unitDir, err)
			return err
		}
	}

	unitFile := constants.YurthubServiceConfPath
	if err := os.WriteFile(unitFile, []byte(unitContent), 0644); err != nil {
		klog.Errorf("Write file %s fail: %v", unitFile, err)
		return err
	}

	return nil
}

func CreateYurthubSystemdService(data joindata.YurtJoinData) error {
	if err := setYurthubMainService(); err != nil {
		return err
	}

	if err := setYurthubUnitService(data); err != nil {
		return err
	}

	cmd := execCommand("systemctl", "daemon-reload")
	if err := cmd.Run(); err != nil {
		return err
	}

	cmd = execCommand("systemctl", "enable", constants.YurtHubServiceName)
	if err := cmd.Run(); err != nil {
		return err
	}

	cmd = execCommand("systemctl", "start", constants.YurtHubServiceName)
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

func CheckYurthubServiceHealth(yurthubServer string) error {
	cmd := execCommand("systemctl", "is-active", constants.YurtHubServiceName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("yurthub service is not active: %v", err)
	}

	if err := checkYurthubHealthzFunc(yurthubServer); err != nil { // Here is the previous CheckYurthubHealthz, called in postcheck.go
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
