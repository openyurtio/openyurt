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

package upgrade

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

const (
	YamlSuffix    string = ".yaml"
	BackupSuffix         = ".bak"
	UpgradeSuffix string = ".upgrade"

	StaticPodHashAnnotation     = "openyurt.io/static-pod-hash"
	OTALatestManifestAnnotation = "openyurt.io/ota-latest-version"
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
func WaitForPodRunning(c kubernetes.Interface, name, namespace, hash string, timeout time.Duration) (bool, error) {
	klog.Infof("WaitForPodRuning name is %s, namespace is %s", name, namespace)
	// Create a watcher to watch the pod's status
	watcher, err := c.CoreV1().Pods(namespace).Watch(context.TODO(), metav1.ListOptions{FieldSelector: "metadata.name=" + name})
	if err != nil {
		return false, err
	}
	defer watcher.Stop()

	// Create a channel to receive updates from the watcher
	ch := watcher.ResultChan()

	// Start a goroutine to monitor the pod's status
	running := make(chan struct{})
	failed := make(chan struct{})

	go func() {
		for event := range ch {
			obj, ok := event.Object.(*corev1.Pod)
			if !ok {
				continue
			}

			h := obj.Annotations[StaticPodHashAnnotation]

			if obj.Status.Phase == corev1.PodRunning && h == hash {
				close(running)
				return
			}

			if obj.Status.Phase == corev1.PodFailed {
				close(failed)
				return
			}
		}
	}()

	// Wait for watch event to finish or the timeout to expire
	select {
	case <-running:
		return true, nil
	case <-failed:
		return false, nil
	case <-time.After(timeout):
		return false, fmt.Errorf("timeout waiting for static pod %s/%s to be running", namespace, name)
	}
}
