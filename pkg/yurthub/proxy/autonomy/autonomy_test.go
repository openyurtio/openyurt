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

package autonomy

import (
	"net/http"
	"net/http/httptest"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"

	appsv1beta1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
	"github.com/openyurtio/openyurt/pkg/yurthub/cachemanager"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/serializer"
	proxyutil "github.com/openyurtio/openyurt/pkg/yurthub/proxy/util"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage/disk"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

var (
	rootDir                   = "/tmp/cache-local"
	fakeClient                = fake.NewSimpleClientset()
	fakeSharedInformerFactory = informers.NewSharedInformerFactory(fakeClient, 0)
)

func TestHttpServeKubeletGetNode(t *testing.T) {
	dStorage, err := disk.NewDiskStorage(rootDir)
	if err != nil {
		t.Errorf("failed to create disk storage, %v", err)
	}
	storageWrapper := cachemanager.NewStorageWrapper(dStorage)
	serializerM := serializer.NewSerializerManager()
	cacheM := cachemanager.NewCacheManager("node1", storageWrapper, serializerM, nil, fakeSharedInformerFactory)

	autonomyProxy := NewAutonomyProxy(nil, cacheM)

	testcases := []struct {
		name string
		info storage.KeyBuildInfo
		node *v1.Node
	}{
		{
			name: "case1",
			info: storage.KeyBuildInfo{
				Group:     "",
				Component: "kubelet",
				Version:   "v1",
				Resources: "nodes",
				Namespace: "default",
				Name:      "node1",
			},
			node: &v1.Node{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Node",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "node1",
					Namespace: "default",
				},
			},
		},
		{
			name: "case2",
			info: storage.KeyBuildInfo{
				Group:     "",
				Version:   "v1",
				Component: "kubelet",
				Resources: "nodes",
				Namespace: "default",
				Name:      "node2",
			},
			node: &v1.Node{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Node",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "node2",
					Namespace: "default",
					Annotations: map[string]string{
						"node.beta.openyurt.io/autonomy": "true",
					},
				},
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			key, _ := dStorage.KeyFunc(tc.info)
			storageWrapper.Create(key, tc.node)
			resp := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", "", nil)
			req.Header.Set("User-Agent", "kubelet")
			ctx := apirequest.WithRequestInfo(req.Context(), &apirequest.RequestInfo{
				IsResourceRequest: true,
				Namespace:         tc.info.Namespace,
				Resource:          tc.info.Resources,
				Name:              tc.info.Name,
				Verb:              "get",
				APIVersion:        "v1",
			})
			handler := proxyutil.WithRequestClientComponent(autonomyProxy, util.WorkingModeEdge)
			handler = proxyutil.WithRequestContentType(handler)
			req = req.WithContext(ctx)
			handler.ServeHTTP(resp, req)
			if resp.Result().StatusCode != http.StatusOK {
				t.Errorf("failed to get node, %v", resp.Result().StatusCode)
			}
		})
	}
}

func TestSetNodeAutonomyCondition(t *testing.T) {
	testcases := []struct {
		name           string
		node           *v1.Node
		expectedStatus v1.ConditionStatus
	}{
		{
			name: "case1",
			node: &v1.Node{
				Status: v1.NodeStatus{
					Conditions: []v1.NodeCondition{
						{
							Type:   v1.NodeReady,
							Status: v1.ConditionTrue,
						},
					},
				},
			},
			expectedStatus: v1.ConditionTrue,
		},
		{
			name: "case2",
			node: &v1.Node{
				Status: v1.NodeStatus{
					Conditions: []v1.NodeCondition{
						{
							Type:   appsv1beta1.NodeAutonomy,
							Status: v1.ConditionTrue,
						},
					},
				},
			},
			expectedStatus: v1.ConditionTrue,
		},
		{
			name: "case3",
			node: &v1.Node{
				Status: v1.NodeStatus{
					Conditions: []v1.NodeCondition{
						{
							Type:   appsv1beta1.NodeAutonomy,
							Status: v1.ConditionFalse,
						},
					},
				},
			},
			expectedStatus: v1.ConditionTrue,
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			setNodeAutonomyCondition(tc.node, tc.expectedStatus, "", "")
			for _, condition := range tc.node.Status.Conditions {
				if condition.Type == appsv1beta1.NodeAutonomy && condition.Status != tc.expectedStatus {
					t.Error("failed to set node autonomy status")
				}
			}
		})
	}
}
