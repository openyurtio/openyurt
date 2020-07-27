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

package ns

import (
	"github.com/onsi/gomega"
	apiv1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
)

func DeleteNameSpace(c clientset.Interface, ns string) (err error) {
	deletePolicy := metav1.DeletePropagationForeground
	err = c.CoreV1().Namespaces().Delete(ns, &metav1.DeleteOptions{
		PropagationPolicy: &deletePolicy,
	})
	if !apierrs.IsNotFound(err) {
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail to delete created namespaces:"+ns)
	}
	err = framework.WaitForNamespacesDeleted(c, []string{ns}, framework.DefaultNamespaceDeletionTimeout)
	return
}

func CreateNameSpace(c clientset.Interface, ns string) (result *apiv1.Namespace, err error) {
	namespaceClient := c.CoreV1().Namespaces()
	namespace := &apiv1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: ns,
		},
	}
	result, err = namespaceClient.Create(namespace)
	return
}

func ListNameSpaces(c clientset.Interface) (result *apiv1.NamespaceList, err error) {
	return c.CoreV1().Namespaces().List(metav1.ListOptions{})
}

func GetNameSpace(c clientset.Interface, ns string) (result *apiv1.Namespace, err error) {
	return c.CoreV1().Namespaces().Get(ns, metav1.GetOptions{})
}
