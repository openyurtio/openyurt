/*
Copyright 2022 The OpenYurt Authors.

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

package etcd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var keyCacheDir = "/tmp/etcd-test"
var etcdDataDir = "/tmp/storagetest.etcd"
var devNull *os.File
var etcdCmd *exec.Cmd
var downloadURL = "https://github.com/etcd-io/etcd/releases/download"
var etcdVersion = "v3.5.0"
var etcdCmdPath = "/tmp/etcd/etcd"

var _ = BeforeSuite(func() {
	Expect(os.RemoveAll(keyCacheDir)).To(BeNil())
	Expect(os.RemoveAll(etcdDataDir)).To(BeNil())

	// start etcd
	var err error
	devNull, err = os.OpenFile("/dev/null", os.O_RDWR, 0755)
	Expect(err).To(BeNil())

	// It will check if etcd cmd can be found in PATH, otherwise
	// it will be installed.
	etcdCmdPath, err = ensureEtcdCmd()
	Expect(err).To(BeNil())
	Expect(len(etcdCmdPath)).ShouldNot(BeZero())

	etcdCmd = exec.Command(etcdCmdPath, "--data-dir="+etcdDataDir)
	etcdCmd.Stdout = devNull
	etcdCmd.Stderr = devNull
	Expect(etcdCmd.Start()).To(BeNil())
})

var _ = AfterSuite(func() {
	Expect(os.RemoveAll(keyCacheDir)).To(BeNil())

	// stop etcd
	Expect(etcdCmd.Process.Kill()).To(BeNil())
	Expect(devNull.Close()).To(BeNil())
})

func ensureEtcdCmd() (string, error) {
	path, err := exec.LookPath("etcd")
	if err == nil {
		return path, nil
	}

	return installEtcd()
}

func installEtcd() (string, error) {
	releaseURL := fmt.Sprintf("%s/%s/etcd-%s-linux-amd64.tar.gz", downloadURL, etcdVersion, etcdVersion)
	downloadPath := fmt.Sprintf("/tmp/etcd/etcd-%s-linux-amd64.tar.gz", etcdVersion)
	downloadDir := "/tmp/etcd"
	if err := exec.Command("bash", "-c", "rm -rf "+downloadDir).Run(); err != nil {
		return "", fmt.Errorf("failed to delete %s, %v", downloadDir, err)
	}

	if err := exec.Command("bash", "-c", "mkdir "+downloadDir).Run(); err != nil {
		return "", fmt.Errorf("failed to create dir %s, %v", downloadDir, err)
	}

	if err := exec.Command("bash", "-c", "curl -L "+releaseURL+" -o "+downloadPath).Run(); err != nil {
		return "", fmt.Errorf("failed to download etcd release %s at %s, %v", releaseURL, downloadPath, err)
	}

	if err := exec.Command("tar", "-zxvf", downloadPath, "-C", downloadDir, "--strip-components=1").Run(); err != nil {
		return "", fmt.Errorf("failed to extract tar at %s, %v", downloadPath, err)
	}

	return filepath.Join(downloadDir, "etcd"), nil
}

func TestEtcd(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ComponentKeyCache Test Suite")
}
