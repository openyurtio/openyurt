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

package util

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
)

const (
	YamlSuffix    string = ".yaml"
	BackupSuffix         = ".bak"
	UpgradeSuffix string = ".upgrade"

	StaticPodHashAnnotation = "openyurt.io/static-pod-hash"
)

func WithYamlSuffix(path string) string {
	return path + YamlSuffix
}

func WithBackupSuffix(path string) string {
	return path + BackupSuffix
}

func WithUpgradeSuffix(path string) string {
	return path + UpgradeSuffix
}

// CopyFile copy file content from src to dst, if destination file not exist, then create it
func CopyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0666)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}
	return nil
}

// WaitForPodRunning waits static pod to run
// Success: Static pod annotation `StaticPodHashAnnotation` value equals to function argument hash
// Failed: Receive PodFailed event
func WaitForPodRunning(namespace, name, hash string, timeout time.Duration) (bool, error) {
	klog.Infof("WaitForPodRunning namespace is %s, name is %s", namespace, name)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	checkPod := func(pod *v1.Pod) (hasResult, result bool) {
		h := pod.Annotations[StaticPodHashAnnotation]
		if pod.Status.Phase == v1.PodRunning && h == hash {
			return true, true
		}

		if pod.Status.Phase == v1.PodFailed {
			return true, false
		}

		return false, false
	}

	for {
		select {
		case <-ctx.Done():
			return false, fmt.Errorf("timeout waiting for static pod %s/%s to be running", namespace, name)
		case <-ticker.C:
			pod, err := GetPodFromYurtHub(namespace, name)
			if err != nil {
				klog.V(4).Infof("Temporarily fail to get pod from YurtHub, %v", err)
			}
			if pod != nil {
				hasResult, result := checkPod(pod)
				if hasResult {
					return result, nil
				}
			}
		}
	}
}
