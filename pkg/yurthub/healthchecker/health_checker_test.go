/*
Copyright 2020 The OpenYurt Authors.

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
	"os"
	"testing"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	clientfake "k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/config"
	"github.com/openyurtio/openyurt/pkg/yurthub/cachemanager"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage/disk"
)

var (
	rootDir = "/tmp/healthz"
)

type fakeMultipleBackendsHealthChecker struct {
	status bool
}

func (fakeChecker *fakeMultipleBackendsHealthChecker) IsHealthy() bool {
	return fakeChecker.status
}

func (fakeChecker *fakeMultipleBackendsHealthChecker) RenewKubeletLeaseTime() {
	// do nothing
}

func (fakeChecker *fakeMultipleBackendsHealthChecker) BackendHealthyStatus(*url.URL) bool {
	return fakeChecker.status
}

func TestNewCoordinatorHealthChecker(t *testing.T) {
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
	var delegateLease *coordinationv1.Lease
	gr := schema.GroupResource{Group: "v1", Resource: "lease"}
	testcases := map[string]struct {
		cloudAPIServerUnhealthy bool
		createReactor           clienttesting.ReactionFunc
		updateReactor           clienttesting.ReactionFunc
		getReactor              clienttesting.ReactionFunc
		initHealthy             bool
		probeHealthy            bool
	}{
		"both init and probe healthy": {
			createReactor: func(action clienttesting.Action) (bool, runtime.Object, error) {
				return true, lease, nil
			},
			updateReactor: func(action clienttesting.Action) (bool, runtime.Object, error) {
				return true, lease, nil
			},
			initHealthy:  true,
			probeHealthy: true,
		},
		"init healthy and probe unhealthy": {
			createReactor: func(action clienttesting.Action) (bool, runtime.Object, error) {
				return true, lease, nil
			},
			updateReactor: func(action clienttesting.Action) (bool, runtime.Object, error) {
				return true, nil, apierrors.NewServerTimeout(gr, "update", 1)
			},
			getReactor: func() func(action clienttesting.Action) (bool, runtime.Object, error) {
				i := 0
				return func(action clienttesting.Action) (bool, runtime.Object, error) {
					i++
					switch i {
					case 1:
						return true, nil, apierrors.NewNotFound(gr, "not found")
					default:
						return true, lease, nil
					}
				}
			}(),
			initHealthy:  true,
			probeHealthy: false,
		},
		"init unhealthy and probe unhealthy": {
			createReactor: func(action clienttesting.Action) (bool, runtime.Object, error) {
				return true, nil, apierrors.NewServerTimeout(gr, "create", 1)
			},
			updateReactor: func(action clienttesting.Action) (bool, runtime.Object, error) {
				return true, nil, apierrors.NewServerTimeout(gr, "update", 1)
			},
			getReactor: func(action clienttesting.Action) (bool, runtime.Object, error) {
				return true, nil, apierrors.NewServerTimeout(gr, "get", 1)
			},
			initHealthy:  false,
			probeHealthy: false,
		},
		"init unhealthy and probe healthy": {
			createReactor: func(action clienttesting.Action) (bool, runtime.Object, error) {
				return true, nil, apierrors.NewServerTimeout(gr, "create", 1)
			},
			updateReactor: func(action clienttesting.Action) (bool, runtime.Object, error) {
				return true, lease, nil
			},
			getReactor: func() func(action clienttesting.Action) (bool, runtime.Object, error) {
				i := 0
				return func(action clienttesting.Action) (bool, runtime.Object, error) {
					i++
					switch {
					case i <= 4:
						return true, nil, apierrors.NewNotFound(gr, "not found")
					default:
						return true, lease, nil
					}
				}
			}(),
			initHealthy:  false,
			probeHealthy: true,
		},
		"cloud apiserver checker is unhealthy": {
			cloudAPIServerUnhealthy: true,
			createReactor: func(action clienttesting.Action) (bool, runtime.Object, error) {
				return true, lease, nil
			},
			updateReactor: func(action clienttesting.Action) (bool, runtime.Object, error) {
				if updateAction, ok := action.(clienttesting.UpdateAction); ok {
					delegateLease, _ = updateAction.GetObject().(*coordinationv1.Lease)
					return true, updateAction.GetObject(), nil
				}
				return true, nil, apierrors.NewServerTimeout(gr, "update", 1)
			},
			getReactor: func(action clienttesting.Action) (bool, runtime.Object, error) {
				return true, lease, nil
			},
			initHealthy:  true,
			probeHealthy: true,
		},
	}

	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			cfg := &config.YurtHubConfiguration{
				CoordinatorServerURL:      &url.URL{Host: "127.0.0.1:18080"},
				NodeName:                  node.Name,
				HeartbeatFailedRetry:      2,
				HeartbeatHealthyThreshold: 1,
				HeartbeatIntervalSeconds:  3,
				KubeletHealthGracePeriod:  40,
			}

			cl := clientfake.NewSimpleClientset(node)
			cl.PrependReactor("create", "leases", tt.createReactor)
			cl.PrependReactor("update", "leases", tt.updateReactor)
			if tt.getReactor != nil {
				cl.PrependReactor("get", "leases", tt.getReactor)
			}

			cloudChecker := &fakeMultipleBackendsHealthChecker{status: !tt.cloudAPIServerUnhealthy}
			stopCh := make(chan struct{})
			checker, _ := NewCoordinatorHealthChecker(cfg, cl, cloudChecker, stopCh)

			initHealthy := checker.IsHealthy()
			if initHealthy != tt.initHealthy {
				t.Errorf("new coordinator health checker, expect init healthy %v, but got %v", tt.initHealthy, initHealthy)
			}

			// wait for the probe completed
			time.Sleep(5 * time.Second)
			probeHealthy := checker.IsHealthy()
			if probeHealthy != tt.probeHealthy {
				t.Errorf("after probe, expect probe healthy %v, but got %v", tt.probeHealthy, probeHealthy)
			}

			if tt.cloudAPIServerUnhealthy {
				if delegateLease == nil || len(delegateLease.Annotations) == 0 {
					t.Errorf("expect delegate heartbeat annotaion, but got nil")
				} else if v, ok := delegateLease.Annotations[DelegateHeartBeat]; !ok || v != "true" {
					t.Errorf("expect delegate heartbeat annotaion and v is true, but got empty or %v", v)
				}
			}

			close(stopCh)
		})
	}
}

func TestNewCloudAPIServerHealthChecker(t *testing.T) {
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

	gr := schema.GroupResource{Group: "v1", Resource: "lease"}
	noConnectionUpdateErr := apierrors.NewServerTimeout(gr, "put", 1)
	testcases := map[string]struct {
		remoteServers []*url.URL
		createReactor []clienttesting.ReactionFunc
		updateReactor []clienttesting.ReactionFunc
		getReactor    []clienttesting.ReactionFunc
		isHealthy     []bool
		serverHealthy bool
	}{
		"init healthy for one server": {
			remoteServers: []*url.URL{
				{Host: "127.0.0.1:18080"},
			},
			createReactor: []clienttesting.ReactionFunc{
				func(action clienttesting.Action) (bool, runtime.Object, error) {
					return true, lease, nil
				},
			},
			updateReactor: []clienttesting.ReactionFunc{
				func(action clienttesting.Action) (bool, runtime.Object, error) {
					return true, lease, nil
				},
			},
			getReactor: []clienttesting.ReactionFunc{
				func(action clienttesting.Action) (bool, runtime.Object, error) {
					return true, nil, apierrors.NewNotFound(gr, "not found")
				},
			},
			isHealthy:     []bool{true},
			serverHealthy: true,
		},
		"init unhealthy for one server": {
			remoteServers: []*url.URL{
				{Host: "127.0.0.1:18080"},
			},
			createReactor: []clienttesting.ReactionFunc{
				func(action clienttesting.Action) (bool, runtime.Object, error) {
					return true, nil, noConnectionUpdateErr
				},
			},
			updateReactor: []clienttesting.ReactionFunc{
				func(action clienttesting.Action) (bool, runtime.Object, error) {
					return true, nil, noConnectionUpdateErr
				},
			},
			getReactor: []clienttesting.ReactionFunc{
				func(action clienttesting.Action) (bool, runtime.Object, error) {
					return true, nil, apierrors.NewNotFound(gr, "not found")
				},
			},
			isHealthy:     []bool{false},
			serverHealthy: false,
		},
		"both init and probe healthy for two servers": {
			remoteServers: []*url.URL{
				{Host: "127.0.0.1:18080"},
				{Host: "127.0.0.1:18081"},
			},
			createReactor: []clienttesting.ReactionFunc{
				func(action clienttesting.Action) (bool, runtime.Object, error) {
					return true, lease, nil
				},
				func(action clienttesting.Action) (bool, runtime.Object, error) {
					return true, lease, nil
				},
			},
			updateReactor: []clienttesting.ReactionFunc{
				func(action clienttesting.Action) (bool, runtime.Object, error) {
					return true, lease, nil
				},
				func(action clienttesting.Action) (bool, runtime.Object, error) {
					return true, lease, nil
				},
			},
			getReactor: []clienttesting.ReactionFunc{
				func(action clienttesting.Action) (bool, runtime.Object, error) {
					return true, nil, apierrors.NewNotFound(gr, "not found")
				},
				func(action clienttesting.Action) (bool, runtime.Object, error) {
					return true, nil, apierrors.NewNotFound(gr, "not found")
				},
			},
			isHealthy:     []bool{true, true},
			serverHealthy: true,
		},
		"one healthy and the other unhealthy": {
			remoteServers: []*url.URL{
				{Host: "127.0.0.1:18080"},
				{Host: "127.0.0.1:18081"},
			},
			createReactor: []clienttesting.ReactionFunc{
				func(action clienttesting.Action) (bool, runtime.Object, error) {
					return true, lease, nil
				},
				func(action clienttesting.Action) (bool, runtime.Object, error) {
					return true, nil, noConnectionUpdateErr
				},
			},
			updateReactor: []clienttesting.ReactionFunc{
				func(action clienttesting.Action) (bool, runtime.Object, error) {
					return true, lease, nil
				},
				func(action clienttesting.Action) (bool, runtime.Object, error) {
					return true, nil, noConnectionUpdateErr
				},
			},
			getReactor: []clienttesting.ReactionFunc{
				func(action clienttesting.Action) (bool, runtime.Object, error) {
					return true, nil, apierrors.NewNotFound(gr, "not found")
				},
				func(action clienttesting.Action) (bool, runtime.Object, error) {
					return true, nil, apierrors.NewNotFound(gr, "not found")
				},
			},
			isHealthy:     []bool{true, false},
			serverHealthy: true,
		},
		"unhealthy two servers": {
			remoteServers: []*url.URL{
				{Host: "127.0.0.1:18080"},
				{Host: "127.0.0.1:18081"},
			},
			createReactor: []clienttesting.ReactionFunc{
				func(action clienttesting.Action) (bool, runtime.Object, error) {
					return true, nil, noConnectionUpdateErr
				},
				func(action clienttesting.Action) (bool, runtime.Object, error) {
					return true, nil, noConnectionUpdateErr
				},
			},
			updateReactor: []clienttesting.ReactionFunc{
				func(action clienttesting.Action) (bool, runtime.Object, error) {
					return true, nil, noConnectionUpdateErr
				},
				func(action clienttesting.Action) (bool, runtime.Object, error) {
					return true, nil, noConnectionUpdateErr
				},
			},
			getReactor: []clienttesting.ReactionFunc{
				func(action clienttesting.Action) (bool, runtime.Object, error) {
					return true, nil, apierrors.NewNotFound(gr, "not found")
				},
				func(action clienttesting.Action) (bool, runtime.Object, error) {
					return true, nil, apierrors.NewNotFound(gr, "not found")
				},
			},
			isHealthy:     []bool{false, false},
			serverHealthy: false,
		},
	}

	store, err := disk.NewDiskStorage(rootDir)
	if err != nil {
		t.Errorf("failed to create disk storage, %v", err)
	}

	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			stopCh := make(chan struct{})
			cfg := &config.YurtHubConfiguration{
				RemoteServers:             tt.remoteServers,
				StorageWrapper:            cachemanager.NewStorageWrapper(store),
				NodeName:                  node.Name,
				HeartbeatFailedRetry:      2,
				HeartbeatHealthyThreshold: 1,
				HeartbeatIntervalSeconds:  3,
				KubeletHealthGracePeriod:  40,
			}

			fakeClients := make(map[string]kubernetes.Interface)
			for i := range tt.remoteServers {
				cl := clientfake.NewSimpleClientset(node)
				cl.PrependReactor("create", "leases", tt.createReactor[i])
				cl.PrependReactor("update", "leases", tt.updateReactor[i])
				cl.PrependReactor("get", "leases", tt.getReactor[i])
				fakeClients[tt.remoteServers[i].String()] = cl
			}

			checker, _ := NewCloudAPIServerHealthChecker(cfg, fakeClients, stopCh)

			// wait for the probe completed
			time.Sleep(time.Duration(5*len(tt.remoteServers)) * time.Second)

			for i := range tt.remoteServers {
				if checker.BackendHealthyStatus(tt.remoteServers[i]) != tt.isHealthy[i] {
					t.Errorf("expect server %s healthy status %v, but got %v", tt.remoteServers[i].String(), tt.isHealthy[i], checker.BackendHealthyStatus(tt.remoteServers[i]))
				}
			}
			if checker.IsHealthy() != tt.serverHealthy {
				t.Errorf("expect all servers healthy status %v, but got %v", tt.serverHealthy, checker.IsHealthy())
			}

			close(stopCh)
		})
	}

	if err := os.RemoveAll(rootDir); err != nil {
		t.Errorf("Got error %v, unable to remove path %s", err, rootDir)
	}
}
