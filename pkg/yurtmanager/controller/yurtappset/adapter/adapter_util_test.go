/*
Copyright 2021 The OpenYurt Authors.
Copyright 2019 The Kruise Authors.
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
package adapter

import (
	"fmt"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openyurtio/openyurt/pkg/apis/apps"
)

func TestGetCurrentPartitionForStrategyOnDelete(t *testing.T) {
	currentPods := buildPodList([]int{0, 1, 2}, []string{"v1", "v2", "v2"}, t)
	if partition := getCurrentPartition(currentPods, "v2"); *partition != 1 {
		t.Fatalf("expected partition 1, got %d", *partition)
	}
	currentPods = buildPodList([]int{0, 1, 2}, []string{"v1", "v1", "v2"}, t)
	if partition := getCurrentPartition(currentPods, "v2"); *partition != 2 {
		t.Fatalf("expected partition 2, got %d", *partition)
	}
	currentPods = buildPodList([]int{0, 1, 2, 3}, []string{"v2", "v1", "v2", "v2"}, t)
	if partition := getCurrentPartition(currentPods, "v2"); *partition != 1 {
		t.Fatalf("expected partition 1, got %d", *partition)
	}
	currentPods = buildPodList([]int{1, 2, 3}, []string{"v1", "v2", "v2"}, t)
	if partition := getCurrentPartition(currentPods, "v2"); *partition != 1 {
		t.Fatalf("expected partition 1, got %d", *partition)
	}
	currentPods = buildPodList([]int{0, 1, 3}, []string{"v2", "v1", "v2"}, t)
	if partition := getCurrentPartition(currentPods, "v2"); *partition != 1 {
		t.Fatalf("expected partition 1, got %d", *partition)
	}
	currentPods = buildPodList([]int{0, 1, 2}, []string{"v1", "v1", "v1"}, t)
	if partition := getCurrentPartition(currentPods, "v2"); *partition != 3 {
		t.Fatalf("expected partition 3, got %d", *partition)
	}
	currentPods = buildPodList([]int{0, 1, 2, 4}, []string{"v1", "", "v2", "v3"}, t)
	if partition := getCurrentPartition(currentPods, "v2"); *partition != 3 {
		t.Fatalf("expected partition 3, got %d", *partition)
	}
}
func buildPodList(ordinals []int, revisions []string, t *testing.T) []*corev1.Pod {
	if len(ordinals) != len(revisions) {
		t.Fatalf("ordinals count should equals to revision count")
	}
	pods := []*corev1.Pod{}
	for i, ordinal := range ordinals {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      fmt.Sprintf("pod-%d", ordinal),
			},
		}
		if revisions[i] != "" {
			pod.Labels = map[string]string{
				apps.ControllerRevisionHashLabelKey: revisions[i],
			}
		}
		pods = append(pods, pod)
	}
	return pods
}
func TestCreateNewPatchedObject(t *testing.T) {
	cases := []struct {
		Name          string
		PatchInfo     *runtime.RawExtension
		OldObj        *appsv1.Deployment
		EqualFunction func(new *appsv1.Deployment) bool
	}{
		{
			Name:      "replace image",
			PatchInfo: &runtime.RawExtension{Raw: []byte(`{"spec":{"template":{"spec":{"containers":[{"image":"nginx:1.18.0","name":"nginx"}]}}}}`)},
			OldObj: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:1.19.0",
								},
							},
						},
					},
				},
			},
			EqualFunction: func(new *appsv1.Deployment) bool {
				return new.Spec.Template.Spec.Containers[0].Image == "nginx:1.18.0"
			},
		},
		{
			Name:      "add other image",
			PatchInfo: &runtime.RawExtension{Raw: []byte(`{"spec":{"template":{"spec":{"containers":[{"image":"nginx:1.18.0","name":"nginx111"}]}}}}`)},
			OldObj: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:1.19.0",
								},
							},
						},
					},
				},
			},
			EqualFunction: func(new *appsv1.Deployment) bool {
				if len(new.Spec.Template.Spec.Containers) != 2 {
					return false
				}
				containerMap := make(map[string]string)
				for _, container := range new.Spec.Template.Spec.Containers {
					containerMap[container.Name] = container.Image
				}
				image, ok := containerMap["nginx"]
				if !ok {
					return false
				}
				image1, ok := containerMap["nginx111"]
				if !ok {
					return false
				}
				return image == "nginx:1.19.0" && image1 == "nginx:1.18.0"
			},
		},
	}
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			newObj := &appsv1.Deployment{}
			if err := CreateNewPatchedObject(c.PatchInfo, c.OldObj, newObj); err != nil {
				t.Fatalf("%s CreateNewPatchedObject error %v", c.Name, err)
			}
			if !c.EqualFunction(newObj) {
				t.Fatalf("%s Not Expect equal function", c.Name)
			}
		})
	}
}
