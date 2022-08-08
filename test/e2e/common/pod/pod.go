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

package pod

import (
	"context"
	"fmt"
	"time"

	"github.com/onsi/ginkgo/v2"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/test/e2e/util"
)

func ListPods(c clientset.Interface, ns string) (pods *apiv1.PodList, err error) {
	return c.CoreV1().Pods(ns).List(context.Background(), metav1.ListOptions{})
}

func CreatePod(c clientset.Interface, ns string, objectMeta metav1.ObjectMeta, spec apiv1.PodSpec) (pods *apiv1.Pod, err error) {

	p := &apiv1.Pod{}
	p.ObjectMeta = objectMeta
	p.Spec = spec
	return c.CoreV1().Pods(ns).Create(context.Background(), p, metav1.CreateOptions{})
}

func GetPod(c clientset.Interface, ns, podName string) (pod *apiv1.Pod, err error) {
	return c.CoreV1().Pods(ns).Get(context.Background(), podName, metav1.GetOptions{})
}

func DeletePod(c clientset.Interface, ns, podName string) (err error) {
	return c.CoreV1().Pods(ns).Delete(context.Background(), podName, metav1.DeleteOptions{})
}

func VerifyPodsRunning(c clientset.Interface, ns, podName string, wantName bool, replicas int32) error {
	pods, err := PodsCreated(c, ns, podName, replicas)
	if err != nil {
		return err
	}
	e := podsRunning(c, pods)
	if len(e) > 0 {
		return fmt.Errorf("failed to wait for pods running: %v", e)
	}
	return nil
}

func WaitTimeoutForPodRunning(c clientset.Interface, podName, ns string, timeout time.Duration) error {
	return wait.PollImmediate(2*time.Second, timeout, podRunning(c, podName, ns))
}

// PodsCreated returns a pod list matched by the given name.
func PodsCreated(c clientset.Interface, ns, name string, replicas int32) (*apiv1.PodList, error) {
	label := labels.SelectorFromSet(labels.Set(map[string]string{"name": name}))
	return PodsCreatedByLabel(c, ns, name, replicas, label)
}

// PodsCreatedByLabel returns a created pod list matched by the given label.
func PodsCreatedByLabel(c clientset.Interface, ns, name string, replicas int32, label labels.Selector) (*apiv1.PodList, error) {
	timeout := 2 * time.Minute
	for start := time.Now(); time.Since(start) < timeout; time.Sleep(5 * time.Second) {
		options := metav1.ListOptions{LabelSelector: label.String()}

		// List the pods, making sure we observe all the replicas.
		pods, err := c.CoreV1().Pods(ns).List(context.TODO(), options)
		if err != nil {
			return nil, err
		}

		created := []apiv1.Pod{}
		for _, pod := range pods.Items {
			if pod.DeletionTimestamp != nil {
				continue
			}
			created = append(created, pod)
		}
		klog.Infof("Pod name %s: Found %d pods out of %d", name, len(created), replicas)

		if int32(len(created)) == replicas {
			pods.Items = created
			return pods, nil
		}
	}
	return nil, fmt.Errorf("Pod name %s: Gave up waiting %v for %d pods to come up", name, timeout, replicas)
}

func podsRunning(c clientset.Interface, pods *apiv1.PodList) []error {
	// Wait for the pods to enter the running state. Waiting loops until the pods
	// are running so non-running pods cause a timeout for this test.
	ginkgo.By("ensuring each pod is running")
	e := []error{}
	errorChan := make(chan error)

	for _, pod := range pods.Items {
		go func(p apiv1.Pod) {
			errorChan <- WaitForPodRunningInNamespace(c, &p)
		}(pod)
	}

	for range pods.Items {
		err := <-errorChan
		if err != nil {
			e = append(e, err)
		}
	}

	return e
}

// WaitForPodRunningInNamespace waits default amount of time (podStartTimeout) for the specified pod to become running.
// Returns an error if timeout occurs first, or pod goes in to failed state.
func WaitForPodRunningInNamespace(c clientset.Interface, pod *apiv1.Pod) error {
	if pod.Status.Phase == apiv1.PodRunning {
		return nil
	}

	return wait.PollImmediate(2*time.Second, util.PodStartTimeout, podRunning(c, pod.Name, pod.Namespace))
}

func podRunning(c clientset.Interface, podName, namespace string) wait.ConditionFunc {
	return func() (bool, error) {
		pod, err := c.CoreV1().Pods(namespace).Get(context.TODO(), podName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		switch pod.Status.Phase {
		case apiv1.PodRunning:
			return true, nil
		case apiv1.PodFailed, apiv1.PodSucceeded:
			return false, fmt.Errorf("pod ran to completion")
		}
		return false, nil
	}
}
