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

package metrics

import (
	"strings"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
)

type LatencyType string

const (
	// duration: yurthub -> apiserver
	Apiserver_latency LatencyType = "apiserver_latency"
	// duration: coming to yurthub -> yurthub to apiserver -> leaving yurthub
	Full_lantency LatencyType = "full_latency"
)

var (
	namespace = "node"
	subsystem = strings.ReplaceAll(projectinfo.GetHubName(), "-", "_")
)

var (
	// Metrics provides access to all hub agent metrics.
	Metrics = newHubMetrics()
)

type HubMetrics struct {
	serversHealthyCollector              *prometheus.GaugeVec
	inFlightRequestsCollector            *prometheus.GaugeVec
	inFlightRequestsGauge                prometheus.Gauge
	inFlightMultiplexerRequestsCollector *prometheus.GaugeVec
	inFlightMultiplexerRequestsGauge     prometheus.Gauge
	closableConnsCollector               *prometheus.GaugeVec
	proxyTrafficCollector                *prometheus.CounterVec
	errorKeysPersistencyStatusCollector  prometheus.Gauge
	errorKeysCountCollector              prometheus.Gauge
}

func newHubMetrics() *HubMetrics {
	serversHealthyCollector := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "server_healthy_status",
			Help:      "healthy status of remote servers. 1: healthy, 0: unhealthy",
		},
		[]string{"server"})
	inFlightRequestsCollector := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "in_flight_requests_collector",
			Help:      "collector of in flight requests handling by hub agent",
		},
		[]string{"verb", "resource", "subresources", "client"})
	inFlightRequestsGauge := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "in_flight_requests_total",
			Help:      "total of in flight requests handling by hub agent",
		})
	inFlightMultiplexerRequestsCollector := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "in_flight_multiplexer_requests_collector",
			Help:      "collector of in flight requests handling by multiplexer manager",
		},
		[]string{"verb", "resource", "subresources", "client"})
	inFlightMultiplexerRequestsGauge := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "in_flight_multiplexer_requests_total",
			Help:      "total of in flight requests handling by multiplexer manager",
		})
	closableConnsCollector := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "closable_conns_collector",
			Help:      "collector of underlay tcp connection from hub agent to remote server",
		},
		[]string{"server"})
	proxyTrafficCollector := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "proxy_traffic_collector",
			Help:      "collector of proxy response traffic by hub agent(unit: byte)",
		},
		[]string{"client", "verb", "resource", "subresources"})
	errorKeysPersistencyStatusCollector := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "error_keys_persistency_status",
			Help:      "error keys persistency status 1: ready, 0: notReady",
		})
	errorKeysCountCollector := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "error_keys_count",
			Help:      "error keys count",
		})
	prometheus.MustRegister(serversHealthyCollector)
	prometheus.MustRegister(inFlightRequestsCollector)
	prometheus.MustRegister(inFlightRequestsGauge)
	prometheus.MustRegister(inFlightMultiplexerRequestsCollector)
	prometheus.MustRegister(inFlightMultiplexerRequestsGauge)
	prometheus.MustRegister(closableConnsCollector)
	prometheus.MustRegister(proxyTrafficCollector)
	prometheus.MustRegister(errorKeysPersistencyStatusCollector)
	prometheus.MustRegister(errorKeysCountCollector)
	return &HubMetrics{
		serversHealthyCollector:              serversHealthyCollector,
		inFlightRequestsCollector:            inFlightRequestsCollector,
		inFlightRequestsGauge:                inFlightRequestsGauge,
		inFlightMultiplexerRequestsCollector: inFlightMultiplexerRequestsCollector,
		inFlightMultiplexerRequestsGauge:     inFlightMultiplexerRequestsGauge,
		closableConnsCollector:               closableConnsCollector,
		proxyTrafficCollector:                proxyTrafficCollector,
		errorKeysPersistencyStatusCollector:  errorKeysPersistencyStatusCollector,
		errorKeysCountCollector:              errorKeysCountCollector,
	}
}

func (hm *HubMetrics) Reset() {
	hm.serversHealthyCollector.Reset()
	hm.inFlightRequestsCollector.Reset()
	hm.inFlightRequestsGauge.Set(float64(0))
	hm.inFlightMultiplexerRequestsCollector.Reset()
	hm.inFlightMultiplexerRequestsGauge.Set(float64(0))
	hm.closableConnsCollector.Reset()
	hm.proxyTrafficCollector.Reset()
	hm.errorKeysPersistencyStatusCollector.Set(float64(0))
	hm.errorKeysCountCollector.Set(float64(0))
}

func (hm *HubMetrics) ObserveServerHealthy(server string, status int) {
	hm.serversHealthyCollector.WithLabelValues(server).Set(float64(status))
}

func (hm *HubMetrics) IncInFlightRequests(verb, resource, subresource, client string) {
	hm.inFlightRequestsCollector.WithLabelValues(verb, resource, subresource, client).Inc()
	hm.inFlightRequestsGauge.Inc()
}

func (hm *HubMetrics) DecInFlightRequests(verb, resource, subresource, client string) {
	hm.inFlightRequestsCollector.WithLabelValues(verb, resource, subresource, client).Dec()
	hm.inFlightRequestsGauge.Dec()
}

func (hm *HubMetrics) IncInFlightMultiplexerRequests(verb, resource, subresource, client string) {
	hm.inFlightMultiplexerRequestsCollector.WithLabelValues(verb, resource, subresource, client).Inc()
	hm.inFlightMultiplexerRequestsGauge.Inc()
}

func (hm *HubMetrics) DecInFlightMultiplexerRequests(verb, resource, subresource, client string) {
	hm.inFlightMultiplexerRequestsCollector.WithLabelValues(verb, resource, subresource, client).Dec()
	hm.inFlightMultiplexerRequestsGauge.Dec()
}

func (hm *HubMetrics) IncClosableConns(server string) {
	hm.closableConnsCollector.WithLabelValues(server).Inc()
}

func (hm *HubMetrics) DecClosableConns(server string) {
	hm.closableConnsCollector.WithLabelValues(server).Dec()
}

func (hm *HubMetrics) SetClosableConns(server string, cnt int) {
	hm.closableConnsCollector.WithLabelValues(server).Set(float64(cnt))
}

func (hm *HubMetrics) AddProxyTrafficCollector(client, verb, resource, subresource string, size int) {
	if size > 0 {
		hm.proxyTrafficCollector.WithLabelValues(client, verb, resource, subresource).Add(float64(size))
	}
}

func (hm *HubMetrics) SetErrorKeysPersistencyStatus(status int) {
	hm.errorKeysPersistencyStatusCollector.Set(float64(status))
}

func (hm *HubMetrics) IncErrorKeysCount() {
	hm.errorKeysCountCollector.Inc()
}

func (hm *HubMetrics) DecErrorKeysCount() {
	hm.errorKeysCountCollector.Dec()
}
