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

package metrics

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	iotv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/iot/v1alpha1"
)

type iotCollector struct {
	client client.Client
}

var (
	deviceDesc = prometheus.NewDesc(
		"yurt_iot_devices_total",
		"Total number of OpenYurt IoT devices",
		[]string{"nodepool", "state"}, nil,
	)
	deviceServiceDesc = prometheus.NewDesc(
		"yurt_iot_device_services_total",
		"Total number of OpenYurt IoT device services",
		[]string{"nodepool"}, nil,
	)
	deviceProfileDesc = prometheus.NewDesc(
		"yurt_iot_device_profiles_total",
		"Total number of OpenYurt IoT device profiles",
		[]string{"nodepool"}, nil,
	)
)

// RegisterCustomMetrics registers the custom collector with the controller-runtime metrics registry
func RegisterCustomMetrics(c client.Client) {
	metrics.Registry.MustRegister(&iotCollector{client: c})
}

func (c *iotCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- deviceDesc
	ch <- deviceServiceDesc
	ch <- deviceProfileDesc
}

func (c *iotCollector) Collect(ch chan<- prometheus.Metric) {
	ctx := context.Background()

	// Devices
	var deviceList iotv1alpha1.DeviceList
	if err := c.client.List(ctx, &deviceList); err == nil {
		counts := make(map[string]map[string]float64)
		for _, d := range deviceList.Items {
			np := d.Spec.NodePool
			if np == "" {
				np = "unknown"
			}
			state := "unsynced"
			if d.Status.Synced {
				state = "synced"
			}
			if counts[np] == nil {
				counts[np] = make(map[string]float64)
			}
			counts[np][state]++
		}
		for np, states := range counts {
			for state, count := range states {
				ch <- prometheus.MustNewConstMetric(deviceDesc, prometheus.GaugeValue, count, np, state)
			}
		}
	}

	// DeviceServices
	var dsList iotv1alpha1.DeviceServiceList
	if err := c.client.List(ctx, &dsList); err == nil {
		counts := make(map[string]float64)
		for _, ds := range dsList.Items {
			np := ds.Spec.NodePool
			if np == "" {
				np = "unknown"
			}
			counts[np]++
		}
		for np, count := range counts {
			ch <- prometheus.MustNewConstMetric(deviceServiceDesc, prometheus.GaugeValue, count, np)
		}
	}

	// DeviceProfiles
	var dpList iotv1alpha1.DeviceProfileList
	if err := c.client.List(ctx, &dpList); err == nil {
		counts := make(map[string]float64)
		for _, dp := range dpList.Items {
			np := dp.Spec.NodePool
			if np == "" {
				np = "unknown"
			}
			counts[np]++
		}
		for np, count := range counts {
			ch <- prometheus.MustNewConstMetric(deviceProfileDesc, prometheus.GaugeValue, count, np)
		}
	}
}
