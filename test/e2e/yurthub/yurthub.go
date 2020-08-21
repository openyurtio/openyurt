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
	"encoding/json"
	"github.com/alibaba/openyurt/pkg/projectinfo"
	"github.com/alibaba/openyurt/pkg/yurtctl/constants"
	nd "github.com/alibaba/openyurt/test/e2e/common/node"
	"github.com/alibaba/openyurt/test/e2e/common/ns"
	p "github.com/alibaba/openyurt/test/e2e/common/pod"
	ycfg "github.com/alibaba/openyurt/test/e2e/yurtconfig"
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog"
	"k8s.io/kubernetes/test/e2e/framework"
	"strconv"
	"time"
)

const (
	YURTHUB_NAMESPACE_NAME = "yurthub-test"
	STOP_NODE_WAIT_MINITE  = 1
	START_NODE_WAIT_MINITE = 1
)

func Register() {
	if !ycfg.YurtE2eCfg.EnableYurtAutonomy {
		klog.Infof("not enable yurt node autonomy test, so yurt-node-autonomy will not run, and will return.")
		return
	}
	var _ = framework.KubeDescribe(YURTHUB_NAMESPACE_NAME, func() {
		var (
			c   clientset.Interface
			err error
		)
		defer ginkgo.GinkgoRecover()
		c, err = framework.LoadClientset()
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail to get client set")

		err = ns.DeleteNameSpace(c, YURTHUB_NAMESPACE_NAME)
		framework.ExpectNoError(err)

		ginkgo.By("yurthub create namespace")
		_, err = ns.CreateNameSpace(c, YURTHUB_NAMESPACE_NAME)
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail to create namespaces")

		ginkgo.AfterEach(func() {
			ginkgo.By("delete namespace:" + YURTHUB_NAMESPACE_NAME)
			err = ns.DeleteNameSpace(c, YURTHUB_NAMESPACE_NAME)
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail to delete created namespaces")
		})

		framework.KubeDescribe("node autonomy", func() {
			ginkgo.It("yurt node autonomy", func() {
				NodeManager, err := nd.NewNodeController(ycfg.YurtE2eCfg.NodeType, ycfg.YurtE2eCfg.RegionId, ycfg.YurtE2eCfg.AccessKeyId, ycfg.YurtE2eCfg.AccessKeySecret)
				framework.ExpectNoError(err)

				podName := "test-yurthub-busybox"
				objectMeta := metav1.ObjectMeta{}
				objectMeta.Name = podName
				objectMeta.Namespace = YURTHUB_NAMESPACE_NAME
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
				pod, err := p.CreatePod(c, YURTHUB_NAMESPACE_NAME, objectMeta, spec)
				gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail create busybox deployment")

				ginkgo.By("waiting running pod:" + podName)
				err = p.VerifyPodsRunning(c, YURTHUB_NAMESPACE_NAME, podName, false, 1)
				gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail wait pod running")
				klog.Infof("check pod status running Successful")

				ginkgo.By("get pod info")
				pod, err = p.GetPod(c, YURTHUB_NAMESPACE_NAME, pod.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail get pod status:"+podName)

				ginkgo.By("set node " + pod.Spec.NodeName + " autonomy")
				patchNode := apiv1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{constants.AnnotationAutonomy: "true"},
					},
				}
				patchData, err := json.Marshal(patchNode)
				gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail marshal patch node")

				node, err := c.CoreV1().Nodes().Patch(pod.Spec.NodeName, types.StrategicMergePatchType, patchData)
				gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail patch node autonomy")

				ginkgo.By("next will stop node")
				err = NodeManager.StopNode(GetNodeId(node))
				gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail stop node")

				ginkgo.By("will wait for the shudown finish in " + strconv.FormatInt(STOP_NODE_WAIT_MINITE, 10) + " minute ")
				time.Sleep(time.Minute * STOP_NODE_WAIT_MINITE)

				ginkgo.By("next will start node")
				err = NodeManager.StartNode(GetNodeId(node))
				gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail start node")

				ginkgo.By("will wait for power on finish in " + strconv.FormatInt(START_NODE_WAIT_MINITE, 10) + " minute ")
				time.Sleep(time.Minute * START_NODE_WAIT_MINITE)

				ginkgo.By("will get pod status:" + podName)
				pod, err = p.GetPod(c, YURTHUB_NAMESPACE_NAME, podName)
				gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail get pod:"+podName)
				gomega.Expect(pod.Status.Phase).Should(gomega.Equal(apiv1.PodRunning), podName+" pod status is not running")

				ginkgo.By("will check container restart")
				gomega.Expect(pod.Status.ContainerStatuses[0].RestartCount).Should(gomega.Equal(int32(1)), "container restart count is not 1 container:"+podName)

				ginkgo.By("will remove pod:" + podName)
				err = p.DeletePod(c, YURTHUB_NAMESPACE_NAME, podName)
				gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail remove pod:"+podName)

			})

		})
	})
}

/*
TODO
*/
func GetNodeId(node *apiv1.Node) string {
	return node.Name
}
