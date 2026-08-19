/*
Copyright 2023 The OpenYurt Authors.

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

//nolint:staticcheck // SA1019: corev1.Endpoints is deprecated but still supported for backward compatibility
package gatewaypublicservice

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openyurtio/openyurt/pkg/apis"
	"github.com/openyurtio/openyurt/pkg/apis/raven"
	ravenv1beta1 "github.com/openyurtio/openyurt/pkg/apis/raven/v1beta1"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/raven/util"
)

const (
	Node1Name    = "node-1"
	Node2Name    = "node-2"
	Node3Name    = "node-3"
	Node4Name    = "node-4"
	Node1Address = "192.168.0.1"
	Node2Address = "192.168.0.2"
	Node3Address = "192.168.0.3"
	Node4Address = "192.168.0.4"
	MockGateway  = "gw-mock"
)

func MockReconcile() *ReconcileService {
	nodeList := &corev1.NodeList{
		Items: []corev1.Node{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: Node1Name,
					Labels: map[string]string{
						raven.LabelCurrentGateway: MockGateway,
					},
				},
				Status: corev1.NodeStatus{
					Addresses: []corev1.NodeAddress{
						{
							Type:    corev1.NodeInternalIP,
							Address: Node1Address,
						},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: Node2Name,
					Labels: map[string]string{
						raven.LabelCurrentGateway: MockGateway,
					},
				},
				Status: corev1.NodeStatus{
					Addresses: []corev1.NodeAddress{
						{
							Type:    corev1.NodeInternalIP,
							Address: Node2Address,
						},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: Node3Name,
				},
				Status: corev1.NodeStatus{
					Addresses: []corev1.NodeAddress{
						{
							Type:    corev1.NodeInternalIP,
							Address: Node3Address,
						},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: Node4Name,
				},
				Status: corev1.NodeStatus{
					Addresses: []corev1.NodeAddress{
						{
							Type:    corev1.NodeInternalIP,
							Address: Node4Address,
						},
					},
				},
			},
		},
	}
	configmaps := &corev1.ConfigMapList{
		Items: []corev1.ConfigMap{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      util.RavenGlobalConfig,
					Namespace: util.WorkingNamespace,
				},
				Data: map[string]string{
					util.RavenEnableProxy:  "true",
					util.RavenEnableTunnel: "true",
				},
			},
		},
	}
	gateways := &ravenv1beta1.GatewayList{
		Items: []ravenv1beta1.Gateway{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: MockGateway,
				},
				Spec: ravenv1beta1.GatewaySpec{
					NodeSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							raven.LabelCurrentGateway: MockGateway,
						},
					},
					ProxyConfig: ravenv1beta1.ProxyConfiguration{
						Replicas:       2,
						ProxyHTTPPort:  "10266,10267,10255,9100",
						ProxyHTTPSPort: "10250,9445",
					},
					TunnelConfig: ravenv1beta1.TunnelConfiguration{
						Replicas: 1,
					},
					Endpoints: []ravenv1beta1.Endpoint{
						{
							NodeName: Node1Name,
							Type:     ravenv1beta1.Proxy,
							Port:     ravenv1beta1.DefaultProxyServerExposedPort,
							UnderNAT: false,
						},
						{
							NodeName: Node2Name,
							Type:     ravenv1beta1.Proxy,
							Port:     ravenv1beta1.DefaultProxyServerExposedPort,
							UnderNAT: false,
						},
					},
					ExposeType: ravenv1beta1.ExposeTypeLoadBalancer,
				},
				Status: ravenv1beta1.GatewayStatus{
					Nodes: []ravenv1beta1.NodeInfo{
						{
							NodeName:  Node1Name,
							PrivateIP: Node1Address,
						},
						{
							NodeName:  Node2Name,
							PrivateIP: Node2Address,
						},
					},
					ActiveEndpoints: []*ravenv1beta1.Endpoint{
						{
							NodeName: Node1Name,
							Type:     ravenv1beta1.Proxy,
							Port:     ravenv1beta1.DefaultProxyServerExposedPort,
							UnderNAT: false,
						},
						{
							NodeName: Node2Name,
							Type:     ravenv1beta1.Proxy,
							Port:     ravenv1beta1.DefaultProxyServerExposedPort,
							UnderNAT: false,
						},
					},
				},
			},
		},
	}
	objs := []runtime.Object{nodeList, gateways, configmaps}
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	if err != nil {
		return nil
	}
	err = apis.AddToScheme(scheme)
	if err != nil {
		return nil
	}

	return &ReconcileService{
		Client:   fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build(),
		recorder: record.NewFakeRecorder(100),
	}
}

func servicesByEndpointNode(t *testing.T, r *ReconcileService) map[string]string {
	t.Helper()
	svcList := &corev1.ServiceList{}
	if err := r.List(context.Background(), svcList); err != nil {
		t.Fatalf("failed to list services: %v", err)
	}
	byNode := make(map[string]string)
	for _, svc := range svcList.Items {
		byNode[svc.Labels[util.LabelCurrentGatewayEndpoints]] = svc.Name
	}
	return byNode
}

func assertEndpointsForService(t *testing.T, r *ReconcileService, name, expectedIP string) {
	t.Helper()
	eps := &corev1.Endpoints{}
	if err := r.Get(context.Background(), types.NamespacedName{Namespace: util.WorkingNamespace, Name: name}, eps); err != nil {
		t.Errorf("expected endpoints %q to exist, got error: %v", name, err)
		return
	}
	if len(eps.Subsets) != 1 || len(eps.Subsets[0].Addresses) != 1 || eps.Subsets[0].Addresses[0].IP != expectedIP {
		t.Errorf("expected endpoints %q to contain IP %q, got %v", name, expectedIP, eps.Subsets)
	}
}

func TestReconcileService_TwoReplicas(t *testing.T) {
	r := MockReconcile()
	if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: MockGateway}}); err != nil {
		t.Fatalf("failed to reconcile service %s: %v", MockGateway, err)
	}

	byNode := servicesByEndpointNode(t, r)
	if len(byNode) != 2 {
		t.Errorf("expected 2 LoadBalancer services for 2 active proxy endpoints, got %d", len(byNode))
	}
	for _, nodeName := range []string{Node1Name, Node2Name} {
		name, ok := byNode[nodeName]
		if !ok {
			t.Errorf("expected a service for active node %q, none found", nodeName)
			continue
		}
		svc := &corev1.Service{}
		if err := r.Get(context.Background(), types.NamespacedName{Namespace: util.WorkingNamespace, Name: name}, svc); err != nil {
			t.Errorf("failed to get service %q: %v", name, err)
			continue
		}
		if svc.Spec.Type != corev1.ServiceTypeLoadBalancer {
			t.Errorf("expected service %q to be of type LoadBalancer, got %s", name, svc.Spec.Type)
		}
		assertEndpointsForService(t, r, name, map[string]string{Node1Name: Node1Address, Node2Name: Node2Address}[nodeName])
	}
}

func TestReconcileService_Failover(t *testing.T) {
	r := MockReconcile()

	gateway := &ravenv1beta1.Gateway{}
	if err := r.Get(context.Background(), types.NamespacedName{Name: MockGateway}, gateway); err != nil {
		t.Fatalf("failed to get gateway: %v", err)
	}
	gateway.Status.ActiveEndpoints = []*ravenv1beta1.Endpoint{
		{
			NodeName: Node1Name,
			Type:     ravenv1beta1.Proxy,
			Port:     ravenv1beta1.DefaultProxyServerExposedPort,
			UnderNAT: false,
		},
	}
	if err := r.Update(context.Background(), gateway); err != nil {
		t.Fatalf("failed to update gateway: %v", err)
	}

	if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: MockGateway}}); err != nil {
		t.Fatalf("failed to reconcile service %s: %v", MockGateway, err)
	}

	byNode := servicesByEndpointNode(t, r)
	if len(byNode) != 1 || byNode[Node1Name] == "" {
		t.Errorf("expected a service only for active node %q, got %v", Node1Name, byNode)
	}

	gateway = &ravenv1beta1.Gateway{}
	if err := r.Get(context.Background(), types.NamespacedName{Name: MockGateway}, gateway); err != nil {
		t.Fatalf("failed to get gateway: %v", err)
	}
	gateway.Status.ActiveEndpoints = []*ravenv1beta1.Endpoint{
		{
			NodeName: Node2Name,
			Type:     ravenv1beta1.Proxy,
			Port:     ravenv1beta1.DefaultProxyServerExposedPort,
			UnderNAT: false,
		},
	}
	if err := r.Update(context.Background(), gateway); err != nil {
		t.Fatalf("failed to update gateway: %v", err)
	}

	if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: MockGateway}}); err != nil {
		t.Fatalf("failed to reconcile service %s: %v", MockGateway, err)
	}

	byNode = servicesByEndpointNode(t, r)
	if len(byNode) != 1 {
		t.Fatalf("expected exactly 1 service after failover, got %v", byNode)
	}
	if byNode[Node1Name] != "" {
		t.Errorf("expected service for old active node %q to be deleted after failover", Node1Name)
	}
	name, ok := byNode[Node2Name]
	if !ok {
		t.Fatalf("expected service for new active node %q to be created, none found", Node2Name)
	}
	assertEndpointsForService(t, r, name, Node2Address)
}

func TestClassifyService_PreservesImmutableFields(t *testing.T) {
	// Simulate an existing Service with Kubernetes-managed immutable fields
	currentSvcList := &corev1.ServiceList{
		Items: []corev1.Service{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "existing-svc",
					Namespace: util.WorkingNamespace,
					Labels: map[string]string{
						raven.LabelCurrentGatewayType:     ravenv1beta1.Proxy,
						util.LabelCurrentGatewayEndpoints: Node1Name,
					},
				},
				Spec: corev1.ServiceSpec{
					Type:                  corev1.ServiceTypeLoadBalancer,
					ClusterIP:             "10.96.1.42",
					ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeLocal,
					HealthCheckNodePort:   30123,
					Ports: []corev1.ServicePort{
						{
							Protocol: corev1.ProtocolTCP,
							Port:     10262,
						},
					},
				},
			},
		},
	}

	// Simulate a desired Service spec (as built by acquiredSpecService) —
	// it does NOT set ClusterIP or HealthCheckNodePort
	specSvcList := &corev1.ServiceList{
		Items: []corev1.Service{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "desired-svc",
					Namespace: util.WorkingNamespace,
					Labels: map[string]string{
						raven.LabelCurrentGatewayType:     ravenv1beta1.Proxy,
						util.LabelCurrentGatewayEndpoints: Node1Name,
					},
				},
				Spec: corev1.ServiceSpec{
					Type:                  corev1.ServiceTypeLoadBalancer,
					ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeLocal,
					Ports: []corev1.ServicePort{
						{
							Protocol: corev1.ProtocolTCP,
							Port:     10263, // changed port
						},
					},
					// ClusterIP deliberately NOT set (zero value "")
					// HealthCheckNodePort deliberately NOT set (zero value 0)
				},
			},
		},
	}

	_, updated, _ := classifyService(currentSvcList, specSvcList)

	if len(updated) != 1 {
		t.Fatalf("expected 1 updated service, got %d", len(updated))
	}

	svc := updated[0]

	// Verify immutable fields are PRESERVED from the existing service
	if svc.Spec.ClusterIP != "10.96.1.42" {
		t.Errorf("expected ClusterIP to be preserved as '10.96.1.42', got '%s'", svc.Spec.ClusterIP)
	}
	if svc.Spec.HealthCheckNodePort != 30123 {
		t.Errorf("expected HealthCheckNodePort to be preserved as 30123, got %d", svc.Spec.HealthCheckNodePort)
	}

	// Verify mutable fields ARE updated from the desired spec
	if len(svc.Spec.Ports) != 1 || svc.Spec.Ports[0].Port != 10263 {
		t.Errorf("expected Port to be updated to 10263, got %v", svc.Spec.Ports)
	}
	if svc.Spec.Type != corev1.ServiceTypeLoadBalancer {
		t.Errorf("expected Type to be LoadBalancer, got %s", svc.Spec.Type)
	}
	if svc.Spec.ExternalTrafficPolicy != corev1.ServiceExternalTrafficPolicyTypeLocal {
		t.Errorf("expected ExternalTrafficPolicy to be Local, got %s", svc.Spec.ExternalTrafficPolicy)
	}
}
