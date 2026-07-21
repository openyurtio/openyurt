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

package components

import (
	"errors"
	"reflect"
	"testing"

	utilerrors "k8s.io/apimachinery/pkg/util/errors"
)

type fakeContainerStopper struct {
	containers []KubeContainer
	stopCalls  []string
	stopErrs   map[string]error
}

func (f *fakeContainerStopper) ListKubeContainers() ([]KubeContainer, error) {
	return f.containers, nil
}

func (f *fakeContainerStopper) StopContainer(containerID string) error {
	f.stopCalls = append(f.stopCalls, containerID)
	return f.stopErrs[containerID]
}

func TestRestartNonPauseContainersStopsAllListedContainersExceptExcludedPod(t *testing.T) {
	oldDetectCRISocketFunc := detectCRISocketFunc
	oldNewContainerStopper := newContainerStopper
	defer func() {
		detectCRISocketFunc = oldDetectCRISocketFunc
		newContainerStopper = oldNewContainerStopper
	}()

	fakeRuntime := &fakeContainerStopper{
		containers: []KubeContainer{
			{ID: "main-running", Namespace: "ns-a", PodName: "workload-a", ContainerName: "main"},
			{ID: "sidecar-running", Namespace: "ns-a", PodName: "workload-a", ContainerName: "sidecar"},
			{ID: "native-no-namespace", PodName: "standalone", ContainerName: "native"},
			{ID: "native-no-pod", Namespace: "ns-a", ContainerName: "native"},
			{ID: "native-no-container", Namespace: "ns-a", PodName: "workload-b"},
			{ID: "pause-running", Namespace: "ns-a", PodName: "workload-a", ContainerName: pauseContainerName},
			{ID: "", Namespace: "ns-a", PodName: "workload-b", ContainerName: "empty-id"},
			{ID: "job-self", Namespace: "kube-system", PodName: "node-servant-conversion-node-a-abcde", ContainerName: "node-servant"},
			{ID: "static-running", Namespace: "kube-system", PodName: "static-pod", ContainerName: "static"},
		},
	}

	detectCRISocketFunc = func() (string, error) {
		return "/run/containerd/containerd.sock", nil
	}
	newContainerStopper = func(criSocket string) (containerStopper, error) {
		if criSocket != "/run/containerd/containerd.sock" {
			t.Fatalf("unexpected cri socket %q", criSocket)
		}
		return fakeRuntime, nil
	}

	if err := RestartNonPauseContainers("node-a", "node-servant-conversion-node-a"); err != nil {
		t.Fatalf("RestartNonPauseContainers() returned error: %v", err)
	}

	if !reflect.DeepEqual(fakeRuntime.stopCalls, []string{"main-running", "sidecar-running", "static-running"}) {
		t.Fatalf("unexpected stop calls: %v", fakeRuntime.stopCalls)
	}
}

func TestRestartNonPauseContainersAggregatesErrors(t *testing.T) {
	oldDetectCRISocketFunc := detectCRISocketFunc
	oldNewContainerStopper := newContainerStopper
	defer func() {
		detectCRISocketFunc = oldDetectCRISocketFunc
		newContainerStopper = oldNewContainerStopper
	}()

	fakeRuntime := &fakeContainerStopper{
		containers: []KubeContainer{
			{ID: "workload-1", Namespace: "ns-a", PodName: "workload-a", ContainerName: "main"},
			{ID: "workload-2", Namespace: "ns-a", PodName: "workload-b", ContainerName: "main"},
		},
		stopErrs: map[string]error{
			"workload-1": errors.New("stop failed"),
			"workload-2": errors.New("stop failed too"),
		},
	}

	detectCRISocketFunc = func() (string, error) {
		return "/run/containerd/containerd.sock", nil
	}
	newContainerStopper = func(criSocket string) (containerStopper, error) {
		return fakeRuntime, nil
	}

	err := RestartNonPauseContainers("node-a", "node-servant-conversion-node-a")
	if err == nil {
		t.Fatal("expected aggregated error")
	}
	agg, ok := err.(utilerrors.Aggregate)
	if !ok {
		t.Fatalf("expected aggregate error, got %T", err)
	}
	if len(agg.Errors()) != 2 {
		t.Fatalf("expected 2 aggregated errors, got %d", len(agg.Errors()))
	}
	if !reflect.DeepEqual(fakeRuntime.stopCalls, []string{"workload-1", "workload-2"}) {
		t.Fatalf("unexpected stop calls: %v", fakeRuntime.stopCalls)
	}
}
