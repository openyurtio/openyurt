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

type IPVRID struct {
	IPs  []string
	VRID int
}

type IPManager struct {
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

	iprs := ParseIP(ipRanges)
	for _, ipr := range iprs {
		manager.ipPools[ipr] = VRIDEVICTED
	}

	return manager, nil
}

// ParseIP support ipv4 / ipv6 parse, like "192.168.0.1-192.168.0.3", "192.168.0.1, 10.0.1.1", "2001:db8::2-2001:db8::4"
// Output is a list of ip strings, which is left closed and right open, like [192.168.0.1, 192.168.0.2], [192.168.0.1, 10.0.1.1],[2001:db8::2, 2001:db8::3]
func ParseIP(ipr string) []string {
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

// Get return a IPVRID with a available IP and VRID combination
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

// Assign assign ips to a vrid, if no available ip, get a new ipvrid from Get()
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
	var vrid int
	for ; vrid < VRIDMAXVALUE; vrid++ {
		if _, ok := m.ipVRIDs[vrid]; !ok {
			m.ipVRIDs[vrid] = append(m.ipVRIDs[vrid], noConflictIPs...)

			for _, ip := range noConflictIPs {
				m.ipPools[ip] = vrid
			}
			break
		}
	}

	// Get fully vrid-ips pair
	return IPVRID{VRID: vrid, IPs: m.ipVRIDs[vrid]}, nil
}

// Release release ips from vrid, if vrid is not assigned, return error
func (m *IPManager) Release(ipVRID IPVRID) error {
	if err := m.IsValid(ipVRID); err != nil {
		return err
	}

	if _, ok := m.ipVRIDs[ipVRID.VRID]; !ok {
		return fmt.Errorf("VRID %d does not assign ips", ipVRID.VRID)
	}
	remain := make([]string, len(m.ipVRIDs[ipVRID.VRID])-len(ipVRID.IPs))

	for _, ip := range m.ipVRIDs[ipVRID.VRID] {
		if m.isIPPresent(ip, ipVRID.IPs) {
			continue
		}

		remain = append(remain, ip)
	}

	if len(remain) == len(m.ipVRIDs[ipVRID.VRID]) {
		return fmt.Errorf("IP %v is not assigned", ipVRID.IPs)
	}

	for _, ip := range remain {
		m.ipPools[ip] = VRIDEVICTED
	}

	return nil
}

// check if ip and vrid is valid in this ip-pools, if not return error
func (m *IPManager) IsValid(ipvrid IPVRID) error {
	if len(ipvrid.IPs) == 0 {
		return fmt.Errorf("IPs is empty")
	}

	for _, ip := range ipvrid.IPs {
		if _, ok := m.ipPools[ip]; !ok {
			return fmt.Errorf("IP: %s is not found in IP-Pools", ip)
		}
	}

	if ipvrid.VRID < 0 || ipvrid.VRID >= VRIDMAXVALUE {
		return fmt.Errorf("VIRD: %d out of range", ipvrid.VRID)
	}
	return nil
}

// Sync sync ips from vrid, if vrid is not assigned, return error
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

// findDiffIPs find the difference between des and cur, return the difference between des and cur
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

// isIPPresent check if ip is present in ips, if present return true, else return false
func (m *IPManager) isIPPresent(tip string, ips []string) bool {
	for _, ip := range ips {
		if ip == tip {
			return true
		}
	}
	return false
}
