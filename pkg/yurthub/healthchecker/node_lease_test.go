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
	"testing"

	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

func TestNodeLeaseManager_Update(t *testing.T) {
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

	gr := schema.GroupResource{Group: "v1", Resource: "leases"}
	noConnectionUpdateErr := apierrors.NewServerTimeout(gr, "put", 1)

	cases := []struct {
		desc          string
		updateReactor func(action clienttesting.Action) (bool, runtime.Object, error)
		getReactor    func(action clienttesting.Action) (bool, runtime.Object, error)
		success       bool
	}{
		{
			desc: "update success",
			updateReactor: func(action clienttesting.Action) (bool, runtime.Object, error) {
				return true, lease, nil
			},
			getReactor: func(action clienttesting.Action) (bool, runtime.Object, error) {
				return true, lease, nil
			},
			success: true,
		},
		{
			desc: "update fail",
			updateReactor: func(action clienttesting.Action) (bool, runtime.Object, error) {
				return true, nil, noConnectionUpdateErr
			},
			getReactor: func(action clienttesting.Action) (bool, runtime.Object, error) {
				return true, nil, noConnectionUpdateErr
			},
			success: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {

			cl := fake.NewSimpleClientset(node)
			cl.PrependReactor("update", "leases", tc.updateReactor)
			cl.PrependReactor("get", "leases", tc.getReactor)
			cl.PrependReactor("create", "leases", tc.updateReactor)
			nl := NewNodeLease(cl, "foo", 40, 3)
			if _, err := nl.Update(nil); tc.success != (err == nil) {
				t.Fatalf("got success %v, expected %v", err == nil, tc.success)
			}
		})
	}

}
