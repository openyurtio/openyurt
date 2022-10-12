package etcd

import (
	"os"
	"os/exec"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var keyCacheDir = "/tmp/etcd-test"
var etcdDataDir = "/tmp/storagetest.etcd"
var devNull *os.File
var etcdCmd *exec.Cmd

var _ = BeforeSuite(func() {
	Expect(os.RemoveAll(keyCacheDir)).To(BeNil())
	Expect(os.RemoveAll(etcdDataDir)).To(BeNil())

	// start etcd
	var err error
	devNull, err = os.OpenFile("/dev/null", os.O_RDWR, 0755)
	Expect(err).To(BeNil())
	etcdCmd = exec.Command("/usr/local/etcd/etcd", "--data-dir="+etcdDataDir)
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

func TestEtcd(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ComponentKeyCache Test Suite")
}
