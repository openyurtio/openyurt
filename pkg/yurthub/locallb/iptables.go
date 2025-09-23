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
	"strings"

	"github.com/coreos/go-iptables/iptables"
	"k8s.io/klog/v2"
)

const (
	LBCHAIN = "LBCHAIN"
)

// IPTables is an interface that abstracts the go-iptables library.
// This allows for mocking in unit tests.
type IPTablesInterface interface {
	ChainExists(table, chain string) (bool, error)
	NewChain(table, chain string) error
	Append(table, chain string, rulespec ...string) error
	ClearChain(table, chain string) error
	Delete(table, chain string, rulespec ...string) error
	DeleteChain(table, chain string) error
}

// IptablesManager manages iptables rules.
type IptablesManager struct {
	ipt IPTablesInterface
}

// NewIptablesManager creates a new IptablesManager with a real iptables client.
func NewIptablesManager() *IptablesManager {
	ipt, _ := iptables.New()
	return newIptablesManagerWithClient(ipt)
}

// newIptablesManagerWithClient is a helper function for creating an IptablesManager,
// primarily used for injecting a mock client in tests.
func newIptablesManagerWithClient(client IPTablesInterface) *IptablesManager {
	return &IptablesManager{
		ipt: client,
	}
}

func (im *IptablesManager) updateIptablesRules(tenantKasService string, apiserverAddrs []string) error {
	klog.Infof("updateIptablesRules: %s", apiserverAddrs)
	if err := im.cleanIptablesRules(); err != nil {
		return err
	}
	if err := im.addIptablesRules(tenantKasService, apiserverAddrs); err != nil {
		return err
	}
	return nil
}

func (im *IptablesManager) addIptablesRules(tenantKasService string, apiserverAddrs []string) error {
	klog.Infof("addIptablesRules: %s", apiserverAddrs)
	// check if LBCHAIN exists, if don't, create LBCHAIN
	exists, err := im.ipt.ChainExists("nat", LBCHAIN)
	if err != nil {
		klog.Errorf("error checking if chain exists: %v", err)
		return err
	}
	if !exists {
		klog.Infof("LBCHAIN doesn't exist, create LBCHAIN")
		err := im.ipt.NewChain("nat", LBCHAIN)
		if err != nil {
			klog.Errorf("error creating new chain, %v", err)
			return err
		}
		klog.Infof("append LBCHAIN to OUTPUT in nat")
		// append LBCHAIN to OUTPUT in nat
		err = im.ipt.Append("nat", "OUTPUT", "-j", LBCHAIN)
		if err != nil {
			klog.Errorf("could not append LBCHAIN, %v", err)
			return err
		}
	}

	svcIP, svcPort, err := net.SplitHostPort(tenantKasService)
	if err != nil {
		klog.Errorf("can't split host and port for tenantKasService, %v", err)
		return err
	}
	ramdomBalancingProbability := im.getRamdomBalancingProbability(len(apiserverAddrs))
	klog.Infof("ramdomBalancingProbability: %v", ramdomBalancingProbability)
	for index, addr := range apiserverAddrs {
		// all packets (from kubelet, etc.) to tenantKasService are loadbalanced to multiple addresses of apiservers deployed in daemonset.
		// the format of addr is ip:port
		args := []string{
			"-d", svcIP,
			"-p", "tcp",
			"--dport", svcPort,
			"-m", "statistic",
			"--mode", "random",
			"--probability", strconv.FormatFloat(ramdomBalancingProbability[index], 'f', -1, 64),
			"-j", "DNAT",
			"--to-destination", addr,
		}
		klog.Infof("Appending iptables rule: iptables -t nat -A %s %s", LBCHAIN, strings.Join(args, " "))
		err := im.ipt.Append("nat", LBCHAIN, args...)
		if err != nil {
			klog.Errorf("could not append iptable rules, %v", err)
			return err
		}
	}
	return nil
}

func (im *IptablesManager) cleanIptablesRules() error {
	klog.Infof("cleanIptablesRules first")
	// check if LBCHAIN exists, if exists, clean LBCHAIN
	exists, err := im.ipt.ChainExists("nat", LBCHAIN)
	if err != nil {
		klog.Errorf("error checking if chain exists: %v", err)
		return err
	}
	if exists {
		klog.Infof("LBCHAIN exists, clean rules in LBCHAIN first")
		err := im.ipt.ClearChain("nat", LBCHAIN)
		// clean LBCHAIN rules
		if err != nil {
			klog.Errorf("error cleaning LBCHAIN: %v", err)
			return err
		}
		// remove LBCHAIN from OUTPUT
		klog.Infof("remove LBCHAIN from OUTPUT")
		err = im.ipt.Delete("nat", "OUTPUT", "-j", LBCHAIN)
		if err != nil {
			klog.Errorf("error removing LBCHAIN from OUTPUT: %v", err)
			return err
		}
		// delete LBCHAIN
		klog.Infof("delete LBCHAIN")
		err = im.ipt.DeleteChain("nat", LBCHAIN)
		if err != nil {
			klog.Errorf("error deleting LBCHAIN: %v", err)
			return err
		}
	}
	return nil
}

func (im *IptablesManager) getRamdomBalancingProbability(numOfIPs int) []float64 {
	klog.Infof("numOfIPs: %v", numOfIPs)
	ramdomBalancingProbability := make([]float64, numOfIPs)
	for i := 0; i < numOfIPs; i++ {
		ramdomBalancingProbability[i] = 1.0 / float64(numOfIPs-i)
	}
	return ramdomBalancingProbability
}
