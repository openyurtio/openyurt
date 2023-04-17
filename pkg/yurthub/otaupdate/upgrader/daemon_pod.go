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

package upgrader

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

type DaemonPodUpgrader struct {
	kubernetes.Interface
	types.NamespacedName
}

// Apply execute pod update process by deleting pod under OnDelete update strategy
func (s *DaemonPodUpgrader) Apply() error {
	err := s.CoreV1().Pods(s.Namespace).Delete(context.TODO(), s.Name, metav1.DeleteOptions{})
	if err != nil {
		klog.Errorf("Update pod %v/%v failed when delete, %v", s.Namespace, s.Name, err)
		return err
	}

	klog.Infof("Start updating pod: %v/%v", s.Namespace, s.Name)
	return nil
}
