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

package node_servant

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestRenderNodeServantConvertJob(t *testing.T) {
	job, err := RenderNodeServantJob("convert", map[string]string{
		"jobNamespace":     "custom-job-ns",
		"nodeServantImage": "openyurt/node-servant:latest",
		"nodePoolName":     "pool-a",
		"kubeadmConfPath":  "/etc/systemd/system/kubelet.service.d/10-kubeadm.conf",
	}, "node-a")
	if err != nil {
		t.Fatalf("RenderNodeServantJob() returned error: %v", err)
	}

	if job.Name != "node-servant-conversion-node-a" {
		t.Fatalf("unexpected job name %q", job.Name)
	}
	if job.Namespace != "custom-job-ns" {
		t.Fatalf("unexpected job namespace %q", job.Namespace)
	}
	if job.Labels[ConversionNodeLabelKey] != "node-a" {
		t.Fatalf("unexpected job label value %q", job.Labels[ConversionNodeLabelKey])
	}
	if job.Spec.Template.Labels[ConversionNodeLabelKey] != "node-a" {
		t.Fatalf("unexpected pod label value %q", job.Spec.Template.Labels[ConversionNodeLabelKey])
	}
	if job.Spec.BackoffLimit == nil || *job.Spec.BackoffLimit != DefaultConversionJobBackoffLimit {
		t.Fatalf("unexpected backoffLimit %#v", job.Spec.BackoffLimit)
	}
	if job.Spec.TTLSecondsAfterFinished == nil || *job.Spec.TTLSecondsAfterFinished != DefaultConversionJobTTLSecondsAfterFinished {
		t.Fatalf("unexpected ttlSecondsAfterFinished %#v", job.Spec.TTLSecondsAfterFinished)
	}

	podSpec := job.Spec.Template.Spec
	if podSpec.NodeName != "node-a" {
		t.Fatalf("unexpected nodeName %q", podSpec.NodeName)
	}
	if len(podSpec.Volumes) != 1 || podSpec.Volumes[0].Name != "host-root" {
		t.Fatalf("unexpected volumes %#v", podSpec.Volumes)
	}
	if len(podSpec.Tolerations) != 2 {
		t.Fatalf("unexpected tolerations %#v", podSpec.Tolerations)
	}
	if podSpec.RestartPolicy != corev1.RestartPolicyNever {
		t.Fatalf("unexpected restartPolicy %q", podSpec.RestartPolicy)
	}
	if podSpec.Tolerations[0].Operator != corev1.TolerationOpExists || podSpec.Tolerations[0].Effect != corev1.TaintEffectNoSchedule {
		t.Fatalf("unexpected first toleration %#v", podSpec.Tolerations[0])
	}
	if podSpec.Tolerations[1].Operator != corev1.TolerationOpExists || podSpec.Tolerations[1].Effect != corev1.TaintEffectNoExecute {
		t.Fatalf("unexpected second toleration %#v", podSpec.Tolerations[1])
	}

	command := podSpec.Containers[0].Args[0]
	wantSubstrings := []string{
		"/usr/local/bin/entry.sh convert",
		"--node-name=node-a",
		"--nodepool-name=pool-a",
		"--kubeadm-conf-path=/etc/systemd/system/kubelet.service.d/10-kubeadm.conf",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(command, want) {
			t.Fatalf("expected %q in command %q", want, command)
		}
	}
}

func TestRenderNodeServantRevertJob(t *testing.T) {
	job, err := RenderNodeServantJob("revert", map[string]string{
		"nodeServantImage": "openyurt/node-servant:latest",
	}, "node-a")
	if err != nil {
		t.Fatalf("RenderNodeServantJob() returned error: %v", err)
	}

	if job.Name != "node-servant-conversion-node-a" {
		t.Fatalf("unexpected job name %q", job.Name)
	}
	command := job.Spec.Template.Spec.Containers[0].Args[0]
	if command != "/usr/local/bin/entry.sh revert --node-name=node-a" {
		t.Fatalf("unexpected command %q", command)
	}
}

func TestRenderNodeServantJobValidation(t *testing.T) {
	if _, err := RenderNodeServantJob("convert", map[string]string{
		"nodeServantImage": "openyurt/node-servant:latest",
	}, "node-a"); err == nil {
		t.Fatal("expected validation error when nodePoolName is missing")
	}

	if _, err := RenderNodeServantJob("invalid", map[string]string{
		"nodeServantImage": "openyurt/node-servant:latest",
	}, "node-a"); err == nil {
		t.Fatal("expected validation error for invalid action")
	}

	if _, err := RenderNodeServantJob("convert", map[string]string{
		"nodeServantImage": "openyurt/node-servant:latest",
		"nodePoolName":     "",
	}, "node-a"); err == nil {
		t.Fatal("expected validation error when nodePoolName is empty")
	}
}
