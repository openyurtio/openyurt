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

package kubernetes

import (
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientsetfake "k8s.io/client-go/kubernetes/fake"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
)

func TestAddEdgeWorkerLabelAndAutonomyAnnotation(t *testing.T) {
	cases := []struct {
		node *corev1.Node
		want *corev1.Node
		lval string
		aval string
	}{
		{
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cloud-node",
					Labels: map[string]string{
						"foo": "yeah~",
					},
					Annotations: map[string]string{
						"foo": "yeah~",
					},
				},
			},
			want: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "cloud-node",
					Labels:      map[string]string{},
					Annotations: map[string]string{},
				},
			},
			lval: "foo",
			aval: "foo~",
		},
		{
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cloud-node",
					Labels: map[string]string{
						"foo": "foo~",
					},
					Annotations: map[string]string{
						"foo": "foo~",
					},
				},
			},
			want: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cloud-node",
					Labels: map[string]string{
						"openyurt.io/is-edge-worker": "ok",
					},
					Annotations: map[string]string{
						"node.beta.openyurt.io/autonomy": "ok",
					},
				},
			},
			lval: "yeah",
			aval: "yeah~",
		},
	}

	for _, v := range cases {
		fakeKubeClient := clientsetfake.NewSimpleClientset(v.node)
		res, err := AddEdgeWorkerLabelAndAutonomyAnnotation(fakeKubeClient, v.node, v.lval, v.aval)
		if err != nil || res.Labels[projectinfo.GetEdgeWorkerLabelKey()] != v.lval || res.Annotations[projectinfo.GetAutonomyAnnotation()] != v.aval {
			t.Logf("couldn't add edge worker label and autonomy annotation")
		}
	}
}

func TestRunJobAndCleanup(t *testing.T) {
	var dummy1 int32
	var dummy2 int32
	dummy1 = 1
	dummy2 = 0

	cases := []struct {
		jobObj *batchv1.Job
		want   error
	}{
		{
			jobObj: &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "job",
					Name:      "job-test",
				},
				Spec: batchv1.JobSpec{
					Completions: &dummy1,
				},
				Status: batchv1.JobStatus{
					Succeeded: 1,
				},
			},
			want: nil,
		},

		{
			jobObj: &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "job",
					Name:      "job-foo",
				},
				Spec: batchv1.JobSpec{
					Completions: &dummy2,
				},
				Status: batchv1.JobStatus{
					Succeeded: 0,
				},
			},
			want: nil,
		},
	}

	for _, v := range cases {
		fakeKubeClient := clientsetfake.NewSimpleClientset()
		err := RunJobAndCleanup(fakeKubeClient, v.jobObj, time.Second*10, time.Second)
		if err != v.want {
			t.Logf("couldn't run job and cleanup")
		}
	}
}
