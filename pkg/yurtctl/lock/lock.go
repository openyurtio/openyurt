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

package lock

import (
	"errors"

	"github.com/alibaba/openyurt/pkg/yurtctl/constants"
	"k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const lockFinalizer = "kubernetes"

// AcquireLock tries to acquire the lock lock configmap/yurtctl-lock
func AcquireLock(cli *kubernetes.Clientset) error {
	lockCm, err := cli.CoreV1().ConfigMaps("kube-system").
		Get(constants.YurtctlLockConfigMapName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			// the lock is not exist, create one
			cm := &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:       constants.YurtctlLockConfigMapName,
					Namespace:  "kube-system",
					Finalizers: []string{lockFinalizer},
				},
				Data: map[string]string{"locked": "true"},
			}
			if _, err := cli.CoreV1().ConfigMaps("kube-system").
				Create(cm); err != nil {
				if apierrors.IsAlreadyExists(err) {
					return errors.New("fail to create the new lock")
				}
				return err
			}
			return nil
		}
	}

	if lockCm.Data["locked"] == "true" {
		// lock has been acquired by others
		return errors.New("the lock is held by others")
	}

	if lockCm.Data["locked"] == "false" {
		lockCm.Data["locked"] = "true"
		if _, err := cli.CoreV1().ConfigMaps("kube-system").
			Update(lockCm); err != nil {
			if apierrors.IsResourceExpired(err) {
				return errors.New("the lock is held by others")
			}
			return err
		}
	}

	return nil
}

// ReleaseLock releases the lock configmap/yurtctl-lock
func ReleaseLock(cli *kubernetes.Clientset) error {
	lockCm, err := cli.CoreV1().ConfigMaps("kube-system").
		Get(constants.YurtctlLockConfigMapName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return errors.New("lock not found")
		}
		return err
	}
	if lockCm.Data["locked"] == "false" {
		return errors.New("lock has already been released")
	}

	// release the lock
	lockCm.Data["locked"] = "false"

	_, err = cli.CoreV1().ConfigMaps("kube-system").Update(lockCm)
	if err != nil {
		if apierrors.IsResourceExpired(err) {
			return errors.New("lock has been touched by" +
				"others during release")
		}
		return err
	}

	return nil
}
