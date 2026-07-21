/*
Copyright 2023 The OpenYurt Authors.

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

package v1beta1

import (
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openyurtio/openyurt/pkg/apis/raven"
)

func TestSetDefaultsGateway(t *testing.T) {
	tests := []struct {
		name     string
		obj      *Gateway
		expected *Gateway
	}{
		{
			name: "NodeSelector is nil",
			obj: &Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-gw",
				},
				Spec: GatewaySpec{},
			},
			expected: &Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-gw",
				},
				Spec: GatewaySpec{
					NodeSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							raven.LabelCurrentGateway: "test-gw",
						},
					},
				},
			},
		},
		{
			name: "MatchLabels is nil",
			obj: &Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-gw-2",
				},
				Spec: GatewaySpec{
					NodeSelector: &metav1.LabelSelector{},
				},
			},
			expected: &Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-gw-2",
				},
				Spec: GatewaySpec{
					NodeSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							raven.LabelCurrentGateway: "test-gw-2",
						},
					},
				},
			},
		},
		{
			name: "MatchLabels exists",
			obj: &Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-gw-3",
				},
				Spec: GatewaySpec{
					NodeSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"existing": "label",
						},
					},
				},
			},
			expected: &Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-gw-3",
				},
				Spec: GatewaySpec{
					NodeSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"existing":                "label",
							raven.LabelCurrentGateway: "test-gw-3",
						},
					},
				},
			},
		},
		{
			name: "Endpoints ports are set",
			obj: &Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-gw-4",
				},
				Spec: GatewaySpec{
					NodeSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{},
					},
					Endpoints: []Endpoint{
						{
							Type: Proxy,
							Port: 0,
						},
						{
							Type: Tunnel,
							Port: 0,
						},
					},
				},
			},
			expected: &Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-gw-4",
				},
				Spec: GatewaySpec{
					NodeSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							raven.LabelCurrentGateway: "test-gw-4",
						},
					},
					Endpoints: []Endpoint{
						{
							Type: Proxy,
							Port: DefaultProxyServerExposedPort,
						},
						{
							Type: Tunnel,
							Port: DefaultTunnelServerExposedPort,
						},
					},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			SetDefaultsGateway(tc.obj)
			if !reflect.DeepEqual(tc.obj, tc.expected) {
				t.Errorf("SetDefaultsGateway() = %v, expected %v", tc.obj, tc.expected)
			}
		})
	}
}
