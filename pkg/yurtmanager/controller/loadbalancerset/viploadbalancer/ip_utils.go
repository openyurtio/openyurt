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

	"k8s.io/apimachinery/pkg/util/sets"
)

const (
	VRIDMAXVALUE = 256
	VRIDEVICTED  = -1
)

type VRRP struct {
	IPs  []string
	VRID int
}

type IPManager struct {
	IPRanges sets.Set[string]
	// ipPools indicates if ip is assigned
	IPPools map[string]int
	// ipVRIDs indicates which IPs are assigned to vrid
	VRRPs map[int][]string
}

func NewVRRP(ips []string, vrid int) VRRP {
	return VRRP{
		IPs:  ips,
		VRID: vrid,
	}
}

func NewIPManager(ipr []string) (*IPManager, error) {
	if len(ipr) == 0 {
		return nil, errors.New("ip ranges is empty")
	}

	manager := &IPManager{
		IPRanges: sets.New(ipr...),
		IPPools:  make(map[string]int),
		VRRPs:    make(map[int][]string),
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

// Get return a VRRP with an available IP and VRID combination
func (m *IPManager) Get() (VRRP, error) {
	for vrid := 0; vrid < VRIDMAXVALUE; vrid++ {
		if ips, ok := m.VRRPs[vrid]; !ok || len(ips) == 0 {
			for _, ip := range m.IPRanges.UnsortedList() {
				if _, exist := m.IPPools[ip]; !exist || m.IPPools[ip] == VRIDEVICTED {
					m.IPPools[ip] = vrid
					m.VRRPs[vrid] = []string{ip}
					return VRRP{IPs: []string{ip}, VRID: vrid}, nil
				}
			}
		}
	}

	return VRRP{}, errors.New("no available IP and VRID combination")
}

// Assign assign ips to a vrid, if no available ip, get a new ipvrid from Get()
func (m *IPManager) Assign(ips []string) (VRRP, error) {
	noConflictIPs := make([]string, 0, len(ips))
	for _, ip := range ips {
		// if conflicted, just use no conflict
		if rid, ok := m.IPPools[ip]; ok && rid != VRIDEVICTED {
			continue
		}
		noConflictIPs = append(noConflictIPs, ip)
	}

	// if no available ip, get a new ipvrid
	if len(noConflictIPs) == 0 {
		return m.Get()
	}
	var vrid int
	for ; vrid < VRIDMAXVALUE; vrid++ {
		if ipa, ok := m.VRRPs[vrid]; ok && len(ipa) != 0 {
			continue
		}

		m.VRRPs[vrid] = append(m.VRRPs[vrid], noConflictIPs...)
		for _, ip := range noConflictIPs {
			m.IPPools[ip] = vrid
		}
		break
	}

	// Get fully vrid-ips pair
	return VRRP{VRID: vrid, IPs: m.VRRPs[vrid]}, nil
}

// Release ips from vrid, if vrid is not assigned, return error
func (m *IPManager) Release(dVrrp VRRP) error {
	if err := m.IsValid(dVrrp); err != nil {
		return err
	}

	assignedIPs := m.VRRPs[dVrrp.VRID]
	for _, ip := range dVrrp.IPs {
		if m.isIPPresent(ip, assignedIPs) {
			m.IPPools[ip] = VRIDEVICTED
		}
	}

	return nil
}

// IsValid check if ip and vrid is valid in this ip-pools, if not return error
func (m *IPManager) IsValid(ipvrid VRRP) error {
	if len(ipvrid.IPs) == 0 {
		return fmt.Errorf("[IsValid] IPs is empty")
	}

	for _, ip := range ipvrid.IPs {
		if !m.IPRanges.Has(ip) {
			return fmt.Errorf("[IsValid] IP: %s is not found in IP-Ranges", ip)
		}
	}

	if ipvrid.VRID < 0 || ipvrid.VRID >= VRIDMAXVALUE {
		return fmt.Errorf("[IsValid] VIRD: %d out of range", ipvrid.VRID)
	}

	if _, ok := m.VRRPs[ipvrid.VRID]; !ok {
		return fmt.Errorf("[IsValid] VRID %d does not assign ips", ipvrid.VRID)
	}

	return nil
}

// Sync sync ips from vrid, if vrid is not assigned, return error
func (m *IPManager) Sync(vrrps []VRRP) error {
	for _, vrrp := range vrrps {
		if err := m.IsValid(vrrp); err != nil {
			return err
		}
		ips, vrid := vrrp.IPs, vrrp.VRID
		app, del := m.findDiffIPs(ips, m.VRRPs[vrid])

		for _, ip := range del {
			m.IPPools[ip] = VRIDEVICTED
		}

		// use app to replace del
		m.VRRPs[vrid] = ips
		for _, ip := range app {
			m.IPPools[ip] = vrid
		}

	}

	return nil
}

// findDiffIPs find the difference between des and cur, return the difference between des and cur
func (m *IPManager) findDiffIPs(des, cur []string) (app, del []string) {
	for _, dip := range des {
		if exist := m.isIPPresent(dip, cur); !exist {
			app = append(app, dip)
		}
	}

	for _, cip := range cur {
		if exist := m.isIPPresent(cip, des); !exist {
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
