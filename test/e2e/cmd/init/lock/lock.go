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
	"context"
	"errors"
	"strconv"
	"time"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeclientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/test/e2e/cmd/init/constants"
)

const (
	AnnotationAcquireTime = "openyurt.io/yurtctllock.acquire.time"
	AnnotationIsLocked    = "openyurt.io/yurtctllock.locked"

	LockTimeoutMin = 5
)

var (
	ErrAcquireLock error = errors.New("fail to acquire lock configmap/yurtctl-lock")
	ErrReleaseLock error = errors.New("fail to release lock configmap/yurtctl-lock")
)

// AcquireLock tries to acquire the lock lock configmap/yurtctl-lock
func AcquireLock(cli kubeclientset.Interface) error {
	lockCm, err := cli.CoreV1().ConfigMaps("kube-system").
		Get(context.Background(), constants.YurtctlLockConfigMapName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			// the lock is not exist, create one
			cm := &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.YurtctlLockConfigMapName,
					Namespace: "kube-system",
					Annotations: map[string]string{
						AnnotationAcquireTime: strconv.FormatInt(time.Now().Unix(), 10),
						AnnotationIsLocked:    "true",
					},
				},
			}
			if _, err := cli.CoreV1().ConfigMaps("kube-system").
				Create(context.Background(), cm, metav1.CreateOptions{}); err != nil {
				klog.Error("the lock configmap/yurtctl-lock is not found, " +
					"but fail to create a new one")
				return ErrAcquireLock
			}
			return nil
		}
		return ErrAcquireLock
	}

	if lockCm.Annotations[AnnotationIsLocked] == "true" {
		// check if the lock expired
		//
		// TODO The timeout mechanism is just a short-term workaround, in the
		// future version, we will use a CRD and controller to manage the
		// openyurt cluster, which also prevents the contention between users.
		old, err := strconv.ParseInt(lockCm.Annotations[AnnotationAcquireTime], 10, 64)
		if err != nil {
			return ErrAcquireLock
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
		return ErrAcquireLock
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

func acquireLockAndUpdateCm(cli kubeclientset.Interface, lockCm *v1.ConfigMap) error {
	lockCm.Annotations[AnnotationIsLocked] = "true"
	lockCm.Annotations[AnnotationAcquireTime] = strconv.FormatInt(time.Now().Unix(), 10)
	if _, err := cli.CoreV1().ConfigMaps("kube-system").
		Update(context.Background(), lockCm, metav1.UpdateOptions{}); err != nil {
		if apierrors.IsResourceExpired(err) {
			klog.Error("the lock is held by others")
			return ErrAcquireLock
		}
		klog.Error("successfully acquire the lock but fail to update it")
		return ErrAcquireLock
	}
	return nil
}

// ReleaseLock releases the lock configmap/yurtctl-lock
func ReleaseLock(cli kubeclientset.Interface) error {
	lockCm, err := cli.CoreV1().ConfigMaps("kube-system").
		Get(context.Background(), constants.YurtctlLockConfigMapName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			klog.Error("lock is not found when try to release, " +
				"please check if the configmap/yurtctl-lock " +
				"is being deleted manually")
			return ErrReleaseLock
		}
		klog.Error("fail to get lock configmap/yurtctl-lock, " +
			"when try to release it")
		return ErrReleaseLock
	}
	if lockCm.Annotations[AnnotationIsLocked] == "false" {
		klog.Error("lock has already been released, " +
			"please check if the configmap/yurtctl-lock " +
			"is being updated manually")
		return ErrReleaseLock
	}

	// release the lock
	lockCm.Annotations[AnnotationIsLocked] = "false"
	delete(lockCm.Annotations, AnnotationAcquireTime)

	_, err = cli.CoreV1().ConfigMaps("kube-system").Update(context.Background(), lockCm, metav1.UpdateOptions{})
	if err != nil {
		if apierrors.IsResourceExpired(err) {
			klog.Error("lock has been touched by others during release, " +
				"which is not supposed to happen. " +
				"Please check if lock is being updated manually.")
			return ErrReleaseLock

		}
		klog.Error("fail to update lock configmap/yurtctl-lock, " +
			"when try to release it")
		return ErrReleaseLock
	}

	return nil
}

// DeleteLock should only be called when you've achieved the lock.
// It will delete the yurtctl-lock configmap.
func DeleteLock(cli kubeclientset.Interface) error {
	if err := cli.CoreV1().ConfigMaps("kube-system").
		Delete(context.Background(), constants.YurtctlLockConfigMapName, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		klog.Error("fail to delete the yurtctl lock", err)
		return err
	}
	return nil
}
