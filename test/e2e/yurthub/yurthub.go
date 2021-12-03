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
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/yurtctl/constants"
	nd "github.com/openyurtio/openyurt/test/e2e/common/node"
	"github.com/openyurtio/openyurt/test/e2e/common/ns"
	p "github.com/openyurtio/openyurt/test/e2e/common/pod"
	"github.com/openyurtio/openyurt/test/e2e/util"
	ycfg "github.com/openyurtio/openyurt/test/e2e/yurtconfig"
)

const (
	YurtHubNamespaceName = "yurthub-test"
	StopNodeWaitMinite   = 1
	StartNodeWaitMinite  = 1
)

func Register() {
	if !ycfg.YurtE2eCfg.EnableYurtAutonomy {
		klog.Infof("not enable yurt node autonomy test, so yurt-node-autonomy will not run, and will return.")
		return
	}
	var _ = util.YurtDescribe(YurtHubNamespaceName, func() {
		var (
			c   clientset.Interface
			err error
		)
		defer ginkgo.GinkgoRecover()
		c = ycfg.YurtE2eCfg.KubeClient
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail to get client set")

		err = ns.DeleteNameSpace(c, YurtHubNamespaceName)
		util.ExpectNoError(err)

		ginkgo.By("yurthub create namespace")
		_, err = ns.CreateNameSpace(c, YurtHubNamespaceName)
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail to create namespaces")

		ginkgo.AfterEach(func() {
			ginkgo.By("delete namespace:" + YurtHubNamespaceName)
			err = ns.DeleteNameSpace(c, YurtHubNamespaceName)
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail to delete created namespaces")
		})

		util.YurtDescribe("node autonomy", func() {
			ginkgo.It("yurt node autonomy", func() {
				NodeManager, err := nd.NewNodeController(ycfg.YurtE2eCfg.NodeType, ycfg.YurtE2eCfg.RegionID, ycfg.YurtE2eCfg.AccessKeyID, ycfg.YurtE2eCfg.AccessKeySecret)
				util.ExpectNoError(err)

				podName := "test-yurthub-busybox"
				objectMeta := metav1.ObjectMeta{}
				objectMeta.Name = podName
				objectMeta.Namespace = YurtHubNamespaceName
				objectMeta.Labels = map[string]string{"name": podName}
				spec := apiv1.PodSpec{}
				container := apiv1.Container{}
				spec.HostNetwork = true
				spec.NodeSelector = map[string]string{projectinfo.GetEdgeWorkerLabelKey(): "true"}
				container.Name = "busybox"
				container.Image = "busybox"
				container.Command = []string{"sleep", "3600"}
				spec.Containers = []apiv1.Container{container}

				ginkgo.By("create pod:" + podName)
				pod, err := p.CreatePod(c, YurtHubNamespaceName, objectMeta, spec)
				gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail create busybox deployment")

				ginkgo.By("waiting running pod:" + podName)
				err = p.VerifyPodsRunning(c, YurtHubNamespaceName, podName, false, 1)
				gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail wait pod running")
				klog.Infof("check pod status running Successful")

				ginkgo.By("get pod info")
				pod, err = p.GetPod(c, YurtHubNamespaceName, pod.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail get pod status:"+podName)

				ginkgo.By("set node " + pod.Spec.NodeName + " autonomy")
				patchNode := apiv1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{constants.AnnotationAutonomy: "true"},
					},
				}
				patchData, err := json.Marshal(patchNode)
				gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail marshal patch node")

				node, err := c.CoreV1().Nodes().Patch(context.Background(), pod.Spec.NodeName, types.StrategicMergePatchType, patchData, metav1.PatchOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail patch node autonomy")

				ginkgo.By("next will stop node")
				err = NodeManager.StopNode(GetNodeID(node))
				gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail stop node")

				ginkgo.By("will wait for the shudown finish in " + strconv.FormatInt(StopNodeWaitMinite, 10) + " minute ")
				time.Sleep(time.Minute * StopNodeWaitMinite)

				ginkgo.By("next will start node")
				err = NodeManager.StartNode(GetNodeID(node))
				gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail start node")

				ginkgo.By("will wait for power on finish in " + strconv.FormatInt(StartNodeWaitMinite, 10) + " minute ")
				time.Sleep(time.Minute * StartNodeWaitMinite)

				ginkgo.By("will get pod status:" + podName)
				pod, err = p.GetPod(c, YurtHubNamespaceName, podName)
				gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail get pod:"+podName)
				gomega.Expect(pod.Status.Phase).Should(gomega.Equal(apiv1.PodRunning), podName+" pod status is not running")

				ginkgo.By("will check container restart")
				gomega.Expect(pod.Status.ContainerStatuses[0].RestartCount).Should(gomega.Equal(int32(1)), "container restart count is not 1 container:"+podName)

				ginkgo.By("will remove pod:" + podName)
				err = p.DeletePod(c, YurtHubNamespaceName, podName)
				gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail remove pod:"+podName)

			})

		})
	})
}

/*
TODO
*/
func GetNodeID(node *apiv1.Node) string {
	return node.Name
}
