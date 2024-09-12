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

package v1alpha1

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openyurtio/openyurt/pkg/apis/raven/v1alpha1"
)

func TestGatewayHandler_ValidateCreate(t *testing.T) {
	testCases := []struct {
		name           string
		obj            runtime.Object
		expectedErrMsg string
	}{
		{
			name:           "should return error when object is not Gateway",
			obj:            &runtime.Unknown{},
			expectedErrMsg: "expected a Gateway but got a *runtime.Unknown",
		},
		{
			name: "should return error when Gateway has no Endpoints",
			obj: &v1alpha1.Gateway{
				Spec: v1alpha1.GatewaySpec{Endpoints: []v1alpha1.Endpoint{}},
			},
			expectedErrMsg: "missing required field 'endpoints'",
		},
		{
			name: "should return error when UnderNAT is different in Endpoints",
			obj: &v1alpha1.Gateway{
				Spec: v1alpha1.GatewaySpec{Endpoints: []v1alpha1.Endpoint{
					{UnderNAT: true, NodeName: "node1"},
					{UnderNAT: false, NodeName: "node2"},
				}},
			},
			expectedErrMsg: "the 'underNAT' field in endpoints must be the same",
		},
		{
			name: "should return error when PublicIP is invalid",
			obj: &v1alpha1.Gateway{
				Spec: v1alpha1.GatewaySpec{Endpoints: []v1alpha1.Endpoint{
					{PublicIP: "invalid-ip", NodeName: "node1"},
				}},
			},
			expectedErrMsg: "the 'publicIP' field must be a validate IP address",
		},
		{
			name: "should return error when PublicIP is valid but exposeType is LoadBalancer",
			obj: &v1alpha1.Gateway{
				Spec: v1alpha1.GatewaySpec{Endpoints: []v1alpha1.Endpoint{
					{
						PublicIP: "192.168.0.1",
						NodeName: "node1",
					},
				},
					ExposeType: v1alpha1.ExposeTypeLoadBalancer,
				},
			},
			expectedErrMsg: "the 'publicIP' field must not be set when spec.exposeType = LoadBalancer",
		},
		{
			name: "should return error when NodeName is empty",
			obj: &v1alpha1.Gateway{
				Spec: v1alpha1.GatewaySpec{Endpoints: []v1alpha1.Endpoint{
					{NodeName: ""},
				}},
			},
			expectedErrMsg: "the 'nodeName' field must not be empty",
		},
	}

	webhook := &GatewayHandler{}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := webhook.ValidateCreate(context.TODO(), tc.obj)
			assert.Contains(t, err.Error(), tc.expectedErrMsg)
		})
	}
}

func TestGatewayHandler_ValidateUpdate(t *testing.T) {
	cases := []struct {
		name           string
		oldObj         runtime.Object
		newObj         runtime.Object
		expectedErrMsg string
	}{
		{
			name:           "should return error when newObj is not Gateway",
			oldObj:         mockGatewayWithEndpoints(),
			newObj:         &runtime.Unknown{},
			expectedErrMsg: "expected a Gateway but got a *runtime.Unknown",
		},
		{
			name:           "should return error when oldObj is not Gateway",
			oldObj:         &runtime.Unknown{},
			newObj:         mockGatewayWithEndpoints(),
			expectedErrMsg: "expected a Gateway but got a *runtime.Unknown",
		},
		{
			name:           "should return error when new Gateway is invalid",
			oldObj:         mockGatewayWithEndpoints(),
			newObj:         mockGatewayWithMissingEndpoints(),
			expectedErrMsg: "missing required field 'endpoints'",
		},
		{
			name:           "should return error when old Gateway is invalid",
			oldObj:         mockGatewayWithMissingEndpoints(),
			newObj:         mockGatewayWithEndpoints(),
			expectedErrMsg: "missing required field 'endpoints'",
		},
		{
			name:           "should validate Gateway when new and old objects are valid",
			oldObj:         mockGatewayWithEndpoints(),
			newObj:         mockGatewayWithEndpoints(),
			expectedErrMsg: "",
		},
	}

	handler := &GatewayHandler{}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := handler.ValidateUpdate(context.TODO(), tc.oldObj, tc.newObj)
			if tc.expectedErrMsg != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErrMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGatewayHandler_ValidateDelete(t *testing.T) {
	cases := []struct {
		name        string
		obj         runtime.Object
		expectError bool
	}{
		{
			name:        "should return error when obj is not Gateway",
			obj:         &runtime.Unknown{},
			expectError: true,
		},
		{
			name:        "should validate Gateway deletion when obj is valid",
			obj:         mockGatewayWithEndpoints(),
			expectError: false,
		},
	}

	handler := &GatewayHandler{}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := handler.ValidateDelete(context.Background(), tc.obj)
			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func mockGatewayWithEndpoints() *v1alpha1.Gateway {
	return &v1alpha1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-gateway",
		},
		Spec: v1alpha1.GatewaySpec{
			Endpoints: []v1alpha1.Endpoint{
				{
					UnderNAT: true,
					PublicIP: "192.168.0.1",
					NodeName: "node1",
				},
			},
		},
	}
}

func mockGatewayWithMissingEndpoints() *v1alpha1.Gateway {
	return &v1alpha1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-gateway",
		},
		Spec: v1alpha1.GatewaySpec{},
	}
}
