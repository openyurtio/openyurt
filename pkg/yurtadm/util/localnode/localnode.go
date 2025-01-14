/*
Copyright 2024 The OpenYurt Authors.

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

package localnode

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/openyurtio/openyurt/pkg/yurtadm/constants"
	"github.com/openyurtio/openyurt/pkg/yurtadm/util/initsystem"
	"k8s.io/klog/v2"
)

// DeployYurthubInSystemd deploys yurthub in systemd
func DeployYurthubInSystemd(hostControlPlaneAddr string, serverAddr string, yurthubBinary string, nodeName string) error {
	if err := SetYurthubService(hostControlPlaneAddr, serverAddr, yurthubBinary, nodeName); err != nil {
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

// SetYurthubService configure yurthub service.
func SetYurthubService(hostControlPlaneAddr string, serverAddr string, yurthubBinary string, nodeName string) error {
	klog.Info("Setting Yurthub service.")
	yurthubServiceDir := filepath.Dir(constants.YurthubServiceFilepath)
	if _, err := os.Stat(yurthubServiceDir); err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(yurthubServiceDir, os.ModePerm); err != nil {
				klog.Errorf("Create dir %s fail: %v", yurthubServiceDir, err)
				return err
			}
		} else {
			klog.Errorf("Describe dir %s fail: %v", yurthubServiceDir, err)
			return err
		}
	}
	// copy yurthub binary to /usr/bin
	cmd := exec.Command("cp", yurthubBinary, "/usr/bin")
	if err := cmd.Run(); err != nil {
		klog.Errorf("Copy yurthub binary to /usr/bin fail: %v", err)
		return err
	}
	klog.Info("yurthub binary is in /usr/bin.")

	// yurthub.default contains the environment variables that yurthub needs
	yurthubSyetmdServiceEnvironmentFileContent := fmt.Sprintf(`
WORKINGMODE=local
NODENAME=%s
SERVERADDR=%s
HOSTCONTROLPLANEADDRESS=%s
`, nodeName, serverAddr, hostControlPlaneAddr)

	if err := os.WriteFile(constants.YurthubEnvironmentFilePath, []byte(yurthubSyetmdServiceEnvironmentFileContent), 0644); err != nil {
		klog.Errorf("Write file %s fail: %v", constants.YurthubEnvironmentFilePath, err)
		return err
	}

	// yurthub.service contains the configuration of yurthub service
	if err := os.WriteFile(constants.YurthubServiceFilepath, []byte(constants.YurthubSyetmdServiceContent), 0644); err != nil {
		klog.Errorf("Write file %s fail: %v", constants.YurthubServiceFilepath, err)
		return err
	}
	return nil
}

// EnableYurthubService enable yurthub service
func EnableYurthubService() error {
	initSystem, err := initsystem.GetInitSystem()
	if err != nil {
		return err
	}

	if !initSystem.ServiceIsEnabled("yurthub") {
		if err = initSystem.ServiceEnable("yurthub"); err != nil {
			return fmt.Errorf("enable yurthub service failed")
		}
	}
	return nil
}

// StartYurthubService start yurthub service
func StartYurthubService() error {
	initSystem, err := initsystem.GetInitSystem()
	if err != nil {
		return err
	}
	if err = initSystem.ServiceStart("yurthub"); err != nil {
		return fmt.Errorf("start yurthub service failed")
	}
	return nil
}

// CheckYurthubStatus check if yurthub is healthy.
func CheckYurthubStatus() error {
	initSystem, err := initsystem.GetInitSystem()
	if err != nil {
		return err
	}
	if ok := initSystem.ServiceIsActive("yurthub"); !ok {
		return fmt.Errorf("yurthub is not active. ")
	}
	return nil
}

// GetTenantApiServerEndpoints get the address of APIServers, which deployed in the form of daemonset in host-K8s.
func GetTenantApiServerEndpoints() (string, error) {
	// ipt, _ := iptables.New()
	// _, err := ipt.ChainExists("nat", "LBCHAIN")
	// if err != nil {
	// 	klog.Errorf("error checking if chain exists: %v", err)
	// 	return "", err
	// }

	// rules, err := ipt.ListChains("nat")
	// if err != nil {
	// 	klog.Errorf("Error list nat rules: %v", err)
	// 	return "", err
	// }

	// for _, rule := range rules {
	// 	// fields := strings.Fields(rule)
	// 	// for i, field := range fields {
	// 	// 	if field == "to:" && i+1 < len(fields) {
	// 	// 		apiserverEndpoint := fields[i+1]
	// 	// 		klog.V(1).Infof("list nat rules: %s", apiserverEndpoint)
	// 	// 		if strings.Contains(apiserverEndpoint, ":") {
	// 	// 			apiserverEndpoints = append(apiserverEndpoints, apiserverEndpoint)
	// 	// 		}
	// 	// 	}
	// 	// }
	// 	klog.V(1).Infof("list nat rules: %s", rule)
	// }

	// rules2, err := ipt.List("nat", "LBCHAIN")
	// if err != nil {
	// 	klog.Errorf("Error list LBCHAIN rules: %v", err)
	// 	return "", err
	// }

	// for _, rule := range rules2 {
	// 	klog.V(1).Infof("list LBCHAIN rules: %s", rule)
	// }

	// var apiserverEndpoints []string

	// return strings.Join(apiserverEndpoints, ","), nil

	cmd := exec.Command("iptables", "-t", "nat", "-S", "LBCHAIN")

	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		klog.Errorf(fmt.Sprint(err) + ": " + stderr.String())
		return "", nil
	}
	klog.V(1).Infof("Result: " + out.String())
	return "", nil
}
