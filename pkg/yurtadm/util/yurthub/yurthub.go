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
)

// createYurthubSystemdService 创建 YurtHub 的 systemd service 配置
func CreateYurthubSystemdService(data joindata.YurtJoinData) error {
	nodePoolName := data.NodeRegistration().NodePoolName
	// 如果 nodePoolName 为空，则使用默认值 "default"
	if nodePoolName == "" {
		nodePoolName = "default"
	}

	// 构建 service 内容
	// 	serviceContent := fmt.Sprintf(`
	// [Unit]
	// Description=YurtHub Service
	// After=network.target

	// [Service]
	// Type=simple
	// Environment="NODE_NAME=${HOSTNAME}"
	// ExecStart=%s \
	//     --v=2 \
	//     --bind-address=127.0.0.1 \
	//     --server-addr=https://%s \
	//     --node-name=%s \
	//     --nodepool-name=%s \
	//     --bootstrap-file=%s \
	//     --working-mode=%s \
	//     --namespace=%s \
	// Restart=always

	// // [Install]
	// // WantedBy=multi-user.target
	// // `,
	// 		constants.YurthubExecStart,
	// 		data.ServerAddr(),
	// 		data.NodeRegistration().Name,
	// 		nodePoolName,
	// 		constants.YurtHubBootstrapConfig,
	// 		data.NodeRegistration().WorkingMode,
	// 		data.Namespace(),
	// 	)

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

	// 写入 systemd service 文件
	serviceFile := "/etc/systemd/system/yurthub.service"
	if err := os.WriteFile(serviceFile, []byte(serviceContent), 0644); err != nil {
		return err
	}

	// 启用并启动服务. // 在写一个函数
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

// 新增
// CheckYurthubServiceHealth 检查 YurtHub 服务的健康状态
func CheckYurthubServiceHealth(yurthubServer string) error {
	// 检查 systemd 服务状态
	cmd := exec.Command("systemctl", "is-active", "yurthub.service")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("yurthub service is not active: %v", err)
	}

	// 检查 YurtHub 健康端点
	if err := CheckYurthubHealthz(yurthubServer); err != nil { // 这里是之前的 CheckYurthubHealthz，在postcheck.go中调用
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
