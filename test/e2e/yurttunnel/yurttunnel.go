/*
Copyright 2014 The Kubernetes Authors.
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

package yurttunnel

import (
	"bytes"
	"fmt"
	"github.com/alibaba/openyurt/pkg/projectinfo"
	"github.com/alibaba/openyurt/pkg/yurtctl/constants"
	"github.com/alibaba/openyurt/test/e2e/common/ns"
	p "github.com/alibaba/openyurt/test/e2e/common/pod"
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	"io"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/klog"
	"k8s.io/kubernetes/test/e2e/framework"
	"k8s.io/kubernetes/test/e2e/framework/ginkgowrapper"
	"net/url"
	"strings"
	"time"
)

const (
	YURTTUNNEL_E2E_NAMESPACE_NAME = "yurttunnel-e2e-test"
	YURTTUNNEL_E2E_TEST_DESC      = "[yurttunnel-e2e-test]"
	YURTTUNNEL_E2E_MIN_NODE_NUM   = 2
)

const (
	PodStartShortTimeout = 1 * time.Minute
)

func PreCheckNode(c clientset.Interface) error {
	nodes, err := c.CoreV1().Nodes().List(metav1.ListOptions{})
	if err != nil {
		klog.Errorf("pre_check_get_nodes failed errmsg:%v", err)
		return err
	}
	if len(nodes.Items) < YURTTUNNEL_E2E_MIN_NODE_NUM {
		err = fmt.Errorf("yurttunnel e2e test need 2 nodes at least")
		return err
	}

	for _, node := range nodes.Items {
		status := node.Status.Conditions[len(node.Status.Conditions)-1].Type
		if status != apiv1.NodeReady {
			err = fmt.Errorf("yurttunnel e2e test pre_check_get_node_status: not_ready, so exit")
			return err
		}
	}
	return nil
}

func PreCheckTunnelPod(c clientset.Interface) error {
	pods, err := c.CoreV1().Pods("").List(metav1.ListOptions{})
	if err != nil {
		klog.Errorf("pre_check_get_pods failed errmsg:%v", err)
		return err
	}
	hasTunnelServer := false
	hasTunnelClient := false
	for _, pod := range pods.Items {
		if strings.Contains(pod.Name, "yurt-tunnel-server") {
			hasTunnelServer = true
			continue
		}
		if strings.Contains(pod.Name, "yurt-tunnel-agent") {
			hasTunnelClient = true
			continue
		}
	}
	if !hasTunnelServer || !hasTunnelClient {
		err = fmt.Errorf("yurttunnel e2e test pre_check pod of tunnel agent and tunnel server are needed")
		return err
	}
	return nil
}

func RunExecWithOutPut(c clientset.Interface, ns, podName, containerName string) (string, string, error) {
	config, err := framework.LoadConfig()
	if err != nil {
		klog.Infof("load_config_failed:%v", err)
		return "", "", err
	}

	const tty = false
	req := c.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(ns).
		SubResource("exec").
		Param("container", containerName)
	req.VersionedParams(&apiv1.PodExecOptions{
		Container: containerName,
		Command:   []string{"date"},
		Stdin:     false,
		Stdout:    true,
		Stderr:    true,
		TTY:       tty,
	}, scheme.ParameterCodec)

	var buffStdout, buffStderr bytes.Buffer
	err = execute("POST", req.URL(), config, nil, &buffStdout, &buffStderr, tty)
	if err != nil {
		return "", "", err
	}
	return buffStdout.String(), buffStderr.String(), nil
}

func execute(method string, url *url.URL, config *restclient.Config, stdin io.Reader, stdout, stderr io.Writer, tty bool) error {
	exec, err := remotecommand.NewSPDYExecutor(config, method, url)
	if err != nil {
		return err
	}
	return exec.Stream(remotecommand.StreamOptions{
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
		Tty:    tty,
	})
}

func Register() {
	var _ = framework.KubeDescribe(YURTTUNNEL_E2E_NAMESPACE_NAME, func() {
		gomega.RegisterFailHandler(ginkgowrapper.Fail)
		defer ginkgo.GinkgoRecover()
		var (
			c   clientset.Interface
			err error
		)
		c, err = framework.LoadClientset()
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail get client set")

		err = PreCheckNode(c)
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), "yurttunnel_e2e_node_not_ok")

		err = PreCheckTunnelPod(c)
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), "yurttunnel_e2e_pod_not_ok")

		err = ns.DeleteNameSpace(c, YURTTUNNEL_E2E_NAMESPACE_NAME)
		framework.ExpectNoError(err)

		ginkgo.By(YURTTUNNEL_E2E_TEST_DESC + "yurttunnel_test_create namespace")
		_, err = ns.CreateNameSpace(c, YURTTUNNEL_E2E_NAMESPACE_NAME)
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail to create namespace")

		framework.KubeDescribe(YURTTUNNEL_E2E_TEST_DESC+": pod_operate_test_on_edge", func() {
			ginkgo.It("yurttunnel_e2e_test_pod_run_on_edge", func() {
				cs := c
				podName := "test-po-on-edge"
				objectMeta := metav1.ObjectMeta{}
				objectMeta.Name = podName
				objectMeta.Namespace = YURTTUNNEL_E2E_NAMESPACE_NAME
				objectMeta.Labels = map[string]string{"name": podName}
				spec := apiv1.PodSpec{}
				container := apiv1.Container{}
				spec.HostNetwork = true
				spec.NodeSelector = map[string]string{projectinfo.GetEdgeWorkerLabelKey(): "true"}
				container.Name = "test-po-yurttunnel-on-edge"
				container.Image = "busybox"
				container.Command = []string{"sleep", "3600"}
				spec.Containers = []apiv1.Container{container}

				ginkgo.By("create pod:" + podName)
				_, err := p.CreatePod(cs, YURTTUNNEL_E2E_NAMESPACE_NAME, objectMeta, spec)
				gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail create pod:"+podName)

				err = p.WaitTimeoutForPodRunning(cs, podName, YURTTUNNEL_E2E_NAMESPACE_NAME, PodStartShortTimeout)
				gomega.Expect(err).NotTo(gomega.HaveOccurred(), "wait create timeout pod:"+podName)

				ginkgo.By("waiting pod running:" + podName)
				err = p.VerifyPodsRunning(cs, YURTTUNNEL_E2E_NAMESPACE_NAME, podName, false, 1)
				gomega.Expect(err).NotTo(gomega.HaveOccurred(), "wait running failed pod: "+podName)

				ginkgo.By("get pod info:" + podName)
				pod, err := p.GetPod(cs, YURTTUNNEL_E2E_NAMESPACE_NAME, podName)
				gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail get status pod:"+podName)
				gomega.Expect(pod.Name).Should(gomega.Equal(podName), podName+" get_pod_name:"+pod.Name+" not equal created pod:"+podName)

				stdOut, _, err := RunExecWithOutPut(c, YURTTUNNEL_E2E_NAMESPACE_NAME, pod.Name, container.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail run exec:"+podName)
				gomega.Expect(stdOut).ShouldNot(gomega.Equal(""), "exec edge pod return empty")

				err = p.DeletePod(cs, YURTTUNNEL_E2E_NAMESPACE_NAME, podName)
				gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail remove pod:"+podName)
				ginkgo.By("delete namespace: " + YURTTUNNEL_E2E_NAMESPACE_NAME)
				err = ns.DeleteNameSpace(cs, YURTTUNNEL_E2E_NAMESPACE_NAME)
				gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail delete created namespaces:"+YURTTUNNEL_E2E_NAMESPACE_NAME)
				framework.ExpectNoError(err)
			})
		})

	})

}
