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

package dns

import (
	"fmt"
	"net"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
)

func isEdgeNode(node *corev1.Node) bool {
	isEdgeNode, ok := node.Labels[projectinfo.GetEdgeWorkerLabelKey()]
	if ok && isEdgeNode == "true" {
		return true
	}
	return false
}

func formatDNSRecord(ip, host string) string {
	return fmt.Sprintf("%s\t%s", ip, host)
}

// getNodeHostIP returns the provided node's "primary" IP
func getNodeHostIP(node *corev1.Node) (string, error) {
	// re-sort the addresses with InternalIPs first and then ExternalIPs
	allIPs := make([]net.IP, 0, len(node.Status.Addresses))
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeInternalIP {
			ip := net.ParseIP(addr.Address)
			if ip != nil {
				allIPs = append(allIPs, ip)
				break
			}
		}
	}
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeExternalIP {
			ip := net.ParseIP(addr.Address)
			if ip != nil {
				allIPs = append(allIPs, ip)
				break
			}
		}
	}
	if len(allIPs) == 0 {
		return "", fmt.Errorf("host IP unknown; known addresses: %v", node.Status.Addresses)
	}

	return allIPs[0].String(), nil
}

func removeRecordByHostname(records []string, hostname string) (result []string, changed bool) {
	result = make([]string, 0, len(records))
	for _, v := range records {
		if !strings.HasSuffix(v, hostname) {
			result = append(result, v)
		}
	}
	return result, len(records) != len(result)
}

func parseHostnameFromDNSRecord(record string) (string, error) {
	arr := strings.Split(record, "\t")
	if len(arr) != 2 {
		return "", fmt.Errorf("could not parse hostname, invalid dns record %q", record)
	}
	return arr[1], nil
}

func addOrUpdateRecord(records []string, record string) (result []string, changed bool, err error) {
	hostname, err := parseHostnameFromDNSRecord(record)
	if err != nil {
		return nil, false, err
	}

	result = make([]string, len(records))
	copy(result, records)

	found := false
	for i, v := range result {
		if strings.HasSuffix(v, hostname) {
			found = true
			if v != record {
				result[i] = record
				changed = true
				break
			}
		}
	}

	if !found {
		result = append(result, record)
		changed = true
	}

	return result, changed, nil
}
