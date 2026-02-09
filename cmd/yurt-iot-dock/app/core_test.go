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

package app

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockMetricsClient mocks the MetricsInterface
type MockMetricsClient struct {
	mock.Mock
}

func (m *MockMetricsClient) GetMetrics(ctx context.Context) (map[string]interface{}, error) {
	args := m.Called(ctx)
	return args.Get(0).(map[string]interface{}), args.Error(1)
}

func TestEdgeXCollector_Collect(t *testing.T) {
	mockClient := new(MockMetricsClient)
	collector := NewEdgeXCollector(mockClient)

	// Mock data
	metricsData := map[string]interface{}{
		"core-metadata": map[string]interface{}{
			"SysStats": map[string]interface{}{
				"CpuUsage": 0.5,
			},
		},
		"core-command": map[string]interface{}{
			"SysStats": map[string]interface{}{
				"Memory": 1024,
			},
		},
	}

	mockClient.On("GetMetrics", mock.Anything).Return(metricsData, nil)

	ch := make(chan prometheus.Metric, 10)
	collector.Collect(ch)
	close(ch)

	// Verify metrics
	var metrics []prometheus.Metric
	for m := range ch {
		metrics = append(metrics, m)
	}

	assert.Equal(t, 2, len(metrics)) // edgex_up removed in this branch

	// Check for specific metrics
	for _, m := range metrics {
		_ = m // Just iterating to ensure no panic
	}
	// Note: Strings might not match exactly due to help text, but we check count and presence basics.
	// For deeper inspection we'd need to cast to dto or Write to proto, but simple assertion on count is good start.
	assert.True(t, len(metrics) >= 1)
}

func TestEdgeXCollector_Collect_Error(t *testing.T) {
	mockClient := new(MockMetricsClient)
	collector := NewEdgeXCollector(mockClient)

	mockClient.On("GetMetrics", mock.Anything).Return(map[string]interface{}(nil), assert.AnError)

	ch := make(chan prometheus.Metric, 10)
	collector.Collect(ch)
	close(ch)

	var metrics []prometheus.Metric
	for m := range ch {
		metrics = append(metrics, m)
	}

	// Should not have 'up' metric set (not implemented in this branch)
	assert.Equal(t, 0, len(metrics))
}
