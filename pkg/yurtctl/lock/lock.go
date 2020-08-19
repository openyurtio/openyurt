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
	"strconv"
	"time"

	"github.com/alibaba/openyurt/pkg/yurtctl/constants"
	"k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog"
)

const (
	lockFinalizer         = "kubernetes"
	AnnotationAcquireTime = "openyurt.io/yurtctllock.acquire.time"
	AnnotationIsLocked    = "openyurt.io/yurtctllock.locked"

	LockTimeoutMin = 5
)

var (
	AcquireLockErr error = errors.New("fail to acquire lock configmap/yurtctl-lock")
	ReleaseLockErr error = errors.New("fail to release lock configmap/yurtctl-lock")
)

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
					Annotations: map[string]string{
						AnnotationAcquireTime: strconv.FormatInt(time.Now().Unix(), 10),
						AnnotationIsLocked:    "true",
					},
				},
			}
			if _, err := cli.CoreV1().ConfigMaps("kube-system").
				Create(cm); err != nil {
				klog.Error("the lock configmap/yurtctl-lock is not found, " +
					"but fail to create a new one")
				return AcquireLockErr
			}
			return nil
		}
		return AcquireLockErr
	}

	if lockCm.Annotations[AnnotationIsLocked] == "true" {
		// check if the lock expired
		//
		// TODO The timeout mechanism is just a short-term workaround, in the
		// future version, we will use a CRD and controller to manage the
		// openyurt cluster, which also prevents the contention between users.
		old, err := strconv.ParseInt(lockCm.Annotations[AnnotationAcquireTime], 10, 64)
		if err != nil {
			return AcquireLockErr
		}
		if isTimeout(old) {
			// if the lock is expired, acquire it
			if err := acquireLockAndUpdateCm(cli, lockCm); err != nil {
				return err
			}
			return nil
		}

		// lock has been acquired by others
		klog.Errorf("the lock is held by others, it was being acquired at %s",
			lockCm.Annotations[AnnotationAcquireTime])
		return AcquireLockErr
	}

	if lockCm.Annotations[AnnotationIsLocked] == "false" {
		if err := acquireLockAndUpdateCm(cli, lockCm); err != nil {
			return err
		}
	}

	return nil
}

func isTimeout(old int64) bool {
	deadline := old + LockTimeoutMin*60
	return time.Now().Unix() > deadline
}

func acquireLockAndUpdateCm(cli kubernetes.Interface, lockCm *v1.ConfigMap) error {
	lockCm.Annotations[AnnotationIsLocked] = "true"
	lockCm.Annotations[AnnotationAcquireTime] = strconv.FormatInt(time.Now().Unix(), 10)
	if _, err := cli.CoreV1().ConfigMaps("kube-system").
		Update(lockCm); err != nil {
		if apierrors.IsResourceExpired(err) {
			klog.Error("the lock is held by others")
			return AcquireLockErr
		}
		klog.Error("successfully acquire the lock but fail to update it")
		return AcquireLockErr
	}
	return nil
}

// ReleaseLock releases the lock configmap/yurtctl-lock
func ReleaseLock(cli *kubernetes.Clientset) error {
	lockCm, err := cli.CoreV1().ConfigMaps("kube-system").
		Get(constants.YurtctlLockConfigMapName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			klog.Error("lock is not found when try to release, " +
				"please check if the configmap/yurtctl-lock " +
				"is being deleted manually")
			return ReleaseLockErr
		}
		klog.Error("fail to get lock configmap/yurtctl-lock, " +
			"when try to release it")
		return ReleaseLockErr
	}
	if lockCm.Annotations[AnnotationIsLocked] == "false" {
		klog.Error("lock has already been released, " +
			"please check if the configmap/yurtctl-lock " +
			"is being updated manually")
		return ReleaseLockErr
	}

	// release the lock
	lockCm.Annotations[AnnotationIsLocked] = "false"
	delete(lockCm.Annotations, AnnotationAcquireTime)

	_, err = cli.CoreV1().ConfigMaps("kube-system").Update(lockCm)
	if err != nil {
		if apierrors.IsResourceExpired(err) {
			klog.Error("lock has been touched by others during release, " +
				"which is not supposed to happen. " +
				"Please check if lock is being updated manually.")
			return ReleaseLockErr

		}
		klog.Error("fail to update lock configmap/yurtctl-lock, " +
			"when try to release it")
		return ReleaseLockErr
	}

	return nil
}
