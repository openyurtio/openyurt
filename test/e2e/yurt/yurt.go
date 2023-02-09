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
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/test/e2e/common/ns"
	p "github.com/openyurtio/openyurt/test/e2e/common/pod"
	"github.com/openyurtio/openyurt/test/e2e/constants"
	"github.com/openyurtio/openyurt/test/e2e/yurtconfig"
)

var _ = ginkgo.Describe(constants.YurtE2ENamespaceName, ginkgo.Ordered, ginkgo.Label("yurt"), func() {
	ginkgo.Describe(constants.YurtE2ETestDesc+": cluster_info", func() {
		ginkgo.It(constants.YurtE2ETestDesc+": should get cluster pod num", func() {
			cs := yurtconfig.YurtE2eCfg.KubeClient
			ginkgo.By("get current pod num")
			pods, err := p.ListPods(cs, "")
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail to list pods")
			klog.Infof("get_all_namespace_pod_num:%v", len(pods.Items))
		})

		ginkgo.It(constants.YurtE2ETestDesc+": should get_all_namespace", func() {
			ginkgo.By("get current all namespace")
			cs := yurtconfig.YurtE2eCfg.KubeClient
			_, err := ns.ListNameSpaces(cs)
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail to list namespaces")
		})

		ginkgo.It(constants.YurtE2ETestDesc+": should get kube-system namespace", func() {
			ginkgo.By("get kube-system namespace")
			cs := yurtconfig.YurtE2eCfg.KubeClient
			result, err := ns.GetNameSpace(cs, "kube-system")
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail to get system namespaces")
			klog.Infof("get_created_namespace Successful Name: %v  Status: %v CreateTime: %v ", result.ObjectMeta.Name, result.Status.Phase, result.CreationTimestamp)
		})

	})

	ginkgo.Describe(constants.YurtE2ETestDesc+": pod_operate_test", func() {
		ginkgo.It("pod_operate", func() {
			cs := yurtconfig.YurtE2eCfg.KubeClient
			podName := "yurt-test-busybox"
			objectMeta := metav1.ObjectMeta{}
			objectMeta.Name = podName
			objectMeta.Namespace = constants.YurtE2ENamespaceName
			objectMeta.Labels = map[string]string{"name": podName}
			spec := apiv1.PodSpec{}
			container := apiv1.Container{}
			spec.HostNetwork = true
			spec.NodeSelector = map[string]string{"openyurt.io/is-edge-worker": "true"}
			container.Name = "yurt-test-busybox"
			container.Image = "busybox"
			container.Command = []string{"sleep", "3600"}
			spec.Containers = []apiv1.Container{container}

			ginkgo.By("create pod:" + podName)
			_, err := p.CreatePod(cs, constants.YurtE2ENamespaceName, objectMeta, spec)
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail create pod:"+podName)

			err = p.WaitTimeoutForPodRunning(cs, podName, constants.YurtE2ENamespaceName, constants.PodStartShortTimeout)
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "wait create timeout pod:"+podName)

			ginkgo.By("waiting pod running:" + podName)
			err = p.VerifyPodsRunning(cs, constants.YurtE2ENamespaceName, podName, false, 1)
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "wait running failed pod: "+podName)

			ginkgo.By("get pod info:" + podName)
			pod, err := p.GetPod(cs, constants.YurtE2ENamespaceName, podName)
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail get status pod:"+podName)
			gomega.Expect(pod.Name).Should(gomega.Equal(podName), podName+" get_pod_name:"+pod.Name+" not equal created pod:"+podName)

			err = p.DeletePod(cs, constants.YurtE2ENamespaceName, podName)
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail remove pod:"+podName)
		})

	})
})
