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

package loadbalancerset

import (
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/openyurtio/openyurt/pkg/apis/network"
)

const (
	mockAnnotationLbId = aggregateKeyPrefix + "lb-id"
)

func TestServicePredicate(t *testing.T) {
	f := NewServicePredicated()
	t.Run("create/delete/generic service enqueue", func(t *testing.T) {
		svc := newService(v1.NamespaceDefault, mockServiceName)

		assertBool(t, true, f.Create(event.CreateEvent{Object: svc}))
		assertBool(t, true, f.Delete(event.DeleteEvent{Object: svc}))
		assertBool(t, true, f.Generic(event.GenericEvent{Object: svc}))

	})

	t.Run("create/delete not lb type service not enqueue", func(t *testing.T) {
		svc := newService(v1.NamespaceDefault, mockServiceName)
		svc.Spec.Type = v1.ServiceTypeClusterIP

		assertBool(t, false, f.Create(event.CreateEvent{Object: svc}))
		assertBool(t, false, f.Delete(event.DeleteEvent{Object: svc}))
		assertBool(t, false, f.Generic(event.GenericEvent{Object: svc}))
	})

	t.Run("create/delete not service class annotations service not enqueue", func(t *testing.T) {
		svc := newService(v1.NamespaceDefault, mockServiceName)
		svc.Spec.LoadBalancerClass = nil

		assertBool(t, false, f.Create(event.CreateEvent{Object: svc}))
		assertBool(t, false, f.Delete(event.DeleteEvent{Object: svc}))
		assertBool(t, false, f.Generic(event.GenericEvent{Object: svc}))
	})

	t.Run("create not nodepool selector annotations not enqueue", func(t *testing.T) {
		svc := newService(v1.NamespaceDefault, mockServiceName)
		delete(svc.Annotations, network.AnnotationNodePoolSelector)

		assertBool(t, false, f.Create(event.CreateEvent{Object: svc}))
		assertBool(t, false, f.Delete(event.DeleteEvent{Object: svc}))
		assertBool(t, false, f.Generic(event.GenericEvent{Object: svc}))
	})

	t.Run("update service not enqueue", func(t *testing.T) {
		svc1 := newService(v1.NamespaceDefault, mockServiceName)
		svc2 := newService(v1.NamespaceDefault, mockServiceName)
		svc2.Spec.ClusterIP = "1.2.3.4"

		got := f.Update(event.UpdateEvent{ObjectOld: svc1, ObjectNew: svc2})
		assertBool(t, false, got)
	})

	t.Run("delete selector annotations", func(t *testing.T) {
		svc1 := newService(v1.NamespaceDefault, mockServiceName)
		svc2 := newService(v1.NamespaceDefault, mockServiceName)
		delete(svc2.Annotations, network.AnnotationNodePoolSelector)

		got := f.Update(event.UpdateEvent{ObjectOld: svc1, ObjectNew: svc2})
		assertBool(t, true, got)
	})

	t.Run("modify selector annotations", func(t *testing.T) {
		svc1 := newService(v1.NamespaceDefault, mockServiceName)
		svc2 := newService(v1.NamespaceDefault, mockServiceName)
		svc2.Annotations[network.AnnotationNodePoolSelector] = "app=online"

		got := f.Update(event.UpdateEvent{ObjectOld: svc1, ObjectNew: svc2})
		assertBool(t, true, got)
	})

	t.Run("add selector annotations", func(t *testing.T) {
		svc1 := newService(v1.NamespaceDefault, mockServiceName)
		delete(svc1.Annotations, network.AnnotationNodePoolSelector)
		svc2 := newService(v1.NamespaceDefault, mockServiceName)

		got := f.Update(event.UpdateEvent{ObjectOld: svc1, ObjectNew: svc2})
		assertBool(t, true, got)
	})

	t.Run("modify service type", func(t *testing.T) {
		svc1 := newService(v1.NamespaceDefault, mockServiceName)
		svc2 := newService(v1.NamespaceDefault, mockServiceName)
		svc2.Spec.Type = v1.ServiceTypeClusterIP

		got := f.Update(event.UpdateEvent{ObjectOld: svc1, ObjectNew: svc2})
		assertBool(t, true, got)
	})

	t.Run("modify finalizer", func(t *testing.T) {
		svc1 := newService(v1.NamespaceDefault, mockServiceName)
		svc1.Finalizers = []string{poolServiceFinalizer}
		svc2 := newService(v1.NamespaceDefault, mockServiceName)
		svc2.Finalizers = []string{"test-finalizer"}

		got := f.Update(event.UpdateEvent{ObjectOld: svc1, ObjectNew: svc2})
		assertBool(t, true, got)
	})

	t.Run("modify aggregate annotations", func(t *testing.T) {
		svc1 := newService(v1.NamespaceDefault, mockServiceName)
		svc1.Annotations[mockAnnotationLbId] = "lb123"
		svc2 := newService(v1.NamespaceDefault, mockServiceName)
		svc2.Annotations[mockAnnotationLbId] = "lb123,lb234"

		got := f.Update(event.UpdateEvent{ObjectOld: svc1, ObjectNew: svc2})
		assertBool(t, true, got)
	})

	t.Run("modify not aggregate annotations", func(t *testing.T) {
		svc1 := newService(v1.NamespaceDefault, mockServiceName)
		svc1.Annotations["key1"] = "value1"
		svc2 := newService(v1.NamespaceDefault, mockServiceName)
		svc2.Annotations["key2"] = "value2"

		got := f.Update(event.UpdateEvent{ObjectOld: svc1, ObjectNew: svc2})
		assertBool(t, false, got)
	})

	t.Run("modify finalizer with not multi lb services", func(t *testing.T) {
		svc1 := newService(v1.NamespaceDefault, mockServiceName)
		delete(svc1.Annotations, network.AnnotationNodePoolSelector)
		svc2 := newService(v1.NamespaceDefault, mockServiceName)
		svc2.Finalizers = []string{"test-finalizer"}
		delete(svc2.Annotations, network.AnnotationNodePoolSelector)

		got := f.Update(event.UpdateEvent{ObjectOld: svc1, ObjectNew: svc2})
		assertBool(t, false, got)
	})

	t.Run("update delete time", func(t *testing.T) {
		svc1 := newService(v1.NamespaceDefault, mockServiceName)
		svc2 := newService(v1.NamespaceDefault, mockServiceName)
		svc2.DeletionTimestamp = &metav1.Time{Time: time.Now()}

		got := f.Update(event.UpdateEvent{ObjectOld: svc1, ObjectNew: svc2})
		assertBool(t, true, got)
	})

	t.Run("modify service status", func(t *testing.T) {
		svc1 := newService(v1.NamespaceDefault, mockServiceName)
		svc2 := newService(v1.NamespaceDefault, mockServiceName)
		svc2.Status.LoadBalancer.Ingress = []v1.LoadBalancerIngress{{IP: "1.2.3.4"}}

		got := f.Update(event.UpdateEvent{ObjectOld: svc1, ObjectNew: svc2})
		assertBool(t, true, got)
	})
}

func assertBool(t testing.TB, expected, got bool) {
	t.Helper()

	if expected != got {
		t.Errorf("expected %v, but got %v", expected, got)
	}
}

func TestPoolServicePredicated(t *testing.T) {
	f := NewPoolServicePredicated()

	t.Run("create/delete/update/generic pool service not managed and service name", func(t *testing.T) {
		ps := newPoolService(v1.NamespaceDefault, "np123", nil, nil, nil)
		delete(ps.Labels, labelManageBy)
		delete(ps.Labels, network.LabelServiceName)

		assertBool(t, false, f.Create(event.CreateEvent{Object: ps}))
		assertBool(t, false, f.Update(event.UpdateEvent{ObjectOld: ps, ObjectNew: ps}))
		assertBool(t, false, f.Delete(event.DeleteEvent{Object: ps}))
		assertBool(t, false, f.Generic(event.GenericEvent{Object: ps}))
	})

	t.Run("create/delete/update/generic pool service not managed", func(t *testing.T) {
		ps := newPoolService(v1.NamespaceDefault, "np123", nil, nil, nil)
		delete(ps.Labels, labelManageBy)

		assertBool(t, false, f.Create(event.CreateEvent{Object: ps}))
		assertBool(t, false, f.Update(event.UpdateEvent{ObjectOld: ps, ObjectNew: ps}))
		assertBool(t, false, f.Delete(event.DeleteEvent{Object: ps}))
		assertBool(t, false, f.Generic(event.GenericEvent{Object: ps}))
	})

	t.Run("create/delete/update/generic pool service not service name", func(t *testing.T) {
		ps := newPoolService(v1.NamespaceDefault, "np123", nil, nil, nil)
		delete(ps.Labels, network.LabelServiceName)

		assertBool(t, false, f.Create(event.CreateEvent{Object: ps}))
		assertBool(t, false, f.Update(event.UpdateEvent{ObjectOld: ps, ObjectNew: ps}))
		assertBool(t, false, f.Delete(event.DeleteEvent{Object: ps}))
		assertBool(t, false, f.Generic(event.GenericEvent{Object: ps}))

	})

	t.Run("create/delete/update/generic pool service", func(t *testing.T) {
		ps := newPoolService(v1.NamespaceDefault, "np123", nil, nil, nil)

		assertBool(t, true, f.Create(event.CreateEvent{Object: ps}))
		assertBool(t, true, f.Update(event.UpdateEvent{ObjectOld: ps, ObjectNew: ps}))
		assertBool(t, true, f.Delete(event.DeleteEvent{Object: ps}))
		assertBool(t, true, f.Generic(event.GenericEvent{Object: ps}))

	})

	t.Run("create/delete/update/generic pool service not service name", func(t *testing.T) {
		ps1 := newPoolService(v1.NamespaceDefault, "np123", nil, nil, nil)
		ps2 := newPoolService(v1.NamespaceDefault, "np123", nil, nil, nil)
		delete(ps2.Labels, network.LabelServiceName)
		delete(ps2.Labels, labelManageBy)

		assertBool(t, true, f.Update(event.UpdateEvent{ObjectOld: ps1, ObjectNew: ps2}))
	})
}

func TestNodePoolPredicated(t *testing.T) {
	f := NewNodePoolPredicated()

	t.Run("create/delete/generic nodepool predicated", func(t *testing.T) {
		np := newNodepool("np123", "name=np123")
		assertBool(t, true, f.Create(event.CreateEvent{Object: np}))
		assertBool(t, true, f.Delete(event.DeleteEvent{Object: np}))
		assertBool(t, true, f.Generic(event.GenericEvent{Object: np}))
	})

	t.Run("create/delete/generic nodepool not predicated", func(t *testing.T) {
		np := newNodepool("np123", "")
		assertBool(t, false, f.Create(event.CreateEvent{Object: np}))
		assertBool(t, false, f.Delete(event.DeleteEvent{Object: np}))
		assertBool(t, false, f.Generic(event.GenericEvent{Object: np}))
	})

	t.Run("update nodepool label predicated", func(t *testing.T) {
		np1 := newNodepool("np123", "name=np124")
		np2 := newNodepool("np123", "name=np124")
		np2.Labels["app"] = "deploy"
		assertBool(t, true, f.Update(event.UpdateEvent{ObjectOld: np1, ObjectNew: np2}))
	})

	t.Run("update nodepool not predicated", func(t *testing.T) {
		np1 := newNodepool("np123", "name=np124")
		np2 := newNodepool("np123", "name=np124")
		np2.Spec.Type = "Cloud"

		assertBool(t, false, f.Update(event.UpdateEvent{ObjectOld: np1, ObjectNew: np2}))
	})

}
