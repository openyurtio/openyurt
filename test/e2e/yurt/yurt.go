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

package yurt

import (
	"github.com/alibaba/openyurt/test/e2e/common/ns"
	p "github.com/alibaba/openyurt/test/e2e/common/pod"
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog"
	"k8s.io/kubernetes/test/e2e/framework"
	"k8s.io/kubernetes/test/e2e/framework/ginkgowrapper"
	"time"
)

const (
	YURT_E2E_NAMESPACE_NAME = "yurt-e2e-test"
	YURT_E2E_TEST_DESC      = "[yurt-e2e-test]"
)
const (
	PodStartShortTimeout = 1 * time.Minute
)

func Register() {
	var _ = framework.KubeDescribe(YURT_E2E_NAMESPACE_NAME, func() {
		gomega.RegisterFailHandler(ginkgowrapper.Fail)
		defer ginkgo.GinkgoRecover()
		var (
			c   clientset.Interface
			err error
		)
		c, err = framework.LoadClientset()
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail get client set")

		err = ns.DeleteNameSpace(c, YURT_E2E_NAMESPACE_NAME)
		framework.ExpectNoError(err)

		ginkgo.By(YURT_E2E_TEST_DESC + "yurt_test_create namespace")
		_, err = ns.CreateNameSpace(c, YURT_E2E_NAMESPACE_NAME)
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail to create namespace")

		framework.KubeDescribe(YURT_E2E_TEST_DESC+": cluster_info", func() {
			ginkgo.It(YURT_E2E_TEST_DESC+": should get cluster pod num", func() {
				cs := c
				ginkgo.By("get current pod num")
				pods, err := p.ListPods(cs, "")
				gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail to list pods")
				klog.Infof("get_all_namespace_pod_num:%v", len(pods.Items))
			})

			ginkgo.It(YURT_E2E_TEST_DESC+": should get_all_namespace", func() {
				ginkgo.By("get current all namespace")
				cs := c
				_, err := ns.ListNameSpaces(cs)
				gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail to list namespaces")
			})

			ginkgo.It(YURT_E2E_TEST_DESC+": should get kube-system namespace", func() {
				ginkgo.By("get kube-system namespace")
				cs := c
				result, err := ns.GetNameSpace(cs, "kube-system")
				gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail to get system namespaces")
				klog.Infof("get_created_namespace Successful Name: %v  Status: %v CreateTime: %v ", result.ObjectMeta.Name, result.Status.Phase, result.CreationTimestamp)
			})

		})

		framework.KubeDescribe(YURT_E2E_TEST_DESC+": pod_operate_test", func() {
			ginkgo.It("pod_operate", func() {
				cs := c
				podName := "yurt-test-busybox"
				objectMeta := metav1.ObjectMeta{}
				objectMeta.Name = podName
				objectMeta.Namespace = YURT_E2E_NAMESPACE_NAME
				objectMeta.Labels = map[string]string{"name": podName}
				spec := apiv1.PodSpec{}
				container := apiv1.Container{}
				spec.HostNetwork = true
				spec.NodeSelector = map[string]string{"alibabacloud.com/is-edge-worker": "true"}
				container.Name = "yurt-test-busybox"
				container.Image = "busybox"
				container.Command = []string{"sleep", "3600"}
				spec.Containers = []apiv1.Container{container}

				ginkgo.By("create pod:" + podName)
				_, err := p.CreatePod(cs, YURT_E2E_NAMESPACE_NAME, objectMeta, spec)
				gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail create pod:"+podName)

				err = p.WaitTimeoutForPodRunning(cs, podName, YURT_E2E_NAMESPACE_NAME, PodStartShortTimeout)
				gomega.Expect(err).NotTo(gomega.HaveOccurred(), "wait create timeout pod:"+podName)

				ginkgo.By("waiting pod running:" + podName)
				err = p.VerifyPodsRunning(cs, YURT_E2E_NAMESPACE_NAME, podName, false, 1)
				gomega.Expect(err).NotTo(gomega.HaveOccurred(), "wait running failed pod: "+podName)

				ginkgo.By("get pod info:" + podName)
				pod, err := p.GetPod(cs, YURT_E2E_NAMESPACE_NAME, podName)
				gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail get status pod:"+podName)
				gomega.Expect(pod.Name).Should(gomega.Equal(podName), podName+" get_pod_name:"+pod.Name+" not equal created pod:"+podName)

				err = p.DeletePod(cs, YURT_E2E_NAMESPACE_NAME, podName)
				gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail remove pod:"+podName)

				ginkgo.By("delete namespace: " + YURT_E2E_NAMESPACE_NAME)
				err = ns.DeleteNameSpace(cs, YURT_E2E_NAMESPACE_NAME)
				gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail delete created namespaces:"+YURT_E2E_NAMESPACE_NAME)
				framework.ExpectNoError(err)
			})

		})
	})

}
