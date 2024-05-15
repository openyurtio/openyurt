/*
Copyright 2024 The OpenYurt Authors.

Licensed under the Apache License, Version 2.0 (the License);
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an AS IS BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package viploadbalancer

import (
	"errors"
	"fmt"
	"net"
	"strings"
)

const (
	VRIDMAXVALUE = 256
	VRIDEVICTED  = -1
)

type IPRange struct {
	// IPRange: [startIP, endIP)
	startIP net.IP
	endIP   net.IP
}

type IPVRID struct {
	IPs  []string
	VRID int
}

type IPManager struct {
	ranges []IPRange
	// ipPools indicates if ip is assign
	ipPools map[string]int
	// ipVRIDs indicates which IPs are assigned to vrid
	ipVRIDs map[int][]string
}

func NewIPVRID(ips []string, vrid int) IPVRID {
	return IPVRID{
		IPs:  ips,
		VRID: vrid,
	}
}

func NewIPManager(ipRanges string) (*IPManager, error) {
	manager := &IPManager{
		ipPools: map[string]int{},
		ipVRIDs: make(map[int][]string),
	}

	iprs := parseIP(ipRanges)
	for _, ipr := range iprs {
		manager.ipPools[ipr] = VRIDEVICTED
	}

	return manager, nil
}

func parseIP(ipr string) []string {
	var ips []string
	ipRanges := strings.Split(ipr, ",")

	for _, ipRange := range ipRanges {
		ipRange = strings.TrimSpace(ipRange)
		if strings.Contains(ipRange, "-") {
			ipr := strings.Split(ipRange, "-")
			ips = append(ips, buildIPRange(net.ParseIP(ipr[0]), net.ParseIP(ipr[1]))...)
		} else {
			ips = append(ips, strings.TrimSpace(ipRange))
		}
	}

	return ips
}

func buildIPRange(startIP, endIP net.IP) []string {
	var ips []string
	for ip := startIP; !ip.Equal(endIP); incrementIP(ip) {
		ips = append(ips, ip.String())
	}
	return ips
}

func incrementIP(ip net.IP) net.IP {
	for i := len(ip) - 1; i >= 0; i-- {
		ip[i]++
		if ip[i] > 0 {
			break
		}
	}

	return ip
}

func (m *IPManager) Get() (IPVRID, error) {

	for vrid := 0; vrid < VRIDMAXVALUE; vrid++ {
		if ips, ok := m.ipVRIDs[vrid]; !ok || len(ips) == 0 {
			for ip, used := range m.ipPools {
				if used == VRIDEVICTED {
					m.ipPools[ip] = vrid
					m.ipVRIDs[vrid] = []string{ip}
					return IPVRID{IPs: []string{ip}, VRID: vrid}, nil
				}
			}
		}
	}

	return IPVRID{}, errors.New("no available IP and VRID combination")
}

func (m *IPManager) Assign(ips []string) (IPVRID, error) {

	var noConflictIPs []string
	for _, ip := range ips {
		// if conflict, just use no conflict
		if m.ipPools[ip] != VRIDEVICTED {
			continue
		}
		noConflictIPs = append(noConflictIPs, ip)
	}

	// if no avalible ip, get a new ipvrid
	if len(noConflictIPs) == 0 {
		return m.Get()
	}

	for vrid := 0; vrid < VRIDMAXVALUE; vrid++ {
		if _, ok := m.ipVRIDs[vrid]; !ok {
			m.ipVRIDs[vrid] = noConflictIPs

			for _, ip := range noConflictIPs {
				m.ipPools[ip] = vrid
			}

			return IPVRID{IPs: noConflictIPs, VRID: vrid}, nil
		}
	}

	return IPVRID{}, errors.New("no available IPs and VRID combination")
}

func (m *IPManager) Release(ipVRID IPVRID) error {

	if err := m.IsValid(ipVRID); err != nil {
		return err
	}
	ips := ipVRID.IPs
	vrid := ipVRID.VRID

	if _, ok := m.ipVRIDs[vrid]; !ok {
		return fmt.Errorf("VRID %d does not assign ips", vrid)
	}
	remain := make([]string, len(m.ipVRIDs[vrid])-len(ips))

	for _, ip := range m.ipVRIDs[vrid] {
		if m.isIPPresent(ip, ips) {
			continue
		}

		remain = append(remain, ip)
	}

	if len(remain) == len(m.ipVRIDs[vrid]) {
		return fmt.Errorf("IP %v is not assigned", ips)
	}

	for _, ip := range remain {
		m.ipPools[ip] = VRIDEVICTED
	}

	return nil
}

func (m *IPManager) IsValid(ipvrid IPVRID) error {
	ips := ipvrid.IPs
	vrid := ipvrid.VRID
	if len(ips) == 0 {
		return fmt.Errorf("IPs is empty")
	}

	for _, ip := range ips {
		if _, ok := m.ipPools[ip]; !ok {
			return fmt.Errorf("IP: %s is not found in IP-Pools", ip)
		}
	}

	if vrid < 0 || vrid >= VRIDMAXVALUE {
		return fmt.Errorf("VIRD: %d out of range", vrid)
	}
	return nil
}

func (m *IPManager) Sync(ipVRIDs []IPVRID) error {

	for _, ipVRID := range ipVRIDs {
		if err := m.IsValid(ipVRID); err != nil {
			return err
		}
		ips := ipVRID.IPs
		vrid := ipVRID.VRID

		app, del := m.findDiffIPs(ips, m.ipVRIDs[vrid])

		for _, ip := range del {
			m.ipPools[ip] = VRIDEVICTED
		}

		m.ipVRIDs[vrid] = ips
		for _, ip := range app {
			m.ipPools[ip] = vrid
		}

	}

	return nil
}

func (m *IPManager) findDiffIPs(des, cur []string) (app, del []string) {
	for _, dip := range des {
		if exsit := m.isIPPresent(dip, cur); !exsit {
			app = append(app, dip)
		}
	}

	for _, cip := range cur {
		if exsit := m.isIPPresent(cip, des); !exsit {
			del = append(del, cip)
		}
	}

	return
}

func (m *IPManager) isIPPresent(tip string, ips []string) bool {
	for _, ip := range ips {
		if ip == tip {
			return true
		}
	}
	return false
}
