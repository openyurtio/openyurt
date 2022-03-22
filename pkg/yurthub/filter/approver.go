/*
Copyright 2021 The OpenYurt Authors.

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

package filter

import (
	"fmt"
	"net/http"
	"strings"
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

type requestInfo struct {
	comp     string
	resource string
	verbs    sets.String
}

type approver struct {
	sync.Mutex
	nameToRequests    map[string][]*requestInfo
	whiteListRequests []*requestInfo
	configMapSynced   cache.InformerSynced
	stopCh            chan struct{}
}

var (
	defaultWhiteListRequests = []*requestInfo{
		{
			comp:     projectinfo.GetHubName(),
			resource: "configmaps",
			verbs:    sets.NewString("list", "watch"),
		},
	}
	defaultFilterCfg = map[string]string{
		MasterServiceFilterName:       "kubelet/services#list;watch",
		EndpointsFilterName:           "nginx-ingress-controller/endpoints#list;watch",
		DiscardCloudServiceFilterName: "kube-proxy/services#list;watch",
		ServiceTopologyFilterName:     "kube-proxy/endpointslices#list;watch",
	}
)

func newApprover(sharedFactory informers.SharedInformerFactory) *approver {
	configMapInformer := sharedFactory.Core().V1().ConfigMaps().Informer()
	na := &approver{
		nameToRequests:    make(map[string][]*requestInfo),
		configMapSynced:   configMapInformer.HasSynced,
		whiteListRequests: make([]*requestInfo, 0),
		stopCh:            make(chan struct{}),
	}
	na.whiteListRequests = append(na.whiteListRequests, defaultWhiteListRequests...)
	configMapInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    na.addConfigMap,
		UpdateFunc: na.updateConfigMap,
	})
	return na
}

func (a *approver) Approve(comp, resource, verb string) bool {
	if a.isWhitelistReq(comp, resource, verb) {
		return false
	}
	if ok := cache.WaitForCacheSync(a.stopCh, a.configMapSynced); !ok {
		panic("wait for configMap cache sync timeout")
	}
	a.Lock()
	defer a.Unlock()
	for _, requests := range a.nameToRequests {
		for _, request := range requests {
			if request.Equal(comp, resource, verb) {

				return true
			}
		}
	}
	return false
}

func (a *approver) GetFilterName(req *http.Request) string {
	ctx := req.Context()
	comp, ok := util.ClientComponentFrom(ctx)
	if !ok {
		return ""
	}

	info, ok := apirequest.RequestInfoFrom(ctx)
	if !ok {
		return ""
	}
	a.Lock()
	defer a.Unlock()
	for name, requests := range a.nameToRequests {
		for _, request := range requests {
			if request.Equal(comp, info.Resource, info.Verb) {
				return name
			}
		}
	}
	return ""
}

// Determine whether it is a whitelist resource
func (a *approver) isWhitelistReq(comp, resource, verb string) bool {
	for _, req := range a.whiteListRequests {
		if req.Equal(comp, resource, verb) {
			return true
		}
	}
	return false
}

// Parse the special format of filter config: filter_{name}: user-agent1/resource#verb1;verb2, user-agent2/resource#verb1;verb2.
func (a *approver) parseFilterConfig(cfg string, filterType string) []*requestInfo {
	var reqs []*requestInfo
	cfg = a.mergeFilterConfig(cfg, filterType)
	for _, configArr := range strings.Split(cfg, ",") {
		cfg := strings.Split(configArr, "#")
		if len(cfg) != 2 {
			continue
		}

		v := strings.Split(cfg[0], "/")
		if len(v) != 2 {
			continue
		}

		req := &requestInfo{
			comp:     v[0],
			resource: v[1],
			verbs:    sets.NewString(strings.Split(cfg[1], ";")...),
		}
		reqs = append(reqs, req)
	}
	return reqs
}

// merge default filter to custom filter
func (a *approver) mergeFilterConfig(cfg, filterType string) string {
	if config, ok := defaultFilterCfg[filterType]; ok {
		if len(cfg) != 0 {
			return fmt.Sprintf("%v,%v", config, cfg)
		}
	}

	return cfg
}

func (a *approver) addConfigMap(obj interface{}) {
	cfg, ok := obj.(*corev1.ConfigMap)
	if !ok {
		return
	}
	for key, value := range cfg.Data {
		if strings.HasPrefix(key, "filter_") {
			a.updateYurtHubFilterCfg(key, value, "add")
		}
	}
}

func (a *approver) updateConfigMap(oldObj, newObj interface{}) {
	oldCfg, ok := oldObj.(*corev1.ConfigMap)
	if !ok {
		return
	}

	newCfg, ok := newObj.(*corev1.ConfigMap)
	if !ok {
		return
	}

	for key, value := range newCfg.Data {
		if _, ok := oldCfg.Data[key]; !ok {
			if strings.HasPrefix(key, "filter_") {
				a.updateYurtHubFilterCfg(key, value, "update")
				continue
			}
		}

		if oldCfg.Data[key] != value {
			if strings.HasPrefix(key, "filter_") {
				a.updateYurtHubFilterCfg(key, value, "update")
			}
		}
	}
}

// update filter cfg
func (a *approver) updateYurtHubFilterCfg(filterCfgKey, filterCfgValue, action string) {
	a.Lock()
	defer a.Unlock()
	s := strings.Split(filterCfgKey, "_")
	a.nameToRequests[s[1]] = a.parseFilterConfig(filterCfgValue, s[1])
	klog.Infof("current filter config: %v after %s", a.nameToRequests, action)
}

// Judge whether the request is allowed to be filtered
func (req *requestInfo) Equal(comp, resource, verb string) bool {
	if req.comp == comp && req.resource == resource && req.verbs.Has(verb) {
		return true
	}

	return false
}
