/*
Copyright 2024 The OpenYurt Authors.

Licensed under the Apache License, Version 2.0 (the License);
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an AS IS BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package viploadbalancer_test

import (
	"testing"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"

	vip "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/loadbalancerset/viploadbalancer"
)

var (
	elbClass = "elb"
)

func TestPoolServicePredicated(t *testing.T) {
	f := vip.NewPoolServicePredicated()

	t.Run("create/delete/update/generic pool service has not LoadBalancerClass", func(t *testing.T) {
		ps := newPoolService(v1.NamespaceDefault, "np123", nil, nil)
		ps.Spec.LoadBalancerClass = nil

		assertBool(t, false, f.Create(event.CreateEvent{Object: ps}))
		assertBool(t, false, f.Update(event.UpdateEvent{ObjectOld: ps, ObjectNew: ps}))
		assertBool(t, false, f.Delete(event.DeleteEvent{Object: ps}))
		assertBool(t, false, f.Generic(event.GenericEvent{Object: ps}))
	})

	t.Run("create/delete/update/generic pool service LoadBalancerClass is not valid", func(t *testing.T) {
		ps := newPoolService(v1.NamespaceDefault, "np123", nil, nil)
		ps.Spec.LoadBalancerClass = &elbClass

		assertBool(t, false, f.Create(event.CreateEvent{Object: ps}))
		assertBool(t, false, f.Update(event.UpdateEvent{ObjectOld: ps, ObjectNew: ps}))
		assertBool(t, false, f.Delete(event.DeleteEvent{Object: ps}))
		assertBool(t, false, f.Generic(event.GenericEvent{Object: ps}))
	})

	t.Run("create/delete/update/generic pool service", func(t *testing.T) {
		ps := newPoolService(v1.NamespaceDefault, "np123", nil, nil)

		assertBool(t, true, f.Create(event.CreateEvent{Object: ps}))
		assertBool(t, true, f.Update(event.UpdateEvent{ObjectOld: ps, ObjectNew: ps}))
		assertBool(t, true, f.Delete(event.DeleteEvent{Object: ps}))
		assertBool(t, true, f.Generic(event.GenericEvent{Object: ps}))

	})
}

func assertBool(t testing.TB, expected, got bool) {
	t.Helper()

	if expected != got {
		t.Errorf("expected %v, but got %v", expected, got)
	}
}
