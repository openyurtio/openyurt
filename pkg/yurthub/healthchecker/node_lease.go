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

package healthchecker

import (
	"context"
	"fmt"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	coordclientset "k8s.io/client-go/kubernetes/typed/coordination/v1"
	"k8s.io/klog/v2"
	"k8s.io/utils/clock"
	"k8s.io/utils/pointer"
)

const (
	maxBackoff = 1 * time.Second
)

type NodeLease interface {
	Update(base *coordinationv1.Lease) (*coordinationv1.Lease, error)
}

type nodeLeaseImpl struct {
	client               clientset.Interface
	leaseClient          coordclientset.LeaseInterface
	holderIdentity       string
	leaseDurationSeconds int32
	failedRetry          int
	clock                clock.Clock
}

func NewNodeLease(client clientset.Interface, holderIdentity string, leaseDurationSeconds int32, failedRetry int) NodeLease {
	return &nodeLeaseImpl{
		client:               client,
		leaseClient:          client.CoordinationV1().Leases(corev1.NamespaceNodeLease),
		holderIdentity:       holderIdentity,
		failedRetry:          failedRetry,
		leaseDurationSeconds: leaseDurationSeconds,
		clock:                clock.RealClock{},
	}
}

func (nl *nodeLeaseImpl) Update(base *coordinationv1.Lease) (*coordinationv1.Lease, error) {
	if base != nil {
		lease, err := nl.retryUpdateLease(base)
		if err == nil {
			return lease, nil
		}
	}
	lease, created, err := nl.backoffEnsureLease()
	if err != nil {
		return nil, err
	}
	if !created {
		return nl.retryUpdateLease(lease)
	}
	return lease, nil
}

func (nl *nodeLeaseImpl) retryUpdateLease(base *coordinationv1.Lease) (*coordinationv1.Lease, error) {
	var err error
	var lease *coordinationv1.Lease
	for i := 0; i < nl.failedRetry; i++ {
		lease, err = nl.leaseClient.Update(context.Background(), nl.newLease(base), metav1.UpdateOptions{})
		if err == nil {
			return lease, nil
		}
		if apierrors.IsConflict(err) {
			base, _, err = nl.backoffEnsureLease()
			if err != nil {
				return nil, err
			}
			continue
		}
		klog.V(3).Infof("update node lease fail: %v, will try it.", err)
	}
	return nil, err
}

func (nl *nodeLeaseImpl) backoffEnsureLease() (*coordinationv1.Lease, bool, error) {
	var (
		lease   *coordinationv1.Lease
		created bool
		err     error
	)

	sleep := 100 * time.Millisecond
	for {
		lease, created, err = nl.ensureLease()
		if err == nil {
			break
		}
		sleep = sleep * 2
		if sleep > maxBackoff {
			return nil, false, fmt.Errorf("backoff ensure lease error: %w", err)
		}
		nl.clock.Sleep(sleep)
	}
	return lease, created, err
}

func (nl *nodeLeaseImpl) ensureLease() (*coordinationv1.Lease, bool, error) {
	lease, err := nl.leaseClient.Get(context.Background(), nl.holderIdentity, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		lease, err := nl.leaseClient.Create(context.Background(), nl.newLease(nil), metav1.CreateOptions{})
		if err != nil {
			return nil, false, err
		}
		return lease, true, nil
	} else if err != nil {
		return nil, false, err
	}
	return lease, false, nil
}

func (nl *nodeLeaseImpl) newLease(base *coordinationv1.Lease) *coordinationv1.Lease {
	var lease *coordinationv1.Lease
	if base == nil {
		lease = &coordinationv1.Lease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      nl.holderIdentity,
				Namespace: corev1.NamespaceNodeLease,
			},
			Spec: coordinationv1.LeaseSpec{
				HolderIdentity:       pointer.StringPtr(nl.holderIdentity),
				LeaseDurationSeconds: pointer.Int32Ptr(nl.leaseDurationSeconds),
			},
		}
	} else {
		lease = base.DeepCopy()
	}

	lease.Spec.RenewTime = &metav1.MicroTime{Time: nl.clock.Now()}
	if lease.OwnerReferences == nil || len(lease.OwnerReferences) == 0 {
		if node, err := nl.client.CoreV1().Nodes().Get(context.Background(), nl.holderIdentity, metav1.GetOptions{}); err == nil {
			lease.OwnerReferences = []metav1.OwnerReference{
				{
					APIVersion: corev1.SchemeGroupVersion.WithKind("Node").Version,
					Kind:       corev1.SchemeGroupVersion.WithKind("Node").Kind,
					Name:       nl.holderIdentity,
					UID:        node.UID,
				},
			}
		} else {
			klog.Errorf("could not get node %q when trying to set owner ref to the node lease: %v", nl.holderIdentity, err)
		}
	}
	return lease
}
