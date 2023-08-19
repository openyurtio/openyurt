/*
Copyright 2017 The Kubernetes Authors.

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

package apiclient

import (
	"context"

	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	clientsetretry "k8s.io/client-go/util/retry"

	"github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
	"github.com/openyurtio/openyurt/pkg/util/kubernetes/kubeadm/app/constants"
)

// ConfigMapMutator is a function that mutates the given ConfigMap and optionally returns an error
type ConfigMapMutator func(*v1.ConfigMap) error

// TODO: We should invent a dynamic mechanism for this using the dynamic client instead of hard-coding these functions per-type

// CreateOrUpdateConfigMap creates a ConfigMap if the target resource doesn't exist. If the resource exists already, this function will update the resource instead.
func CreateOrUpdateConfigMap(client clientset.Interface, cm *v1.ConfigMap) error {
	if _, err := client.CoreV1().ConfigMaps(cm.ObjectMeta.Namespace).Create(context.TODO(), cm, metav1.CreateOptions{}); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return errors.Wrap(err, "unable to create ConfigMap")
		}

		if _, err := client.CoreV1().ConfigMaps(cm.ObjectMeta.Namespace).Update(context.TODO(), cm, metav1.UpdateOptions{}); err != nil {
			return errors.Wrap(err, "unable to update ConfigMap")
		}
	}
	return nil
}

// CreateOrUpdateSecret creates a Secret if the target resource doesn't exist. If the resource exists already, this function will update the resource instead.
func CreateOrUpdateSecret(client clientset.Interface, secret *v1.Secret) error {
	if _, err := client.CoreV1().Secrets(secret.ObjectMeta.Namespace).Create(context.TODO(), secret, metav1.CreateOptions{}); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return errors.Wrap(err, "unable to create secret")
		}

		if _, err := client.CoreV1().Secrets(secret.ObjectMeta.Namespace).Update(context.TODO(), secret, metav1.UpdateOptions{}); err != nil {
			return errors.Wrap(err, "unable to update secret")
		}
	}
	return nil
}

// CreateOrUpdateRole creates a Role if the target resource doesn't exist. If the resource exists already, this function will update the resource instead.
func CreateOrUpdateRole(client clientset.Interface, role *rbac.Role) error {
	var lastError error
	err := wait.PollImmediate(constants.APICallRetryInterval, constants.APICallWithWriteTimeout, func() (bool, error) {
		if _, err := client.RbacV1().Roles(role.ObjectMeta.Namespace).Create(context.TODO(), role, metav1.CreateOptions{}); err != nil {
			if !apierrors.IsAlreadyExists(err) {
				lastError = errors.Wrap(err, "unable to create RBAC role")
				return false, nil
			}

			if _, err := client.RbacV1().Roles(role.ObjectMeta.Namespace).Update(context.TODO(), role, metav1.UpdateOptions{}); err != nil {
				lastError = errors.Wrap(err, "unable to update RBAC role")
				return false, nil
			}
		}
		return true, nil
	})
	if err == nil {
		return nil
	}
	return lastError
}

// CreateOrUpdateRoleBinding creates a RoleBinding if the target resource doesn't exist. If the resource exists already, this function will update the resource instead.
func CreateOrUpdateRoleBinding(client clientset.Interface, roleBinding *rbac.RoleBinding) error {
	var lastError error
	err := wait.PollImmediate(constants.APICallRetryInterval, constants.APICallWithWriteTimeout, func() (bool, error) {
		if _, err := client.RbacV1().RoleBindings(roleBinding.ObjectMeta.Namespace).Create(context.TODO(), roleBinding, metav1.CreateOptions{}); err != nil {
			if !apierrors.IsAlreadyExists(err) {
				lastError = errors.Wrap(err, "unable to create RBAC rolebinding")
				return false, nil
			}

			if _, err := client.RbacV1().RoleBindings(roleBinding.ObjectMeta.Namespace).Update(context.TODO(), roleBinding, metav1.UpdateOptions{}); err != nil {
				lastError = errors.Wrap(err, "unable to update RBAC rolebinding")
				return false, nil
			}
		}
		return true, nil
	})
	if err == nil {
		return nil
	}
	return lastError
}

// GetConfigMapWithRetry tries to retrieve a ConfigMap using the given client,
// retrying if we get an unexpected error.
//
// TODO: evaluate if this can be done better. Potentially remove the retry if feasible.
func GetConfigMapWithRetry(client clientset.Interface, namespace, name string) (*v1.ConfigMap, error) {
	var cm *v1.ConfigMap
	var lastError error
	err := wait.ExponentialBackoff(clientsetretry.DefaultBackoff, func() (bool, error) {
		var err error
		cm, err = client.CoreV1().ConfigMaps(namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if err == nil {
			return true, nil
		}
		lastError = err
		return false, nil
	})
	if err == nil {
		return cm, nil
	}
	return nil, lastError
}

func GetNodePoolInfoWithRetry(cfg *clientcmdapi.Config, name string) (*v1beta1.NodePool, error) {
	gvr := v1beta1.GroupVersion.WithResource("nodepools")

	clientConfig := clientcmd.NewDefaultClientConfig(*cfg, &clientcmd.ConfigOverrides{})
	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, err
	}
	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, err
	}

	var obj *unstructured.Unstructured
	var lastError error
	err = wait.ExponentialBackoff(clientsetretry.DefaultBackoff, func() (bool, error) {
		var err error
		obj, err = dynamicClient.Resource(gvr).Get(context.TODO(), name, metav1.GetOptions{})
		if err == nil {
			return true, nil
		}
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		lastError = err
		return false, nil
	})
	if err == nil {
		np := new(v1beta1.NodePool)
		if err = runtime.DefaultUnstructuredConverter.FromUnstructured(obj.UnstructuredContent(), np); err != nil {
			return nil, err
		}
		return np, nil
	}
	return nil, lastError
}
