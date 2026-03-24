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
	"fmt"
	"strings"

	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog/v2"
	utilsexec "k8s.io/utils/exec"
)

type containerStopper interface {
	ListKubeContainers() ([]KubeContainer, error)
	StopContainer(containerID string) error
}

const pauseContainerName = "POD"

var (
	detectCRISocketFunc = DetectCRISocket
	newRuntimeExecer    = utilsexec.New
	newContainerStopper = func(criSocket string) (containerStopper, error) {
		return NewContainerRuntimeForImage(newRuntimeExecer(), criSocket)
	}
)

func RestartNonPauseContainers(nodeName, excludedPodPrefix string) error {
	if strings.TrimSpace(nodeName) == "" {
		return fmt.Errorf("node name is empty")
	}

	criSocket, err := detectCRISocketFunc()
	if err != nil {
		return err
	}
	klog.Infof("restartContainers: detected runtime endpoint %s on node %s", criSocket, nodeName)

	runtime, err := newContainerStopper(criSocket)
	if err != nil {
		return err
	}

	containers, err := runtime.ListKubeContainers()
	if err != nil {
		return err
	}
	klog.Infof("restartContainers: discovered %d running containers on node %s", len(containers), nodeName)

	var errs []error
	var stopped, skippedEmptyID, skippedNonKube, skippedPause, skippedExcluded int
	for _, container := range containers {
		if container.ID == "" {
			skippedEmptyID++
			continue
		}
		if container.Namespace == "" || container.PodName == "" || container.ContainerName == "" {
			skippedNonKube++
			continue
		}
		if container.ContainerName == pauseContainerName {
			skippedPause++
			continue
		}
		if excludedPodPrefix != "" && strings.HasPrefix(container.PodName, excludedPodPrefix) {
			skippedExcluded++
			continue
		}

		klog.Infof("stop container %s in pod %s on node %s and let kubelet recreate it", container.ContainerName, container.PodName, nodeName)
		if err := runtime.StopContainer(container.ID); err != nil {
			errs = append(errs, fmt.Errorf("stop container %s in pod %s: %w", container.ID, container.PodName, err))
			continue
		}
		stopped++
	}

	klog.Infof("restartContainers: finished on node %s, stopped=%d skippedEmptyID=%d skippedNonKube=%d skippedPause=%d skippedExcluded=%d errors=%d",
		nodeName, stopped, skippedEmptyID, skippedNonKube, skippedPause, skippedExcluded, len(errs))
	return utilerrors.NewAggregate(errs)
}
