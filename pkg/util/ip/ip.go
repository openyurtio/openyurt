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

package ip

import (
	"net"
	"strings"

	"k8s.io/klog/v2"
	utilnet "k8s.io/utils/net"
)

const (
	DefaultLoopbackIP4 = "127.0.0.1"
	DefaultLoopbackIP6 = "::1"
)

// MustGetLoopbackIP is a wrapper for GetLoopbackIP. If any error occurs or loopback interface is not found,
// will fall back to 127.0.0.1 for ipv4 or ::1 for ipv6.
func MustGetLoopbackIP(wantIPv6 bool) string {
	ip, err := GetLoopbackIP(wantIPv6)
	if err != nil {
		klog.Errorf("failed to get loopback addr: %v", err)
	}
	if ip != "" {
		return ip
	}
	if wantIPv6 {
		return DefaultLoopbackIP6
	}
	return DefaultLoopbackIP4
}

// GetLoopbackIP returns the ip address of local loopback interface.
func GetLoopbackIP(wantIPv6 bool) (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}
	for _, address := range addrs {
		if ipnet, ok := address.(*net.IPNet); ok && ipnet.IP.IsLoopback() && wantIPv6 == utilnet.IsIPv6(ipnet.IP) {
			return ipnet.IP.String(), nil
		}
	}
	return "", nil
}

func JoinIPStrings(ips []net.IP) string {
	var strs []string
	for _, ip := range ips {
		strs = append(strs, ip.String())
	}
	return strings.Join(strs, ",")
}

// RemoveDupIPs removes duplicate ips from the ip list and returns a new created list
func RemoveDupIPs(ips []net.IP) []net.IP {
	results := make([]net.IP, 0, len(ips))
	temp := map[string]bool{}
	for _, ip := range ips {
		if _, ok := temp[string(ip)]; ip != nil && !ok {
			temp[string(ip)] = true
			results = append(results, ip)
		}
	}
	return results
}

// Parse a list of IP Strings to net.IP type
func ParseIPList(ips []string) []net.IP {
	results := make([]net.IP, len(ips))
	for idx, ip := range ips {
		results[idx] = net.ParseIP(ip)
	}
	return results
}

// searchIP returns true if ip is in ipList
func SearchIP(ipList []net.IP, ip net.IP) bool {
	for _, ipItem := range ipList {
		if ipItem.Equal(ip) {
			return true
		}
	}
	return false
}

// searchAllIP returns true if all ips are in ipList
func SearchAllIP(ipList []net.IP, ips []net.IP) bool {
	for _, ip := range ips {
		if !SearchIP(ipList, ip) {
			return false
		}
	}
	return true
}
