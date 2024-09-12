/*
Copyright 2023 The OpenYurt Authors.

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

package v1beta1

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openyurtio/openyurt/pkg/apis/raven/v1beta1"
)

func TestGatewayHandler_ValidateCreate(t *testing.T) {
	tests := []struct {
		name           string
		obj            runtime.Object
		expectedErrMsg string
	}{
		{
			name:           "should return error when object is not a Gateway",
			obj:            &runtime.Unknown{},
			expectedErrMsg: "expected a Gateway but got a *runtime.Unknown",
		},
		{
			name:           "should return error when Gateway has invalid ExposeType",
			obj:            mockGatewayWithExposeType("InvalidExposeType", false),
			expectedErrMsg: "the 'exposeType' field is irregularity",
		},
		{
			name:           "should return error when Gateway has valid ExposeType but underNAT is true",
			obj:            mockGatewayWithExposeType(v1beta1.ExposeTypeLoadBalancer, true),
			expectedErrMsg: "the 'underNAT' field for exposed gateway",
		},
		{
			name:           "should return error when Gateway TunnelConfig.Replicas >1",
			obj:            mockGatewayWithReplicas(2),
			expectedErrMsg: "the 'Replicas' field  can not be greater than 1",
		},
		{
			name:           "should return error when Gateway ProxyConfig.Replicas >1 and Endpoints count =1",
			obj:            mockGatewayWithReplicas(2),
			expectedErrMsg: "the 'endpoints' field available proxy endpoints 1 is less than the 'proxyConfig.Replicas'2",
		},
		{
			name:           "should return error when Gateway ip invalid",
			obj:            mockGatewayWithIp("invalid-ip"),
			expectedErrMsg: "the 'publicIP' field must be a validate IP address",
		},
		{
			name:           "should return error when Gateway nodeName is empty",
			obj:            mockGatewayWithNodeName(""),
			expectedErrMsg: "the 'nodeName' field must not be empty",
		},
		{
			name: "should return error when Gateway has inconsistent UnderNAT field in Endpoints",
			obj: &v1beta1.Gateway{
				Spec: v1beta1.GatewaySpec{Endpoints: []v1beta1.Endpoint{
					{UnderNAT: true, NodeName: "node1"},
					{UnderNAT: false, NodeName: "node2"},
				}},
			},
			expectedErrMsg: "the 'underNAT' field in endpoints must be the same",
		},
		{
			name:           "should pass when object is a valid Gateway",
			obj:            mockGateway(),
			expectedErrMsg: "",
		},
	}

	handler := &GatewayHandler{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := handler.ValidateCreate(context.Background(), tt.obj)
			if tt.expectedErrMsg != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErrMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGatewayHandler_ValidateUpdate(t *testing.T) {
	tests := []struct {
		name           string
		oldObj         runtime.Object
		newObj         runtime.Object
		expectedErrMsg string
	}{
		{
			name:           "should return error when new object is not a Gateway",
			oldObj:         mockGateway(),
			newObj:         &runtime.Unknown{},
			expectedErrMsg: "expected a Gateway but got a *runtime.Unknown",
		},
		{
			name:           "should return error when old object is not a Gateway",
			oldObj:         &runtime.Unknown{},
			newObj:         mockGateway(),
			expectedErrMsg: "expected a Gateway but got a *runtime.Unknown",
		},
		{
			name:           "should return error when Gateway name changes",
			oldObj:         mockGateway(),
			newObj:         mockGatewayWithNameChange(),
			expectedErrMsg: "gateway name can not change",
		},
		{
			name:           "should pass when Gateway is valid and unchanged",
			oldObj:         mockGateway(),
			newObj:         mockGateway(),
			expectedErrMsg: "",
		},
	}

	handler := &GatewayHandler{}
	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := handler.ValidateUpdate(ctx, tt.oldObj, tt.newObj)
			if tt.expectedErrMsg != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErrMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGatewayHandler_ValidateDelete(t *testing.T) {
	tests := []struct {
		name           string
		obj            runtime.Object
		expectedErrMsg string
	}{
		{
			name:           "should pass with no error",
			obj:            mockGateway(),
			expectedErrMsg: "",
		},
	}

	handler := &GatewayHandler{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := handler.ValidateDelete(context.Background(), tt.obj)
			if tt.expectedErrMsg != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErrMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func mockGateway() *v1beta1.Gateway {
	return &v1beta1.Gateway{
		Spec: v1beta1.GatewaySpec{
			ExposeType: v1beta1.ExposeTypeLoadBalancer,
			Endpoints: []v1beta1.Endpoint{
				{
					UnderNAT: false,
					PublicIP: "192.168.1.1",
					NodeName: "node1",
					Type:     v1beta1.Proxy,
				},
			},
			TunnelConfig: v1beta1.TunnelConfiguration{
				Replicas: 1,
			},
			ProxyConfig: v1beta1.ProxyConfiguration{
				Replicas: 1,
			},
		},
	}
}

func mockGatewayWithExposeType(ExposeType string, UnderNAT bool) *v1beta1.Gateway {
	g := mockGateway()
	if ExposeType != "" {
		g.Spec.ExposeType = ExposeType
	}
	g.Spec.Endpoints[0].UnderNAT = UnderNAT
	return g
}

func mockGatewayWithNameChange() *v1beta1.Gateway {
	g := mockGateway()
	g.Name = "new-name"
	return g
}

func mockGatewayWithIp(ip string) *v1beta1.Gateway {
	g := mockGateway()
	if ip != "" {
		g.Spec.Endpoints[0].PublicIP = ip
	}
	return g
}

func mockGatewayWithNodeName(nodeName string) *v1beta1.Gateway {
	g := mockGateway()
	g.Spec.Endpoints[0].NodeName = nodeName
	return g
}

func mockGatewayWithReplicas(Replicas int) *v1beta1.Gateway {
	g := mockGateway()
	g.Spec.ProxyConfig.Replicas = Replicas
	g.Spec.TunnelConfig.Replicas = Replicas
	return g
}
