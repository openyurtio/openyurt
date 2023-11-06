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
	"fmt"
	"net"
	"strings"

	"github.com/vishvananda/netlink"
	"k8s.io/klog/v2"
)

type DummyInterfaceController interface {
	EnsureDummyInterface(ifName string, ifIP net.IP) error
	DeleteDummyInterface(ifName string) error
	ListDummyInterface(ifName string) ([]net.IP, error)
}

type dummyInterfaceController struct {
	netlink.Handle
}

// NewDummyInterfaceController returns an instance for create/delete dummy net interface
func NewDummyInterfaceController() DummyInterfaceController {
	return &dummyInterfaceController{
		Handle: netlink.Handle{},
	}
}

// EnsureDummyInterface make sure the dummy net interface with specified name and ip exist
func (dic *dummyInterfaceController) EnsureDummyInterface(ifName string, ifIP net.IP) error {
	link, err := dic.LinkByName(ifName)
	if err == nil {
		addrs, err := dic.AddrList(link, 0)
		if err != nil {
			return err
		}

		for _, addr := range addrs {
			if addr.IP != nil && addr.IP.Equal(ifIP) {
				return nil
			}
		}

		klog.Infof("ip address for %s interface changed to %s", ifName, ifIP.String())
		return dic.AddrReplace(link, &netlink.Addr{IPNet: netlink.NewIPNet(ifIP)})
	}

	if strings.Contains(err.Error(), "Link not found") && link == nil {
		return dic.addDummyInterface(ifName, ifIP)
	}

	return err
}

// addDummyInterface creates a dummy net interface with the specified name and ip
func (dic *dummyInterfaceController) addDummyInterface(ifName string, ifIP net.IP) error {
	_, err := dic.LinkByName(ifName)
	if err == nil {
		return fmt.Errorf("Link %s exists", ifName)
	}

	dummy := &netlink.Dummy{
		LinkAttrs: netlink.LinkAttrs{Name: ifName},
	}
	err = dic.LinkAdd(dummy)
	if err != nil {
		return err
	}

	link, err := dic.LinkByName(ifName)
	if err != nil {
		return err
	}
	return dic.AddrAdd(link, &netlink.Addr{IPNet: netlink.NewIPNet(ifIP)})
}

// DeleteDummyInterface delete the dummy net interface with specified name
func (dic *dummyInterfaceController) DeleteDummyInterface(ifName string) error {
	link, err := dic.LinkByName(ifName)
	if err != nil {
		return err
	}
	return dic.LinkDel(link)
}

// ListDummyInterface list all ips for network interface specified by ifName
func (dic *dummyInterfaceController) ListDummyInterface(ifName string) ([]net.IP, error) {
	ips := make([]net.IP, 0)
	link, err := dic.LinkByName(ifName)
	if err != nil {
		return ips, err
	}

	addrs, err := dic.AddrList(link, 0)
	if err != nil {
		return ips, err
	}

	for _, addr := range addrs {
		if addr.IP != nil {
			ips = append(ips, addr.IP)
		}
	}

	return ips, nil
}
