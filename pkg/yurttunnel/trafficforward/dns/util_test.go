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
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestIsEdgeNode(t *testing.T) {
	tests := []struct {
		desc   string
		node   *corev1.Node
		expect bool
	}{
		{
			desc: "node has edge worker label which equals true",
			node: &corev1.Node{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"openyurt.io/is-edge-worker": "true",
					},
				},
				Spec:   corev1.NodeSpec{},
				Status: corev1.NodeStatus{},
			},
			expect: true,
		},
		{
			desc: "node has edge worker label which equals false",
			node: &corev1.Node{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"openyurt.io/is-edge-worker": "false",
					},
				},
				Spec:   corev1.NodeSpec{},
				Status: corev1.NodeStatus{},
			},
			expect: false,
		},
		{
			desc: "node dose not has edge worker label",
			node: &corev1.Node{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"yin": "ruixing",
					},
				},
				Spec:   corev1.NodeSpec{},
				Status: corev1.NodeStatus{},
			},
			expect: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			act := isEdgeNode(tt.node)
			if act != tt.expect {
				t.Errorf("the result we want is: %v, but the actual result is: %v\n", tt.expect, act)
			}
		})
	}
}

func TestFormatDnsRecord(t *testing.T) {
	var (
		ip     = "10.10.102.60"
		host   = "k8s-xing-master"
		expect = ip + "\t" + host
	)

	act := formatDNSRecord(ip, host)
	if act != expect {
		t.Errorf("the result we want is: %v, but the actual result is: %v\n", expect, act)
	}

}

func TestGetNodeHostIP(t *testing.T) {
	tests := []struct {
		desc   string
		node   *corev1.Node
		expect string
	}{
		{
			desc: "get node primary host ip",
			node: &corev1.Node{
				TypeMeta:   metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{},
				Spec:       corev1.NodeSpec{},
				Status: corev1.NodeStatus{
					Addresses: []corev1.NodeAddress{
						{
							Type:    corev1.NodeExternalIP,
							Address: "205.20.20.2",
						},
						{
							Type:    corev1.NodeInternalIP,
							Address: "102.10.10.60",
						},
						{
							Type:    corev1.NodeHostName,
							Address: "k8s-edge-node",
						},
					},
				},
			},
			expect: "102.10.10.60",
		},
		{
			desc: "get node primary host ip",
			node: &corev1.Node{
				TypeMeta:   metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{},
				Spec:       corev1.NodeSpec{},
				Status: corev1.NodeStatus{
					Addresses: []corev1.NodeAddress{
						{
							Type:    corev1.NodeExternalIP,
							Address: "205.20.20.2",
						},
						{
							Type:    corev1.NodeHostName,
							Address: "k8s-edge-node",
						},
					},
				},
			},
			expect: "205.20.20.2",
		},
		{
			desc: "get node primary host ip",
			node: &corev1.Node{
				TypeMeta:   metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{},
				Spec:       corev1.NodeSpec{},
				Status: corev1.NodeStatus{
					Addresses: []corev1.NodeAddress{
						{
							Type:    corev1.NodeHostName,
							Address: "k8s-edge-node",
						},
					},
				},
			},
			expect: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			act, _ := getNodeHostIP(tt.node)
			if act != tt.expect {
				t.Errorf("the result we want is: %v, but the actual result is: %v\n", tt.expect, act)
			}
		})
	}
}

func TestRemoveRecordByHostname(t *testing.T) {
	var (
		records  = []string{"10.1.218.68\tk8s-xing-61", "10.10.102.60\tk8s-xing-master"}
		hostname = "k8s-xing-61"
		expect   = []string{"10.10.102.60\tk8s-xing-master"}
	)

	act, changed := removeRecordByHostname(records, hostname)

	if !changed && !reflect.DeepEqual(act, records) {
		t.Errorf("the result we want is: %v, but the actual result is: %v\n", records, act)
	} else if !reflect.DeepEqual(act, expect) {
		t.Errorf("the result we want is: %v, but the actual result is: %v\n", expect, act)
	}
}

func TestParseHostnameFromDNSRecord(t *testing.T) {
	tests := []struct {
		desc   string
		record string
		expect string
	}{
		{
			desc:   "parse host name from dns record",
			record: "10.1.218.68\tk8s-xing-61",
			expect: "k8s-xing-61",
		},

		{
			desc:   "parse invalid dns recode",
			record: "10.10.102.2invalid dns record",
			expect: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			act, _ := parseHostnameFromDNSRecord(tt.record)
			if act != tt.expect {
				t.Errorf("the result we want is: %v, but the actual result is: %v\n", tt.expect, act)
			}
		})
	}
}

func TestAddOrUpdateRecord(t *testing.T) {
	tests := []struct {
		desc    string
		records []string
		record  string
		expect  []string
	}{
		{
			desc:    "test add record",
			records: []string{"10.1.218.68\tk8s-xing-61"},
			record:  "10.1.10.62\tk8s-xing-62",
			expect:  []string{"10.1.218.68\tk8s-xing-61", "10.1.10.62\tk8s-xing-62"},
		},

		{
			desc:    "test update record",
			records: []string{"10.1.218.68\tk8s-xing-61"},
			record:  "10.1.10.62\tk8s-xing-61",
			expect:  []string{"10.1.10.62\tk8s-xing-61"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			act, _, err := addOrUpdateRecord(tt.records, tt.record)
			if err != nil {
				t.Error(err)
			}
			if !stringSliceEqual(act, tt.expect) {
				t.Errorf("the result we want is: %v, but the actual result is: %v\n", tt.expect, act)
			}
		})
	}
}

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	if (a == nil) != (b == nil) {
		return false
	}

	for i, v := range a {
		if v != b[i] {
			return false
		}
	}

	return true
}
