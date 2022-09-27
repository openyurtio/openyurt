/*
Copyright 2020 The OpenYurt Authors.

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
	"github.com/onsi/ginkgo/v2"
	"k8s.io/klog/v2"

	ycfg "github.com/openyurtio/openyurt/test/e2e/yurtconfig"
)

var _ = ginkgo.Describe("edge-autonomy"+YurtHubNamespaceName, ginkgo.Ordered, func() {
	if !ycfg.YurtE2eCfg.EnableYurtAutonomy {
		klog.Infof("not enable yurt node autonomy test, so yurt-node-autonomy will not run, and will return.")
		return
	}
	ginkgo.BeforeEach()

	var _ = ginkgo.Describe("kubelet"+YurtHubNamespaceName, func() {
		ginkgo.It("kubelet edge-autonomy test", ginkgo.Label("edge-autonomy"), func() {
			//Todo:
			//1. restart kubelet,
			//2. check if kubelet restarted successfully,
			//3. check if busybox restarted successfully.
		})
	})

	var _ = ginkgo.Describe("yurthub"+YurtHubNamespaceName, func() {

	})

	var _ = ginkgo.Describe("flannel"+YurtHubNamespaceName, func() {

	})

	var _ = ginkgo.Describe("kube-proxy"+YurtHubNamespaceName, func() {

	})

	var _ = ginkgo.Describe("coredns"+YurtHubNamespaceName, func() {

	})
})

// func TestEdgeAutonomy(t *testing.T) {
// ginkgo.BeforeSuite(func() {
// 	if !ycfg.YurtE2eCfg.EnableYurtAutonomy {
// 		klog.Infof("not enable yurt node autonomy test, so yurt-node-autonomy will not run, and will return.")
// 		return
// 	}
// 	c = ycfg.YurtE2eCfg.KubeClient
// 	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail to get client set")

// 	err = ns.DeleteNameSpace(c, YurtHubNamespaceName)
// 	util.ExpectNoError(err)
// 	ginkgo.By("yurthub create namespace")
// 	_, err = ns.CreateNameSpace(c, YurtHubNamespaceName)
// 	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail to create namespaces")
// 	gomega.RegisterFailHandler(ginkgowrapper.Fail)
// })

// ginkgo.AfterSuite(func() {
// 	ginkgo.By("delete namespace:" + YurtHubNamespaceName)
// 	err = ns.DeleteNameSpace(c, YurtHubNamespaceName)
// 	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail to delete created namespaces")
// })
// 	ginkgo.RunSpecs(t, "yurt-edge-autonomy")
// }
