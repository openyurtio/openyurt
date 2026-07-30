/*
Copyright 2022 The OpenYurt Authors.

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

package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/informers"
	admissionregistration "k8s.io/client-go/informers/admissionregistration"
	apiserverinternal "k8s.io/client-go/informers/apiserverinternal"
	apps "k8s.io/client-go/informers/apps"
	autoscaling "k8s.io/client-go/informers/autoscaling"
	batch "k8s.io/client-go/informers/batch"
	certificates "k8s.io/client-go/informers/certificates"
	coordination "k8s.io/client-go/informers/coordination"
	coreinformers "k8s.io/client-go/informers/core"
	corev1informers "k8s.io/client-go/informers/core/v1"
	discovery "k8s.io/client-go/informers/discovery"
	events "k8s.io/client-go/informers/events"
	extensions "k8s.io/client-go/informers/extensions"
	flowcontrol "k8s.io/client-go/informers/flowcontrol"
	internalinterfaces "k8s.io/client-go/informers/internalinterfaces"
	networking "k8s.io/client-go/informers/networking"
	node "k8s.io/client-go/informers/node"
	policy "k8s.io/client-go/informers/policy"
	rbac "k8s.io/client-go/informers/rbac"
	resource "k8s.io/client-go/informers/resource"
	scheduling "k8s.io/client-go/informers/scheduling"
	storage "k8s.io/client-go/informers/storage"
	storagemigration "k8s.io/client-go/informers/storagemigration"
	"k8s.io/client-go/kubernetes/fake"
	listersv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"

	otautil "github.com/openyurtio/openyurt/pkg/yurthub/otaupdate/util"
)

func TestGetPodList_Success(t *testing.T) {
	pod1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod1",
			Namespace: "default",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}
	pod2 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod2",
			Namespace: "default",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
		},
	}

	fakeClientset := fake.NewSimpleClientset(pod1, pod2)
	factory := informers.NewSharedInformerFactory(fakeClientset, 0)

	factory.Core().V1().Pods().Informer()

	stopCh := make(chan struct{})
	defer close(stopCh)
	factory.Start(stopCh)
	factory.WaitForCacheSync(stopCh)

	handler := getPodList(factory)

	req := httptest.NewRequest(http.MethodGet, "/pods", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var got, expected corev1.PodList
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	expectedPodList := &corev1.PodList{}
	for _, p := range []*corev1.Pod{pod1, pod2} {
		expectedPodList.Items = append(expectedPodList.Items, *p)
	}
	expectedData, err := otautil.EncodePods(expectedPodList)
	if err != nil {
		t.Fatalf("Failed to encode expected pod list: %v", err)
	}
	if err := json.Unmarshal(expectedData, &expected); err != nil {
		t.Fatalf("Failed to decode expected: %v", err)
	}

	if len(got.Items) != len(expected.Items) {
		t.Errorf("Expected %d pods, got %d", len(expected.Items), len(got.Items))
	}
	gotByName := make(map[string]bool)
	for _, p := range got.Items {
		gotByName[p.Name] = true
	}
	for _, p := range expected.Items {
		if !gotByName[p.Name] {
			t.Errorf("Expected pod %s not found in response", p.Name)
		}
	}
}

func TestGetPodList_ListError(t *testing.T) {
	factory := &mockFactory{
		core: &mockCore{
			v1iface: &mockV1Interface{
				podInformer: &mockPodInformer{
					lister: &mockPodLister{
						err: fmt.Errorf("list error"),
					},
				},
			},
		},
	}

	handler := getPodList(factory)

	req := httptest.NewRequest(http.MethodGet, "/pods", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}

	if !strings.Contains(w.Body.String(), "Get pods key failed") {
		t.Errorf("Expected body to contain 'Get pods key failed', got: %s", w.Body.String())
	}
}

func TestGetPodList_EncodeError(t *testing.T) {
	defer func(old func(*corev1.PodList) ([]byte, error)) {
		encodePodsFn = old
	}(encodePodsFn)

	encodePodsFn = func(pl *corev1.PodList) ([]byte, error) {
		return nil, fmt.Errorf("encode error")
	}

	factory := &mockFactory{
		core: &mockCore{
			v1iface: &mockV1Interface{
				podInformer: &mockPodInformer{
					lister: &mockPodLister{
						pods: []*corev1.Pod{
							{
								ObjectMeta: metav1.ObjectMeta{
									Name:      "pod1",
									Namespace: "default",
								},
							},
						},
					},
				},
			},
		},
	}

	handler := getPodList(factory)
	req := httptest.NewRequest(http.MethodGet, "/pods", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}

	if !strings.Contains(w.Body.String(), "Encode pod list failed") {
		t.Errorf("Expected body to contain 'Encode pod list failed', got: %s", w.Body.String())
	}
}

type mockPodLister struct {
	pods []*corev1.Pod
	err  error
}

func (m *mockPodLister) List(selector labels.Selector) ([]*corev1.Pod, error) {
	return m.pods, m.err
}

func (m *mockPodLister) Pods(namespace string) listersv1.PodNamespaceLister {
	return nil
}

type mockPodInformer struct {
	lister listersv1.PodLister
}

func (m *mockPodInformer) Informer() cache.SharedIndexInformer {
	return nil
}

func (m *mockPodInformer) Lister() listersv1.PodLister {
	return m.lister
}

type mockV1Interface struct {
	podInformer corev1informers.PodInformer
}

func (m *mockV1Interface) Pods() corev1informers.PodInformer {
	return m.podInformer
}
func (m *mockV1Interface) ComponentStatuses() corev1informers.ComponentStatusInformer {
	return nil
}
func (m *mockV1Interface) ConfigMaps() corev1informers.ConfigMapInformer   { return nil }
func (m *mockV1Interface) Endpoints() corev1informers.EndpointsInformer    { return nil }
func (m *mockV1Interface) Events() corev1informers.EventInformer           { return nil }
func (m *mockV1Interface) LimitRanges() corev1informers.LimitRangeInformer { return nil }
func (m *mockV1Interface) Namespaces() corev1informers.NamespaceInformer   { return nil }
func (m *mockV1Interface) Nodes() corev1informers.NodeInformer             { return nil }
func (m *mockV1Interface) PersistentVolumes() corev1informers.PersistentVolumeInformer {
	return nil
}
func (m *mockV1Interface) PersistentVolumeClaims() corev1informers.PersistentVolumeClaimInformer {
	return nil
}
func (m *mockV1Interface) PodTemplates() corev1informers.PodTemplateInformer { return nil }
func (m *mockV1Interface) ReplicationControllers() corev1informers.ReplicationControllerInformer {
	return nil
}
func (m *mockV1Interface) ResourceQuotas() corev1informers.ResourceQuotaInformer { return nil }
func (m *mockV1Interface) Secrets() corev1informers.SecretInformer               { return nil }
func (m *mockV1Interface) Services() corev1informers.ServiceInformer             { return nil }
func (m *mockV1Interface) ServiceAccounts() corev1informers.ServiceAccountInformer {
	return nil
}

type mockCore struct {
	v1iface corev1informers.Interface
}

func (m *mockCore) V1() corev1informers.Interface {
	return m.v1iface
}

type mockFactory struct {
	core coreinformers.Interface
}

func (m *mockFactory) Core() coreinformers.Interface { return m.core }
func (m *mockFactory) Start(stopCh <-chan struct{})  {}
func (m *mockFactory) Shutdown()                     {}
func (m *mockFactory) WaitForCacheSync(stopCh <-chan struct{}) map[reflect.Type]bool {
	return nil
}
func (m *mockFactory) ForResource(resource schema.GroupVersionResource) (informers.GenericInformer, error) {
	return nil, nil
}
func (m *mockFactory) InformerFor(obj runtime.Object, newFunc internalinterfaces.NewInformerFunc) cache.SharedIndexInformer {
	return nil
}
func (m *mockFactory) Admissionregistration() admissionregistration.Interface { return nil }
func (m *mockFactory) Internal() apiserverinternal.Interface                  { return nil }
func (m *mockFactory) Apps() apps.Interface                                   { return nil }
func (m *mockFactory) Autoscaling() autoscaling.Interface                     { return nil }
func (m *mockFactory) Batch() batch.Interface                                 { return nil }
func (m *mockFactory) Certificates() certificates.Interface                   { return nil }
func (m *mockFactory) Coordination() coordination.Interface                   { return nil }
func (m *mockFactory) Discovery() discovery.Interface                         { return nil }
func (m *mockFactory) Events() events.Interface                               { return nil }
func (m *mockFactory) Extensions() extensions.Interface                       { return nil }
func (m *mockFactory) Flowcontrol() flowcontrol.Interface                     { return nil }
func (m *mockFactory) Networking() networking.Interface                       { return nil }
func (m *mockFactory) Node() node.Interface                                   { return nil }
func (m *mockFactory) Policy() policy.Interface                               { return nil }
func (m *mockFactory) Rbac() rbac.Interface                                   { return nil }
func (m *mockFactory) Resource() resource.Interface                           { return nil }
func (m *mockFactory) Scheduling() scheduling.Interface                       { return nil }
func (m *mockFactory) Storage() storage.Interface                             { return nil }
func (m *mockFactory) Storagemigration() storagemigration.Interface           { return nil }
