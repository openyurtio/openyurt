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

package network

import (
	"strings"

	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog/v2"
	"k8s.io/utils/exec"
	utilnet "k8s.io/utils/net"

	"github.com/openyurtio/openyurt/pkg/util/iptables"
)

type iptablesRule struct {
	pos   iptables.RulePosition
	table iptables.Table
	chain iptables.Chain
	args  []string
}

type IptablesManager struct {
	iptables iptables.Interface
	rules    []iptablesRule
}

func NewIptablesManager(dummyIfIP, dummyIfPort string) *IptablesManager {
	protocol := iptables.ProtocolIpv4
	if utilnet.IsIPv6String(dummyIfIP) {
		protocol = iptables.ProtocolIpv6
	}
	execer := exec.New()
	iptInterface := iptables.New(execer, protocol)

	im := &IptablesManager{
		iptables: iptInterface,
		rules:    makeupIptablesRules(dummyIfIP, dummyIfPort),
	}

	return im
}

func makeupIptablesRules(ifIP, ifPort string) []iptablesRule {
	return []iptablesRule{
		// accept traffic to 169.254.2.1:10261/169.254.2.1:10268
		{iptables.Prepend, iptables.TableFilter, iptables.ChainInput, []string{"-p", "tcp", "-m", "comment", "--comment", "for container access hub agent", "--dport", ifPort, "--destination", ifIP, "-j", "ACCEPT"}},
		// accept traffic from 169.254.2.1:10261/169.254.2.1:10268
		{iptables.Prepend, iptables.TableFilter, iptables.ChainOutput, []string{"-p", "tcp", "--sport", ifPort, "-s", ifIP, "-j", "ACCEPT"}},
	}
}

func (im *IptablesManager) EnsureIptablesRules() error {
	var errs []error
	for _, rule := range im.rules {
		_, err := im.iptables.EnsureRule(rule.pos, rule.table, rule.chain, rule.args...)
		if err != nil {
			errs = append(errs, err)
			klog.Errorf("could not ensure iptables rule(%s -t %s %s %s), %v", rule.pos, rule.table, rule.chain, strings.Join(rule.args, ","), err)
			continue
		}
	}
	return utilerrors.NewAggregate(errs)
}

func (im *IptablesManager) CleanUpIptablesRules() error {
	var errs []error
	for _, rule := range im.rules {
		err := im.iptables.DeleteRule(rule.table, rule.chain, rule.args...)
		if err != nil {
			errs = append(errs, err)
			klog.Errorf("failed to delete iptables rule(%s -t %s %s %s), %v", rule.pos, rule.table, rule.chain, strings.Join(rule.args, " "), err)
		}
	}
	return utilerrors.NewAggregate(errs)
}
