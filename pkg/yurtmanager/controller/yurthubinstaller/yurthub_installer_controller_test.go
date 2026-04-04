/*
Copyright 2026 The OpenYurt Authors.

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

package yurthubinstaller

import (
	"context"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	installerconfig "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/yurthubinstaller/config"
)

func TestReconcileYurtHubInstaller_Reconcile(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = batchv1.AddToScheme(scheme)

	tests := []struct {
		name          string
		node          *corev1.Node
		existingJobs  []*batchv1.Job
		expectedJob   bool
		expectedAnnot map[string]string
	}{
		{
			name: "install yurthub when label is added",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-1",
					Labels: map[string]string{
						EdgeWorkerLabel: "true",
					},
				},
			},
			expectedJob: true,
			expectedAnnot: map[string]string{
				YurtHubInstallationInProgressAnnotation: "true",
			},
		},
		{
			name: "uninstall yurthub when label is removed",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-node-2",
					Labels: map[string]string{},
					Annotations: map[string]string{
						YurtHubInstalledAnnotation: "true",
					},
				},
			},
			expectedJob: true,
			expectedAnnot: map[string]string{
				YurtHubInstallationInProgressAnnotation: "true",
			},
		},
		{
			name: "do nothing when already installed",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-3",
					Labels: map[string]string{
						EdgeWorkerLabel: "true",
					},
					Annotations: map[string]string{
						YurtHubInstalledAnnotation: "true",
					},
				},
			},
			expectedJob: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objs := []client.Object{tt.node}
			for _, job := range tt.existingJobs {
				objs = append(objs, job)
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(objs...).
				Build()

			r := &ReconcileYurtHubInstaller{
				Client:   fakeClient,
				recorder: record.NewFakeRecorder(100),
				cfg: installerconfig.YurtHubInstallerControllerConfiguration{
					NodeServantImage: "test-image:latest",
					YurtHubVersion:   "latest",
				},
			}

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: tt.node.Name,
				},
			}

			_, err := r.Reconcile(context.Background(), req)
			if err != nil {
				t.Errorf("Reconcile() error = %v", err)
			}

			// Check if job was created
			jobList := &batchv1.JobList{}
			err = fakeClient.List(context.Background(), jobList, client.InNamespace("kube-system"))
			if err != nil {
				t.Errorf("Failed to list jobs: %v", err)
			}

			if tt.expectedJob && len(jobList.Items) == 0 {
				t.Errorf("Expected job to be created, but none found")
			}

			// Check node annotations
			if tt.expectedAnnot != nil {
				updatedNode := &corev1.Node{}
				err = fakeClient.Get(context.Background(), types.NamespacedName{Name: tt.node.Name}, updatedNode)
				if err != nil {
					t.Errorf("Failed to get updated node: %v", err)
				}

				for key, expectedValue := range tt.expectedAnnot {
					if actualValue, ok := updatedNode.Annotations[key]; !ok || actualValue != expectedValue {
						t.Errorf("Expected annotation %s=%s, got %s=%s", key, expectedValue, key, actualValue)
					}
				}
			}
		})
	}
}

func TestIsYurtHubJob(t *testing.T) {
	tests := []struct {
		name     string
		job      *batchv1.Job
		expected bool
	}{
		{
			name: "node-servant convert job",
			job: &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-servant-convert-node1",
				},
			},
			expected: true,
		},
		{
			name: "node-servant revert job",
			job: &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-servant-revert-node1",
				},
			},
			expected: true,
		},
		{
			name: "unrelated job",
			job: &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name: "some-other-job",
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isYurtHubJob(tt.job)
			if result != tt.expected {
				t.Errorf("isYurtHubJob() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestGetNodeNameFromJob(t *testing.T) {
	tests := []struct {
		name     string
		job      *batchv1.Job
		expected string
	}{
		{
			name: "job with nodeName in spec",
			job: &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-job",
				},
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							NodeName: "test-node",
						},
					},
				},
			},
			expected: "test-node",
		},
		{
			name: "convert job with node name in job name",
			job: &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-servant-convert-mynode",
				},
			},
			expected: "mynode",
		},
		{
			name: "revert job with node name in job name",
			job: &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-servant-revert-mynode",
				},
			},
			expected: "mynode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getNodeNameFromJob(tt.job)
			if result != tt.expected {
				t.Errorf("getNodeNameFromJob() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestCheckInstallationProgress(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = batchv1.AddToScheme(scheme)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Annotations: map[string]string{
				YurtHubInstallationInProgressAnnotation: "true",
			},
		},
	}

	successJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "node-servant-convert-test-node",
			Namespace: "kube-system",
			Labels: map[string]string{
				"node": "test-node",
			},
			CreationTimestamp: metav1.Time{Time: time.Now()},
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					NodeName: "test-node",
					Containers: []corev1.Container{
						{
							Name: "node-servant",
							Args: []string{"/usr/local/bin/entry.sh convert"},
						},
					},
				},
			},
		},
		Status: batchv1.JobStatus{
			Succeeded: 1,
		},
	}

	objs := []client.Object{node, successJob}
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		Build()

	r := &ReconcileYurtHubInstaller{
		Client:   fakeClient,
		recorder: record.NewFakeRecorder(100),
		cfg: installerconfig.YurtHubInstallerControllerConfiguration{
			NodeServantImage: "test-image:latest",
		},
	}

	result, err := r.checkInstallationProgress(context.Background(), node)
	if err != nil {
		t.Errorf("checkInstallationProgress() error = %v", err)
	}

	// Should not requeue as job completed
	if result.RequeueAfter != 0 {
		t.Errorf("Expected no requeue, got RequeueAfter = %v", result.RequeueAfter)
	}

	// Verify node annotations updated
	updatedNode := &corev1.Node{}
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: node.Name}, updatedNode)
	if err != nil {
		t.Errorf("Failed to get updated node: %v", err)
	}

	if updatedNode.Annotations[YurtHubInstalledAnnotation] != "true" {
		t.Errorf("Expected installed annotation to be set")
	}

	if _, exists := updatedNode.Annotations[YurtHubInstallationInProgressAnnotation]; exists {
		t.Errorf("Expected in-progress annotation to be removed")
	}
}
