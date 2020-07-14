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

package iptables

import (
	"k8s.io/apimachinery/pkg/labels"
	clientset "k8s.io/client-go/kubernetes"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/klog"
	"k8s.io/kubernetes/pkg/util/iptables"
	"k8s.io/utils/exec"
)

// IptableManager interface defines the method for adding dnat rules to host
// that needs to send network packages to kubelets
type IptablesManager interface {
	Run()
}

// iptablesManager implements the IptablesManager
type iptablesManager struct {
	kubeClient     clientset.Interface
	iptables       iptables.Interface
	execer         exec.Interface
	conntrackPath  string
	nodeLister     corelisters.NodeLister
	secureDnatDest string
	selector       labels.Selector
	lastNodesIP    []string
	lastDnatPorts  []string
	syncPeriod     int
	stopCh         <-chan struct{}
}

// NewIptablesManager returns an IptablesManager
func NewIptablesManager(client clientset.Interface,
	nodeIP string,
	syncPeriod int,
	stopCh <-chan struct{}) IptablesManager {
	klog.Error("NOT IMPLEMENT YET")
	return nil
}

// Run starts the iptablesManager that will updates dnat rules periodically
func (im *iptablesManager) Run() {
	klog.Error("NOT IMPLEMENT YET")
	return
}
