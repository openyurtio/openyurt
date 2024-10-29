/*
Copyright 2024 The OpenYurt Authors.

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

package locallb

import (
	"net"
	"strconv"

	"github.com/coreos/go-iptables/iptables"
	"k8s.io/klog/v2"
)

const (
	LBCHAIN = "LBCHAIN"
)

type IptablesManager struct {
	ipt iptables.IPTables
}

func NewIptablesManager() *IptablesManager {
	iptable, _ := iptables.New()
	m := &IptablesManager{
		ipt: *iptable,
	}
	return m
}

func (im *IptablesManager) updateIptablesRules(tenantKasService string, apiserverAddrs []string) error {
	if err := im.cleanIptablesRules(); err != nil {
		return err
	}
	if err := im.addIptablesRules(tenantKasService, apiserverAddrs); err != nil {
		return err
	}
	return nil
}

func (im *IptablesManager) addIptablesRules(tenantKasService string, apiserverAddrs []string) error {
	// check if LBCHAIN exists, if don't, create LBCHAIN
	exists, err := im.ipt.ChainExists("nat", LBCHAIN)
	if err != nil {
		klog.Errorf("error checking if chain exists: %v", err)
		return err
	}
	if !exists {
		err := im.ipt.NewChain("nat", LBCHAIN)
		if err != nil {
			klog.Errorf("error creating new chain, %v", err)
			return err
		}
		// append LBCHAIN to OUTPUT in nat
		err = im.ipt.Append("nat", "OUTPUT", "-j", LBCHAIN)
		if err != nil {
			klog.Errorf("could not append LBCHAIN, %v", err)
			return err
		}
	}

	svcIP, svcPort, err := net.SplitHostPort(tenantKasService)
	if err != nil {
		return err
	}
	ramdomBalancingProbability := im.getRamdomBalancingProbability(len(apiserverAddrs))
	for index, addr := range apiserverAddrs {
		// all packets (from kubelet, kubeproxy, pods, etc.) to tenantKasService are loadbalanced to multiple addresses of apiservers deployed in daemonset.
		// the format of addr is ip:port
		err := im.ipt.Append("nat", LBCHAIN, "-d", svcIP, "-p", "tcp", "--dport", svcPort, "-m", "statistic", "--mode", "random", "--probability", strconv.FormatFloat(ramdomBalancingProbability[index], 'f', -1, 64), "-j", "DNAT", "--to-destination", addr)
		if err != nil {
			klog.Errorf("could not append iptable rules, %v", err)
			return err
		}
	}
	return nil
}

func (im *IptablesManager) cleanIptablesRules() error {
	// check if LBCHAIN exists, if exists, clean LBCHAIN
	exists, err := im.ipt.ChainExists("nat", LBCHAIN)
	if err != nil {
		klog.Errorf("error checking if chain exists: %v", err)
		return err
	}
	if exists {
		err := im.ipt.ClearChain("nat", LBCHAIN)
		// clean LBCHAIN rules
		if err != nil {
			klog.Errorf("error cleaning LBCHAIN: %v", err)
			return err
		}
		// remove LBCHAIN from OUTPUT
		err = im.ipt.Delete("nat", "OUTPUT", "-j", LBCHAIN)
		if err != nil {
			klog.Errorf("error removing LBCHAIN from OUTPUT: %v", err)
			return err
		}
		// delete LBCHAIN
		err = im.ipt.DeleteChain("nat", LBCHAIN)
		if err != nil {
			klog.Errorf("error deleting LBCHAIN: %v", err)
			return err
		}
	}
	return nil
}

func (im *IptablesManager) getRamdomBalancingProbability(numOfIPs int) []float64 {
	ramdomBalancingProbability := make([]float64, numOfIPs)
	for i := 0; i < numOfIPs; i++ {
		ramdomBalancingProbability[i] = 1.0 / float64(numOfIPs-i)
	}
	return ramdomBalancingProbability
}
