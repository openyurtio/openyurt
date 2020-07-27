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
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	framepod "k8s.io/kubernetes/test/e2e/framework/pod"
	"time"
)

func ListPods(c clientset.Interface, ns string) (pods *apiv1.PodList, err error) {
	return c.CoreV1().Pods(ns).List(metav1.ListOptions{})
}

func CreatePod(c clientset.Interface, ns string, objectMeta metav1.ObjectMeta, spec apiv1.PodSpec) (pods *apiv1.Pod, err error) {

	p := &apiv1.Pod{}
	p.ObjectMeta = objectMeta
	p.Spec = spec
	return c.CoreV1().Pods(ns).Create(p)
}

func GetPod(c clientset.Interface, ns, podName string) (pod *apiv1.Pod, err error) {
	return c.CoreV1().Pods(ns).Get(podName, metav1.GetOptions{})
}

func DeletePod(c clientset.Interface, ns, podName string) (err error) {
	return c.CoreV1().Pods(ns).Delete(podName, &metav1.DeleteOptions{})
}

func VerifyPodsRunning(c clientset.Interface, ns, podName string, wantName bool, replicas int32) error {
	return framepod.VerifyPodsRunning(c, ns, podName, wantName, replicas)
}

func WaitTimeoutForPodRunning(c clientset.Interface, podName, ns string, timeout time.Duration) error {
	return framepod.WaitTimeoutForPodRunningInNamespace(c, podName, ns, timeout)
}
