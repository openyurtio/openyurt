/*
Copyright 2026 The OpenYurt Authors.

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

package v1alpha1

import (
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openyurtio/openyurt/pkg/apis/raven/v1beta1"
)

func TestGatewayConvertTo(t *testing.T) {
	selector := &metav1.LabelSelector{MatchLabels: map[string]string{"raven.openyurt.io/gateway": "gw-a"}}
	cases := map[string]struct {
		src  *Gateway
		want *v1beta1.Gateway
	}{
		"endpoints and active endpoint become tunnel endpoints": {
			src: &Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw-a"},
				Spec: GatewaySpec{
					NodeSelector: selector,
					ExposeType:   ExposeTypeLoadBalancer,
					Endpoints: []Endpoint{
						{NodeName: "node-a", UnderNAT: true, Config: map[string]string{"k": "v"}},
						{NodeName: "node-b", PublicIP: "1.2.3.4"},
					},
				},
				Status: GatewayStatus{
					Nodes: []NodeInfo{
						{NodeName: "node-a", PrivateIP: "10.0.0.1", Subnets: []string{"10.244.0.0/24"}},
					},
					ActiveEndpoint: &Endpoint{NodeName: "node-b", PublicIP: "1.2.3.4"},
				},
			},
			want: &v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw-a"},
				Spec: v1beta1.GatewaySpec{
					NodeSelector: selector,
					ExposeType:   v1beta1.ExposeTypeLoadBalancer,
					ProxyConfig:  v1beta1.ProxyConfiguration{Replicas: 1},
					TunnelConfig: v1beta1.TunnelConfiguration{Replicas: 1},
					Endpoints: []v1beta1.Endpoint{
						{NodeName: "node-a", UnderNAT: true, Config: map[string]string{"k": "v"},
							Type: v1beta1.Tunnel, Port: v1beta1.DefaultTunnelServerExposedPort},
						{NodeName: "node-b", PublicIP: "1.2.3.4",
							Type: v1beta1.Tunnel, Port: v1beta1.DefaultTunnelServerExposedPort},
					},
				},
				Status: v1beta1.GatewayStatus{
					Nodes: []v1beta1.NodeInfo{
						{NodeName: "node-a", PrivateIP: "10.0.0.1", Subnets: []string{"10.244.0.0/24"}},
					},
					ActiveEndpoints: []*v1beta1.Endpoint{
						{NodeName: "node-b", PublicIP: "1.2.3.4",
							Type: v1beta1.Tunnel, Port: v1beta1.DefaultTunnelServerExposedPort},
					},
				},
			},
		},
		"no active endpoint": {
			src: &Gateway{ObjectMeta: metav1.ObjectMeta{Name: "gw-empty"}},
			want: &v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw-empty"},
				Spec: v1beta1.GatewaySpec{
					ProxyConfig:  v1beta1.ProxyConfiguration{Replicas: 1},
					TunnelConfig: v1beta1.TunnelConfiguration{Replicas: 1},
				},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			dst := &v1beta1.Gateway{}
			if err := tc.src.ConvertTo(dst); err != nil {
				t.Fatalf("ConvertTo() error = %v", err)
			}
			if !reflect.DeepEqual(dst, tc.want) {
				t.Errorf("ConvertTo() got\n%+v\nwant\n%+v", dst, tc.want)
			}
		})
	}
}

func TestGatewayConvertFrom(t *testing.T) {
	cases := map[string]struct {
		src                *v1beta1.Gateway
		wantActiveEndpoint *Endpoint
		wantEndpoints      []Endpoint
	}{
		"proxy endpoint sorted before tunnel endpoint": {
			// ActiveEndpoints is sorted by node name by the gateway pickup
			// controller, so a proxy endpoint can come first.
			src: &v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw-a"},
				Spec: v1beta1.GatewaySpec{
					Endpoints: []v1beta1.Endpoint{
						{NodeName: "node-a", Type: v1beta1.Proxy, Port: v1beta1.DefaultProxyServerExposedPort},
						{NodeName: "node-b", Type: v1beta1.Tunnel, Port: v1beta1.DefaultTunnelServerExposedPort, UnderNAT: true},
					},
				},
				Status: v1beta1.GatewayStatus{
					ActiveEndpoints: []*v1beta1.Endpoint{
						{NodeName: "node-a", Type: v1beta1.Proxy, PublicIP: "1.1.1.1"},
						{NodeName: "node-b", Type: v1beta1.Tunnel, PublicIP: "2.2.2.2", UnderNAT: true,
							Config: map[string]string{"enable-l3-tunnel": "true"}},
					},
				},
			},
			wantActiveEndpoint: &Endpoint{NodeName: "node-b", PublicIP: "2.2.2.2", UnderNAT: true,
				Config: map[string]string{"enable-l3-tunnel": "true"}},
			wantEndpoints: []Endpoint{
				{NodeName: "node-a"},
				{NodeName: "node-b", UnderNAT: true},
			},
		},
		"same node serves proxy and tunnel": {
			// The relative order of entries with equal node names is not
			// guaranteed by electActiveEndpoint (sort.Slice is not stable),
			// so this case constructs the proxy-first order explicitly.
			src: &v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw-same"},
				Status: v1beta1.GatewayStatus{
					ActiveEndpoints: []*v1beta1.Endpoint{
						{NodeName: "node-a", Type: v1beta1.Proxy, PublicIP: "1.1.1.1"},
						{NodeName: "node-a", Type: v1beta1.Tunnel, PublicIP: "2.2.2.2"},
					},
				},
			},
			wantActiveEndpoint: &Endpoint{NodeName: "node-a", PublicIP: "2.2.2.2"},
		},
		"only tunnel endpoint is active": {
			src: &v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw-tunnel"},
				Status: v1beta1.GatewayStatus{
					ActiveEndpoints: []*v1beta1.Endpoint{
						{NodeName: "node-b", Type: v1beta1.Tunnel, PublicIP: "2.2.2.2"},
					},
				},
			},
			wantActiveEndpoint: &Endpoint{NodeName: "node-b", PublicIP: "2.2.2.2"},
		},
		"multiple tunnel endpoints pick the first one": {
			src: &v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw-multi"},
				Status: v1beta1.GatewayStatus{
					ActiveEndpoints: []*v1beta1.Endpoint{
						{NodeName: "node-a", Type: v1beta1.Proxy},
						{NodeName: "node-b", Type: v1beta1.Tunnel},
						{NodeName: "node-c", Type: v1beta1.Tunnel},
					},
				},
			},
			wantActiveEndpoint: &Endpoint{NodeName: "node-b"},
		},
		"nil entries are skipped": {
			// Defensive: the API server does not normally return null
			// elements, but conversion must not panic on them.
			src: &v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw-nil-entry"},
				Status: v1beta1.GatewayStatus{
					ActiveEndpoints: []*v1beta1.Endpoint{
						nil,
						{NodeName: "node-b", Type: v1beta1.Tunnel},
					},
				},
			},
			wantActiveEndpoint: &Endpoint{NodeName: "node-b"},
		},
		"only proxy endpoints are active": {
			src: &v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw-proxy"},
				Status: v1beta1.GatewayStatus{
					ActiveEndpoints: []*v1beta1.Endpoint{
						{NodeName: "node-a", Type: v1beta1.Proxy},
					},
				},
			},
			wantActiveEndpoint: nil,
		},
		"empty active endpoints": {
			src: &v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw-empty"},
				Status:     v1beta1.GatewayStatus{ActiveEndpoints: []*v1beta1.Endpoint{}},
			},
			wantActiveEndpoint: nil,
		},
		"nil active endpoints": {
			src:                &v1beta1.Gateway{ObjectMeta: metav1.ObjectMeta{Name: "gw-nil"}},
			wantActiveEndpoint: nil,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			dst := &Gateway{}
			if err := dst.ConvertFrom(tc.src); err != nil {
				t.Fatalf("ConvertFrom() error = %v", err)
			}
			if dst.Name != tc.src.Name {
				t.Errorf("ConvertFrom() name = %q, want %q", dst.Name, tc.src.Name)
			}
			if !reflect.DeepEqual(dst.Status.ActiveEndpoint, tc.wantActiveEndpoint) {
				t.Errorf("ConvertFrom() activeEndpoint = %+v, want %+v", dst.Status.ActiveEndpoint, tc.wantActiveEndpoint)
			}
			if !reflect.DeepEqual(dst.Spec.Endpoints, tc.wantEndpoints) {
				t.Errorf("ConvertFrom() endpoints = %+v, want %+v", dst.Spec.Endpoints, tc.wantEndpoints)
			}
		})
	}
}

func TestGatewayConvertFromClearsStaleActiveEndpoint(t *testing.T) {
	cases := map[string][]*v1beta1.Endpoint{
		"nil active endpoints":   nil,
		"empty active endpoints": {},
		"only proxy endpoints":   {{NodeName: "node-a", Type: v1beta1.Proxy}},
	}
	for name, eps := range cases {
		t.Run(name, func(t *testing.T) {
			dst := &Gateway{Status: GatewayStatus{ActiveEndpoint: &Endpoint{NodeName: "stale"}}}
			src := &v1beta1.Gateway{Status: v1beta1.GatewayStatus{ActiveEndpoints: eps}}
			if err := dst.ConvertFrom(src); err != nil {
				t.Fatalf("ConvertFrom() error = %v", err)
			}
			if dst.Status.ActiveEndpoint != nil {
				t.Errorf("ConvertFrom() activeEndpoint = %+v, want nil", dst.Status.ActiveEndpoint)
			}
		})
	}
}
