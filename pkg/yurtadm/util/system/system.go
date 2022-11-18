/*
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

package system

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/opencontainers/selinux/go-selinux"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurtadm/constants"
	"github.com/openyurtio/openyurt/pkg/yurtadm/util/edgenode"
)

const (
	ip_forward = "/proc/sys/net/ipv4/ip_forward"
	bridgenf   = "/proc/sys/net/bridge/bridge-nf-call-iptables"
	bridgenf6  = "/proc/sys/net/bridge/bridge-nf-call-ip6tables"

	kubernetsBridgeSetting = `
net.bridge.bridge-nf-call-ip6tables = 1
net.bridge.bridge-nf-call-iptables = 1`
)

// SetIpv4Forward turn on the node ipv4 forward.
func SetIpv4Forward() error {
	klog.Infof("Setting ipv4 forward")
	if err := os.WriteFile(ip_forward, []byte("1"), 0644); err != nil {
		return fmt.Errorf("Write content 1 to file %s fail: %w ", ip_forward, err)
	}
	return nil
}

// SetBridgeSetting turn on the node bridge-nf-call-iptables.
func SetBridgeSetting() error {
	klog.Info("Setting bridge settings for kubernetes.")
	if err := os.WriteFile(constants.SysctlK8sConfig, []byte(kubernetsBridgeSetting), 0644); err != nil {
		return fmt.Errorf("Write file %s fail: %w ", constants.SysctlK8sConfig, err)
	}

	if exist, _ := edgenode.FileExists(bridgenf); !exist {
		cmd := exec.Command("bash", "-c", "modprobe br-netfilter")
		if err := edgenode.Exec(cmd); err != nil {
			return err
		}
	}
	if err := os.WriteFile(bridgenf, []byte("1"), 0644); err != nil {
		return fmt.Errorf("Write file %s fail: %w ", bridgenf, err)
	}
	if err := os.WriteFile(bridgenf6, []byte("1"), 0644); err != nil {
		return fmt.Errorf("Write file %s fail: %w ", bridgenf, err)
	}
	return nil
}

// SetSELinux turn off the node selinux.
func SetSELinux() error {
	klog.Info("Disabling SELinux.")
	selinux.SetDisabled()
	return nil
}
