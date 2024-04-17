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

package yurt

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
	"github.com/openyurtio/openyurt/test/e2e/util"
	ycfg "github.com/openyurtio/openyurt/test/e2e/yurtconfig"
)

const (
	staticPodPath    string                  = "/etc/kubernetes/manifests"
	PodNeedUpgrade   corev1.PodConditionType = "PodNeedUpgrade"
	ServerName       string                  = "127.0.0.1"
	ServerPort       string                  = "10267"
	FlannelNamespace string                  = "kube-flannel"
)

var _ = Describe("yurtStaticSet Test", Ordered, func() {
	ctx := context.Background()
	timeout := 90 * time.Second
	k8sClient := ycfg.YurtE2eCfg.RuntimeClient
	nodeToImageMap := make(map[string]string)

	var updateStrategyType string
	var namespaceName string

	yurtStaticSetName := "busybox"
	podName := "busybox"
	testContainerName := "bb"
	testImg1 := "busybox"
	testImg2 := "busybox:1.36.0"

	createNamespace := func(name string) {
		ns := corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		}

		// Delete the namespace if it exists. Ignore the NotFound error.
		err := k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationForeground))
		Expect(client.IgnoreNotFound(err)).To(BeNil())

		// Wait for the namespace to be fully deleted if it was present.
		if client.IgnoreNotFound(err) != nil {
			// Namespace deletion was initiated, wait for completion.
			Eventually(
				func() bool {
					return client.IgnoreNotFound(k8sClient.Get(ctx, client.ObjectKey{Name: name}, &ns)) == nil
				},
				timeout,
				time.Millisecond*500,
			).Should(BeTrue(), "Namespace should be deleted")
		}
		By("All resources in the namespace have been removed")

		// Create the namespace and expect no error or that it already exists.
		err = k8sClient.Create(ctx, &ns)
		Expect(client.IgnoreAlreadyExists(err)).To(BeNil())
		if client.IgnoreAlreadyExists(err) != nil {
			By(fmt.Sprintf("couldn't create namespace %s", name))
		} else {
			By(fmt.Sprintf("Namespace %s created", name))
		}
	}

	createStaticPod := func(nodeName string) {
		staticPodStr := fmt.Sprintf(`
apiVersion: v1
kind: Pod
metadata:
  name: %s
  namespace: %s
  labels:
    app: %s
spec:
  containers:
  - name: %s
    image: %s
    command:
        - "/bin/sh"
    args:
        - "-c"
        - "while true; do echo hello; sleep 10; done"
`, podName, namespaceName, podName, testContainerName, testImg1)
		cmd := fmt.Sprintf("cat << EOF > %s/%s.yaml%sEOF", staticPodPath, podName, staticPodStr)
		dockerCmd := "docker exec -t " + nodeName + " /bin/bash -c " + "'" + cmd + "'"

		_, err := exec.Command("/bin/bash", "-c", dockerCmd).CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "fail to create static pod")
	}

	deleteStaticPod := func(nodeName string) {
		cmd := fmt.Sprintf("rm -f %s/%s.yaml", staticPodPath, podName)
		dockerCmd := "docker exec -t " + nodeName + " /bin/bash -c \"" + cmd + "\""

		_, err := exec.Command("/bin/bash", "-c", dockerCmd).CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "fail to delete static pod")
	}

	createYurtStaticSet := func() {
		yss := &v1alpha1.YurtStaticSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      yurtStaticSetName,
				Namespace: namespaceName,
			},
		}

		// Attempt to delete any existing YurtStaticSet with a given name and namespace.
		Eventually(func() error {
			return client.IgnoreNotFound(k8sClient.Delete(ctx, yss))
		}).WithTimeout(timeout).WithPolling(time.Millisecond * 300).Should(BeNil())

		// Define the pod labels and YurtStaticSet specs.
		testLabel := map[string]string{"app": podName}
		yss.Spec = v1alpha1.YurtStaticSetSpec{
			StaticPodManifest: podName,
			UpgradeStrategy: v1alpha1.YurtStaticSetUpgradeStrategy{
				Type: v1alpha1.YurtStaticSetUpgradeStrategyType(updateStrategyType),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: testLabel,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:    testContainerName,
							Image:   testImg1,
							Command: []string{"/bin/sh"},
							Args:    []string{"-c", "while true; do echo hello; sleep 10; done"},
						},
					},
				},
			},
		}
		// Attempt to create a new YurtStaticSet.
		Eventually(func() error {
			return client.IgnoreAlreadyExists(k8sClient.Create(ctx, yss))
		}).WithTimeout(timeout).WithPolling(time.Millisecond * 300).Should(BeNil())
	}

	updateYurtStaticSet := func() {
		// Define the object key to identify the YurtStaticSet.
		key := client.ObjectKey{
			Name:      yurtStaticSetName,
			Namespace: namespaceName,
		}

		// Retries updating the YurtStaticSet until it succeeds or times out.
		Eventually(func() error {
			// Fetch the existing YurtStaticSet instance.
			testYurtStaticSet := &v1alpha1.YurtStaticSet{}
			if err := k8sClient.Get(ctx, key, testYurtStaticSet); err != nil {
				return err
			}

			// Update the image of the first container.
			testYurtStaticSet.Spec.Template.Spec.Containers[0].Image = testImg2

			// Submit the updated YurtStaticSet to the Kubernetes API server.
			return k8sClient.Update(ctx, testYurtStaticSet)
		}, timeout, 500*time.Millisecond).Should(BeNil()) // Directly assert that the error should be nil.
	}

	checkPodStatusAndUpdate := func() {
		nodeToImageMap = map[string]string{}
		Eventually(func() error {
			testPods := &corev1.PodList{}
			if err := k8sClient.List(ctx, testPods, client.InNamespace(namespaceName), client.MatchingLabels{"app": podName}); err != nil {
				return err
			}
			if len(testPods.Items) != 2 {
				return fmt.Errorf("not reconcile")
			}
			for _, pod := range testPods.Items {
				if pod.Status.Phase != corev1.PodRunning {
					return fmt.Errorf("not running")
				}
				nodeToImageMap[pod.Spec.NodeName] = pod.Spec.Containers[0].Image
			}
			return nil
		}).WithTimeout(timeout).WithPolling(time.Millisecond * 500).Should(SatisfyAny(BeNil()))
	}

	checkNodeStatus := func(nodeName string) error {
		node := &corev1.Node{}
		if err := k8sClient.Get(ctx, client.ObjectKey{Name: nodeName}, node); err != nil {
			return err
		}
		for _, condition := range node.Status.Conditions {
			if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
				return nil
			}
		}
		return fmt.Errorf("node openyurt-e2e-test-worker2 is not ready")
	}

	reconnectNode := func(nodeName string) {
		// reconnect node
		cmd := exec.Command("/bin/bash", "-c", "docker network connect kind "+nodeName)
		err := cmd.Run()
		Expect(err).NotTo(HaveOccurred(), "fail to reconnect "+nodeName+" node to kind bridge")

		Eventually(func() error {
			return checkNodeStatus(nodeName)
		}).WithTimeout(120 * time.Second).WithPolling(1 * time.Second).Should(Succeed())
		By("node " + nodeName + "becomes ready again")

		// restart flannel pod on node to recover flannel NIC
		Eventually(func() error {
			flannelPods := &corev1.PodList{}
			if err := k8sClient.List(ctx, flannelPods, client.InNamespace(FlannelNamespace)); err != nil {
				return err
			}
			if len(flannelPods.Items) != 3 {
				return fmt.Errorf("not reconcile")
			}
			for _, pod := range flannelPods.Items {
				if pod.Spec.NodeName == nodeName {
					if err := k8sClient.Delete(ctx, &pod); err != nil {
						return err
					}
				}
			}
			return nil
		}).WithTimeout(timeout).Should(SatisfyAny(BeNil()))
	}

	BeforeEach(func() {
		By("Start to run yurtStaticSet test, clean up previous resources")
		nodeToImageMap = map[string]string{}
		k8sClient = ycfg.YurtE2eCfg.RuntimeClient
		namespaceName = "yurtstaticset-e2e-test" + "-" + rand.String(4)
		createNamespace(namespaceName)
	})

	AfterEach(func() {
		By("Cleanup resources after test")
		deleteStaticPod("openyurt-e2e-test-worker")
		deleteStaticPod("openyurt-e2e-test-worker2")

		By(fmt.Sprintf("Delete the entire namespaceName %s", namespaceName))
		Expect(k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespaceName}}, client.PropagationPolicy(metav1.DeletePropagationForeground))).Should(BeNil())
	})

	Describe("Test YurtStaticSet AdvancedRollingUpdate upgrade model", func() {
		It("Test one worker disconnect", func() {
			By("Run staticpod AdvancedRollingUpdate upgrade model test")
			// disconnect openyurt-e2e-test-worker2 node
			cmd := exec.Command("/bin/bash", "-c", "docker network disconnect kind openyurt-e2e-test-worker2")
			err := cmd.Run()
			Expect(err).NotTo(HaveOccurred(), "fail to disconnect openyurt-e2e-test-worker2 node to kind bridge: docker network disconnect kind %s")
			Eventually(func() error {
				return checkNodeStatus("openyurt-e2e-test-worker2")
			}).WithTimeout(120 * time.Second).WithPolling(1 * time.Second).Should(SatisfyAll(HaveOccurred(), Not(&util.NotFoundMatcher{})))
			By("node openyurt-e2e-test-worker2 is not ready, then start to update yurtstaticset")

			// update the yurtStaticSet
			updateYurtStaticSet()
			By("yurtstaticset is updated, start to check pod image")

			// check image version
			Eventually(func() error {
				checkPodStatusAndUpdate()
				if nodeToImageMap["openyurt-e2e-test-worker"] == testImg2 && nodeToImageMap["openyurt-e2e-test-worker2"] == testImg1 {
					return nil
				}
				return fmt.Errorf("error image update")
			}).WithTimeout(timeout * 2).WithPolling(time.Millisecond * 1000).Should(Succeed())
			By("pod on node openyurt-e2e-test-worker is updated, then start to reconnect the node openyurt-e2e-test-worker2")

			// recover network environment
			reconnectNode("openyurt-e2e-test-worker2")
			By("node openyurt-e2e-test-worker2 is reconnected, and start to wait pod upgrade on node openyurt-e2e-test-worker2")

			// check image version
			Eventually(func() error {
				checkPodStatusAndUpdate()
				if nodeToImageMap["openyurt-e2e-test-worker"] == testImg2 && nodeToImageMap["openyurt-e2e-test-worker2"] == testImg2 {
					return nil
				}
				return fmt.Errorf("error image update")
			}).WithTimeout(timeout).WithPolling(time.Millisecond * 500).Should(Succeed())
			By("pod on node openyurt-e2e-test-worker2 is updated")
		})

		It("Testing situation where upgrade is not required", func() {
			Consistently(func() error {
				podList := &corev1.PodList{}
				if err := k8sClient.List(ctx, podList, client.InNamespace(namespaceName)); err != nil {
					return err
				}
				if len(podList.Items) != 2 {
					return fmt.Errorf("should no worker pod be created")
				}
				return nil
			}, 10*time.Second, 500*time.Millisecond).Should(Succeed())
		})

		BeforeEach(func() {
			By("Prepare for staticpod AdvancedRollingUpdate upgrade model test")
			updateStrategyType = "AdvancedRollingUpdate"

			createStaticPod("openyurt-e2e-test-worker")
			createStaticPod("openyurt-e2e-test-worker2")

			checkPodStatusAndUpdate()
			createYurtStaticSet()
		})

		AfterEach(func() {
			By("Reconnect openyurt-e2e-test-worker2 node if it is disconnected")
			if err := checkNodeStatus("openyurt-e2e-test-worker2"); err == nil {
				return
			}
			// reconnect openyurt-e2e-test-worker2 node to avoid impact on other tests
			reconnectNode("openyurt-e2e-test-worker2")
		})
	})

	Describe("Test YurtStaticSet ota upgrade model", func() {
		It("Test ota update for one worker", func() {
			By("Run staticpod ota upgrade model test")
			var pN2 string
			updateStrategyType = "OTA"

			createStaticPod("openyurt-e2e-test-worker")
			createStaticPod("openyurt-e2e-test-worker2")

			createYurtStaticSet()
			checkPodStatusAndUpdate()

			// update the yurtstaticset
			updateYurtStaticSet()

			// check status condition PodNeedUpgrade
			Eventually(func() error {
				testPods := &corev1.PodList{}
				if err := k8sClient.List(ctx, testPods, client.InNamespace(namespaceName), client.MatchingLabels{"app": podName}); err != nil {
					return err
				}
				if len(testPods.Items) != 2 {
					return fmt.Errorf("not reconcile")
				}
				for _, pod := range testPods.Items {
					for _, condition := range pod.Status.Conditions {
						if condition.Type == PodNeedUpgrade && condition.Status != corev1.ConditionTrue {
							return fmt.Errorf("pod %s status condition PodNeedUpgrade is not true", pod.Name)
						}
					}
					if pod.Spec.NodeName == "openyurt-e2e-test-worker2" {
						pN2 = pod.Name
					}
				}
				return nil
			}).WithTimeout(timeout).WithPolling(time.Millisecond * 500).Should(SatisfyAny(BeNil()))

			// ota update for openyurt-e2e-test-worker2 node
			Eventually(func() string {
				curlCmd := fmt.Sprintf("curl -X POST %s:%s/openyurt.io/v1/namespaces/%s/pods/%s/upgrade", ServerName, ServerPort, namespaceName, pN2)
				opBytes, err := exec.Command("/bin/bash", "-c", "docker exec -t openyurt-e2e-test-worker2 /bin/bash -c '"+curlCmd+"'").CombinedOutput()

				if err != nil {
					return ""
				}
				return string(opBytes)
			}).WithTimeout(10*time.Second).WithPolling(1*time.Second).Should(ContainSubstring("Start updating pod"), "fail to ota update for pod")

			// check image version
			Eventually(func() error {
				checkPodStatusAndUpdate()
				if nodeToImageMap["openyurt-e2e-test-worker"] == testImg1 && nodeToImageMap["openyurt-e2e-test-worker2"] == testImg2 {
					return nil
				}
				return fmt.Errorf("error image update")
			}).WithTimeout(timeout).WithPolling(time.Millisecond * 500).Should(Succeed())
		})
	})
})
