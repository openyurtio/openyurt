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

package configuration

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/discardcloudservice"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/forwardkubesvctraffic"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/nodeportisolation"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/serviceenvupdater"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/servicetopology"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

func TestManager(t *testing.T) {
	testcases := map[string]struct {
		nodeName         string
		addCM            *v1.ConfigMap
		updateCM         *v1.ConfigMap
		deleteCM         *v1.ConfigMap
		initAgents       sets.Set[string]
		addedAgents      sets.Set[string]
		updatedAgents    sets.Set[string]
		deletedAgents    sets.Set[string]
		cacheableAgents  []string
		comp             string
		initFilterSet    map[string]sets.Set[string]
		addedFilterSet   map[string]sets.Set[string]
		updatedFilterSet map[string]sets.Set[string]
		deletedFilterSet map[string]sets.Set[string]
	}{
		"check the init status of configuration manager": {
			nodeName:        "foo",
			initAgents:      sets.New[string]("kubelet", "kube-proxy", "flanneld", "coredns", "raven-agent-ds", projectinfo.GetAgentName(), projectinfo.GetHubName(), util.MultiplexerProxyClientUserAgentPrefix+"foo"),
			cacheableAgents: []string{"kubelet", "kube-proxy", "flanneld", "coredns", "raven-agent-ds", projectinfo.GetAgentName(), projectinfo.GetHubName(), util.MultiplexerProxyClientUserAgentPrefix + "foo"},
			initFilterSet: map[string]sets.Set[string]{
				"kubelet/list/pods":              sets.New[string](serviceenvupdater.FilterName),
				"kubelet/watch/pods":             sets.New[string](serviceenvupdater.FilterName),
				"kubelet/get/pods":               sets.New[string](serviceenvupdater.FilterName),
				"kubelet/patch/pods":             sets.New[string](serviceenvupdater.FilterName),
				"kube-proxy/list/endpoints":      sets.New[string](servicetopology.FilterName),
				"kube-proxy/list/endpointslices": sets.New[string](servicetopology.FilterName, forwardkubesvctraffic.FilterName),
				"foo/list/pods":                  sets.New[string](),
			},
		},
		"check the configuration manager status after adding settings": {
			nodeName: "foo",
			addCM: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "yurt-hub-cfg",
					Namespace: "kube-system",
				},
				Data: map[string]string{
					cacheUserAgentsKey:         "foo, bar",
					servicetopology.FilterName: "foo",
				},
			},
			initAgents:      sets.New[string]("kubelet", "kube-proxy", "flanneld", "coredns", "raven-agent-ds", projectinfo.GetAgentName(), projectinfo.GetHubName(), util.MultiplexerProxyClientUserAgentPrefix+"foo"),
			addedAgents:     sets.New[string]("kubelet", "kube-proxy", "flanneld", "coredns", "raven-agent-ds", projectinfo.GetAgentName(), projectinfo.GetHubName(), util.MultiplexerProxyClientUserAgentPrefix+"foo", "foo", "bar"),
			cacheableAgents: []string{"kubelet", "kube-proxy", "flanneld", "coredns", "raven-agent-ds", projectinfo.GetAgentName(), projectinfo.GetHubName(), util.MultiplexerProxyClientUserAgentPrefix + "foo", "foo", "bar"},
			addedFilterSet: map[string]sets.Set[string]{
				"foo/list/endpoints":       sets.New[string](servicetopology.FilterName),
				"foo/watch/endpointslices": sets.New[string](servicetopology.FilterName),
			},
		},
		"check the configuration manager status after adding and updating settings": {
			nodeName: "foo",
			addCM: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "yurt-hub-cfg",
					Namespace: "kube-system",
				},
				Data: map[string]string{
					cacheUserAgentsKey:         "foo, bar",
					servicetopology.FilterName: "bar",
				},
			},
			updateCM: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "yurt-hub-cfg",
					Namespace: "kube-system",
				},
				Data: map[string]string{
					cacheUserAgentsKey:             "foo, zag",
					servicetopology.FilterName:     "zag",
					nodeportisolation.FilterName:   "zag",
					discardcloudservice.FilterName: "zag",
				},
			},
			initAgents:      sets.New[string]("kubelet", "kube-proxy", "flanneld", "coredns", "raven-agent-ds", projectinfo.GetAgentName(), projectinfo.GetHubName(), util.MultiplexerProxyClientUserAgentPrefix+"foo"),
			addedAgents:     sets.New[string]("kubelet", "kube-proxy", "flanneld", "coredns", "raven-agent-ds", projectinfo.GetAgentName(), projectinfo.GetHubName(), util.MultiplexerProxyClientUserAgentPrefix+"foo", "foo", "bar"),
			updatedAgents:   sets.New[string]("kubelet", "kube-proxy", "flanneld", "coredns", "raven-agent-ds", projectinfo.GetAgentName(), projectinfo.GetHubName(), util.MultiplexerProxyClientUserAgentPrefix+"foo", "foo", "zag"),
			cacheableAgents: []string{"kubelet", "kube-proxy", "flanneld", "coredns", "raven-agent-ds", projectinfo.GetAgentName(), projectinfo.GetHubName(), util.MultiplexerProxyClientUserAgentPrefix + "foo", "foo/xxx", "zag/xxx"},
			addedFilterSet: map[string]sets.Set[string]{
				"bar/list/endpoints":       sets.New[string](servicetopology.FilterName),
				"bar/watch/endpointslices": sets.New[string](servicetopology.FilterName),
			},
			updatedFilterSet: map[string]sets.Set[string]{
				"bar/list/endpoints":       sets.New[string](),
				"bar/watch/endpointslices": sets.New[string](),
				"/watch/endpointslices":    sets.New[string](),
				"zag/list/endpoints":       sets.New[string](servicetopology.FilterName),
				"zag/watch/endpointslices": sets.New[string](servicetopology.FilterName),
				"zag/list/services":        sets.New[string](nodeportisolation.FilterName, discardcloudservice.FilterName),
			},
		},
		"check the configuration manager status after adding and updating and deleting settings": {
			nodeName: "foo",
			addCM: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "yurt-hub-cfg",
					Namespace: "kube-system",
				},
				Data: map[string]string{
					cacheUserAgentsKey: "foo, bar",
				},
			},
			updateCM: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "yurt-hub-cfg",
					Namespace: "kube-system",
				},
				Data: map[string]string{
					cacheUserAgentsKey:         "foo, zag",
					servicetopology.FilterName: "zag",
				},
			},
			deleteCM: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "yurt-hub-cfg",
					Namespace: "kube-system",
				},
			},
			initAgents:      sets.New[string]("kubelet", "kube-proxy", "flanneld", "coredns", "raven-agent-ds", projectinfo.GetAgentName(), projectinfo.GetHubName(), util.MultiplexerProxyClientUserAgentPrefix+"foo"),
			addedAgents:     sets.New[string]("kubelet", "kube-proxy", "flanneld", "coredns", "raven-agent-ds", projectinfo.GetAgentName(), projectinfo.GetHubName(), util.MultiplexerProxyClientUserAgentPrefix+"foo", "foo", "bar"),
			updatedAgents:   sets.New[string]("kubelet", "kube-proxy", "flanneld", "coredns", "raven-agent-ds", projectinfo.GetAgentName(), projectinfo.GetHubName(), util.MultiplexerProxyClientUserAgentPrefix+"foo", "foo", "zag"),
			deletedAgents:   sets.New[string]("kubelet", "kube-proxy", "flanneld", "coredns", "raven-agent-ds", projectinfo.GetAgentName(), projectinfo.GetHubName(), util.MultiplexerProxyClientUserAgentPrefix+"foo"),
			cacheableAgents: []string{"kubelet", "kube-proxy", "flanneld", "coredns", "raven-agent-ds", projectinfo.GetAgentName(), projectinfo.GetHubName(), util.MultiplexerProxyClientUserAgentPrefix + "foo"},
			comp:            "zag/xxx",
			updatedFilterSet: map[string]sets.Set[string]{
				"/list/endpoints":       sets.New[string](servicetopology.FilterName),
				"/watch/endpointslices": sets.New[string](servicetopology.FilterName),
			},
			deletedFilterSet: map[string]sets.Set[string]{
				"kube-proxy/list/endpoints":      sets.New[string](servicetopology.FilterName),
				"kube-proxy/list/endpointslices": sets.New[string](servicetopology.FilterName, forwardkubesvctraffic.FilterName),
				"/list/endpoints":                sets.New[string](),
				"/watch/endpointslices":          sets.New[string](),
			},
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			client := fake.NewSimpleClientset()
			informerfactory := informers.NewSharedInformerFactory(client, 0)
			manager := NewConfigurationManager(tc.nodeName, informerfactory)

			stopCh := make(chan struct{})
			informerfactory.Start(stopCh)
			defer close(stopCh)

			if ok := cache.WaitForCacheSync(stopCh, manager.HasSynced); !ok {
				t.Errorf("configuration manager is not ready")
				return
			}

			initAgents := sets.New[string](manager.ListAllCacheAgents()...)
			if !initAgents.Equal(tc.initAgents) {
				t.Errorf("expect init agents %v, but got %v", tc.initAgents.UnsortedList(), initAgents.UnsortedList())
				return
			}

			if len(tc.initFilterSet) != 0 {
				for key, filterSet := range tc.initFilterSet {
					parts := strings.Split(key, "/")
					req := new(http.Request)
					comp := parts[0]
					if len(parts[0]) == 0 && tc.comp != "" {
						comp = tc.comp
					}
					ctx := util.WithClientComponent(context.Background(), comp)
					ctx = apirequest.WithRequestInfo(ctx, &apirequest.RequestInfo{Verb: parts[1], Resource: parts[2]})
					req = req.WithContext(ctx)

					filters := manager.FindFiltersFor(req)
					if !filterSet.Equal(sets.New[string](filters...)) {
						t.Errorf("expect filters %v, but got %v", filterSet.UnsortedList(), filters)
					}
				}
			}

			// add configmap
			if tc.addCM != nil {
				_, err := client.CoreV1().ConfigMaps("kube-system").Create(context.Background(), tc.addCM, metav1.CreateOptions{})
				if err != nil {
					t.Errorf("couldn't create configmap, %v", err)
					return
				}

				time.Sleep(time.Second * 1)
				addedAgents := sets.New[string](manager.ListAllCacheAgents()...)
				if !addedAgents.Equal(tc.addedAgents) {
					t.Errorf("expect added agents %v, but got %v", tc.addedAgents.UnsortedList(), addedAgents.UnsortedList())
					return
				}

				if len(tc.addedFilterSet) != 0 {
					for key, filterSet := range tc.addedFilterSet {
						parts := strings.Split(key, "/")
						req := new(http.Request)
						comp := parts[0]
						if len(parts[0]) == 0 && tc.comp != "" {
							comp = tc.comp
						}
						ctx := util.WithClientComponent(context.Background(), comp)
						ctx = apirequest.WithRequestInfo(ctx, &apirequest.RequestInfo{Verb: parts[1], Resource: parts[2]})
						req = req.WithContext(ctx)

						filters := manager.FindFiltersFor(req)
						if !filterSet.Equal(sets.New[string](filters...)) {
							t.Errorf("expect filters %v, but got %v", filterSet.UnsortedList(), filters)
						}
					}
				}
			}

			// update configmap
			if tc.updateCM != nil {
				_, err := client.CoreV1().ConfigMaps("kube-system").Update(context.Background(), tc.updateCM, metav1.UpdateOptions{})
				if err != nil {
					t.Errorf("couldn't update configmap, %v", err)
					return
				}

				time.Sleep(time.Second * 1)
				updatedAgents := sets.New[string](manager.ListAllCacheAgents()...)
				if !updatedAgents.Equal(tc.updatedAgents) {
					t.Errorf("expect updated agents %v, but got %v", tc.updatedAgents.UnsortedList(), updatedAgents.UnsortedList())
					return
				}

				if len(tc.updatedFilterSet) != 0 {
					for key, filterSet := range tc.updatedFilterSet {
						parts := strings.Split(key, "/")
						req := new(http.Request)
						comp := parts[0]
						if len(parts[0]) == 0 && tc.comp != "" {
							comp = tc.comp
						}
						ctx := util.WithClientComponent(context.Background(), comp)
						ctx = apirequest.WithRequestInfo(ctx, &apirequest.RequestInfo{Verb: parts[1], Resource: parts[2]})
						req = req.WithContext(ctx)

						filters := manager.FindFiltersFor(req)
						if !filterSet.Equal(sets.New[string](filters...)) {
							t.Errorf("expect filters %v, but got %v", filterSet.UnsortedList(), filters)
						}
					}
				}
			}

			// delete configmap
			if tc.deleteCM != nil {
				err := client.CoreV1().ConfigMaps("kube-system").Delete(context.Background(), tc.deleteCM.Name, metav1.DeleteOptions{})
				if err != nil {
					t.Errorf("couldn't delete configmap, %v", err)
					return
				}

				time.Sleep(time.Second * 1)
				deletedAgents := sets.New[string](manager.ListAllCacheAgents()...)
				if !deletedAgents.Equal(tc.deletedAgents) {
					t.Errorf("expect deleted agents %v, but got %v", tc.deletedAgents.UnsortedList(), deletedAgents.UnsortedList())
					return
				}

				if len(tc.deletedFilterSet) != 0 {
					for key, filterSet := range tc.deletedFilterSet {
						parts := strings.Split(key, "/")
						req := new(http.Request)
						comp := parts[0]
						if len(parts[0]) == 0 && tc.comp != "" {
							comp = tc.comp
						}
						ctx := util.WithClientComponent(context.Background(), comp)
						ctx = apirequest.WithRequestInfo(ctx, &apirequest.RequestInfo{Verb: parts[1], Resource: parts[2]})
						req = req.WithContext(ctx)

						filters := manager.FindFiltersFor(req)
						if !filterSet.Equal(sets.New[string](filters...)) {
							t.Errorf("expect filters %v, but got %v", filterSet.UnsortedList(), filters)
						}
					}
				}
			}

			if len(tc.cacheableAgents) != 0 {
				for _, agent := range tc.cacheableAgents {
					if !manager.IsCacheable(agent) {
						t.Errorf("agent(%s) is not cacheable", agent)
						return
					}
				}
			}
		})
	}

}
