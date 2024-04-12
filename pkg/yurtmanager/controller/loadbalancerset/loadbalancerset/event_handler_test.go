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

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/workqueue"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openyurtio/openyurt/cmd/yurt-manager/names"
	"github.com/openyurtio/openyurt/pkg/apis/network"
	"github.com/openyurtio/openyurt/pkg/apis/network/v1alpha1"
)

func TestPoolServiceEventHandler(t *testing.T) {
	f := NewPoolServiceEventHandler()
	t.Run("create pool service", func(t *testing.T) {
		ps := newPoolServiceWithServiceNameAndNodepoolName(mockServiceName, "np123")
		q := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "pool_services")

		f.Create(event.CreateEvent{
			Object: ps,
		}, q)

		assertAndDoneQueue(t, q, []string{v1.NamespaceDefault + "/" + mockServiceName})
	})

	t.Run("create pool service not service name", func(t *testing.T) {
		ps := newPoolServiceWithServiceNameAndNodepoolName(mockServiceName, "np123")
		delete(ps.Labels, network.LabelServiceName)
		q := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "pool_services")

		f.Create(event.CreateEvent{
			Object: ps,
		}, q)
		assertAndDoneQueue(t, q, []string{})
	})

	t.Run("update service name with pool service", func(t *testing.T) {
		oldPs := newPoolServiceWithServiceNameAndNodepoolName("mock1", "np123")
		newPs := newPoolServiceWithServiceNameAndNodepoolName("mock2", "np123")

		q := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "pool_services")

		f.Update(event.UpdateEvent{ObjectOld: oldPs, ObjectNew: newPs}, q)

		assertAndDoneQueue(t, q, []string{
			v1.NamespaceDefault + "/" + "mock1",
			v1.NamespaceDefault + "/" + "mock2",
		})
	})

	t.Run("delete service name with pool service", func(t *testing.T) {
		oldPs := newPoolServiceWithServiceNameAndNodepoolName("mock1", "np123")
		newPs := newPoolServiceWithServiceNameAndNodepoolName("mock1", "np123")
		delete(newPs.Labels, network.LabelServiceName)

		q := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "pool_services")

		f.Update(event.UpdateEvent{ObjectOld: oldPs, ObjectNew: newPs}, q)

		assertAndDoneQueue(t, q, []string{v1.NamespaceDefault + "/" + "mock1"})
	})

	t.Run("update pool service annotations", func(t *testing.T) {
		oldPs := newPoolServiceWithServiceNameAndNodepoolName(mockServiceName, "np123")
		newPs := newPoolServiceWithServiceNameAndNodepoolName(mockServiceName, "np123")
		newPs.Annotations = map[string]string{"test": "app"}

		q := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "pool_services")

		f.Update(event.UpdateEvent{
			ObjectOld: oldPs,
			ObjectNew: newPs,
		}, q)

		assertAndDoneQueue(t, q, []string{v1.NamespaceDefault + "/" + mockServiceName})
	})

}

func newPoolServiceWithServiceNameAndNodepoolName(serviceName string, poolName string) *v1alpha1.PoolService {
	return &v1alpha1.PoolService{
		TypeMeta: v1.TypeMeta{
			Kind:       "PoolService",
			APIVersion: v1alpha1.GroupVersion.String(),
		},
		ObjectMeta: v1.ObjectMeta{
			Namespace: v1.NamespaceDefault,
			Name:      serviceName + "-" + poolName,
			Labels:    map[string]string{network.LabelServiceName: serviceName, network.LabelNodePoolName: poolName, labelManageBy: names.LoadBalancerSetController},
		},
		Spec: v1alpha1.PoolServiceSpec{
			LoadBalancerClass: &elbClass,
		},
	}
}

func assertAndDoneQueue(t testing.TB, q workqueue.Interface, expectedItemNames []string) {
	t.Helper()

	if q.Len() != len(expectedItemNames) {
		t.Errorf("expected queue %d, but got %d", len(expectedItemNames), q.Len())
		return
	}

	for _, expectedItem := range expectedItemNames {
		gotItem, _ := q.Get()
		r, ok := gotItem.(reconcile.Request)

		if !ok {
			t.Errorf("expected item is reconcile request, but not")
		}

		if r.String() != expectedItem {
			t.Errorf("expected request is %s, but got %s", expectedItem, r.String())
		}
		q.Done(gotItem)
	}
}

func TestNodePoolEventHandler(t *testing.T) {
	scheme := initScheme(t)

	t.Run("create/update nodepool enqueue all multi lb service", func(t *testing.T) {
		np := newNodepool("np123", "name=np123")

		svc1 := newService(v1.NamespaceDefault, mockServiceName)
		svc2 := newService(v1.NamespaceDefault, "mock")
		svc2.Spec.Type = corev1.ServiceTypeClusterIP

		c := fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(svc1).WithObjects(svc2).Build()
		f := NewNodePoolEventHandler(c)
		q := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "pool_services")

		f.Create(event.CreateEvent{Object: np}, q)
		assertAndDoneQueue(t, q, []string{v1.NamespaceDefault + "/" + mockServiceName})

		f.Update(event.UpdateEvent{ObjectOld: np, ObjectNew: np}, q)
		assertAndDoneQueue(t, q, []string{v1.NamespaceDefault + "/" + mockServiceName})
	})

	t.Run("delete/generic need enqueue", func(t *testing.T) {
		ps1 := newPoolServiceWithServiceNameAndNodepoolName("mock1", "np123")
		ps2 := newPoolServiceWithServiceNameAndNodepoolName("mock2", "np123")
		ps3 := newPoolServiceWithServiceNameAndNodepoolName("mock3", "np123")

		c := fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(ps1).WithObjects(ps2).WithObjects(ps3).Build()

		f := NewNodePoolEventHandler(c)

		np := newNodepool("np123", "name=np123,app=deploy")
		q := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "pool_services")

		f.Delete(event.DeleteEvent{Object: np}, q)
		assertAndDoneQueue(t, q, []string{
			v1.NamespaceDefault + "/" + "mock1",
			v1.NamespaceDefault + "/" + "mock2",
			v1.NamespaceDefault + "/" + "mock3",
		})

		f.Generic(event.GenericEvent{Object: np}, q)
		assertAndDoneQueue(t, q, []string{
			v1.NamespaceDefault + "/" + "mock1",
			v1.NamespaceDefault + "/" + "mock2",
			v1.NamespaceDefault + "/" + "mock3",
		})
	})

	t.Run("update/delete/generic not enqueue", func(t *testing.T) {
		ps := newPoolServiceWithServiceNameAndNodepoolName(mockServiceName, "np123")
		c := fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(ps).Build()

		f := NewNodePoolEventHandler(c)

		np := newNodepool("np234", "name=np234,app=deploy")
		q := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "pool_services")

		f.Create(event.CreateEvent{Object: np}, q)
		assertAndDoneQueue(t, q, []string{})

		f.Update(event.UpdateEvent{ObjectOld: np, ObjectNew: np}, q)
		assertAndDoneQueue(t, q, []string{})

		f.Delete(event.DeleteEvent{Object: np}, q)
		assertAndDoneQueue(t, q, []string{})

		f.Generic(event.GenericEvent{Object: np}, q)
		assertAndDoneQueue(t, q, []string{})
	})

}
