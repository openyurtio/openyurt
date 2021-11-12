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
	"io/ioutil"

	"github.com/opencontainers/selinux/go-selinux"
	"k8s.io/klog"

	"github.com/openyurtio/openyurt/pkg/yurtctl/constants"
)

const (
	ip_forward = "/proc/sys/net/ipv4/ip_forward"
	bridgenf   = "/proc/sys/net/bridge/bridge-nf-call-iptables"
	bridgenf6  = "/proc/sys/net/bridge/bridge-nf-call-ip6tables"

	kubernetsBridgeSetting = `
net.bridge.bridge-nf-call-ip6tables = 1
net.bridge.bridge-nf-call-iptables = 1`
)

//setIpv4Forward turn on the node ipv4 forward.
func SetIpv4Forward() error {
	klog.Infof("Setting ipv4 forward")
	if err := ioutil.WriteFile(ip_forward, []byte("1"), 0644); err != nil {
		return fmt.Errorf("Write content 1 to file %s fail: %v ", ip_forward, err)
	}
	return nil
}

//setBridgeSetting turn on the node bridge-nf-call-iptables.
func SetBridgeSetting() error {
	klog.Info("Setting bridge settings for kubernetes.")
	if err := ioutil.WriteFile(constants.Sysctl_k8s_config, []byte(kubernetsBridgeSetting), 0644); err != nil {
		return fmt.Errorf("Write file %s fail: %v ", constants.Sysctl_k8s_config, err)
	}
	if err := ioutil.WriteFile(bridgenf, []byte("1"), 0644); err != nil {
		return fmt.Errorf("Write file %s fail: %v ", bridgenf, err)
	}
	if err := ioutil.WriteFile(bridgenf6, []byte("1"), 0644); err != nil {
		return fmt.Errorf("Write file %s fail: %v ", bridgenf, err)
	}
	return nil
}

// setSELinux turn off the node selinux.
func SetSELinux() error {
	klog.Info("Disabling SELinux.")
	selinux.SetDisabled()
	return nil
}
