/*
Copyright 2024 The OpenYurt Authors.

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

package multiplexer

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apiserver/pkg/endpoints/filters"
	"k8s.io/apiserver/pkg/server"
	kstorage "k8s.io/apiserver/pkg/storage"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/config"
	fakeHealthChecker "github.com/openyurtio/openyurt/pkg/yurthub/healthchecker/fake"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/meta"
	"github.com/openyurtio/openyurt/pkg/yurthub/multiplexer/storage"
	"github.com/openyurtio/openyurt/pkg/yurthub/proxy/remote"
	"github.com/openyurtio/openyurt/pkg/yurthub/transport"
)

func newService(namespace, name string) *v1.Service {
	return &v1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
	}
}

func TestShareCacheManager_ResourceCache(t *testing.T) {
	svcStorage := storage.NewFakeServiceStorage(
		[]v1.Service{
			*newService(metav1.NamespaceSystem, "coredns"),
			*newService(metav1.NamespaceDefault, "nginx"),
		})

	storageMap := map[string]kstorage.Interface{
		serviceGVR.String(): svcStorage,
	}
	dsm := storage.NewDummyStorageManager(storageMap)

	tmpDir, err := os.MkdirTemp("", "test")
	if err != nil {
		t.Fatalf("failed to make temp dir, %v", err)
	}
	restMapperManager, _ := meta.NewRESTMapperManager(tmpDir)

	poolScopeResources := []schema.GroupVersionResource{
		{Group: "", Version: "v1", Resource: "services"},
		{Group: "discovery.k8s.io", Version: "v1", Resource: "endpointslices"},
	}

	clientset := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(clientset, 0)

	healthChecher := fakeHealthChecker.NewFakeChecker(map[*url.URL]bool{})
	loadBalancer := remote.NewLoadBalancer("round-robin", []*url.URL{}, nil, nil, healthChecher, nil, context.Background().Done())
	cfg := &config.YurtHubConfiguration{
		PoolScopeResources:       poolScopeResources,
		RESTMapperManager:        restMapperManager,
		SharedFactory:            factory,
		LoadBalancerForLeaderHub: loadBalancer,
	}

	scm := NewRequestMultiplexerManager(cfg, dsm, healthChecher)
	cache, _, _ := scm.ResourceCache(serviceGVR)
	wait.PollUntilContextCancel(context.Background(), 100*time.Millisecond, true, func(context.Context) (done bool, err error) {
		if cache.ReadinessCheck() == nil {
			return true, nil
		}
		return false, nil
	})

	serviceList := &v1.ServiceList{}
	err = cache.GetList(context.Background(), "", mockListOptions(), serviceList)

	assert.Nil(t, err)
	assert.Equal(t, []v1.Service{
		*newService(metav1.NamespaceDefault, "nginx"),
		*newService(metav1.NamespaceSystem, "coredns"),
	}, serviceList.Items)
}

func TestSyncConfigMap(t *testing.T) {
	multiplexerPort := 10269
	poolName := "foo"
	nodeName := "node1"
	poolScopeResources := []schema.GroupVersionResource{
		{Group: "", Version: "v1", Resource: "services"},
		{Group: "discovery.k8s.io", Version: "v1", Resource: "endpointslices"},
	}
	transportMgr := transport.NewFakeTransportManager(http.StatusOK, map[string]kubernetes.Interface{})

	testcases := map[string]struct {
		addCM                    *v1.ConfigMap
		resultSource             string
		resultRequest            map[string]bool
		resultServerHosts        sets.Set[string]
		updateCM                 *v1.ConfigMap
		updatedResultSource      string
		updatedResultRequest     map[string]bool
		updatedResultServerHosts sets.Set[string]
	}{
		"no leader hub endpoints": {
			addCM: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("leader-hub-%s", poolName),
					Namespace: "kube-system",
				},
				Data: map[string]string{
					"pool-scoped-metadata":   "/v1/services,/v1/nodes",
					"enable-leader-election": "true",
				},
			},
			resultSource: "api",
			resultRequest: map[string]bool{
				"/api/v1/nodes":                       true,
				"/discovery.k8s.io/v1/endpointslices": false,
			},
			resultServerHosts: sets.New[string](),
		},
		"only one leader hub endpoints": {
			addCM: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("leader-hub-%s", poolName),
					Namespace: "kube-system",
				},
				Data: map[string]string{
					"pool-scoped-metadata":   "/v1/services,/v1/nodes",
					"enable-leader-election": "true",
					"leaders":                "test-node1/192.168.1.1",
				},
			},
			resultSource: "pool",
			resultRequest: map[string]bool{
				"/api/v1/nodes":                       true,
				"/discovery.k8s.io/v1/endpointslices": false,
			},
			resultServerHosts: sets.New[string]("192.168.1.1:10269"),
		},
		"multiple leader hub endpoints": {
			addCM: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("leader-hub-%s", poolName),
					Namespace: "kube-system",
				},
				Data: map[string]string{
					"pool-scoped-metadata":   "/v1/services,/v1/nodes,openyurt.io/v1/bars",
					"enable-leader-election": "true",
					"leaders":                "test-node1/192.168.1.1,test-node2/192.168.1.2",
				},
			},
			resultSource: "pool",
			resultRequest: map[string]bool{
				"/api/v1/nodes": true,
				"/apis/discovery.k8s.io/v1/endpointslices": false,
				"/apis/openyurt.io/v1/bars":                true,
			},
			resultServerHosts: sets.New[string]("192.168.1.1:10269", "192.168.1.2:10269"),
		},
		"multiple leader hub endpoints include node": {
			addCM: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("leader-hub-%s", poolName),
					Namespace: "kube-system",
				},
				Data: map[string]string{
					"pool-scoped-metadata":   "/v1/services,openyurt.io/v1/foos",
					"enable-leader-election": "true",
					"leaders":                "test-node1/192.168.1.1,node1/192.168.1.2",
				},
			},
			resultSource: "api",
			resultRequest: map[string]bool{
				"/api/v1/nodes": false,
				"/apis/discovery.k8s.io/v1/endpointslices": false,
				"/apis/openyurt.io/v1/foos":                true,
			},
			resultServerHosts: sets.New[string]("192.168.1.1:10269", "192.168.1.2:10269"),
		},
		"update enable leader election from true to false": {
			addCM: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("leader-hub-%s", poolName),
					Namespace: "kube-system",
				},
				Data: map[string]string{
					"pool-scoped-metadata":   "/v1/services,/v1/nodes,openyurt.io/v1/bars",
					"enable-leader-election": "true",
					"leaders":                "test-node1/192.168.1.1,test-node2/192.168.1.2",
				},
			},
			resultSource: "pool",
			resultRequest: map[string]bool{
				"/api/v1/nodes": true,
				"/apis/discovery.k8s.io/v1/endpointslices": false,
				"/apis/openyurt.io/v1/bars":                true,
			},
			resultServerHosts: sets.New[string]("192.168.1.1:10269", "192.168.1.2:10269"),
			updateCM: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("leader-hub-%s", poolName),
					Namespace: "kube-system",
				},
				Data: map[string]string{
					"pool-scoped-metadata":   "/v1/services,openyurt.io/v1/foos",
					"enable-leader-election": "false",
					"leaders":                "test-node1/192.168.1.1,test-node2/192.168.1.2",
				},
			},
			updatedResultSource: "api",
			updatedResultRequest: map[string]bool{
				"/api/v1/nodes": false,
				"/apis/discovery.k8s.io/v1/endpointslices": false,
				"/apis/openyurt.io/v1/bars":                false,
				"/apis/openyurt.io/v1/foos":                true,
			},
			updatedResultServerHosts: sets.New[string]("192.168.1.1:10269", "192.168.1.2:10269"),
		},
		"update leader hub endpoints from 2 to 3": {
			addCM: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("leader-hub-%s", poolName),
					Namespace: "kube-system",
				},
				Data: map[string]string{
					"pool-scoped-metadata":   "/v1/services,/v1/nodes,openyurt.io/v1/bars",
					"enable-leader-election": "true",
					"leaders":                "test-node1/192.168.1.1,test-node2/192.168.1.2",
				},
			},
			resultSource: "pool",
			resultRequest: map[string]bool{
				"/api/v1/nodes": true,
				"/apis/discovery.k8s.io/v1/endpointslices": false,
				"/apis/openyurt.io/v1/bars":                true,
			},
			resultServerHosts: sets.New[string]("192.168.1.1:10269", "192.168.1.2:10269"),
			updateCM: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("leader-hub-%s", poolName),
					Namespace: "kube-system",
				},
				Data: map[string]string{
					"pool-scoped-metadata":   "/v1/services,openyurt.io/v1/foos",
					"enable-leader-election": "true",
					"leaders":                "test-node1/192.168.1.1,test-node2/192.168.1.2, test-node3/192.168.1.3",
				},
			},
			updatedResultSource: "pool",
			updatedResultRequest: map[string]bool{
				"/api/v1/nodes": false,
				"/apis/discovery.k8s.io/v1/endpointslices": false,
				"/apis/openyurt.io/v1/bars":                false,
				"/apis/openyurt.io/v1/foos":                true,
			},
			updatedResultServerHosts: sets.New[string]("192.168.1.1:10269", "192.168.1.2:10269", "192.168.1.3:10269"),
		},
		"enable leader election and pool scope metadata are not updated": {
			addCM: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("leader-hub-%s", poolName),
					Namespace: "kube-system",
				},
				Data: map[string]string{
					"pool-scoped-metadata":   "/v1/services,/v1/nodes,openyurt.io/v1/bars",
					"enable-leader-election": "true",
					"leaders":                "test-node1/192.168.1.1,test-node2/192.168.1.2",
				},
			},
			resultSource: "pool",
			resultRequest: map[string]bool{
				"/api/v1/nodes": true,
				"/apis/discovery.k8s.io/v1/endpointslices": false,
				"/apis/openyurt.io/v1/bars":                true,
			},
			resultServerHosts: sets.New[string]("192.168.1.1:10269", "192.168.1.2:10269"),
			updateCM: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("leader-hub-%s", poolName),
					Namespace: "kube-system",
				},
				Data: map[string]string{
					"pool-scoped-metadata":   "/v1/services,/v1/nodes,openyurt.io/v1/bars",
					"enable-leader-election": "true",
					"leaders":                "test-node2/192.168.1.2",
				},
			},
			updatedResultSource: "pool",
			updatedResultRequest: map[string]bool{
				"/api/v1/nodes": true,
				"/apis/discovery.k8s.io/v1/endpointslices": false,
				"/apis/openyurt.io/v1/bars":                true,
				"/apis/openyurt.io/v1/foos":                false,
			},
			updatedResultServerHosts: sets.New[string]("192.168.1.2:10269"),
		},
	}

	cfg := &server.Config{
		LegacyAPIGroupPrefixes: sets.NewString(server.DefaultLegacyAPIPrefix),
	}
	resolver := server.NewRequestInfoResolver(cfg)

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			stopCh := make(chan struct{})
			client := fake.NewSimpleClientset()
			informerFactory := informers.NewSharedInformerFactory(client, 0)
			checker := fakeHealthChecker.NewFakeChecker(map[*url.URL]bool{})
			lb := remote.NewLoadBalancer("round-robin", []*url.URL{}, nil, transportMgr, checker, nil, stopCh)

			cfg := &config.YurtHubConfiguration{
				PortForMultiplexer:       multiplexerPort,
				NodeName:                 nodeName,
				NodePoolName:             poolName,
				PoolScopeResources:       poolScopeResources,
				SharedFactory:            informerFactory,
				LoadBalancerForLeaderHub: lb,
			}
			m := NewRequestMultiplexerManager(cfg, nil, checker)

			informerFactory.Start(stopCh)
			defer close(stopCh)

			if ok := cache.WaitForCacheSync(stopCh, m.HasSynced); !ok {
				t.Errorf("configmap is not synced")
				return
			}

			if tc.addCM != nil {
				_, err := client.CoreV1().ConfigMaps("kube-system").Create(context.Background(), tc.addCM, metav1.CreateOptions{})
				if err != nil {
					t.Errorf("couldn't create configmap, %v", err)
					return
				}

				time.Sleep(1 * time.Second)
				sourceForPoolScopeMetadata := m.SourceForPoolScopeMetadata()
				if tc.resultSource != sourceForPoolScopeMetadata {
					t.Errorf("expect sourceForPoolScopeMetadata %s, but got %s", tc.resultSource, sourceForPoolScopeMetadata)
					return
				}

				for path, isRequestForPoolScopeMetadata := range tc.resultRequest {
					req, _ := http.NewRequest("GET", path, nil)

					var capturedRequest *http.Request
					handler := filters.WithRequestInfo(http.HandlerFunc(
						func(w http.ResponseWriter, r *http.Request) {
							capturedRequest = r
						}), resolver)
					recorder := httptest.NewRecorder()

					handler.ServeHTTP(recorder, req)

					if m.IsRequestForPoolScopeMetadata(capturedRequest) != isRequestForPoolScopeMetadata {
						t.Errorf("path(%s): expect isRequestForPoolScopeMetadata %v, but got %v", path, isRequestForPoolScopeMetadata, m.IsRequestForPoolScopeMetadata(capturedRequest))
						return
					}
				}
				fakeChecker := checker.(*fakeHealthChecker.FakeChecker)
				addedServerHosts := fakeChecker.ListServerHosts()
				if !tc.resultServerHosts.Equal(addedServerHosts) {
					t.Errorf("expect server hosts %+v, but got %+v", tc.resultServerHosts, addedServerHosts)
				}
			}

			if tc.updateCM != nil {
				_, err := client.CoreV1().ConfigMaps("kube-system").Update(context.Background(), tc.updateCM, metav1.UpdateOptions{})
				if err != nil {
					t.Errorf("couldn't update configmap, %v", err)
					return
				}

				time.Sleep(1 * time.Second)
				sourceForPoolScopeMetadata := m.SourceForPoolScopeMetadata()
				if tc.updatedResultSource != sourceForPoolScopeMetadata {
					t.Errorf("expect sourceForPoolScopeMetadata %s, but got %s", tc.updatedResultSource, sourceForPoolScopeMetadata)
					return
				}

				for path, isRequestForPoolScopeMetadata := range tc.updatedResultRequest {
					req, _ := http.NewRequest("GET", path, nil)

					var updatedCapturedRequest *http.Request
					handler := filters.WithRequestInfo(http.HandlerFunc(
						func(w http.ResponseWriter, r *http.Request) {
							updatedCapturedRequest = r
						}), resolver)
					recorder := httptest.NewRecorder()

					handler.ServeHTTP(recorder, req)

					if m.IsRequestForPoolScopeMetadata(updatedCapturedRequest) != isRequestForPoolScopeMetadata {
						t.Errorf("path(%s): expect isRequestForPoolScopeMetadata %v, but got %v", path, isRequestForPoolScopeMetadata, m.IsRequestForPoolScopeMetadata(updatedCapturedRequest))
						return
					}
				}
				fakeChecker := checker.(*fakeHealthChecker.FakeChecker)
				updatedServerHosts := fakeChecker.ListServerHosts()
				if !tc.updatedResultServerHosts.Equal(updatedServerHosts) {
					t.Errorf("expect server hosts %+v, but got %+v", tc.updatedResultServerHosts, updatedServerHosts)
				}
			}
		})
	}
}
