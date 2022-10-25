/*
Copyright 2022 The OpenYurt Authors.

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

package healthchecker

import (
	"net/url"
	"testing"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	clientfake "k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

func TestIsHealthy(t *testing.T) {
	var latestLease *coordinationv1.Lease
	setLease := func(l *coordinationv1.Lease) error {
		latestLease = l
		return nil
	}
	getLease := func() *coordinationv1.Lease {
		return latestLease
	}

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "foo",
			UID:  types.UID("foo-uid"),
		},
	}

	lease := &coordinationv1.Lease{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "coordination.k8s.io/v1",
			Kind:       "Lease",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            "foo",
			Namespace:       "kube-node-lease",
			ResourceVersion: "115883910",
		},
	}
	remoteServer := &url.URL{Host: "127.0.0.1:18080"}

	gr := schema.GroupResource{Group: "v1", Resource: "lease"}
	noConnectionUpdateErr := apierrors.NewServerTimeout(gr, "put", 1)

	testcases := map[string]struct {
		renewTime     int
		createReactor clienttesting.ReactionFunc
		updateReactor []func(action clienttesting.Action) (bool, runtime.Object, error)
		getReactor    func(action clienttesting.Action) (bool, runtime.Object, error)
		initHealthy   bool
		probeHealthy  []bool
		isHealthy     []bool
	}{
		"init probe healthy": {
			createReactor: func(action clienttesting.Action) (bool, runtime.Object, error) {
				return true, lease, nil
			},
			initHealthy: true,
		},
		"init probe unhealthy": {
			createReactor: func(action clienttesting.Action) (bool, runtime.Object, error) {
				return true, nil, noConnectionUpdateErr
			},
			initHealthy: false,
		},
		"normal probe from healthy to healthy": {
			createReactor: func(action clienttesting.Action) (bool, runtime.Object, error) {
				return true, lease, nil
			},
			updateReactor: []func(action clienttesting.Action) (bool, runtime.Object, error){
				func(action clienttesting.Action) (bool, runtime.Object, error) {
					return true, lease, nil
				},
			},
			initHealthy:  true,
			probeHealthy: []bool{true},
			isHealthy:    []bool{true},
		},
		"normal probe from healthy to unhealthy": {
			createReactor: func(action clienttesting.Action) (bool, runtime.Object, error) {
				return true, lease, nil
			},
			updateReactor: []func(action clienttesting.Action) (bool, runtime.Object, error){
				func(action clienttesting.Action) (bool, runtime.Object, error) {
					return true, nil, noConnectionUpdateErr
				},
			},
			getReactor: func(action clienttesting.Action) (bool, runtime.Object, error) {
				return true, lease, nil
			},
			initHealthy:  true,
			probeHealthy: []bool{false},
			isHealthy:    []bool{false},
		},
		"normal probe from unhealthy to unhealthy": {
			createReactor: func(action clienttesting.Action) (bool, runtime.Object, error) {
				return true, lease, nil
			},
			updateReactor: []func(action clienttesting.Action) (bool, runtime.Object, error){
				func(action clienttesting.Action) (bool, runtime.Object, error) {
					return true, nil, noConnectionUpdateErr
				},
				func(action clienttesting.Action) (bool, runtime.Object, error) {
					return true, nil, noConnectionUpdateErr
				},
			},
			getReactor: func(action clienttesting.Action) (bool, runtime.Object, error) {
				return true, lease, nil
			},
			initHealthy:  true,
			probeHealthy: []bool{false, false},
			isHealthy:    []bool{false, false},
		},
		"normal probe from unhealthy to healthy": {
			createReactor: func(action clienttesting.Action) (bool, runtime.Object, error) {
				return true, lease, nil
			},
			updateReactor: []func(action clienttesting.Action) (bool, runtime.Object, error){
				func(action clienttesting.Action) (bool, runtime.Object, error) {
					return true, nil, noConnectionUpdateErr
				},
				func(action clienttesting.Action) (bool, runtime.Object, error) {
					return true, lease, nil
				},
				func(action clienttesting.Action) (bool, runtime.Object, error) {
					return true, lease, nil
				},
			},
			getReactor: func(action clienttesting.Action) (bool, runtime.Object, error) {
				return true, lease, nil
			},
			initHealthy:  true,
			probeHealthy: []bool{false, true, true},
			isHealthy:    []bool{false, false, true},
		},
		"kubelet stopped": {
			renewTime: -60,
			createReactor: func(action clienttesting.Action) (bool, runtime.Object, error) {
				return true, lease, nil
			},
			updateReactor: []func(action clienttesting.Action) (bool, runtime.Object, error){
				func(action clienttesting.Action) (bool, runtime.Object, error) {
					return true, nil, noConnectionUpdateErr
				},
				func(action clienttesting.Action) (bool, runtime.Object, error) {
					return true, lease, nil
				},
			},
			getReactor: func(action clienttesting.Action) (bool, runtime.Object, error) {
				return true, lease, nil
			},
			initHealthy:  true,
			probeHealthy: []bool{false, false},
			isHealthy:    []bool{false, false},
		},
	}

	for k, tt := range testcases {
		t.Run(k, func(t2 *testing.T) {
			cl := clientfake.NewSimpleClientset(node)
			cl.PrependReactor("create", "leases", tt.createReactor)
			prober := newProber(cl, remoteServer.String(), node.Name, 2, 2, 40*time.Second, setLease, getLease)
			if prober.IsHealthy() != tt.initHealthy {
				t.Errorf("expect server init healthy %v, but got %v", tt.initHealthy, prober.IsHealthy())
			}

			if tt.renewTime != 0 {
				prober.RenewKubeletLeaseTime(time.Now().Add(time.Duration(tt.renewTime) * time.Second))
			}

			if tt.getReactor != nil {
				cl.PrependReactor("get", "leases", tt.getReactor)
			}
			for i := range tt.updateReactor {
				cl.PrependReactor("update", "leases", tt.updateReactor[i])
				probeHealthy := prober.Probe(ProbePhaseNormal)
				if probeHealthy != tt.probeHealthy[i] {
					t.Errorf("expect probe return value should be %v, but got %v", tt.probeHealthy[i], probeHealthy)
				}

				if prober.IsHealthy() != tt.isHealthy[i] {
					t.Errorf("expect server healthy should be %v after probe, but got %v", tt.isHealthy[i], prober.IsHealthy())
				}
			}
		})
	}
}
