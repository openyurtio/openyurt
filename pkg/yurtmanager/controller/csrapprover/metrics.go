/*
Copyright 2024 The OpenYurt Authors.

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

package csrapprover

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	csrApproverSubsystem = "csr_approver"
)

var (
	// csrApprovedTotal is the total number of CSRs auto-approved by type.
	csrApprovedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: csrApproverSubsystem,
			Name:      "approved_total",
			Help:      "Total number of CSRs auto-approved by component type.",
		},
		[]string{"type"},
	)

	// csrRejectedTotal is the total number of CSRs not approved (rejected/ignored) by reason.
	csrRejectedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: csrApproverSubsystem,
			Name:      "rejected_total",
			Help:      "Total number of CSRs not approved by reason.",
		},
		[]string{"reason"},
	)

	// csrPendingDurationSeconds is the time from CSR creation to approval decision.
	csrPendingDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Subsystem: csrApproverSubsystem,
			Name:      "pending_duration_seconds",
			Help:      "Time in seconds from CSR creation to approval decision.",
			Buckets:   []float64{0.1, 0.2, 0.5, 1, 2, 5, 10, 20, 50, 100}, // 0.1s -> 100s
		},
		[]string{"type"},
	)
)

var registerMetrics sync.Once

// RegisterMetrics registers the CSR approver metrics to controller-runtime's registry.
func RegisterMetrics() {
	registerMetrics.Do(func() {
		metrics.Registry.MustRegister(csrApprovedTotal)
		metrics.Registry.MustRegister(csrRejectedTotal)
		metrics.Registry.MustRegister(csrPendingDurationSeconds)
	})
}
