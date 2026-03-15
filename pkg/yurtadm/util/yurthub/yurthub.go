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
	downloadFile            = yurtadmutil.DownloadFile
	copyFile                = edgenode.CopyFile
)

func CheckAndInstallYurthub(yurthubVersion string) error {
	return CheckAndInstallYurthubWithConfig(&YurthubHostConfig{
		Version: yurthubVersion,
	})
}

func CheckAndInstallYurthubWithConfig(cfg *YurthubHostConfig) error {
	if err := cfg.validateForBinaryInstall(); err != nil {
		return err
	}

	klog.Infof("Check and install yurthub, version=%s, binaryURL=%s", cfg.Version, cfg.BinaryURL)
	if _, err := lookPath(yurthubExecStartPath); err == nil {
		klog.Infof("Yurthub binary already exists, skip install.")
		return nil
	}

	savePath := filepath.Join(constants.TmpDownloadDir, filepath.Base(yurthubExecStartPath))
	packageURL := cfg.binaryDownloadURL()
	klog.V(1).Infof("Download yurthub from: %s", packageURL)
	if err := downloadFile(packageURL, savePath, 3); err != nil {
		return fmt.Errorf("download yurthub fail: %w", err)
	}
	if err := copyFile(savePath, yurthubExecStartPath, 0755); err != nil {
		return err
	}

	return nil
}

func setYurthubMainService(serviceDir string) error {
	klog.Info("Setting yurthub main service.")

	serviceFile := serviceDir + "/yurthub.service"

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

func setYurthubUnitService(hubUnitConfigDir string, data joindata.YurtJoinData) error {
	return setYurthubUnitServiceWithConfig(hubUnitConfigDir, NewYurthubHostConfigFromJoinData(data))
}

func setYurthubUnitServiceWithConfig(hubUnitConfigDir string, cfg *YurthubHostConfig) error {
	klog.Info("Setting yurthub unit service.")

	if err := cfg.validateForSystemdService(); err != nil {
		return err
	}

	ctx := map[string]string{
		"bindAddress":   cfg.bindAddress(),
		"bootstrapArgs": cfg.bootstrapArgs(),
		"namespace":     cfg.namespace(),
		"nodeName":      cfg.NodeName,
		"serverAddr":    cfg.normalizedServerAddr(),
		"workingMode":   cfg.WorkingMode,
	}

	if len(cfg.NodePoolName) != 0 {
		ctx["nodePoolName"] = cfg.NodePoolName
	}

	unitContent, err := templates.SubstituteTemplate(constants.YurtHubUnitConfig, ctx)
	if err != nil {
		klog.Errorf("SubstituteTemplate error: %v", err)
		return err
	}

	if _, err := os.Stat(hubUnitConfigDir); err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(hubUnitConfigDir, os.ModePerm); err != nil {
				klog.Errorf("Create dir %s fail: %v", hubUnitConfigDir, err)
				return err
			}
		} else {
			klog.Errorf("Describe dir %s fail: %v", hubUnitConfigDir, err)
			return err
		}
	}

	unitFile := filepath.Join(hubUnitConfigDir, filepath.Base(yurthubServiceConfFilePath))
	if err := os.WriteFile(unitFile, []byte(unitContent), 0644); err != nil {
		klog.Errorf("Write file %s fail: %v", unitFile, err)
		return err
	}

	return nil
}

func CreateYurthubSystemdService(data joindata.YurtJoinData) error {
	return CreateYurthubSystemdServiceWithConfig(NewYurthubHostConfigFromJoinData(data))
}

func CreateYurthubSystemdServiceWithConfig(cfg *YurthubHostConfig) error {
	if err := setYurthubMainService(filepath.Dir(yurthubServiceFilePath)); err != nil {
		return err
	}

	if err := setYurthubUnitServiceWithConfig(filepath.Dir(yurthubServiceConfFilePath), cfg); err != nil {
		return err
	}

	if err := ReloadYurthubSystemdConfig(); err != nil {
		return err
	}

	if err := EnableYurthubService(); err != nil {
		return err
	}

	if err := StartYurthubService(); err != nil {
		return err
	}

	return nil
}

func ReloadYurthubSystemdConfig() error {
	return runSystemctl("daemon-reload")
}

func EnableYurthubService() error {
	return runSystemctl("enable", constants.YurtHubServiceName)
}

func StartYurthubService() error {
	return runSystemctl("start", constants.YurtHubServiceName)
}

func StopYurthubService() error {
	return runSystemctlWithIgnoredErrors([]string{"not loaded", "not found", "does not exist"}, "stop", constants.YurtHubServiceName)
}

func DisableYurthubService() error {
	return runSystemctlWithIgnoredErrors([]string{"not loaded", "not found", "does not exist"}, "disable", constants.YurtHubServiceName)
}

func RemoveYurthubSystemdService() error {
	if err := os.RemoveAll(yurthubServiceFilePath); err != nil {
		return err
	}

	if err := os.RemoveAll(filepath.Dir(yurthubServiceConfFilePath)); err != nil {
		return err
	}

	return ReloadYurthubSystemdConfig()
}

func RemoveYurthubBinary() error {
	return os.RemoveAll(yurthubExecStartPath)
}

func CleanYurthubHostArtifacts() error {
	if err := StopYurthubService(); err != nil {
		return err
	}
	if err := DisableYurthubService(); err != nil {
		return err
	}
	if err := RemoveYurthubSystemdService(); err != nil {
		return err
	}
	if err := RemoveYurthubBinary(); err != nil {
		return err
	}
	if err := CleanHubBootstrapConfig(); err != nil {
		return err
	}
	if err := os.RemoveAll(yurthubWorkDirPath); err != nil {
		return err
	}
	return nil
}

func runSystemctl(args ...string) error {
	return runSystemctlWithIgnoredErrors(nil, args...)
}

func runSystemctlWithIgnoredErrors(ignoredSubstrings []string, args ...string) error {
	cmd := execCommand("systemctl", args...)
	output, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}

	outputText := strings.ToLower(strings.TrimSpace(string(output)))
	for _, ignoredSubstring := range ignoredSubstrings {
		if strings.Contains(outputText, ignoredSubstring) {
			klog.V(1).Infof("Ignore systemctl error, args=%v, output=%s", args, outputText)
			return nil
		}
	}

	if len(outputText) == 0 {
		return err
	}

	return fmt.Errorf("%w: %s", err, outputText)
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
		if err := os.MkdirAll(filepath.Dir(yurthubBootstrapConfigPath), os.ModePerm); err != nil {
			return err
		}

		if err = kubeconfigutil.WriteToDisk(yurthubBootstrapConfigPath, tlsBootstrapCfg); err != nil {
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
	if err := os.RemoveAll(yurthubBootstrapConfigPath); err != nil {
		klog.Warningf("Clean file %s fail: %v, please clean it manually.", yurthubBootstrapConfigPath, err)
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
