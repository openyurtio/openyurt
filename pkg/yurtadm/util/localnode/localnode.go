/*
Copyright 2025 The OpenYurt Authors.

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
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/coreos/go-iptables/iptables"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurtadm/constants"
	"github.com/openyurtio/openyurt/pkg/yurtadm/util"
	"github.com/openyurtio/openyurt/pkg/yurtadm/util/edgenode"
	"github.com/openyurtio/openyurt/pkg/yurtadm/util/initsystem"
)

// iptablesInterface defines an interface for mocking the iptables client.
type iptablesInterface interface {
	ChainExists(table, chain string) (bool, error)
	List(table, chain string) ([]string, error)
}

// Replace direct function calls with package-level variables to allow monkey patching in tests.
var (
	// os functions
	osMkdirAll   = os.MkdirAll
	osRemoveAll  = os.RemoveAll
	filepathWalk = filepath.Walk

	// util functions
	utilDownloadFile = util.DownloadFile
	utilUntar        = util.Untar
	edgenodeCopyFile = edgenode.CopyFile

	// factory functions
	newIptables = func() (iptablesInterface, error) {
		return iptables.New()
	}

	// internal function calls
	checkChainFunc = CheckIptablesChainAndRulesExists

	// Convert constant to variable for testing.
	yurthubTmpDir = constants.YurthubTmpDir
)

// DownloadAndDeployYurthubInSystemd downloads yurthub binary and deploys yurthub in systemd
func DownloadAndDeployYurthubInSystemd(hostControlPlaneAddr string, serverAddr string, yurthubBinaryUrl string, nodeName string) error {
	// download yurthub (tar.gz) from yurthubBinaryUrl and install it to /usr/bin/yurthub
	if err := DownloadAndInstallYurthub(yurthubBinaryUrl); err != nil {
		return err
	}
	// stop yurthub service at first
	if err := StopYurthubService(); err != nil {
		return err
	}
	// set and start yurthub service in systemd
	if err := SetYurthubService(hostControlPlaneAddr, serverAddr, nodeName); err != nil {
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

// DownloadAndInstallYurthub gets yurthub binary from URL and saves to /usr/bin/yurthub
func DownloadAndInstallYurthub(yurthubBinaryUrl string) error {
	//download yurthub (format: tar.gz) from yurthubBinaryUrl
	originalFileName := path.Base(yurthubBinaryUrl)
	if err := osMkdirAll(yurthubTmpDir, 0755); err != nil {
		return err
	}
	defer osRemoveAll(yurthubTmpDir)
	savePath := fmt.Sprintf("%s/%s", yurthubTmpDir, originalFileName)
	klog.V(1).Infof("Download yurthub from: %s", yurthubBinaryUrl)
	if err := utilDownloadFile(yurthubBinaryUrl, savePath, 3); err != nil {
		return fmt.Errorf("download yurthub fail: %w", err)
	}
	// untar the tar.gz file to YurthubTmpDir
	if err := utilUntar(savePath, yurthubTmpDir); err != nil {
		return err
	}

	// look for a binary file named "yurthub" in the untared directory
	var foundBinaryPath string
	binaryToFind := "yurthub"

	walkFn := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err // if an error occurs during the traversal, the error is returned directly
		}
		// we only care about the file, and the file name must be "yurthub"
		if !info.IsDir() && info.Name() == binaryToFind {
			foundBinaryPath = path // found it and record its full path
			return io.EOF          // io.EOF is a special signal that tells the Walk function to stop traversing.
		}
		return nil // continue traversing
	}

	// start traversal search from the root of the untared directory
	if err := filepathWalk(yurthubTmpDir, walkFn); err != nil && err != io.EOF {
		return fmt.Errorf("error looking up %s: %w", binaryToFind, err)
	}

	// check if the file was found
	if foundBinaryPath == "" {
		return fmt.Errorf("no binary file named '%s' found in the archive", binaryToFind)
	}
	klog.V(1).Infof("binary file found: %s", foundBinaryPath)

	// copy the found binary files to the final destination
	klog.V(1).Infof("copying %s to %s", foundBinaryPath, "/usr/bin/yurthub")
	if err := edgenodeCopyFile(foundBinaryPath, "/usr/bin/yurthub", constants.DirMode); err != nil {
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
func SetYurthubService(hostControlPlaneAddr string, serverAddr string, nodeName string) error {
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
	ipt, err := newIptables()
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

// WaitForIptablesChainReadyWithTimeout waits for the iptables chain and its rules to be ready within a specified timeout.
func WaitForIptablesChainReadyWithTimeout(table, chain string, interval, timeout time.Duration) error {
	// Create a context that will be canceled automatically after the timeout duration.
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Create a ticker to check periodically.
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	klog.Infof("Waiting for iptables chain '%s' in table '%s' to be ready (timeout: %s)...", chain, table, timeout)

	// Loop and use a 'select' statement to wait for either the ticker or the timeout.
	for {
		select {
		// This case is triggered when the context's timeout is exceeded.
		case <-ctx.Done():
			return fmt.Errorf("timed out after %s waiting for iptables chain '%s' in table '%s'", timeout, chain, table)

		// This case is triggered at each interval defined by the ticker.
		case <-ticker.C:
			exists, err := checkChainFunc(table, chain)
			if err != nil {
				// An error occurred during the check; log it and retry on the next tick.
				klog.Errorf("CheckIptablesChainAndRulesExists failed: %v, retrying...", err)
				continue
			}

			if exists {
				// The condition is met; log success and return nil (no error).
				klog.Infof("iptables chain '%s' in table '%s' is ready.", chain, table)
				return nil
			}

			// The chain is not ready yet; log and wait for the next tick.
			klog.Infof("Chain '%s' is not ready yet, retrying in %s...", chain, interval)
		}
	}
}
