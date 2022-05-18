//go:build !windows
// +build !windows

/*
Copyright 2020 The Kubernetes Authors.
Copyright 2021 The OpenYurt Authors.

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

package kubelet

import (
	"k8s.io/klog/v2"
	utilsexec "k8s.io/utils/exec"

	"github.com/openyurtio/openyurt/pkg/yurtadm/cmd/join/joindata"
	"github.com/openyurtio/openyurt/pkg/yurtadm/kubernetes/kubeadm/app/constants"
	kubeadmutil "github.com/openyurtio/openyurt/pkg/yurtadm/kubernetes/kubeadm/app/util"
	"github.com/openyurtio/openyurt/pkg/yurtadm/kubernetes/kubeadm/app/util/initsystem"
)

// buildKubeletArgMap takes a kubeletFlagsOpts object and builds based on that a string-string map with flags
// that should be given to the local Linux kubelet daemon.
func buildKubeletArgMap(data joindata.YurtJoinData) map[string]string {
	kubeletFlags := buildKubeletArgMapCommon(data)

	// TODO: Conditionally set `--cgroup-driver` to either `systemd` or `cgroupfs` for CRI other than Docker
	nodeReg := data.NodeRegistration()
	if nodeReg.CRISocket == constants.DefaultDockerCRISocket {
		driver, err := kubeadmutil.GetCgroupDriverDocker(utilsexec.New())
		if err != nil {
			klog.Warningf("cannot automatically assign a '--cgroup-driver' value when starting the Kubelet: %v\n", err)
		} else {
			kubeletFlags["cgroup-driver"] = driver
		}
	}

	initSystem, err := initsystem.GetInitSystem()
	if err != nil {
		klog.Warningf("cannot get init system: %v\n", err)
		return kubeletFlags
	}

	if initSystem.ServiceIsActive("systemd-resolved") {
		kubeletFlags["resolv-conf"] = "/run/systemd/resolve/resolv.conf"
	}

	return kubeletFlags
}
