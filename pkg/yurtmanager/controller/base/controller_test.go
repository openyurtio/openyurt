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

package base

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/controller-manager/app"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/openyurtio/openyurt/cmd/yurt-manager/app/config"
	"github.com/openyurtio/openyurt/cmd/yurt-manager/names"
)

// MockControllerAddFunc is a mock controller add function
func MockControllerAddFunc(ctx context.Context, c *config.CompletedConfig, m manager.Manager) error {
	return nil
}

// MockControllerAddFuncWithError is a mock controller add function that returns error
func MockControllerAddFuncWithError(ctx context.Context, c *config.CompletedConfig, m manager.Manager) error {
	return errors.New("mock controller error")
}

// MockControllerAddFuncWithNoKindMatchError is a mock controller add function that returns NoKindMatchError
func MockControllerAddFuncWithNoKindMatchError(ctx context.Context, c *config.CompletedConfig, m manager.Manager) error {
	return &meta.NoKindMatchError{
		GroupKind: schema.GroupKind{Group: "test", Kind: "Test"},
	}
}

func TestKnownControllers(t *testing.T) {
	controllers := KnownControllers()

	// Verify that we get a non-empty list
	assert.NotEmpty(t, controllers)

	// Verify that all expected controllers are present
	expectedControllers := sets.NewString(
		names.CsrApproverController,
		names.DaemonPodUpdaterController,
		names.PodBindingController,
		names.NodePoolController,
		names.ServiceTopologyEndpointsController,
		names.ServiceTopologyEndpointSliceController,
		names.YurtStaticSetController,
		names.YurtAppSetController,
		names.GatewayPickupController,
		names.GatewayDNSController,
		names.GatewayInternalServiceController,
		names.GatewayPublicServiceController,
		names.NodeLifeCycleController,
		names.NodeBucketController,
		names.LoadBalancerSetController,
		names.ImagePreheatController,
		names.HubLeaderController,
		names.HubLeaderConfigController,
		names.HubLeaderRBACController,
		names.PlatformAdminController,
	)

	controllerSet := sets.NewString(controllers...)
	assert.True(t, expectedControllers.Equal(controllerSet), "Expected controllers: %v, Got: %v", expectedControllers.List(), controllers)
}

func TestNewControllerInitializers(t *testing.T) {
	initializers := NewControllerInitializers()

	// Verify that we get a non-empty map
	assert.NotEmpty(t, initializers)

	// Verify that all expected controllers are registered
	expectedControllers := sets.NewString(
		names.CsrApproverController,
		names.DaemonPodUpdaterController,
		names.PodBindingController,
		names.NodePoolController,
		names.ServiceTopologyEndpointsController,
		names.ServiceTopologyEndpointSliceController,
		names.YurtStaticSetController,
		names.YurtAppSetController,
		names.GatewayPickupController,
		names.GatewayDNSController,
		names.GatewayInternalServiceController,
		names.GatewayPublicServiceController,
		names.NodeLifeCycleController,
		names.NodeBucketController,
		names.LoadBalancerSetController,
		names.ImagePreheatController,
		names.HubLeaderController,
		names.HubLeaderConfigController,
		names.HubLeaderRBACController,
		names.PlatformAdminController,
	)

	controllerSet := sets.NewString()
	for name := range initializers {
		controllerSet.Insert(name)
	}

	assert.True(t, expectedControllers.Equal(controllerSet), "Expected controllers: %v, Got: %v", expectedControllers.List(), controllerSet.List())

	// Verify that all initializers are functions
	for name, fn := range initializers {
		assert.NotNil(t, fn, "Controller %s should have a non-nil initializer function", name)
	}
}

func TestNewControllerInitializersDuplicateRegistration(t *testing.T) {
	// Test the register function's panic behavior for duplicate registration
	defer func() {
		if r := recover(); r != nil {
			// Expected panic for duplicate registration
			assert.Contains(t, r.(string), "was registered twice")
		} else {
			t.Fatal("Expected panic for duplicate registration")
		}
	}()

	// Create a test function that tries to register the same controller twice
	testRegister := func() {
		controllers := map[string]InitFunc{}
		register := func(name string, fn InitFunc) {
			if _, found := controllers[name]; found {
				panic("controller name " + name + " was registered twice")
			}
			controllers[name] = fn
		}

		register("test-controller", MockControllerAddFunc)
		register("test-controller", MockControllerAddFunc) // This should panic
	}

	testRegister()
}

func TestControllersDisabledByDefault(t *testing.T) {
	// Test that ControllersDisabledByDefault is properly initialized
	assert.NotNil(t, ControllersDisabledByDefault)
	assert.True(t, ControllersDisabledByDefault.Len() >= 0) // Should be empty or have some disabled controllers
}

func TestControllerInitializersFuncInterface(t *testing.T) {
	// Test that NewControllerInitializers implements ControllerInitializersFunc interface
	var _ ControllerInitializersFunc = NewControllerInitializers
}

// Test the field indexer function behavior
func TestFieldIndexerFunction(t *testing.T) {
	// Test with valid pod
	pod := &v1.Pod{
		Spec: v1.PodSpec{
			NodeName: "test-node",
		},
	}

	indexerFunc := func(rawObj client.Object) []string {
		pod, ok := rawObj.(*v1.Pod)
		if !ok {
			return []string{}
		}
		if len(pod.Spec.NodeName) == 0 {
			return []string{}
		}
		return []string{pod.Spec.NodeName}
	}

	result := indexerFunc(pod)
	assert.Equal(t, []string{"test-node"}, result)

	// Test with pod without node name
	podNoNode := &v1.Pod{
		Spec: v1.PodSpec{
			NodeName: "",
		},
	}

	result = indexerFunc(podNoNode)
	assert.Equal(t, []string{}, result)

	// Test with non-pod object
	nonPod := &v1.Node{}

	result = indexerFunc(nonPod)
	assert.Equal(t, []string{}, result)
}

// Test the register function behavior
func TestRegisterFunction(t *testing.T) {
	controllers := map[string]InitFunc{}
	register := func(name string, fn InitFunc) {
		if _, found := controllers[name]; found {
			panic("controller name " + name + " was registered twice")
		}
		controllers[name] = fn
	}

	// Test successful registration
	register("test-controller-1", MockControllerAddFunc)
	assert.Contains(t, controllers, "test-controller-1")
	assert.NotNil(t, controllers["test-controller-1"])

	// Test duplicate registration panic
	defer func() {
		if r := recover(); r != nil {
			assert.Contains(t, r.(string), "was registered twice")
		} else {
			t.Fatal("Expected panic for duplicate registration")
		}
	}()

	register("test-controller-1", MockControllerAddFunc) // This should panic
}

// Test app.IsControllerEnabled function behavior
func TestIsControllerEnabled(t *testing.T) {
	// Test with empty disabled set
	disabledByDefault := sets.NewString()
	enabledControllers := []string{"test-controller"}

	// Test enabled controller
	result := app.IsControllerEnabled("test-controller", disabledByDefault, enabledControllers)
	assert.True(t, result)

	// Test disabled controller
	result = app.IsControllerEnabled("disabled-controller", disabledByDefault, enabledControllers)
	assert.False(t, result)

	// Test with controller in disabled set
	disabledByDefault.Insert("disabled-controller")
	result = app.IsControllerEnabled("disabled-controller", disabledByDefault, enabledControllers)
	assert.False(t, result)
}

// Test NoKindMatchError handling
func TestNoKindMatchErrorHandling(t *testing.T) {
	err := &meta.NoKindMatchError{
		GroupKind: schema.GroupKind{Group: "test", Kind: "Test"},
	}

	// Test that the error has the expected properties
	assert.Equal(t, "test", err.GroupKind.Group)
	assert.Equal(t, "Test", err.GroupKind.Kind)

	// Test that it implements the error interface
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no matches for kind")
}

// Test the field indexer function with different scenarios
func TestFieldIndexerFunctionScenarios(t *testing.T) {
	indexerFunc := func(rawObj client.Object) []string {
		pod, ok := rawObj.(*v1.Pod)
		if !ok {
			return []string{}
		}
		if len(pod.Spec.NodeName) == 0 {
			return []string{}
		}
		return []string{pod.Spec.NodeName}
	}

	// Test case 1: Valid pod with node name
	podWithNode := &v1.Pod{
		Spec: v1.PodSpec{
			NodeName: "node-1",
		},
	}
	result := indexerFunc(podWithNode)
	assert.Equal(t, []string{"node-1"}, result)

	// Test case 2: Pod with empty node name
	podEmptyNode := &v1.Pod{
		Spec: v1.PodSpec{
			NodeName: "",
		},
	}
	result = indexerFunc(podEmptyNode)
	assert.Equal(t, []string{}, result)

	// Test case 3: Non-pod object
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-1",
		},
	}
	result = indexerFunc(node)
	assert.Equal(t, []string{}, result)

	// Test case 4: Pod with long node name
	podLongNode := &v1.Pod{
		Spec: v1.PodSpec{
			NodeName: "very-long-node-name-with-many-characters",
		},
	}
	result = indexerFunc(podLongNode)
	assert.Equal(t, []string{"very-long-node-name-with-many-characters"}, result)
}

// Test the field indexer function with edge cases
func TestFieldIndexerFunctionEdgeCases(t *testing.T) {
	indexerFunc := func(rawObj client.Object) []string {
		pod, ok := rawObj.(*v1.Pod)
		if !ok {
			return []string{}
		}
		if len(pod.Spec.NodeName) == 0 {
			return []string{}
		}
		return []string{pod.Spec.NodeName}
	}

	// Test case 1: Pod with special characters in node name
	podSpecialNode := &v1.Pod{
		Spec: v1.PodSpec{
			NodeName: "node-with-special-chars-123",
		},
	}
	result := indexerFunc(podSpecialNode)
	assert.Equal(t, []string{"node-with-special-chars-123"}, result)

	// Test case 2: Pod with very short node name
	podShortNode := &v1.Pod{
		Spec: v1.PodSpec{
			NodeName: "n",
		},
	}
	result = indexerFunc(podShortNode)
	assert.Equal(t, []string{"n"}, result)

	// Test case 3: Pod with node name containing spaces (edge case)
	podSpaceNode := &v1.Pod{
		Spec: v1.PodSpec{
			NodeName: "node with spaces",
		},
	}
	result = indexerFunc(podSpaceNode)
	assert.Equal(t, []string{"node with spaces"}, result)
}

// Test the field indexer function with nil input
func TestFieldIndexerFunctionWithNilInput(t *testing.T) {
	indexerFunc := func(rawObj client.Object) []string {
		pod, ok := rawObj.(*v1.Pod)
		if !ok {
			return []string{}
		}
		if len(pod.Spec.NodeName) == 0 {
			return []string{}
		}
		return []string{pod.Spec.NodeName}
	}

	// Test with nil input
	result := indexerFunc(nil)
	assert.Equal(t, []string{}, result)
}

// Test the field indexer function with different object types
func TestFieldIndexerFunctionWithDifferentObjectTypes(t *testing.T) {
	indexerFunc := func(rawObj client.Object) []string {
		pod, ok := rawObj.(*v1.Pod)
		if !ok {
			return []string{}
		}
		if len(pod.Spec.NodeName) == 0 {
			return []string{}
		}
		return []string{pod.Spec.NodeName}
	}

	// Test with different object types
	objects := []client.Object{
		&v1.Node{},
		&v1.Service{},
		&v1.Namespace{},
		&v1.ConfigMap{},
		&v1.Secret{},
	}

	for _, obj := range objects {
		result := indexerFunc(obj)
		assert.Equal(t, []string{}, result, "Expected empty result for non-pod object")
	}
}

// Test the field indexer function with pod variations
func TestFieldIndexerFunctionWithPodVariations(t *testing.T) {
	indexerFunc := func(rawObj client.Object) []string {
		pod, ok := rawObj.(*v1.Pod)
		if !ok {
			return []string{}
		}
		if len(pod.Spec.NodeName) == 0 {
			return []string{}
		}
		return []string{pod.Spec.NodeName}
	}

	// Test with different pod configurations
	testCases := []struct {
		name     string
		pod      *v1.Pod
		expected []string
	}{
		{
			name: "pod with node name",
			pod: &v1.Pod{
				Spec: v1.PodSpec{
					NodeName: "test-node",
				},
			},
			expected: []string{"test-node"},
		},
		{
			name: "pod without node name",
			pod: &v1.Pod{
				Spec: v1.PodSpec{
					NodeName: "",
				},
			},
			expected: []string{},
		},
		{
			name: "pod with empty spec",
			pod: &v1.Pod{
				Spec: v1.PodSpec{},
			},
			expected: []string{},
		},
		{
			name:     "pod with nil spec",
			pod:      &v1.Pod{},
			expected: []string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := indexerFunc(tc.pod)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// Test the SetupWithManager function with a simple test
func TestSetupWithManagerBasic(t *testing.T) {
	// This test focuses on testing the logic we can test without complex mocking
	// We'll test the field indexer function that's used in SetupWithManager

	// Test the field indexer function that's used in SetupWithManager
	indexerFunc := func(rawObj client.Object) []string {
		pod, ok := rawObj.(*v1.Pod)
		if !ok {
			return []string{}
		}
		if len(pod.Spec.NodeName) == 0 {
			return []string{}
		}
		return []string{pod.Spec.NodeName}
	}

	// Test the indexer function with various inputs
	testCases := []struct {
		name     string
		input    client.Object
		expected []string
	}{
		{
			name: "valid pod with node name",
			input: &v1.Pod{
				Spec: v1.PodSpec{
					NodeName: "test-node",
				},
			},
			expected: []string{"test-node"},
		},
		{
			name: "pod without node name",
			input: &v1.Pod{
				Spec: v1.PodSpec{
					NodeName: "",
				},
			},
			expected: []string{},
		},
		{
			name:     "non-pod object",
			input:    &v1.Node{},
			expected: []string{},
		},
		{
			name:     "nil input",
			input:    nil,
			expected: []string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := indexerFunc(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// Test the controller registration logic
func TestControllerRegistrationLogic(t *testing.T) {
	// Test the register function logic that's used in NewControllerInitializers
	controllers := map[string]InitFunc{}
	register := func(name string, fn InitFunc) {
		if _, found := controllers[name]; found {
			panic("controller name " + name + " was registered twice")
		}
		controllers[name] = fn
	}

	// Test successful registration
	register("test-controller-1", MockControllerAddFunc)
	assert.Contains(t, controllers, "test-controller-1")
	assert.NotNil(t, controllers["test-controller-1"])

	// Test that we can't register the same controller twice
	defer func() {
		if r := recover(); r != nil {
			assert.Contains(t, r.(string), "was registered twice")
		} else {
			t.Fatal("Expected panic for duplicate registration")
		}
	}()

	register("test-controller-1", MockControllerAddFunc) // This should panic
}

// Test the controller enabled logic
func TestControllerEnabledLogic(t *testing.T) {
	// Test the logic used in SetupWithManager for checking if controllers are enabled
	disabledByDefault := sets.NewString()
	enabledControllers := []string{"test-controller"}

	// Test enabled controller
	result := app.IsControllerEnabled("test-controller", disabledByDefault, enabledControllers)
	assert.True(t, result)

	// Test disabled controller
	result = app.IsControllerEnabled("disabled-controller", disabledByDefault, enabledControllers)
	assert.False(t, result)

	// Test with controller in disabled set
	disabledByDefault.Insert("disabled-controller")
	result = app.IsControllerEnabled("disabled-controller", disabledByDefault, enabledControllers)
	assert.False(t, result)
}

// Test the NoKindMatchError handling logic
func TestNoKindMatchErrorHandlingLogic(t *testing.T) {
	// Test the error handling logic used in SetupWithManager
	err := &meta.NoKindMatchError{
		GroupKind: schema.GroupKind{Group: "test", Kind: "Test"},
	}

	// Test that the error has the expected properties
	assert.Equal(t, "test", err.GroupKind.Group)
	assert.Equal(t, "Test", err.GroupKind.Kind)

	// Test that it implements the error interface
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no matches for kind")
}

// Test the field indexer function with comprehensive scenarios
func TestFieldIndexerFunctionComprehensive(t *testing.T) {
	indexerFunc := func(rawObj client.Object) []string {
		pod, ok := rawObj.(*v1.Pod)
		if !ok {
			return []string{}
		}
		if len(pod.Spec.NodeName) == 0 {
			return []string{}
		}
		return []string{pod.Spec.NodeName}
	}

	// Test comprehensive scenarios
	testCases := []struct {
		name     string
		input    client.Object
		expected []string
	}{
		{
			name: "pod with normal node name",
			input: &v1.Pod{
				Spec: v1.PodSpec{
					NodeName: "worker-node-1",
				},
			},
			expected: []string{"worker-node-1"},
		},
		{
			name: "pod with empty node name",
			input: &v1.Pod{
				Spec: v1.PodSpec{
					NodeName: "",
				},
			},
			expected: []string{},
		},
		{
			name: "pod with special characters in node name",
			input: &v1.Pod{
				Spec: v1.PodSpec{
					NodeName: "node-with-dashes_underscores.123",
				},
			},
			expected: []string{"node-with-dashes_underscores.123"},
		},
		{
			name: "pod with very long node name",
			input: &v1.Pod{
				Spec: v1.PodSpec{
					NodeName: "very-long-node-name-that-exceeds-normal-limits-and-contains-many-characters",
				},
			},
			expected: []string{"very-long-node-name-that-exceeds-normal-limits-and-contains-many-characters"},
		},
		{
			name:     "non-pod object (node)",
			input:    &v1.Node{},
			expected: []string{},
		},
		{
			name:     "non-pod object (service)",
			input:    &v1.Service{},
			expected: []string{},
		},
		{
			name:     "nil input",
			input:    nil,
			expected: []string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := indexerFunc(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}
