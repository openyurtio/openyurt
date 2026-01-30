/*
Copyright 2025 The OpenYurt Authors.

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

package transport

import (
	"net/url"
	"sync"
	"testing"

	"k8s.io/client-go/kubernetes"
	clientfake "k8s.io/client-go/kubernetes/fake"
)

var (
	testServer1 = &url.URL{Host: "127.0.0.1:6443"}
	testServer2 = &url.URL{Host: "127.0.0.2:6443"}
	testServer3 = &url.URL{Host: "127.0.0.3:6443"}
)

func TestGetDirectClientset(t *testing.T) {
	server1URL := testServer1
	server2URL := testServer2
	client1 := clientfake.NewSimpleClientset()
	client2 := clientfake.NewSimpleClientset()

	fakeClients := map[string]kubernetes.Interface{
		server1URL.String(): client1,
		server2URL.String(): client2,
	}

	manager := NewFakeTransportManager(200, fakeClients)

	testcases := map[string]struct {
		url      *url.URL
		expected kubernetes.Interface
	}{
		"nil url should return nil": {
			url:      nil,
			expected: nil,
		},
		"valid server1 url should return client1": {
			url:      server1URL,
			expected: client1,
		},
		"valid server2 url should return client2": {
			url:      server2URL,
			expected: client2,
		},
		"non-existent server should return nil": {
			url:      testServer3,
			expected: nil,
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			result := manager.GetDirectClientset(tc.url)
			if result != tc.expected {
				t.Errorf("expect %v, but got %v", tc.expected, result)
			}
		})
	}
}

func TestGetDirectClientsetAtRandom(t *testing.T) {
	server1URL := testServer1
	server2URL := testServer2
	client1 := clientfake.NewSimpleClientset()
	client2 := clientfake.NewSimpleClientset()

	fakeClients := map[string]kubernetes.Interface{
		server1URL.String(): client1,
		server2URL.String(): client2,
	}

	manager := NewFakeTransportManager(200, fakeClients)

	result := manager.GetDirectClientsetAtRandom()
	if result == nil {
		t.Errorf("expect a clientset, but got nil")
	}
	if result != client1 && result != client2 {
		t.Errorf("expect one of the clientsets, but got %v", result)
	}

	emptyManager := NewFakeTransportManager(200, map[string]kubernetes.Interface{})
	emptyResult := emptyManager.GetDirectClientsetAtRandom()
	if emptyResult != nil {
		t.Errorf("expect nil for empty map, but got %v", emptyResult)
	}
}

func TestListDirectClientset(t *testing.T) {
	server1URL := testServer1
	server2URL := testServer2
	client1 := clientfake.NewSimpleClientset()
	client2 := clientfake.NewSimpleClientset()

	fakeClients := map[string]kubernetes.Interface{
		server1URL.String(): client1,
		server2URL.String(): client2,
	}

	manager := NewFakeTransportManager(200, fakeClients)

	result := manager.ListDirectClientset()

	if len(result) != 2 {
		t.Errorf("expect 2 clientsets, but got %d", len(result))
	}

	if result[server1URL.String()] != client1 {
		t.Errorf("expect client1 for server1, but got %v", result[server1URL.String()])
	}

	if result[server2URL.String()] != client2 {
		t.Errorf("expect client2 for server2, but got %v", result[server2URL.String()])
	}

	result[testServer3.String()] = clientfake.NewSimpleClientset()

	secondResult := manager.ListDirectClientset()
	if len(secondResult) != 2 {
		t.Errorf("mutating returned map should not affect internal map, expect 2, but got %d", len(secondResult))
	}
}

func TestConcurrentAccess(t *testing.T) {
	server1URL := testServer1
	server2URL := testServer2
	client1 := clientfake.NewSimpleClientset()
	client2 := clientfake.NewSimpleClientset()

	fakeClients := map[string]kubernetes.Interface{
		server1URL.String(): client1,
		server2URL.String(): client2,
	}

	manager := NewFakeTransportManager(200, fakeClients)

	var wg sync.WaitGroup
	concurrency := 50

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = manager.GetDirectClientset(server1URL)
			_ = manager.GetDirectClientsetAtRandom()
			_ = manager.ListDirectClientset()
		}()
	}

	wg.Wait()
}

func TestListDirectClientsetReturnsCopy(t *testing.T) {
	server1URL := testServer1
	client1 := clientfake.NewSimpleClientset()

	fakeClients := map[string]kubernetes.Interface{
		server1URL.String(): client1,
	}

	manager := NewFakeTransportManager(200, fakeClients)

	result1 := manager.ListDirectClientset()
	result2 := manager.ListDirectClientset()

	if &result1 == &result2 {
		t.Errorf("expect different map instances, but got same address")
	}

	if result1[server1URL.String()] != result2[server1URL.String()] {
		t.Errorf("expect same clientset in both maps, but got different values")
	}
}

func TestTransportAndClientManager_GetDirectClientset(t *testing.T) {
	client1 := clientfake.NewSimpleClientset()
	client2 := clientfake.NewSimpleClientset()

	tcm := &transportAndClientManager{
		serverToClientset: map[string]kubernetes.Interface{
			testServer1.String(): client1,
			testServer2.String(): client2,
		},
	}

	testcases := map[string]struct {
		url      *url.URL
		expected kubernetes.Interface
	}{
		"nil url should return nil": {
			url:      nil,
			expected: nil,
		},
		"valid server1 should return client1": {
			url:      testServer1,
			expected: client1,
		},
		"valid server2 should return client2": {
			url:      testServer2,
			expected: client2,
		},
		"non-existent server should return nil": {
			url:      testServer3,
			expected: nil,
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			result := tcm.GetDirectClientset(tc.url)
			if result != tc.expected {
				t.Errorf("expect %v, but got %v", tc.expected, result)
			}
		})
	}
}

func TestTransportAndClientManager_GetDirectClientsetAtRandom(t *testing.T) {
	client1 := clientfake.NewSimpleClientset()
	client2 := clientfake.NewSimpleClientset()

	tcm := &transportAndClientManager{
		serverToClientset: map[string]kubernetes.Interface{
			testServer1.String(): client1,
			testServer2.String(): client2,
		},
	}

	result := tcm.GetDirectClientsetAtRandom()
	if result == nil {
		t.Errorf("expect a clientset, but got nil")
	}
	if result != client1 && result != client2 {
		t.Errorf("expect one of the clientsets, but got %v", result)
	}

	emptyTcm := &transportAndClientManager{
		serverToClientset: map[string]kubernetes.Interface{},
	}
	emptyResult := emptyTcm.GetDirectClientsetAtRandom()
	if emptyResult != nil {
		t.Errorf("expect nil for empty map, but got %v", emptyResult)
	}
}

func TestTransportAndClientManager_ListDirectClientset(t *testing.T) {
	client1 := clientfake.NewSimpleClientset()
	client2 := clientfake.NewSimpleClientset()

	tcm := &transportAndClientManager{
		serverToClientset: map[string]kubernetes.Interface{
			testServer1.String(): client1,
			testServer2.String(): client2,
		},
	}

	result := tcm.ListDirectClientset()

	if len(result) != 2 {
		t.Errorf("expect 2 clientsets, but got %d", len(result))
	}
	if result[testServer1.String()] != client1 {
		t.Errorf("expect client1 for server1, but got %v", result[testServer1.String()])
	}
	if result[testServer2.String()] != client2 {
		t.Errorf("expect client2 for server2, but got %v", result[testServer2.String()])
	}

	result[testServer3.String()] = clientfake.NewSimpleClientset()
	secondResult := tcm.ListDirectClientset()
	if len(secondResult) != 2 {
		t.Errorf("mutating returned map should not affect internal map, expect 2, but got %d", len(secondResult))
	}
}
