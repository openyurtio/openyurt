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
	"sync"

	"k8s.io/apimachinery/pkg/util/sets"
)

const (
	VRIDMAXVALUE = 256
	VRIDEVICTED  = -1
)

type VRRP struct {
	IPs  sets.Set[string]
	VRID int
}

type IPManager struct {
	sync.Mutex
	IPRanges sets.Set[string]
	// ipPools indicates if ip is assigned
	IPPools map[string]int
	// ipVRIDs indicates which IPs are assigned to vrid
	VRRPs map[int]sets.Set[string]
}

func NewVRRP(ips []string, vrid int) VRRP {
	return VRRP{
		IPs:  sets.New(ips...),
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
		VRRPs:    make(map[int]sets.Set[string]),
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
	m.Lock()
	defer m.Unlock()

	for vrid := 0; vrid < VRIDMAXVALUE; vrid++ {
		if ips, ok := m.VRRPs[vrid]; !ok || len(ips) == 0 {
			availableIPs := m.findAvailableIPs()
			if len(availableIPs) == 0 {
				break
			}

			vrrp := NewVRRP(availableIPs, vrid)
			m.buildVRRP(vrrp)
			return vrrp, nil
		}
	}

	return VRRP{}, errors.New("no available IP and VRID combination")
}

func (m *IPManager) findAvailableIPs() []string {
	ips := make([]string, 0, 1)
	for _, ip := range m.IPRanges.UnsortedList() {
		if rid, ok := m.IPPools[ip]; !ok || rid == VRIDEVICTED {
			ips = append(ips, ip)
			break
		}
	}

	return ips
}

func (m *IPManager) buildVRRP(vrrp VRRP) {
	m.VRRPs[vrrp.VRID] = vrrp.IPs

	for ip := range vrrp.IPs {
		m.IPPools[ip] = vrrp.VRID
	}
}

// Assign assign ips to a vrid, if no available ip, get a new ipvrid from Get()
func (m *IPManager) Assign(ips []string) (VRRP, error) {
	m.Lock()

	noConflictIPs := make([]string, 0, len(ips))
	for _, ip := range ips {
		// if conflicted, just use no conflict
		if rid, ok := m.IPPools[ip]; ok && rid != VRIDEVICTED {
			continue
		}
		noConflictIPs = append(noConflictIPs, ip)
	}

	m.Unlock()

	// if no available ip, get a new ipvrid
	if len(noConflictIPs) == 0 {
		return m.Get()
	}

	m.Lock()
	defer m.Unlock()
	var vrrp VRRP
	for vrid := 0; vrid < VRIDMAXVALUE; vrid++ {
		if ipa, ok := m.VRRPs[vrid]; ok && len(ipa) != 0 {
			continue
		}
		// build vrid-ips pair
		vrrp = NewVRRP(noConflictIPs, vrid)
		m.buildVRRP(vrrp)
		break
	}

	// Get fully vrid-ips pair
	return vrrp, nil
}

// Release ips from vrid, if vrid is not assigned, return error
func (m *IPManager) Release(dVrrp VRRP) error {
	m.Lock()
	defer m.Unlock()
	if err := m.IsValid(dVrrp); err != nil {
		return err
	}

	assignedIPs := m.VRRPs[dVrrp.VRID]

	for ip := range dVrrp.IPs {
		if assignedIPs.Has(ip) {
			assignedIPs.Delete(ip)
			m.IPPools[ip] = VRIDEVICTED
		}
	}

	m.VRRPs[dVrrp.VRID] = assignedIPs

	return nil
}

// IsValid check if ip and vrid is valid in this ip-pools, if not return error
func (m *IPManager) IsValid(vrrp VRRP) error {
	if len(vrrp.IPs) == 0 {
		return fmt.Errorf("[IsValid] IPs is empty")
	}

	for ip := range vrrp.IPs {
		if !m.IPRanges.Has(ip) {
			return fmt.Errorf("[IsValid] IP: %s is not found in IP-Ranges", ip)
		}
	}

	if vrrp.VRID < 0 || vrrp.VRID >= VRIDMAXVALUE {
		return fmt.Errorf("[IsValid] VIRD: %d out of range", vrrp.VRID)
	}

	if _, ok := m.VRRPs[vrrp.VRID]; !ok {
		return fmt.Errorf("[IsValid] VRID %d does not assign ips", vrrp.VRID)
	}

	return nil
}

// Sync sync ips from vrid, if vrid is not assigned, return error
func (m *IPManager) Sync(vrrps []VRRP) error {
	m.Lock()
	defer m.Unlock()
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
func (m *IPManager) findDiffIPs(des, cur sets.Set[string]) (app, del []string) {
	for dip := range des {
		if !cur.Has(dip) {
			app = append(app, dip)
		}
	}

	for cip := range cur {
		if !des.Has(cip) {
			del = append(del, cip)
		}
	}

	return
}
