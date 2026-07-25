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

package controllers

import (
	"runtime"
	"testing"
	"time"

	"k8s.io/klog/v2"
)

// assertRunStopsOnSignal starts run in a goroutine and verifies that, once the
// stop channel is closed, run returns promptly and does not leave a background
// goroutine behind.
//
// The syncers are registered as controller-runtime runnables; on manager
// shutdown their stop channel is closed and Run must return so its goroutine
// terminates. A previous implementation ran the sync loop in a detached inner
// goroutine that never observed stop, so it leaked and kept hitting the edge
// platform and the API server after shutdown.
//
// A large syncPeriod keeps the loop parked in its select, so no real sync round
// runs (no edge/kube clients are required) and the test isolates the shutdown
// behaviour.
func assertRunStopsOnSignal(t *testing.T, name string, run func(stop <-chan struct{})) {
	t.Helper()

	// Warm up klog's background flush daemon so it is not mistaken for a leaked
	// goroutine when we snapshot the baseline below.
	klog.V(1).Info("warming up klog before goroutine snapshot")
	klog.Flush()
	time.Sleep(50 * time.Millisecond)

	baseline := runtime.NumGoroutine()

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		run(stop)
		close(done)
	}()

	close(stop)

	select {
	case <-done:
		// Run returned after stop was closed, as expected.
	case <-time.After(5 * time.Second):
		t.Fatalf("%s.Run did not return after stop was closed; the sync goroutine leaked", name)
	}

	// After Run returns, the sync loop must not have left a goroutine running.
	// Poll, because the runtime may take a moment to reap the finished goroutine.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if runtime.NumGoroutine() <= baseline {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s.Run leaked a goroutine after stop was closed: baseline=%d, still running=%d",
		name, baseline, runtime.NumGoroutine())
}

func TestDeviceSyncerRunStopsOnSignal(t *testing.T) {
	ds := &DeviceSyncer{syncPeriod: time.Hour}
	assertRunStopsOnSignal(t, "DeviceSyncer", ds.Run)
}

func TestDeviceServiceSyncerRunStopsOnSignal(t *testing.T) {
	dss := &DeviceServiceSyncer{syncPeriod: time.Hour}
	assertRunStopsOnSignal(t, "DeviceServiceSyncer", dss.Run)
}

func TestDeviceProfileSyncerRunStopsOnSignal(t *testing.T) {
	dps := &DeviceProfileSyncer{syncPeriod: time.Hour}
	assertRunStopsOnSignal(t, "DeviceProfileSyncer", dps.Run)
}
