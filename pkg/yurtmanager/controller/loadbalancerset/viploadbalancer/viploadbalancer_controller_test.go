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
	"context"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openyurtio/openyurt/pkg/apis"
	"github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
	network "github.com/openyurtio/openyurt/pkg/apis/network"
	"github.com/openyurtio/openyurt/pkg/apis/network/v1alpha1"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/loadbalancerset/viploadbalancer"
)

const (
	mockServiceName     = "test"
	mockPoolServiceName = "test-np123"
	mockNodePoolLabel   = "app=deploy"
	mockServiceUid      = "c0af506a-7096-4ef9-b39a-eac2feb5c07g"
	mockNodePoolUid     = "f47dd9db-d3bc-40f3-8d03-7409930b6289"
	vridLable           = "service.openyurt.io/vrid"
)

var (
	viplbClass = "service.openyurt.io/viplb"
)

func TestReconcileVipLoadBalancer_Reconcile(t *testing.T) {
	scheme := initScheme(t)

	t.Run("test get poolservice not found", func(t *testing.T) {
		rc := viploadbalancer.ReconcileVipLoadBalancer{
			Client: fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects().Build(),
		}

		_, err := rc.Reconcile(context.Background(), newReconcileRequest(v1.NamespaceDefault, mockServiceName))

		assertErrNil(t, err)
	})

	t.Run("test update pool services", func(t *testing.T) {
		svc := newService(v1.NamespaceDefault, mockServiceName)
		poolsvc := newPoolService(v1.NamespaceDefault, "np123", nil, nil)
		np1 := newNodepool("np123", "name=np123,app=deploy")
		c := fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(svc).WithObjects(np1).WithObjects(poolsvc).Build()

		vridManage := viploadbalancer.NewVRIDManager()
		rc := viploadbalancer.ReconcileVipLoadBalancer{
			Client:      c,
			VRIDManager: vridManage,
		}

		_, err := rc.Reconcile(context.Background(), newReconcileRequest(v1.NamespaceDefault, mockPoolServiceName))

		psl := &v1alpha1.PoolServiceList{}
		c.List(context.Background(), psl)

		assertErrNil(t, err)
		assertPoolServicesNameList(t, psl, []string{"test-np123"})
		assertPoolServicesLBClass(t, psl, svc.Spec.LoadBalancerClass)
		assertPoolServiceLabels(t, psl, svc.Name)
		assertPoolServiceVRIDLabels(t, psl, "0")
	})

	t.Run("test delete pool services with delete time not zero", func(t *testing.T) {
		svc := newService(v1.NamespaceDefault, mockServiceName)
		np1 := newNodepool("np123", "name=np123,app=deploy")
		ps1 := newPoolService(v1.NamespaceDefault, "np123", nil, nil)

		vridManage := viploadbalancer.NewVRIDManager()
		vrid := vridManage.GetVRID("np123")

		ps1.Annotations = map[string]string{viploadbalancer.AnnotationVipLoadBalancerVRID: strconv.Itoa(vrid)}
		ps1.DeletionTimestamp = &v1.Time{Time: time.Now()}

		c := fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(svc).WithObjects(np1).WithObjects(ps1).Build()

		rc := viploadbalancer.ReconcileVipLoadBalancer{
			Client:      c,
			VRIDManager: vridManage,
		}

		_, err := rc.Reconcile(context.Background(), newReconcileRequest(v1.NamespaceDefault, mockPoolServiceName))

		psl := &v1alpha1.PoolServiceList{}
		c.List(context.Background(), psl)

		assertErrNil(t, err)
		assertPoolServiceNil(t, psl)
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
			LoadBalancerClass: &viplbClass,
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

func assertPoolServiceNil(t testing.TB, psl *v1alpha1.PoolServiceList) {
	t.Helper()

	if len(psl.Items) != 0 {
		t.Errorf("expected pool service list is nil")
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

		if ps.Labels[network.LabelServiceName] != serviceName {
			t.Errorf("expected pool service name is %s, but got %s", serviceName, ps.Labels[network.LabelServiceName])
		}
	}
}

func assertPoolServiceVRIDLabels(t testing.TB, psl *v1alpha1.PoolServiceList, vrid string) {
	t.Helper()

	for _, ps := range psl.Items {
		if ps.Annotations == nil {
			t.Errorf("expected labels not nil")
			return
		}

		if ps.Annotations[vridLable] != vrid {
			t.Errorf("expected pool service name is %s, but got %s", vrid, ps.Labels[vridLable])
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
			Labels:      map[string]string{network.LabelServiceName: mockServiceName, network.LabelNodePoolName: poolName},
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
			LoadBalancerClass: &viplbClass,
		},
		Status: v1alpha1.PoolServiceStatus{
			LoadBalancer: corev1.LoadBalancerStatus{
				Ingress: lbIngress,
			},
		},
	}
}
