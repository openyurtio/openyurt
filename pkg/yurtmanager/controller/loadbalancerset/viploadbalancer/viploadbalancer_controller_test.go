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
	"fmt"
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
	"k8s.io/utils/strings/slices"
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
	mockEndpointName    = "test-ep"
	mockNodePoolLabel   = "app=deploy"
	mockServiceUid      = "c0af506a-7096-4ef9-b39a-eac2feb5c07g"
	mockNodePoolUid     = "f47dd9db-d3bc-40f3-8d03-7409930b6289"
	mockEndpointUid     = "f7a5a351-3e33-47a9-bb00-685b357a62e5"
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

	t.Run("test get pool services", func(t *testing.T) {
		svc := newService(v1.NamespaceDefault, mockServiceName)
		poolsvc := newPoolService(v1.NamespaceDefault, "np123", nil, nil)
		np1 := newNodepool("np123", "name=np123,app=deploy")
		adressPool := "192.168.0.1-192.168.1.1"
		np1.Annotations = map[string]string{viploadbalancer.AnnotationNodePoolAddressPools: adressPool}
		c := fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(svc).WithObjects(np1).WithObjects(poolsvc).Build()

		rc := viploadbalancer.ReconcileVipLoadBalancer{
			Client:     c,
			IPManagers: map[string]*viploadbalancer.IPManager{},
		}

		_, err := rc.Reconcile(context.Background(), newReconcileRequest(v1.NamespaceDefault, mockPoolServiceName))

		psl := &v1alpha1.PoolServiceList{}
		c.List(context.Background(), psl)

		assertErrNil(t, err)
		assertPoolServicesNameList(t, psl, []string{"test-np123"})
		assertPoolServicesLBClass(t, psl, svc.Spec.LoadBalancerClass)
		assertPoolServiceLabels(t, psl, svc.Name)
		assertPoolServiceFinalizer(t, psl)
		assertPoolServiceVRIDLabels(t, psl, "0")
	})

	t.Run("test get pool services with valid vrid", func(t *testing.T) {
		svc := newService(v1.NamespaceDefault, mockServiceName)
		poolsvc := newPoolService(v1.NamespaceDefault, "np123", nil, nil)
		np1 := newNodepool("np123", "name=np123,app=deploy")
		adressPool := "192.168.0.1-192.168.1.1"
		np1.Annotations = map[string]string{viploadbalancer.AnnotationNodePoolAddressPools: adressPool}

		poolsvc.Annotations = map[string]string{viploadbalancer.AnnotationVipLoadBalancerVRID: "6"}

		c := fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(svc).WithObjects(np1).WithObjects(poolsvc).Build()

		rc := viploadbalancer.ReconcileVipLoadBalancer{
			Client:     c,
			IPManagers: map[string]*viploadbalancer.IPManager{},
		}

		_, err := rc.Reconcile(context.Background(), newReconcileRequest(v1.NamespaceDefault, mockPoolServiceName))

		psl := &v1alpha1.PoolServiceList{}
		c.List(context.Background(), psl)

		assertErrNil(t, err)
		assertPoolServicesNameList(t, psl, []string{"test-np123"})
		assertPoolServicesLBClass(t, psl, svc.Spec.LoadBalancerClass)
		assertPoolServiceLabels(t, psl, svc.Name)
		assertPoolServiceFinalizer(t, psl)
		assertPoolServiceVRIDLabels(t, psl, "0")
		assertPoolServiceIPAddress(t, psl, nil)
	})

	t.Run("test update pool services with invalid vrid", func(t *testing.T) {
		svc := newService(v1.NamespaceDefault, mockServiceName)
		poolsvc := newPoolService(v1.NamespaceDefault, "np123", nil, nil)
		np1 := newNodepool("np123", "name=np123,app=deploy")
		adressPool := "192.168.0.1-192.168.1.1"
		np1.Annotations = map[string]string{viploadbalancer.AnnotationNodePoolAddressPools: adressPool}

		poolsvc.Annotations = map[string]string{viploadbalancer.AnnotationVipLoadBalancerVRID: "256"}

		c := fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(svc).WithObjects(np1).WithObjects(poolsvc).Build()

		rc := viploadbalancer.ReconcileVipLoadBalancer{
			Client:     c,
			IPManagers: map[string]*viploadbalancer.IPManager{},
		}
		_, err := rc.Reconcile(context.Background(), newReconcileRequest(v1.NamespaceDefault, mockPoolServiceName))

		psl := &v1alpha1.PoolServiceList{}
		c.List(context.Background(), psl)

		assertErrNil(t, err)
		assertPoolServicesNameList(t, psl, []string{"test-np123"})
		assertPoolServicesLBClass(t, psl, svc.Spec.LoadBalancerClass)
		assertPoolServiceLabels(t, psl, svc.Name)
		assertPoolServiceFinalizer(t, psl)
		assertPoolServiceVRIDLabels(t, psl, "0")
	})

	t.Run("test update pool services with specify ip", func(t *testing.T) {
		svc := newService(v1.NamespaceDefault, mockServiceName)
		svc.Annotations = map[string]string{viploadbalancer.AnnotationServiceVIPAddress: "192.168.1.1"}
		poolsvc := newPoolService(v1.NamespaceDefault, "np123", nil, nil)
		np1 := newNodepool("np123", "name=np123,app=deploy")
		adressPool := "192.168.0.1-192.168.2.2"
		np1.Annotations = map[string]string{viploadbalancer.AnnotationNodePoolAddressPools: adressPool}
		endpoint := newEndpoint(v1.NamespaceDefault, mockEndpointName)
		endpoint.Annotations = map[string]string{
			viploadbalancer.AnnotationServiceVIPStatus: viploadbalancer.AnnotationServiceVIPStatusOnline,
		}

		c := fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(svc).WithObjects(np1).WithObjects(poolsvc).WithObjects(endpoint).Build()

		rc := viploadbalancer.ReconcileVipLoadBalancer{
			Client:     c,
			IPManagers: map[string]*viploadbalancer.IPManager{},
		}
		_, err := rc.Reconcile(context.Background(), newReconcileRequest(v1.NamespaceDefault, mockPoolServiceName))

		psl := &v1alpha1.PoolServiceList{}
		c.List(context.Background(), psl)

		assertErrNil(t, err)
		assertPoolServiceVRIDLabels(t, psl, "0")
		assertPoolServiceIPAddress(t, psl, []string{"192.168.1.1"})
	})

	t.Run("test update pool services with specify ip range", func(t *testing.T) {
		svc := newService(v1.NamespaceDefault, mockServiceName)
		svcIPRange := "192.168.1.1-192.168.1.3"
		expectIPs := viploadbalancer.ParseIP(svcIPRange)
		svc.Annotations = map[string]string{viploadbalancer.AnnotationServiceVIPAddress: svcIPRange}
		poolsvc := newPoolService(v1.NamespaceDefault, "np123", nil, nil)
		np1 := newNodepool("np123", "name=np123,app=deploy")
		adressPool := "192.168.0.1-192.168.2.2"
		np1.Annotations = map[string]string{viploadbalancer.AnnotationNodePoolAddressPools: adressPool}
		endpoint := newEndpoint(v1.NamespaceDefault, mockEndpointName)
		endpoint.Annotations = map[string]string{
			viploadbalancer.AnnotationServiceVIPStatus: viploadbalancer.AnnotationServiceVIPStatusOnline,
		}

		c := fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(svc).WithObjects(np1).WithObjects(poolsvc).WithObjects(endpoint).Build()

		rc := viploadbalancer.ReconcileVipLoadBalancer{
			Client:     c,
			IPManagers: map[string]*viploadbalancer.IPManager{},
		}
		_, err := rc.Reconcile(context.Background(), newReconcileRequest(v1.NamespaceDefault, mockPoolServiceName))

		psl := &v1alpha1.PoolServiceList{}
		c.List(context.Background(), psl)

		assertErrNil(t, err)
		assertPoolServiceVRIDLabels(t, psl, "0")
		assertPoolServiceIPAddress(t, psl, expectIPs)
	})

	t.Run("test update pool services with specify ip mix", func(t *testing.T) {
		svc := newService(v1.NamespaceDefault, mockServiceName)
		svcIPRange := "192.168.1.1-192.168.1.3, 10.1.0.1"
		exceptIPs := viploadbalancer.ParseIP(svcIPRange)
		svc.Annotations = map[string]string{viploadbalancer.AnnotationServiceVIPAddress: svcIPRange}
		poolsvc := newPoolService(v1.NamespaceDefault, "np123", nil, nil)
		np1 := newNodepool("np123", "name=np123,app=deploy")
		adressPool := "192.168.0.1-192.168.2.2, 10.1.0.1-10.1.0.2"
		np1.Annotations = map[string]string{viploadbalancer.AnnotationNodePoolAddressPools: adressPool}
		endpoint := newEndpoint(v1.NamespaceDefault, mockEndpointName)
		endpoint.Annotations = map[string]string{
			viploadbalancer.AnnotationServiceVIPStatus: viploadbalancer.AnnotationServiceVIPStatusOnline,
		}

		c := fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(svc).WithObjects(np1).WithObjects(poolsvc).WithObjects(endpoint).Build()

		rc := viploadbalancer.ReconcileVipLoadBalancer{
			Client:     c,
			IPManagers: map[string]*viploadbalancer.IPManager{},
		}
		_, err := rc.Reconcile(context.Background(), newReconcileRequest(v1.NamespaceDefault, mockPoolServiceName))

		psl := &v1alpha1.PoolServiceList{}
		c.List(context.Background(), psl)

		assertErrNil(t, err)
		assertPoolServiceVRIDLabels(t, psl, "0")
		assertPoolServiceIPAddress(t, psl, exceptIPs)
	})

	t.Run("test delete pool services with invalid vrid", func(t *testing.T) {
		svc := newService(v1.NamespaceDefault, mockServiceName)
		np1 := newNodepool("np123", "name=np123,app=deploy")
		ps1 := newPoolService(v1.NamespaceDefault, "np123", nil, nil)
		ps1.Finalizers = []string{viploadbalancer.VipLoadBalancerFinalizer}

		ipManage, err := viploadbalancer.NewIPManager("192.168.0.1-192.168.1.1")
		if err != nil {
			t.Fatalf("Failed to create IPManager: %v", err)
		}

		ps1.Annotations = map[string]string{viploadbalancer.AnnotationVipLoadBalancerVRID: "256"}
		ps1.DeletionTimestamp = &v1.Time{Time: time.Now()}
		ps1.Finalizers = []string{viploadbalancer.VipLoadBalancerFinalizer}

		c := fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(svc).WithObjects(np1).WithObjects(ps1).Build()

		rc := viploadbalancer.ReconcileVipLoadBalancer{
			Client: c,
			IPManagers: map[string]*viploadbalancer.IPManager{
				"np123": ipManage,
			},
		}

		_, err = rc.Reconcile(context.Background(), newReconcileRequest(v1.NamespaceDefault, mockPoolServiceName))

		psl := &v1alpha1.PoolServiceList{}
		c.List(context.Background(), psl)

		assertErrNil(t, err)
		assertPoolServiceFinalizer(t, psl)
	})

	t.Run("test delete pool services with empty vrid", func(t *testing.T) {
		svc := newService(v1.NamespaceDefault, mockServiceName)
		np1 := newNodepool("np123", "name=np123,app=deploy")
		ps1 := newPoolService(v1.NamespaceDefault, "np123", nil, nil)
		ps1.Finalizers = []string{viploadbalancer.VipLoadBalancerFinalizer}

		ipManage, err := viploadbalancer.NewIPManager("192.168.0.1-192.168.1.1")
		if err != nil {
			t.Fatalf("Failed to create IPManager: %v", err)
		}

		ps1.DeletionTimestamp = &v1.Time{Time: time.Now()}
		ps1.Finalizers = []string{viploadbalancer.VipLoadBalancerFinalizer}

		c := fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(svc).WithObjects(np1).WithObjects(ps1).Build()

		rc := viploadbalancer.ReconcileVipLoadBalancer{
			Client: c,
			IPManagers: map[string]*viploadbalancer.IPManager{
				"np123": ipManage,
			},
		}

		_, err = rc.Reconcile(context.Background(), newReconcileRequest(v1.NamespaceDefault, mockPoolServiceName))

		psl := &v1alpha1.PoolServiceList{}
		c.List(context.Background(), psl)

		assertErrNil(t, err)
		assertPoolServiceFinalizer(t, psl)
	})

	t.Run("test delete pool services with delete time not zero and vip empty", func(t *testing.T) {
		svc := newService(v1.NamespaceDefault, mockServiceName)
		np1 := newNodepool("np123", "name=np123,app=deploy")
		ps1 := newPoolService(v1.NamespaceDefault, "np123", nil, nil)
		ps1.Finalizers = []string{viploadbalancer.VipLoadBalancerFinalizer}
		ps1.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{{IP: "192.168.0.1"}}

		ipManage, err := viploadbalancer.NewIPManager("192.168.0.1-192.168.1.1")
		if err != nil {
			t.Fatalf("Failed to create IPManager: %v", err)
		}
		ipvrid, err := ipManage.Get()
		if err != nil {
			t.Fatalf("Failed to get IPVRID: %v", err)
		}

		ps1.Annotations = map[string]string{viploadbalancer.AnnotationVipLoadBalancerVRID: strconv.Itoa(ipvrid.VRID)}
		ps1.DeletionTimestamp = &v1.Time{Time: time.Now()}

		c := fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(svc).WithObjects(np1).WithObjects(ps1).Build()

		rc := viploadbalancer.ReconcileVipLoadBalancer{
			Client: c,
			IPManagers: map[string]*viploadbalancer.IPManager{
				"np123": ipManage,
			},
		}

		_, err = rc.Reconcile(context.Background(), newReconcileRequest(v1.NamespaceDefault, mockPoolServiceName))

		psl := &v1alpha1.PoolServiceList{}
		c.List(context.Background(), psl)

		assertErrNil(t, err)
		assertPoolServiceFinalizer(t, psl)
	})

	t.Run("test delete pool services with delete time not zero and vip ready", func(t *testing.T) {
		svc := newService(v1.NamespaceDefault, mockServiceName)
		np1 := newNodepool("np123", "name=np123,app=deploy")
		ps1 := newPoolService(v1.NamespaceDefault, "np123", nil, nil)
		ps1.Finalizers = []string{viploadbalancer.VipLoadBalancerFinalizer}
		ps1.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{{IP: "192.168.0.1"}}

		ipManage, err := viploadbalancer.NewIPManager("192.168.0.1-192.168.1.1")
		if err != nil {
			t.Fatalf("Failed to create IPManager: %v", err)
		}
		ipvrid, err := ipManage.Get()
		if err != nil {
			t.Fatalf("Failed to get IPVRID: %v", err)
		}

		ps1.Annotations = map[string]string{viploadbalancer.AnnotationVipLoadBalancerVRID: strconv.Itoa(ipvrid.VRID), viploadbalancer.AnnotationServiceVIPStatus: viploadbalancer.AnnotationServiceVIPStatusOnline}
		ps1.DeletionTimestamp = &v1.Time{Time: time.Now()}

		c := fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(svc).WithObjects(np1).WithObjects(ps1).Build()

		rc := viploadbalancer.ReconcileVipLoadBalancer{
			Client: c,
			IPManagers: map[string]*viploadbalancer.IPManager{
				"np123": ipManage,
			},
		}

		_, err = rc.Reconcile(context.Background(), newReconcileRequest(v1.NamespaceDefault, mockPoolServiceName))

		psl := &v1alpha1.PoolServiceList{}
		c.List(context.Background(), psl)

		assertErrNil(t, err)
		assertPoolServiceFinalizer(t, psl)
	})

	t.Run("test delete pool services with delete time not zero and vip unready", func(t *testing.T) {
		svc := newService(v1.NamespaceDefault, mockServiceName)
		np1 := newNodepool("np123", "name=np123,app=deploy")
		ps1 := newPoolService(v1.NamespaceDefault, "np123", nil, nil)
		ps1.Finalizers = []string{viploadbalancer.VipLoadBalancerFinalizer}
		ps1.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{{IP: "192.168.0.1"}}

		ipManage, err := viploadbalancer.NewIPManager("192.168.0.1-192.168.1.1")
		if err != nil {
			t.Fatalf("Failed to create IPManager: %v", err)
		}
		ipvrid, err := ipManage.Get()
		if err != nil {
			t.Fatalf("Failed to get IPVRID: %v", err)
		}

		ps1.Annotations = map[string]string{viploadbalancer.AnnotationVipLoadBalancerVRID: strconv.Itoa(ipvrid.VRID), viploadbalancer.AnnotationServiceVIPStatus: viploadbalancer.AnnotationServiceVIPStatusOffline}
		ps1.DeletionTimestamp = &v1.Time{Time: time.Now()}

		endpoint := newEndpoint(v1.NamespaceDefault, mockEndpointName)
		endpoint.Annotations = map[string]string{
			viploadbalancer.AnnotationServiceVIPStatus: viploadbalancer.AnnotationServiceVIPStatusOffline,
		}

		c := fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(svc).WithObjects(np1).WithObjects(ps1).WithObjects(endpoint).Build()

		rc := viploadbalancer.ReconcileVipLoadBalancer{
			Client: c,
			IPManagers: map[string]*viploadbalancer.IPManager{
				"np123": ipManage,
			},
		}

		_, err = rc.Reconcile(context.Background(), newReconcileRequest(v1.NamespaceDefault, mockPoolServiceName))

		psl := &v1alpha1.PoolServiceList{}
		c.List(context.Background(), psl)

		assertErrNil(t, err)
		assertPoolServiceFinalizerNil(t, psl)
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

func newEndpoint(namespace string, name string) *corev1.Endpoints {
	return &corev1.Endpoints{
		TypeMeta: v1.TypeMeta{
			Kind:       "Endpoints",
			APIVersion: "v1",
		},
		ObjectMeta: v1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
			Labels: map[string]string{
				network.LabelServiceName: mockServiceName,
			},
			UID: mockEndpointUid,
		},
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

func assertPoolServiceIPAddress(t testing.TB, psl *v1alpha1.PoolServiceList, ips []string) {
	t.Helper()

	for _, ps := range psl.Items {
		if ips != nil && ps.Status.LoadBalancer.Ingress == nil {
			t.Errorf("expected loadbalancer ingress is not nil, but got %v", ps.Status.LoadBalancer.Ingress)
			return
		}

		for _, lbIngress := range ps.Status.LoadBalancer.Ingress {
			if lbIngress.IP == "" {
				t.Errorf("expected loadbalancer ingress IP is not empty, but got %s", lbIngress.IP)
			}

			if len(ips) != 0 && !slices.Contains(ips, lbIngress.IP) {
				t.Errorf("excepted IPs are %v, but got %s", ips, lbIngress.IP)
			}

			fmt.Printf("IP: %s\n", lbIngress.IP)
		}
	}

}

func assertPoolServiceFinalizer(t testing.TB, psl *v1alpha1.PoolServiceList) {
	t.Helper()

	for _, ps := range psl.Items {
		if len(ps.Finalizers) == 0 {
			t.Errorf("expected finalizer is not nil, but got %v", ps.Finalizers)
			return
		}
		for _, f := range ps.Finalizers {
			if f == viploadbalancer.VipLoadBalancerFinalizer {
				return
			}
		}
		t.Errorf("expected finalizer is %s, but got %s", viploadbalancer.VipLoadBalancerFinalizer, ps.Finalizers)
	}

}

func assertPoolServiceFinalizerNil(t testing.TB, psl *v1alpha1.PoolServiceList) {
	t.Helper()

	for _, ps := range psl.Items {
		if len(ps.Finalizers) != 0 {
			t.Errorf("expected finalizer is nil, but got %v", ps.Finalizers)
		}
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
			t.Errorf("expected pool service vird is %s, but got %s", vrid, ps.Annotations[vridLable])
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
