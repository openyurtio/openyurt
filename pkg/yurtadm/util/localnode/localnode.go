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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/coreos/go-iptables/iptables"
	"github.com/openyurtio/openyurt/pkg/yurtadm/constants"
	"github.com/openyurtio/openyurt/pkg/yurtadm/util/initsystem"
	"k8s.io/klog/v2"
)

// DeployYurthubInSystemd deploys yurthub in systemd
func DeployYurthubInSystemd(hostControlPlaneAddr string, serverAddr string, yurthubBinary string, nodeName string) error {
	// stop yurthub service at first
	if err := StopYurthubService(); err != nil {
		return err
	}
	// set and start yurthub service in systemd
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

// StopYurthubService stop yurthub service
func StopYurthubService() error {
	initSystem, err := initsystem.GetInitSystem()
	if err != nil {
		return err
	}
	if ok := initSystem.ServiceIsActive("yurthub"); ok {
		if err = initSystem.ServiceStop("yurthub"); err != nil {
			return fmt.Errorf("stop yurthub service failed")
		}
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
func CheckYurthubStatus() (bool, error) {
	initSystem, err := initsystem.GetInitSystem()
	if err != nil {
		return false, err
	}
	if ok := initSystem.ServiceIsActive("yurthub"); !ok {
		return false, fmt.Errorf("yurthub is not active. ")
	}
	return true, nil
}

// CheckIptablesChainAndRulesExists checks if the iptables chain and rules exist.
func CheckIptablesChainAndRulesExists(table, chain string) (bool, error) {
	ipt, err := iptables.New()
	if err != nil {
		klog.Errorf("Create iptables client failed: %v", err)
		return false, err
	}

	// check if LBCHAIN exists
	exists, err := ipt.ChainExists(table, chain)
	if err != nil {
		klog.Errorf("error checking if chain exists: %v", err)
		return false, err
	}
	if !exists {
		klog.Errorf("Chain %s does not exist in table %s", chain, table)
		return false, nil
	}

	// List all rules in the chain
	rules, err := ipt.List(table, chain)
	if err != nil {
		klog.Errorf("List rules in chain %s failed: %v", chain, err)
		return false, err
	}

	// The first rule is always the chain creation command
	// Valid rules start from the second entry
	if len(rules) <= 1 {
		klog.Errorf("Chain %s has no effective rules", chain)
		return false, nil
	}

	return true, nil
}

// WaitForIptablesChainReady waits for the LBCHAIN and rules to be ready.
func WaitForIptablesChainReady(table, chain string, iptablesDone chan<- bool) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		exists, err := CheckIptablesChainAndRulesExists(table, chain)
		if err != nil {
			klog.Errorf("CheckIptablesChainAndRulesExists fail: %v", err)
			continue
		}

		if exists {
			klog.Info("LBCHAIN and rules exist in nat table, continue to next step.")
			iptablesDone <- true
			return
		}

		klog.Info("LBCHAIN is not ready, retry after 10 second...")
	}
}
