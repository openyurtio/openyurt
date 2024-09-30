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
	"time"

	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/options"
	"github.com/openyurtio/openyurt/pkg/yurthub/network"
)

const (
	SyncNetworkPeriod = 60
)

type NetworkManager struct {
	ifController    network.DummyInterfaceController
	iptablesManager *network.IptablesManager
	dummyIfIPs      []net.IP
	dummyIfNames    []string
	enableIptables  bool
}

func NewNetworkManager(options *options.YurtHubOptions) (*NetworkManager, error) {
	m := &NetworkManager{
		ifController:    network.NewDummyInterfaceController(),
		iptablesManager: network.NewIptablesManager("255.255.255.255", "255"),
		dummyIfIPs:      []net.IP{net.ParseIP("255.255.255.255")},
		dummyIfNames:    []string{"test255"},
		enableIptables:  options.EnableIptables,
	}
	if err := m.configureNetwork(); err != nil {
		return nil, err
	}

	return m, nil
}

func (m *NetworkManager) Run(stopCh <-chan struct{}) {
	go func() {
		ticker := time.NewTicker(SyncNetworkPeriod * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-stopCh:
				klog.Infof("exit network manager run goroutine normally")
				if m.enableIptables {
					if err := m.iptablesManager.CleanUpIptablesRules(); err != nil {
						klog.Errorf("could not cleanup iptables, %v", err)
					}
				}
				for _, dummyIfName := range m.dummyIfNames {
					err := m.ifController.DeleteDummyInterface(dummyIfName)
					if err != nil {
						klog.Errorf("could not delete dummy interface %s, %v", dummyIfName, err)
					} else {
						klog.Infof("remove dummy interface %s successfully", dummyIfName)
					}
				}
				return
			case <-ticker.C:
				if err := m.configureNetwork(); err != nil {
					// do nothing here
					klog.Warningf("could not configure network, %v", err)
				}
			}
		}
	}()
}

func (m *NetworkManager) configureNetwork() error {
	for i := 0; i < len(m.dummyIfIPs); i++ {
		err := m.ifController.EnsureDummyInterface(m.dummyIfNames[i], m.dummyIfIPs[i])
		if err != nil {
			klog.Errorf("ensure dummy interface failed, %v", err)
			return err
		}
	}

	if m.enableIptables {
		err := m.iptablesManager.EnsureIptablesRules()
		if err != nil {
			klog.Errorf("ensure iptables for dummy interface failed, %v", err)
			return err
		}
	}

	return nil
}
