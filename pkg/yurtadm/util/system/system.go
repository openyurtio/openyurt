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
	ipForward = "/proc/sys/net/ipv4/ip_forward"
	bridgeNF  = "/proc/sys/net/bridge/bridge-nf-call-iptables"
	bridgeNF6 = "/proc/sys/net/bridge/bridge-nf-call-ip6tables"

	kubernetesBridgeSetting = `
net.bridge.bridge-nf-call-ip6tables = 1
net.bridge.bridge-nf-call-iptables = 1`
)

// SetIpv4Forward turn on the node ipv4 forward.
func SetIpv4Forward() error {
	klog.Infof("Setting ipv4 forward")
	if err := os.WriteFile(ipForward, []byte("1"), 0644); err != nil {
		return fmt.Errorf("write content 1 to file %s failed: %w", ipForward, err)
	}
	return nil
}

// SetBridgeSetting turn on the node bridge-nf-call-iptables.
func SetBridgeSetting() error {
	klog.Info("Setting bridge settings for kubernetes.")
	if err := os.WriteFile(constants.SysctlK8sConfig, []byte(kubernetesBridgeSetting), 0644); err != nil {
		return fmt.Errorf("write file %s failed: %w", constants.SysctlK8sConfig, err)
	}

	if exist, _ := edgenode.FileExists(bridgeNF); !exist {
		cmd := exec.Command("bash", "-c", "modprobe br-netfilter")
		if err := edgenode.Exec(cmd); err != nil {
			return err
		}
	}
	if err := os.WriteFile(bridgeNF, []byte("1"), 0644); err != nil {
		return fmt.Errorf("write file %s failed: %w", bridgeNF, err)
	}
	if err := os.WriteFile(bridgeNF6, []byte("1"), 0644); err != nil {
		return fmt.Errorf("write file %s failed: %w", bridgeNF6, err)
	}
	return nil
}

// SetSELinux turn off the node selinux.
func SetSELinux() error {
	klog.Info("Disabling SELinux.")
	selinux.SetDisabled()
	return nil
}
