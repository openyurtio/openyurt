/*
Copyright 2026 The OpenYurt Authors.

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
	"context"
	"fmt"
	"regexp"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	nodeservant "github.com/openyurtio/openyurt/pkg/node-servant"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
	yurthubutil "github.com/openyurtio/openyurt/pkg/yurthub/util"
	"github.com/openyurtio/openyurt/test/e2e/constants"
	ycfg "github.com/openyurtio/openyurt/test/e2e/yurtconfig"
)

const (
	conversionNodeName                                    = "openyurt-e2e-test-worker4"
	conversionNodePoolName                                = "yurt-pool3"
	conversionConditionType      corev1.NodeConditionType = "YurtNodeConversionFailed"
	reasonConverted                                       = "Converted"
	reasonReverted                                        = "Reverted"
	conversionProbePodName                                = "yurt-nodeconversion-probe"
	conversionProbeContainerName                          = "probe"
	kubernetesServiceName                                 = "kubernetes"
)

var conversionProbeEnvPattern = regexp.MustCompile(`HOST=([^\s]+)\s+PORT=([^\s]+)`)

var _ = Describe("Label driven node conversion", Serial, func() {
	ctx := context.Background()

	var (
		k8sClient    client.Client
		kubeClient   clientset.Interface
		waitTimeout  = 10 * time.Minute
		waitInterval = 5 * time.Second
	)

	getNode := func() (*corev1.Node, error) {
		node := &corev1.Node{}
		if err := k8sClient.Get(ctx, client.ObjectKey{Name: conversionNodeName}, node); err != nil {
			return nil, err
		}
		return node, nil
	}

	patchNodePoolLabel := func(poolName string) error {
		return retry.RetryOnConflict(retry.DefaultRetry, func() error {
			node, err := getNode()
			if err != nil {
				return err
			}

			patch := client.MergeFrom(node.DeepCopy())
			if node.Labels == nil {
				node.Labels = map[string]string{}
			}
			if poolName == "" {
				delete(node.Labels, projectinfo.GetNodePoolLabel())
			} else {
				node.Labels[projectinfo.GetNodePoolLabel()] = poolName
			}
			return k8sClient.Patch(ctx, node, patch)
		})
	}

	listConversionJobs := func() (*batchv1.JobList, error) {
		return kubeClient.BatchV1().Jobs(nodeservant.DefaultConversionJobNamespace).List(ctx, metav1.ListOptions{
			LabelSelector: labels.SelectorFromSet(map[string]string{
				nodeservant.ConversionNodeLabelKey: conversionNodeName,
			}).String(),
		})
	}

	waitForConversionState := func(expectedPool, expectedReason string, expectedEdgeWorker bool) error {
		node, err := getNode()
		if err != nil {
			return err
		}

		if poolName := node.Labels[projectinfo.GetNodePoolLabel()]; poolName != expectedPool {
			return fmt.Errorf("nodepool label of node %s is %q, want %q", conversionNodeName, poolName, expectedPool)
		}

		isEdgeWorker := node.Labels[projectinfo.GetEdgeWorkerLabelKey()] == "true"
		if isEdgeWorker != expectedEdgeWorker {
			return fmt.Errorf("edge-worker label of node %s is %t, want %t", conversionNodeName, isEdgeWorker, expectedEdgeWorker)
		}

		if node.Spec.Unschedulable {
			return fmt.Errorf("node %s is still unschedulable", conversionNodeName)
		}

		cond := getConversionCondition(node)
		if cond == nil {
			return fmt.Errorf("node %s has no %s condition", conversionNodeName, conversionConditionType)
		}
		if cond.Status != corev1.ConditionFalse || cond.Reason != expectedReason {
			return fmt.Errorf("node %s condition is status=%s reason=%s, want status=%s reason=%s", conversionNodeName, cond.Status, cond.Reason, corev1.ConditionFalse, expectedReason)
		}

		jobs, err := listConversionJobs()
		if err != nil {
			return err
		}
		for i := range jobs.Items {
			job := &jobs.Items[i]
			if isJobFailed(job) {
				return fmt.Errorf("conversion job %s failed for node %s", job.Name, conversionNodeName)
			}
			if !isJobFinished(job) {
				return fmt.Errorf("conversion job %s is still running for node %s", job.Name, conversionNodeName)
			}
		}

		return nil
	}

	waitForPodRunning := func(namespace, podName string) error {
		return wait.PollUntilContextTimeout(ctx, 2*time.Second, waitTimeout, true, func(ctx context.Context) (bool, error) {
			pod, err := kubeClient.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
			if err != nil {
				return false, err
			}
			if pod.Status.Phase != corev1.PodRunning {
				return false, nil
			}
			for _, cond := range pod.Status.Conditions {
				if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
					return true, nil
				}
			}
			return false, nil
		})
	}

	getProbePod := func() (*corev1.Pod, error) {
		return kubeClient.CoreV1().Pods(constants.YurtE2ENamespaceName).Get(ctx, conversionProbePodName, metav1.GetOptions{})
	}

	getProbeRestartCount := func() (int32, error) {
		pod, err := getProbePod()
		if err != nil {
			return 0, err
		}
		for _, status := range pod.Status.ContainerStatuses {
			if status.Name == conversionProbeContainerName {
				return status.RestartCount, nil
			}
		}
		return 0, fmt.Errorf("container %s not found in pod %s", conversionProbeContainerName, conversionProbePodName)
	}

	waitForProbeRestart := func(previousRestartCount int32) error {
		return wait.PollUntilContextTimeout(ctx, 2*time.Second, waitTimeout, true, func(ctx context.Context) (bool, error) {
			restartCount, err := getProbeRestartCount()
			if err != nil {
				return false, err
			}
			return restartCount > previousRestartCount, nil
		})
	}

	getProbeEnv := func() (string, string, error) {
		req := kubeClient.CoreV1().Pods(constants.YurtE2ENamespaceName).GetLogs(conversionProbePodName, &corev1.PodLogOptions{
			Container: conversionProbeContainerName,
		})
		logs, err := req.DoRaw(ctx)
		if err != nil {
			return "", "", err
		}
		matches := conversionProbeEnvPattern.FindSubmatch(logs)
		if len(matches) != 3 {
			return "", "", fmt.Errorf("unexpected probe logs %q", string(logs))
		}
		return string(matches[1]), string(matches[2]), nil
	}

	BeforeEach(func() {
		k8sClient = ycfg.YurtE2eCfg.RuntimeClient
		kubeClient = ycfg.YurtE2eCfg.KubeClient

		if _, err := getNode(); err != nil {
			if apierrors.IsNotFound(err) {
				Skip("label-driven round-trip e2e requires openyurt-e2e-test-worker4")
			}
			Expect(err).NotTo(HaveOccurred())
		}
	})

	It("should revert and convert a node by removing and re-adding the nodepool label", func() {
		DeferCleanup(func() {
			Eventually(func() error {
				return patchNodePoolLabel(conversionNodePoolName)
			}, time.Minute, time.Second).Should(Succeed())

			Eventually(func() error {
				return waitForConversionState(conversionNodePoolName, reasonConverted, true)
			}, waitTimeout, waitInterval).Should(Succeed())
		})

		By("wait for the init baseline to finish convert")
		Eventually(func() error {
			return waitForConversionState(conversionNodePoolName, reasonConverted, true)
		}, waitTimeout, waitInterval).Should(Succeed())

		By("remove the nodepool label to trigger revert")
		Eventually(func() error {
			return patchNodePoolLabel("")
		}, time.Minute, time.Second).Should(Succeed())

		Eventually(func() error {
			return waitForConversionState("", reasonReverted, false)
		}, waitTimeout, waitInterval).Should(Succeed())

		By("add the nodepool label back to trigger convert")
		Eventually(func() error {
			return patchNodePoolLabel(conversionNodePoolName)
		}, time.Minute, time.Second).Should(Succeed())

		Eventually(func() error {
			return waitForConversionState(conversionNodePoolName, reasonConverted, true)
		}, waitTimeout, waitInterval).Should(Succeed())
	})

	It("should restart existing workloads and re-inject kubernetes service envs during convert", func() {
		DeferCleanup(func() {
			_ = kubeClient.CoreV1().Pods(constants.YurtE2ENamespaceName).Delete(ctx, conversionProbePodName, metav1.DeleteOptions{})

			Eventually(func() error {
				return patchNodePoolLabel(conversionNodePoolName)
			}, time.Minute, time.Second).Should(Succeed())

			Eventually(func() error {
				return waitForConversionState(conversionNodePoolName, reasonConverted, true)
			}, waitTimeout, waitInterval).Should(Succeed())
		})

		By("wait for the init baseline to finish convert")
		Eventually(func() error {
			return waitForConversionState(conversionNodePoolName, reasonConverted, true)
		}, waitTimeout, waitInterval).Should(Succeed())

		By("remove the nodepool label to trigger revert")
		Eventually(func() error {
			return patchNodePoolLabel("")
		}, time.Minute, time.Second).Should(Succeed())

		Eventually(func() error {
			return waitForConversionState("", reasonReverted, false)
		}, waitTimeout, waitInterval).Should(Succeed())

		By("create a probe pod on the reverted node")
		_ = kubeClient.CoreV1().Pods(constants.YurtE2ENamespaceName).Delete(ctx, conversionProbePodName, metav1.DeleteOptions{})
		Eventually(func() bool {
			_, err := getProbePod()
			return apierrors.IsNotFound(err)
		}, time.Minute, time.Second).Should(BeTrue())

		_, err := kubeClient.CoreV1().Pods(constants.YurtE2ENamespaceName).Create(ctx, &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      conversionProbePodName,
				Namespace: constants.YurtE2ENamespaceName,
				Labels: map[string]string{
					"name": conversionProbePodName,
				},
			},
			Spec: corev1.PodSpec{
				NodeName:      conversionNodeName,
				RestartPolicy: corev1.RestartPolicyAlways,
				Containers: []corev1.Container{
					{
						Name:            conversionProbeContainerName,
						Image:           "busybox",
						ImagePullPolicy: corev1.PullIfNotPresent,
						Command:         []string{"sh", "-c"},
						Args:            []string{"echo HOST=$KUBERNETES_SERVICE_HOST PORT=$KUBERNETES_SERVICE_PORT; while true; do sleep 3600; done"},
					},
				},
			},
		}, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())

		Expect(waitForPodRunning(constants.YurtE2ENamespaceName, conversionProbePodName)).To(Succeed())

		kubernetesService, err := kubeClient.CoreV1().Services(constants.YurtDefaultNamespaceName).Get(ctx, kubernetesServiceName, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())

		initialHost, initialPort, err := getProbeEnv()
		Expect(err).NotTo(HaveOccurred())
		Expect(initialHost).To(Equal(kubernetesService.Spec.ClusterIP))
		Expect(initialPort).To(Equal(strconv.Itoa(int(kubernetesService.Spec.Ports[0].Port))))

		initialRestartCount, err := getProbeRestartCount()
		Expect(err).NotTo(HaveOccurred())

		By("add the nodepool label back to trigger convert")
		Eventually(func() error {
			return patchNodePoolLabel(conversionNodePoolName)
		}, time.Minute, time.Second).Should(Succeed())

		Eventually(func() error {
			return waitForConversionState(conversionNodePoolName, reasonConverted, true)
		}, waitTimeout, waitInterval).Should(Succeed())

		By("wait for the probe workload to be restarted by kubelet")
		Expect(waitForProbeRestart(initialRestartCount)).To(Succeed())

		convertedHost, convertedPort, err := getProbeEnv()
		Expect(err).NotTo(HaveOccurred())
		Expect(convertedHost).NotTo(Equal(initialHost))
		Expect(convertedPort).To(Equal(strconv.Itoa(yurthubutil.YurtHubProxySecurePort)))
	})
})

func getConversionCondition(node *corev1.Node) *corev1.NodeCondition {
	if node == nil {
		return nil
	}
	for i := range node.Status.Conditions {
		if node.Status.Conditions[i].Type == conversionConditionType {
			return &node.Status.Conditions[i]
		}
	}
	return nil
}

func isJobFinished(job *batchv1.Job) bool {
	return isJobSucceeded(job) || isJobFailed(job)
}

func isJobSucceeded(job *batchv1.Job) bool {
	if job == nil {
		return false
	}
	for _, cond := range job.Status.Conditions {
		if cond.Type == batchv1.JobComplete && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func isJobFailed(job *batchv1.Job) bool {
	if job == nil {
		return false
	}
	for _, cond := range job.Status.Conditions {
		if cond.Type == batchv1.JobFailed && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}
