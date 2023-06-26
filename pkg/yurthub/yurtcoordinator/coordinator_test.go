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

package yurtcoordinator

import (
	"context"
	"sync"
	"testing"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/utils/pointer"
)

var leaseGVR = schema.GroupVersionResource{
	Group:    coordinationv1.SchemeGroupVersion.Group,
	Version:  coordinationv1.SchemeGroupVersion.Version,
	Resource: "leases",
}

func TestInformerSyncLeaseAddedAndUpdated(t *testing.T) {
	var isPoolCacheSynced bool
	var mtx sync.Mutex
	var poolCacheSyncLease *coordinationv1.Lease = &coordinationv1.Lease{
		ObjectMeta: v1.ObjectMeta{
			Name:      nameInformerLease,
			Namespace: namespaceInformerLease,
		},
		Spec: coordinationv1.LeaseSpec{},
	}

	cases := []struct {
		Description         string
		LeaseUpdateInterval time.Duration
		StaleTimeout        time.Duration
		LeaseUpdateTimes    int
		PollInterval        time.Duration
		Expect              bool
	}{
		{
			Description:         "should set isPoolCacheSynced as true if lease is updated before timeout",
			LeaseUpdateInterval: 100 * time.Millisecond,
			StaleTimeout:        2 * time.Second,
			LeaseUpdateTimes:    10,
			PollInterval:        50 * time.Millisecond,
			Expect:              true,
		},
		{
			Description:         "should set isPoolCacheSynced as false is lease is not updated until timeout",
			LeaseUpdateInterval: 100 * time.Millisecond,
			StaleTimeout:        2 * time.Second,
			LeaseUpdateTimes:    1,
			PollInterval:        4 * time.Second,
			Expect:              false,
		},
	}

	for _, c := range cases {
		t.Run(c.Description, func(t *testing.T) {
			ctx := context.Background()
			fakeClient := fake.NewSimpleClientset()
			exited := false

			poolCacheSyncedDetector := &poolCacheSyncedDetector{
				ctx:            ctx,
				updateNotifyCh: make(chan struct{}),
				syncLeaseManager: &coordinatorLeaseInformerManager{
					ctx:               ctx,
					coordinatorClient: fakeClient,
				},
				staleTimeout: c.StaleTimeout,
				isPoolCacheSyncSetter: func(value bool) {
					mtx.Lock()
					defer mtx.Unlock()
					isPoolCacheSynced = value
				},
			}

			poolCacheSyncedDetector.EnsureStart()
			defer poolCacheSyncedDetector.EnsureStop()

			go func() {
				initLease := poolCacheSyncLease.DeepCopy()
				initLease.Spec.RenewTime = &v1.MicroTime{
					Time: time.Now(),
				}
				if err := fakeClient.Tracker().Add(initLease); err != nil {
					t.Errorf("failed to add lease at case %s, %v", c.Description, err)
				}
				for i := 0; i < c.LeaseUpdateTimes; i++ {
					time.Sleep(c.LeaseUpdateInterval)
					newLease := poolCacheSyncLease.DeepCopy()
					newLease.Spec.RenewTime = &v1.MicroTime{
						Time: time.Now(),
					}
					if err := fakeClient.Tracker().Update(leaseGVR, newLease, namespaceInformerLease); err != nil {
						t.Errorf("failed to update lease at case %s, %v", c.Description, err)
					}
				}
				exited = true
			}()

			ticker := time.NewTicker(c.PollInterval)
			defer ticker.Stop()
			for {
				<-ticker.C
				if isPoolCacheSynced != c.Expect {
					t.Errorf("unexpected value at case: %s, want: %v, got: %v", c.Description, c.Expect, isPoolCacheSynced)
				}
				if exited {
					return
				}
			}
		})
	}
}

func TestInformerSyncLeaseDelete(t *testing.T) {
	t.Run("should set isPoolCacheSynced as false if the lease is deleted", func(t *testing.T) {
		var isPoolCacheSynced bool
		var mtx sync.Mutex
		var poolCacheSyncLease *coordinationv1.Lease = &coordinationv1.Lease{
			ObjectMeta: v1.ObjectMeta{
				Name:      nameInformerLease,
				Namespace: namespaceInformerLease,
			},
			Spec: coordinationv1.LeaseSpec{
				RenewTime: &v1.MicroTime{
					Time: time.Now(),
				},
			},
		}
		ctx := context.Background()
		fakeClient := fake.NewSimpleClientset(poolCacheSyncLease)
		poolCacheSyncedDetector := &poolCacheSyncedDetector{
			ctx:            ctx,
			updateNotifyCh: make(chan struct{}),
			syncLeaseManager: &coordinatorLeaseInformerManager{
				ctx:               ctx,
				coordinatorClient: fakeClient,
			},
			staleTimeout: 100 * time.Second,
			isPoolCacheSyncSetter: func(value bool) {
				mtx.Lock()
				defer mtx.Unlock()
				isPoolCacheSynced = value
			},
		}

		poolCacheSyncedDetector.EnsureStart()
		defer poolCacheSyncedDetector.EnsureStop()

		err := wait.PollUntil(50*time.Millisecond, func() (done bool, err error) {
			if isPoolCacheSynced {
				return true, nil
			}
			return false, nil
		}, ctx.Done())
		if err != nil {
			t.Errorf("failed to wait isPoolCacheSynced to be initialized as true")
		}

		if err := fakeClient.Tracker().Delete(leaseGVR, namespaceInformerLease, nameInformerLease); err != nil {
			t.Errorf("failed to delete lease, %v", err)
		}

		err = wait.PollUntil(50*time.Millisecond, func() (done bool, err error) {
			if isPoolCacheSynced {
				return false, nil
			}
			return true, nil
		}, ctx.Done())
		if err != nil {
			t.Errorf("unexpect err when waitting isPoolCacheSynced to be false, %v", err)
		}
	})
}

func TestIfInformerSyncLease(t *testing.T) {
	cases := []struct {
		Description string
		Lease       *coordinationv1.Lease
		Expect      bool
	}{
		{
			Description: "return true if it is informer sync lease",
			Lease: &coordinationv1.Lease{
				ObjectMeta: v1.ObjectMeta{
					Name:      nameInformerLease,
					Namespace: namespaceInformerLease,
				},
				Spec: coordinationv1.LeaseSpec{
					HolderIdentity: pointer.StringPtr("leader-yurthub"),
				},
			},
			Expect: true,
		},
		{
			Description: "return false if it is not informer sync lease",
			Lease: &coordinationv1.Lease{
				ObjectMeta: v1.ObjectMeta{
					Name:      "other-lease",
					Namespace: "kube-system",
				},
				Spec: coordinationv1.LeaseSpec{
					HolderIdentity: pointer.StringPtr("other-lease"),
				},
			},
			Expect: false,
		},
	}

	for _, c := range cases {
		t.Run(c.Description, func(t *testing.T) {
			got := ifInformerSyncLease(c.Lease)
			if got != c.Expect {
				t.Errorf("unexpected value for %s, want: %v, got: %v", c.Description, c.Expect, got)
			}
		})
	}
}
