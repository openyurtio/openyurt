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
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openyurtio/openyurt/cmd/yurt-manager/names"
	"github.com/openyurtio/openyurt/pkg/apis"
	"github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
	"github.com/openyurtio/openyurt/pkg/apis/network"
	"github.com/openyurtio/openyurt/pkg/apis/network/v1alpha1"
)

const (
	mockServiceName   = "test"
	mockNodePoolLabel = "app=deploy"
	mockServiceUid    = "c0af506a-7096-4ef9-b39a-eac2feb5c07g"
	mockNodePoolUid   = "f47dd9db-d3bc-40f3-8d03-7409930b6289"
)

var (
	elbClass = "elb"
)

func TestReconcilePoolService_Reconcile(t *testing.T) {
	scheme := initScheme(t)

	t.Run("test get service not found", func(t *testing.T) {
		rc := ReconcileLoadBalancerSet{
			Client: fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects().Build(),
		}

		_, err := rc.Reconcile(context.Background(), newReconcileRequest(v1.NamespaceDefault, mockServiceName))

		assertErrNil(t, err)
	})

	t.Run("test create pool services", func(t *testing.T) {
		svc := newService(v1.NamespaceDefault, mockServiceName)
		np1 := newNodepool("np123", "name=np123,app=deploy")
		np2 := newNodepool("np234", "name=np234,app=deploy")
		np3 := newNodepool("np345", "name=np345")
		c := fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(svc).WithObjects(np1).WithObjects(np3).WithObjects(np2).Build()
		rc := ReconcileLoadBalancerSet{
			Client: c,
		}

		_, err := rc.Reconcile(context.Background(), newReconcileRequest(v1.NamespaceDefault, mockServiceName))

		psl := &v1alpha1.PoolServiceList{}
		c.List(context.Background(), psl)

		assertErrNil(t, err)
		assertPoolServicesNameList(t, psl, []string{"test-np123", "test-np234"})
		assertPoolServicesLBClass(t, psl, svc.Spec.LoadBalancerClass)
		assertPoolServiceLabels(t, psl, svc.Name)
	})

	t.Run("test nodepool selector is nil", func(t *testing.T) {
		svc := newService(v1.NamespaceDefault, mockServiceName)
		svc.Annotations[network.AnnotationNodePoolSelector] = ""

		np1 := newNodepool("np123", "name=np123,app=deploy")
		c := fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(svc).WithObjects(np1).Build()
		rc := ReconcileLoadBalancerSet{
			Client: c,
		}

		rc.Reconcile(context.Background(), newReconcileRequest(v1.NamespaceDefault, mockServiceName))

		psl := &v1alpha1.PoolServiceList{}
		c.List(context.Background(), psl)
		assertPoolServiceCounts(t, 0, len(psl.Items))
	})

	t.Run("test create multi pool service in kube-system namespace", func(t *testing.T) {
		svc := newService(v1.NamespaceSystem, mockServiceName)
		np1 := newNodepool("np123", "name=np123,app=deploy")
		np2 := newNodepool("np234", "name=np234,app=deploy")
		c := fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(svc).WithObjects(np1).WithObjects(np2).Build()
		rc := ReconcileLoadBalancerSet{
			Client: c,
		}

		rc.Reconcile(context.Background(), newReconcileRequest(v1.NamespaceSystem, mockServiceName))

		psl := &v1alpha1.PoolServiceList{}
		err := c.List(context.Background(), psl)

		assertErrNil(t, err)
		assertPoolServicesNameList(t, psl, []string{"test-np123", "test-np234"})
		assertPoolServicesLBClass(t, psl, svc.Spec.LoadBalancerClass)
	})

	t.Run("test delete pool services", func(t *testing.T) {
		svc := newService(v1.NamespaceDefault, mockServiceName)
		np1 := newNodepool("np123", "name=np123,app=deploy")
		np2 := newNodepool("np234", "name=np234")

		ps1 := newPoolService(v1.NamespaceDefault, "np123", nil, nil)
		ps2 := newPoolService(v1.NamespaceDefault, "np234", nil, nil)

		c := fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(svc).WithObjects(np1).WithObjects(np2).WithObjects(ps1).WithObjects(ps2).Build()

		rc := ReconcileLoadBalancerSet{
			Client: c,
		}

		_, err := rc.Reconcile(context.Background(), newReconcileRequest(v1.NamespaceDefault, mockServiceName))

		psl := &v1alpha1.PoolServiceList{}
		c.List(context.Background(), psl)

		assertErrNil(t, err)
		assertPoolServicesNameList(t, psl, []string{"test-np123"})
	})

	t.Run("test aggregate ingress status multi pool services", func(t *testing.T) {
		svc := newService(v1.NamespaceDefault, mockServiceName)

		np1 := newNodepool("np123", "name=np123,app=deploy")
		np2 := newNodepool("np234", "name=np234,app=deploy")

		ps1 := newPoolService(v1.NamespaceDefault, "np123", nil, []corev1.LoadBalancerIngress{{IP: "2.2.3.4"}})
		ps2 := newPoolService(v1.NamespaceDefault, "np234", nil, []corev1.LoadBalancerIngress{{IP: "1.2.3.4"}})
		ps3 := newPoolService(v1.NamespaceSystem, "np234", nil, []corev1.LoadBalancerIngress{{IP: "3.4.5.6"}})

		c := fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(svc).WithObjects(np1).WithObjects(np2).WithObjects(ps1).WithObjects(ps2).WithObjects(ps3).Build()

		rc := ReconcileLoadBalancerSet{
			Client: c,
		}

		_, err := rc.Reconcile(context.Background(), newReconcileRequest(v1.NamespaceDefault, mockServiceName))
		assertErrNil(t, err)

		newSvc := &corev1.Service{}
		c.Get(context.Background(), types.NamespacedName{
			Namespace: v1.NamespaceDefault,
			Name:      mockServiceName,
		}, newSvc)
		expectedStatus := []corev1.LoadBalancerIngress{{IP: "1.2.3.4"}, {IP: "2.2.3.4"}}

		assertLoadBalancerStatusEqual(t, expectedStatus, newSvc.Status.LoadBalancer.Ingress)
	})

	t.Run("test aggregate annotations multi pool services", func(t *testing.T) {
		svc := newService(v1.NamespaceDefault, mockServiceName)

		np1 := newNodepool("np123", "name=np123,app=deploy")
		np2 := newNodepool("np234", "name=np234,app=deploy")

		ps1 := newPoolService(v1.NamespaceDefault, "np123", map[string]string{network.AggregateAnnotationsKeyPrefix + "/lb-id": "lb34567"}, nil)
		ps2 := newPoolService(v1.NamespaceDefault, "np234", map[string]string{network.AggregateAnnotationsKeyPrefix + "/lb-id": "lb23456"}, nil)
		ps3 := newPoolService(v1.NamespaceDefault, "np345", map[string]string{network.AggregateAnnotationsKeyPrefix + "/lb-id": "lb12345"}, nil)
		ps4 := newPoolService(v1.NamespaceDefault, "np456", map[string]string{network.AggregateAnnotationsKeyPrefix + "/lb-id": "lb12345"}, nil)

		c := fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(svc).WithObjects(np1).WithObjects(np2).
			WithObjects(ps1).WithObjects(ps2).WithObjects(ps3).WithObjects(ps4).Build()

		rc := ReconcileLoadBalancerSet{
			Client: c,
		}

		_, err := rc.Reconcile(context.Background(), newReconcileRequest(v1.NamespaceDefault, mockServiceName))
		assertErrNil(t, err)

		newSvc := &corev1.Service{}
		c.Get(context.Background(), types.NamespacedName{
			Namespace: v1.NamespaceDefault,
			Name:      mockServiceName,
		}, newSvc)

		expectedAnnotations := map[string]string{network.AggregateAnnotationsKeyPrefix + "/lb-id": "lb23456,lb34567"}

		assertOpenYurtAnnotationsEqual(t, expectedAnnotations, newSvc.Annotations)
		assertResourceVersion(t, "1001", newSvc.ResourceVersion)
	})

	t.Run("test aggregate service not update", func(t *testing.T) {
		svc := newService(v1.NamespaceDefault, mockServiceName)

		np1 := newNodepool("np123", "name=np123,app=deploy")
		np2 := newNodepool("np234", "name=np234,app=deploy")

		ps1 := newPoolService(v1.NamespaceDefault, "np123", map[string]string{network.AggregateAnnotationsKeyPrefix + "/lb-id": "lb34567"}, nil)
		ps2 := newPoolService(v1.NamespaceDefault, "np234", map[string]string{network.AggregateAnnotationsKeyPrefix + "/lb-id": "lb23456"}, nil)
		svc.Annotations[network.AggregateAnnotationsKeyPrefix+"/lb-id"] = "lb23456,lb34567"

		c := fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(svc).WithObjects(np1).WithObjects(np2).
			WithObjects(ps1).WithObjects(ps2).Build()

		rc := ReconcileLoadBalancerSet{
			Client: c,
		}

		_, err := rc.Reconcile(context.Background(), newReconcileRequest(v1.NamespaceDefault, mockServiceName))
		assertErrNil(t, err)

		newSvc := &corev1.Service{}
		c.Get(context.Background(), types.NamespacedName{
			Namespace: v1.NamespaceDefault,
			Name:      mockServiceName,
		}, newSvc)

		expectedAnnotations := map[string]string{network.AggregateAnnotationsKeyPrefix + "/lb-id": "lb23456,lb34567"}

		assertOpenYurtAnnotationsEqual(t, expectedAnnotations, newSvc.Annotations)
		assertResourceVersion(t, "1000", newSvc.ResourceVersion)
	})

	t.Run("test aggregate service annotations delete value", func(t *testing.T) {
		svc := newService(v1.NamespaceDefault, mockServiceName)

		np1 := newNodepool("np123", "name=np123,app=deploy")
		np2 := newNodepool("np234", "name=np234,app=deploy")

		ps1 := newPoolService(v1.NamespaceDefault, "np123", map[string]string{network.AggregateAnnotationsKeyPrefix + "/lb-id": "lb34567"}, nil)
		svc.Annotations[network.AggregateAnnotationsKeyPrefix+"/lb-id"] = "lb23456,lb34567"

		c := fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(svc).WithObjects(np1).WithObjects(np2).
			WithObjects(ps1).Build()

		rc := ReconcileLoadBalancerSet{
			Client: c,
		}

		_, err := rc.Reconcile(context.Background(), newReconcileRequest(v1.NamespaceDefault, mockServiceName))
		assertErrNil(t, err)

		newSvc := &corev1.Service{}
		c.Get(context.Background(), types.NamespacedName{
			Namespace: v1.NamespaceDefault,
			Name:      mockServiceName,
		}, newSvc)

		expectedAnnotations := map[string]string{network.AggregateAnnotationsKeyPrefix + "/lb-id": "lb34567"}

		assertOpenYurtAnnotationsEqual(t, expectedAnnotations, newSvc.Annotations)
		assertResourceVersion(t, "1001", newSvc.ResourceVersion)
	})

	t.Run("test aggregate annotations delete key", func(t *testing.T) {
		svc := newService(v1.NamespaceDefault, mockServiceName)

		np1 := newNodepool("np123", "name=np123,app=deploy")
		np2 := newNodepool("np234", "name=np234,app=deploy")

		svc.Annotations[network.AggregateAnnotationsKeyPrefix+"/lb-id"] = "lb23456,lb34567"

		c := fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(svc).WithObjects(np1).WithObjects(np2).Build()

		rc := ReconcileLoadBalancerSet{
			Client: c,
		}

		_, err := rc.Reconcile(context.Background(), newReconcileRequest(v1.NamespaceDefault, mockServiceName))
		assertErrNil(t, err)

		newSvc := &corev1.Service{}
		c.Get(context.Background(), types.NamespacedName{
			Namespace: v1.NamespaceDefault,
			Name:      mockServiceName,
		}, newSvc)

		expectedAnnotations := map[string]string{}

		assertOpenYurtAnnotationsEqual(t, expectedAnnotations, newSvc.Annotations)
		assertResourceVersion(t, "1001", newSvc.ResourceVersion)
	})

	t.Run("add service finalizer", func(t *testing.T) {
		svc := newService(v1.NamespaceDefault, mockServiceName)

		c := fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build()

		rc := ReconcileLoadBalancerSet{
			Client: c,
		}

		_, err := rc.Reconcile(context.Background(), newReconcileRequest(v1.NamespaceDefault, mockServiceName))

		newSvc := &corev1.Service{}
		c.Get(context.Background(), types.NamespacedName{
			Namespace: v1.NamespaceDefault,
			Name:      mockServiceName,
		}, newSvc)

		assertErrNil(t, err)

		assertResourceVersion(t, "1000", newSvc.ResourceVersion)
		assertFinalizerExist(t, newSvc)
	})

	t.Run("don't need to add service finalizer", func(t *testing.T) {
		svc := newService(v1.NamespaceDefault, mockServiceName)
		controllerutil.AddFinalizer(svc, poolServiceFinalizer)

		c := fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build()

		rc := ReconcileLoadBalancerSet{
			Client: c,
		}

		_, err := rc.Reconcile(context.Background(), newReconcileRequest(v1.NamespaceDefault, mockServiceName))

		newSvc := &corev1.Service{}
		c.Get(context.Background(), types.NamespacedName{
			Namespace: v1.NamespaceDefault,
			Name:      mockServiceName,
		}, newSvc)

		assertErrNil(t, err)

		assertResourceVersion(t, "999", newSvc.ResourceVersion)
		assertFinalizerExist(t, newSvc)
	})

	t.Run("delete service finalizer", func(t *testing.T) {
		svc := newService(v1.NamespaceDefault, mockServiceName)
		controllerutil.AddFinalizer(svc, poolServiceFinalizer)
		svc.DeletionTimestamp = &v1.Time{Time: time.Now()}

		c := fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build()

		rc := ReconcileLoadBalancerSet{
			Client: c,
		}

		rc.Reconcile(context.Background(), newReconcileRequest(v1.NamespaceDefault, mockServiceName))

		newSvc := &corev1.Service{}
		err := c.Get(context.Background(), types.NamespacedName{
			Namespace: v1.NamespaceDefault,
			Name:      mockServiceName,
		}, newSvc)

		assertNotFountError(t, err)
	})

	t.Run("don't delete/update service with delete time not zero", func(t *testing.T) {
		svc := newService(v1.NamespaceDefault, mockServiceName)
		controllerutil.AddFinalizer(svc, poolServiceFinalizer)
		svc.DeletionTimestamp = &v1.Time{Time: time.Now()}

		ps1 := newPoolService(v1.NamespaceDefault, "np123", map[string]string{network.AggregateAnnotationsKeyPrefix + "/lb-id": "lb34567"}, nil)

		c := fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(svc).WithObjects(ps1).Build()

		rc := ReconcileLoadBalancerSet{
			Client: c,
		}

		_, err := rc.Reconcile(context.Background(), newReconcileRequest(v1.NamespaceDefault, mockServiceName))

		assertErrNil(t, err)

		newSvc := &corev1.Service{}
		err = c.Get(context.Background(), types.NamespacedName{
			Namespace: v1.NamespaceDefault,
			Name:      mockServiceName,
		}, newSvc)

		assertErrNil(t, err)
		assertFinalizerExist(t, newSvc)

	})

	t.Run("clean pool services", func(t *testing.T) {
		svc := newService(v1.NamespaceDefault, mockServiceName)
		controllerutil.AddFinalizer(svc, poolServiceFinalizer)
		svc.DeletionTimestamp = &v1.Time{Time: time.Now()}

		ps1 := newPoolService(v1.NamespaceDefault, "np123", map[string]string{network.AggregateAnnotationsKeyPrefix + "/lb-id": "lb34567"}, nil)
		ps2 := newPoolService(v1.NamespaceDefault, "np234", map[string]string{network.AggregateAnnotationsKeyPrefix + "/lb-id": "lb45678"}, nil)
		controllerutil.AddFinalizer(ps1, "elb.openyurt.io/resources")

		c := fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(svc).WithObjects(ps1).WithObjects(ps2).Build()

		r := ReconcileLoadBalancerSet{
			Client: c,
		}

		_, err := r.Reconcile(context.Background(), newReconcileRequest(v1.NamespaceDefault, mockServiceName))
		assertErrNil(t, err)

		newPs1 := &v1alpha1.PoolService{}
		c.Get(context.Background(), types.NamespacedName{
			Namespace: v1.NamespaceDefault, Name: "test-np123"}, newPs1)
		assertPoolServiceIsDeleting(t, newPs1)
		assertResourceVersion(t, "1000", newPs1.ResourceVersion)

		newPs2 := &v1alpha1.PoolService{}
		err = c.Get(context.Background(), types.NamespacedName{
			Namespace: v1.NamespaceDefault, Name: "test-np234"}, newPs2)
		assertNotFountError(t, err)
	})

	t.Run("check pool service owner references", func(t *testing.T) {
		svc := newService(v1.NamespaceDefault, mockServiceName)

		np := newNodepool("np123", "name=np123,app=deploy")

		c := fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(svc).WithObjects(np).Build()

		rc := &ReconcileLoadBalancerSet{
			Client: c,
		}

		rc.Reconcile(context.Background(), newReconcileRequest(v1.NamespaceDefault, mockServiceName))

		ps := &v1alpha1.PoolService{}
		err := c.Get(context.Background(), types.NamespacedName{
			Namespace: v1.NamespaceDefault,
			Name:      mockServiceName + "-np123",
		}, ps)

		assertErrNil(t, err)

		controller, blockOwnerDeletion := true, true
		expectedOwnerReferences := []v1.OwnerReference{
			{
				APIVersion:         "v1",
				Kind:               "Service",
				Name:               mockServiceName,
				UID:                mockServiceUid,
				BlockOwnerDeletion: &blockOwnerDeletion,
				Controller:         &controller,
			},
			{
				APIVersion:         "apps.openyurt.io/v1beta1",
				Kind:               "NodePool",
				Name:               "np123",
				UID:                mockNodePoolUid,
				BlockOwnerDeletion: &blockOwnerDeletion,
			},
		}

		assertOwnerReferences(t, expectedOwnerReferences, ps.OwnerReferences)
	})

	t.Run("nodepool is deletion", func(t *testing.T) {
		svc := newService(v1.NamespaceDefault, mockServiceName)
		np := newNodepool("np123", "name=np123,app=deploy")
		np.DeletionTimestamp = &v1.Time{Time: time.Now()}

		ps := newPoolService(v1.NamespaceDefault, "np123", nil, nil)

		c := fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(svc).WithObjects(np).WithObjects(ps).Build()

		r := ReconcileLoadBalancerSet{
			Client: c,
		}

		r.Reconcile(context.Background(), newReconcileRequest(v1.NamespaceDefault, mockServiceName))

		psList := &v1alpha1.PoolServiceList{}
		c.List(context.Background(), psList)

		assertPoolServiceCounts(t, 0, len(psList.Items))
	})

	t.Run("modify service type to not LB", func(t *testing.T) {
		svc := newService(v1.NamespaceDefault, mockServiceName)
		svc.Spec.Type = corev1.ServiceTypeClusterIP
		np := newNodepool("np123", "name=np123,app=deploy")
		ps := newPoolService(v1.NamespaceDefault, "np123", nil, nil)

		c := fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(svc).WithObjects(np).WithObjects(ps).Build()

		r := ReconcileLoadBalancerSet{
			Client: c,
		}
		r.Reconcile(context.Background(), newReconcileRequest(v1.NamespaceDefault, mockServiceName))

		psList := &v1alpha1.PoolServiceList{}
		c.List(context.Background(), psList)

		newSvc := &corev1.Service{}
		c.Get(context.Background(), types.NamespacedName{
			Namespace: v1.NamespaceDefault,
			Name:      mockServiceName,
		}, newSvc)

		assertPoolServiceCounts(t, 0, len(psList.Items))
		assertFinalizerNotExist(t, newSvc)
	})

	t.Run("have not pool selector annotation", func(t *testing.T) {
		svc := newService(v1.NamespaceDefault, mockServiceName)
		delete(svc.Annotations, network.AnnotationNodePoolSelector)

		np := newNodepool("np123", "name=np123,app=deploy")
		ps := newPoolService(v1.NamespaceDefault, "np123", nil, nil)

		c := fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(svc).WithObjects(np).WithObjects(ps).Build()

		r := ReconcileLoadBalancerSet{
			Client: c,
		}
		r.Reconcile(context.Background(), newReconcileRequest(v1.NamespaceDefault, mockServiceName))

		psList := &v1alpha1.PoolServiceList{}
		c.List(context.Background(), psList)

		assertPoolServiceCounts(t, 0, len(psList.Items))
	})

	t.Run("not managed by controller", func(t *testing.T) {
		svc := newService(v1.NamespaceDefault, mockServiceName)
		ps := newPoolService(v1.NamespaceDefault, "np123", nil, nil)
		delete(ps.Labels, labelManageBy)

		c := fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(svc).WithObjects(ps).Build()

		r := ReconcileLoadBalancerSet{
			Client: c,
		}
		r.Reconcile(context.Background(), newReconcileRequest(v1.NamespaceDefault, mockServiceName))

		psList := &v1alpha1.PoolServiceList{}
		c.List(context.Background(), psList)

		assertPoolServicesNameList(t, psList, []string{mockServiceName + "-np123"})
	})

	t.Run("Modifying the service name to reference a non-existent service", func(t *testing.T) {
		svc := newService(v1.NamespaceDefault, mockServiceName)

		ps := newPoolService(v1.NamespaceDefault, "np123", nil, nil)
		ps.Labels[network.LabelServiceName] = "mock"
		ps.OwnerReferences = nil
		ps.Labels[network.LabelNodePoolName] = "np111"

		np := newNodepool("np123", "name=np123,app=deploy")

		c := fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(svc).WithObjects(ps).WithObjects(np).Build()
		recorder := &record.FakeRecorder{
			Events: make(chan string, 1),
		}

		r := ReconcileLoadBalancerSet{
			Client:   c,
			recorder: recorder,
		}
		_, err := r.Reconcile(context.Background(), newReconcileRequest(v1.NamespaceDefault, mockServiceName))

		assertErrNil(t, err)

		newPs := &v1alpha1.PoolService{}
		c.Get(context.Background(), types.NamespacedName{
			Namespace: v1.NamespaceDefault, Name: "test-np123",
		}, newPs)

		assertString(t, mockServiceName, newPs.Labels[network.LabelServiceName])
		assertString(t, "np123", newPs.Labels[network.LabelNodePoolName])

		controller, blockOwnerDeletion := true, true
		expectedOwnerReferences := []v1.OwnerReference{
			{
				APIVersion:         "v1",
				Kind:               "Service",
				Name:               mockServiceName,
				UID:                mockServiceUid,
				BlockOwnerDeletion: &blockOwnerDeletion,
				Controller:         &controller,
			},
			{
				APIVersion:         "apps.openyurt.io/v1beta1",
				Kind:               "NodePool",
				Name:               "np123",
				UID:                mockNodePoolUid,
				BlockOwnerDeletion: &blockOwnerDeletion,
			},
		}

		assertOwnerReferences(t, expectedOwnerReferences, newPs.OwnerReferences)

		eve := <-recorder.Events
		expected := fmt.Sprintf("%s %s %s%s", corev1.EventTypeWarning, "Modified", "PoolService default/test-np123 resource is manually modified, the controller will overwrite this modification", "")
		assertString(t, expected, eve)
	})

	t.Run("Modifying the service name to reference a existent service", func(t *testing.T) {
		svc1 := newService(v1.NamespaceDefault, mockServiceName)
		svc2 := newService(v1.NamespaceDefault, "mock")

		ps := newPoolService(v1.NamespaceDefault, "np123", nil, nil)
		ps.Labels[network.LabelServiceName] = "mock"
		ps.OwnerReferences = nil
		ps.Labels[network.LabelNodePoolName] = "np111"

		np := newNodepool("np123", "name=np123,app=deploy")

		c := fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(svc1).WithObjects(svc2).WithObjects(ps).WithObjects(np).Build()
		recorder := &record.FakeRecorder{
			Events: make(chan string, 1),
		}

		r := ReconcileLoadBalancerSet{
			Client:   c,
			recorder: recorder,
		}
		_, err := r.Reconcile(context.Background(), newReconcileRequest(v1.NamespaceDefault, "mock"))

		assertErrNil(t, err)

		newPs := &v1alpha1.PoolService{}
		err = c.Get(context.Background(), types.NamespacedName{
			Namespace: v1.NamespaceDefault, Name: "test-np123",
		}, newPs)
		assertErrNil(t, err)
	})

	t.Run("modifying the nodepool name", func(t *testing.T) {
		svc := newService(v1.NamespaceDefault, mockServiceName)
		np := newNodepool("np123", "name=np123,app=deploy")

		ps := newPoolService(v1.NamespaceDefault, "np123", nil, nil)
		ps.Labels[network.LabelNodePoolName] = "np111"

		c := fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(svc).WithObjects(ps).WithObjects(np).Build()
		recorder := record.NewFakeRecorder(1)

		r := &ReconcileLoadBalancerSet{
			Client:   c,
			recorder: recorder,
		}

		_, err := r.Reconcile(context.Background(), newReconcileRequest(v1.NamespaceDefault, mockServiceName))

		assertErrNil(t, err)

		newPs := &v1alpha1.PoolService{}
		c.Get(context.Background(), types.NamespacedName{
			Namespace: v1.NamespaceDefault,
			Name:      "test-np123",
		}, newPs)

		assertString(t, newPs.Labels[network.LabelNodePoolName], "np123")
		assertResourceVersion(t, "1000", newPs.ResourceVersion)
	})

	t.Run("modifying the pool service is not managed by controller", func(t *testing.T) {
		svc := newService(v1.NamespaceDefault, mockServiceName)

		ps := newPoolService(v1.NamespaceDefault, "np123", nil, nil)
		ps.Labels[network.LabelServiceName] = "mock"
		delete(ps.Labels, labelManageBy)

		np := newNodepool("np123", "name=np123,app=deploy")

		c := fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(svc).WithObjects(ps).WithObjects(np).Build()
		recorder := &record.FakeRecorder{
			Events: make(chan string, 1),
		}

		r := ReconcileLoadBalancerSet{
			Client:   c,
			recorder: recorder,
		}
		_, err := r.Reconcile(context.Background(), newReconcileRequest(v1.NamespaceDefault, mockServiceName))

		assertErrNil(t, err)

		newPs := &v1alpha1.PoolService{}
		c.Get(context.Background(), types.NamespacedName{
			Namespace: v1.NamespaceDefault, Name: "test-np123",
		}, newPs)

		assertString(t, newPs.Labels[network.LabelServiceName], "mock")

		eve := <-recorder.Events
		expected := fmt.Sprintf("%s %s %s%s", corev1.EventTypeWarning, "ManagedConflict", "PoolService default/test-np123 is not managed by pool-service-controller, but the nodepool-labelselector of service default/test include it", "")
		assertString(t, expected, eve)
	})
}

func initScheme(t *testing.T) *runtime.Scheme {
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal("Fail to add kubernetes clint-go custom resource")
	}
	apis.AddToScheme(scheme)

	return scheme
}

func newReconcileRequest(namespace string, name string) reconcile.Request {
	return reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: namespace,
			Name:      name,
		}}
}

func assertErrNil(t testing.TB, err error) {
	t.Helper()

	if err != nil {
		t.Errorf("expected err is nil, but got %v", err)
	}
}

func newService(namespace string, name string) *corev1.Service {
	return &corev1.Service{
		TypeMeta: v1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: v1.ObjectMeta{
			Annotations: map[string]string{
				network.AnnotationNodePoolSelector: mockNodePoolLabel,
			},
			Namespace: namespace,
			Name:      name,
			UID:       mockServiceUid,
		},
		Spec: corev1.ServiceSpec{
			Type:              corev1.ServiceTypeLoadBalancer,
			LoadBalancerClass: &elbClass,
		},
	}
}

func newNodepool(name string, labelStr string) *v1beta1.NodePool {
	var splitLabels []string
	if labelStr != "" {
		splitLabels = strings.Split(labelStr, ",")
	}

	labels := make(map[string]string)
	for _, s := range splitLabels {
		kv := strings.Split(s, "=")
		labels[kv[0]] = kv[1]
	}

	return &v1beta1.NodePool{
		TypeMeta: v1.TypeMeta{
			Kind:       "NodePool",
			APIVersion: "apps.openyurt.io/v1beta1",
		},
		ObjectMeta: v1.ObjectMeta{
			Labels: labels,
			Name:   name,
			UID:    mockNodePoolUid,
		},
	}
}

func assertPoolServicesNameList(t testing.TB, psl *v1alpha1.PoolServiceList, expectedNameList []string) {
	t.Helper()

	sort.Strings(expectedNameList)

	gotNameList := getPoolServicesNameList(t, psl)

	if !reflect.DeepEqual(expectedNameList, gotNameList) {
		t.Errorf("expected name list %v, but got name list %v", expectedNameList, gotNameList)
	}
}

func getPoolServicesNameList(t testing.TB, psl *v1alpha1.PoolServiceList) []string {
	t.Helper()

	var gotPoolServiceNameList []string
	for _, item := range psl.Items {
		gotPoolServiceNameList = append(gotPoolServiceNameList, item.Name)
	}
	sort.Strings(gotPoolServiceNameList)

	return gotPoolServiceNameList
}

func assertPoolServicesLBClass(t testing.TB, psl *v1alpha1.PoolServiceList, lbClass *string) {
	t.Helper()

	for _, ps := range psl.Items {
		if *ps.Spec.LoadBalancerClass != *lbClass {
			t.Errorf("expected loadbalancer class %s, but got %s", *lbClass, *ps.Spec.LoadBalancerClass)
		}
	}
}

func assertPoolServiceLabels(t testing.TB, psl *v1alpha1.PoolServiceList, serviceName string) {
	t.Helper()

	for _, ps := range psl.Items {
		if ps.Labels == nil {
			t.Errorf("expected labels not nil")
			return
		}
		if ps.Labels[labelManageBy] != names.LoadBalancerSetController {
			t.Errorf("expected pool service managed by %s", names.LoadBalancerSetController)
		}
		if ps.Labels[network.LabelServiceName] != serviceName {
			t.Errorf("expected pool service name is %s, but got %s", serviceName, ps.Labels[network.LabelServiceName])
		}
	}
}

func newPoolService(namespace string, poolName string, annotations map[string]string, lbIngress []corev1.LoadBalancerIngress) *v1alpha1.PoolService {
	blockOwnerDeletion := true
	controller := true
	return &v1alpha1.PoolService{
		TypeMeta: v1.TypeMeta{
			Kind:       "PoolService",
			APIVersion: v1alpha1.GroupVersion.String(),
		},
		ObjectMeta: v1.ObjectMeta{
			Namespace:   namespace,
			Name:        mockServiceName + "-" + poolName,
			Labels:      map[string]string{network.LabelServiceName: mockServiceName, network.LabelNodePoolName: poolName, labelManageBy: names.LoadBalancerSetController},
			Annotations: annotations,
			OwnerReferences: []v1.OwnerReference{
				{
					APIVersion:         "v1",
					BlockOwnerDeletion: &blockOwnerDeletion,
					Controller:         &controller,
					Kind:               "Service",
					Name:               mockServiceName,
					UID:                mockServiceUid,
				}, {
					APIVersion:         "apps.openyurt.io/v1alpha1",
					BlockOwnerDeletion: &blockOwnerDeletion,
					Controller:         &controller,
					Kind:               "NodePool",
					Name:               poolName,
					UID:                mockNodePoolUid,
				},
			},
		},
		Spec: v1alpha1.PoolServiceSpec{
			LoadBalancerClass: &elbClass,
		},
		Status: v1alpha1.PoolServiceStatus{
			LoadBalancer: corev1.LoadBalancerStatus{
				Ingress: lbIngress,
			},
		},
	}
}

func assertLoadBalancerStatusEqual(t testing.TB, expected, got []corev1.LoadBalancerIngress) {
	t.Helper()

	if len(expected) != len(got) {
		t.Errorf("expected %v, but got %v", expected, got)
		return
	}

	sort.Slice(expected, func(i, j int) bool {
		return expected[i].IP < expected[j].IP
	})

	for i := 0; i < len(expected); i++ {
		if !reflect.DeepEqual(expected[i], got[i]) {
			t.Errorf("expected %++v, but got %++v", expected[i], got[i])
		}
	}
}

func assertOpenYurtAnnotationsEqual(t testing.TB, expected, got map[string]string) {
	t.Helper()

	for k, v := range expected {
		if !strings.HasPrefix(k, network.AggregateAnnotationsKeyPrefix) || (k == network.AnnotationNodePoolSelector) {
			continue
		}
		if got[k] != v {
			t.Errorf("expected key %s value is %s, but got %s", k, v, got[k])
		}
	}

	for k, v := range got {
		if !strings.HasPrefix(k, network.AggregateAnnotationsKeyPrefix) || (k == network.AnnotationNodePoolSelector) {
			continue
		}

		if expected[k] != v {
			t.Errorf("expected key value %s is %s, but got %s", k, expected[k], v)
		}
	}
}

func assertResourceVersion(t testing.TB, expected, got string) {
	t.Helper()

	if expected != got {
		t.Errorf("expected resource version is %s, but got %s", expected, got)
	}
}

func assertFinalizerExist(t testing.TB, svc *corev1.Service) {
	t.Helper()

	if !controllerutil.ContainsFinalizer(svc, poolServiceFinalizer) {
		t.Errorf("expected service has finalizer %s", poolServiceFinalizer)
	}
}

func assertFinalizerNotExist(t testing.TB, svc *corev1.Service) {
	t.Helper()

	if controllerutil.ContainsFinalizer(svc, poolServiceFinalizer) {
		t.Errorf("expected service finalizer %s is not exist, but it is exist", poolServiceFinalizer)
	}
}

func assertPoolServiceIsDeleting(t testing.TB, ps *v1alpha1.PoolService) {
	t.Helper()

	if ps.DeletionTimestamp.IsZero() {
		t.Errorf("expected pool service is deleting")
	}
}

func assertNotFountError(t testing.TB, err error) {
	t.Helper()

	if !apierrors.IsNotFound(err) {
		t.Errorf("exptected error is not found, but got %v", err)
	}
}

func assertOwnerReferences(t testing.TB, expected, got []v1.OwnerReference) {
	t.Helper()

	if !reflect.DeepEqual(expected, got) {
		t.Errorf("expected owner reference is %v, but got %v", expected, got)
	}
}

func assertPoolServiceCounts(t testing.TB, expected, got int) {
	t.Helper()

	if expected != got {
		t.Errorf("expected pool service counts is %d, but got %d", expected, got)
	}
}

func assertString(t testing.TB, expected, got string) {
	t.Helper()

	if expected != got {
		t.Errorf("expected %s, but got %s", expected, got)
	}
}
