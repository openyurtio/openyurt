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

package e2e

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/types"
	"github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	yurthub "github.com/openyurtio/openyurt/test/e2e/autonomy"
	cmd2 "github.com/openyurtio/openyurt/test/e2e/cmd"
	"github.com/openyurtio/openyurt/test/e2e/common/ns"
	p "github.com/openyurtio/openyurt/test/e2e/common/pod"
	"github.com/openyurtio/openyurt/test/e2e/constants"
	"github.com/openyurtio/openyurt/test/e2e/util"
	"github.com/openyurtio/openyurt/test/e2e/util/ginkgowrapper"
	_ "github.com/openyurtio/openyurt/test/e2e/yurt"
	"github.com/openyurtio/openyurt/test/e2e/yurtconfig"
)

var (
	c clientset.Interface
)

func TestMain(m *testing.M) {
	cmd := cmd2.NewE2eCommand(m)
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func TestE2E(t *testing.T) {
	gomega.RegisterFailHandler(ginkgowrapper.Fail)
	ginkgo.RunSpecs(t, "openyurt e2e suite")
}

var _ = ginkgo.BeforeSuite(func() {
	suiteConfig, _ := ginkgo.GinkgoConfiguration()
	labelFilter, err := types.ParseLabelFilter(suiteConfig.LabelFilter)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail get labelFilter")

	error := util.SetYurtE2eCfg()
	gomega.Expect(error).NotTo(gomega.HaveOccurred(), "fail set Yurt E2E Config")

	c = yurtconfig.YurtE2eCfg.KubeClient
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail get client set")

	err = ns.DeleteNameSpace(c, constants.YurtE2ENamespaceName)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail to delete namespaces")

	_, err = ns.CreateNameSpace(c, constants.YurtE2ENamespaceName)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail to create namespace")

	err = util.PrepareNodePoolWithNode(context.TODO(), yurtconfig.YurtE2eCfg.RuntimeClient, "openyurt-e2e-test-worker")
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail to create a nodepool with node")

	if labelFilter([]string{"edge-autonomy"}) {
		// get nginx podIP on edge node worker2
		cs := c
		podName := "yurt-e2e-test-nginx-openyurt-e2e-test-worker2"
		ginkgo.By("get pod info:" + podName)
		pod, err := p.GetPod(cs, constants.YurtDefaultNamespaceName, podName)
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail to get pod nginx on edge node 2")

		yurthub.Edge2NginxPodIP = pod.Status.PodIP
		klog.Infof("get PodIP of Nginx on edge node 2: %s", yurthub.Edge2NginxPodIP)

		// get nginx serviceIP
		ginkgo.By("get service info" + constants.NginxServiceName)
		nginxSvc, err := c.CoreV1().Services(constants.YurtDefaultNamespaceName).Get(context.Background(), constants.NginxServiceName, metav1.GetOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail to get service : "+constants.NginxServiceName)

		yurthub.NginxServiceIP = nginxSvc.Spec.ClusterIP
		klog.Infof("get ServiceIP of service : " + constants.NginxServiceName + " IP: " + yurthub.NginxServiceIP)

		//get coredns serviceIP
		ginkgo.By("get service info" + constants.CoreDNSServiceName)
		coreDNSSvc, error := c.CoreV1().Services(constants.YurtSystemNamespaceName).Get(context.Background(), constants.CoreDNSServiceName, metav1.GetOptions{})
		gomega.Expect(error).NotTo(gomega.HaveOccurred(), "fail to get service : "+constants.CoreDNSServiceName)

		yurthub.CoreDNSServiceIP = coreDNSSvc.Spec.ClusterIP
		klog.Infof("get ServiceIP of service : " + constants.CoreDNSServiceName + " IP: " + yurthub.CoreDNSServiceIP)

		// disconnect cloud node
		cmd := exec.Command("/bin/bash", "-c", "docker network disconnect kind "+constants.YurtCloudNodeName)
		error = cmd.Run()
		gomega.Expect(error).NotTo(gomega.HaveOccurred(), "fail to disconnect cloud node to kind bridge: docker network disconnect kind %s", constants.YurtCloudNodeName)
		klog.Infof("successfully disconnected cloud node")
	}
})

var _ = ginkgo.AfterSuite(func() {
	suiteConfig, _ := ginkgo.GinkgoConfiguration()
	labelFilter, err := types.ParseLabelFilter(suiteConfig.LabelFilter)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail get labelFilter")
	if labelFilter([]string{"edge-autonomy"}) {
		// reconnect cloud node to docker network
		cmd := exec.Command("/bin/bash", "-c", "docker network connect kind "+constants.YurtCloudNodeName)
		err := cmd.Run()
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail to reconnect cloud node to kind bridge")
		klog.Infof("successfully reconnected cloud node")

		gomega.Eventually(func() error {
			_, err = c.Discovery().ServerVersion()
			return err
		}).WithTimeout(120 * time.Second).WithPolling(1 * time.Second).Should(gomega.Succeed())
	}

	ginkgo.By("delete namespace:" + constants.YurtE2ENamespaceName)
	err = ns.DeleteNameSpace(c, constants.YurtE2ENamespaceName)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail to delete created namespaces")
})
