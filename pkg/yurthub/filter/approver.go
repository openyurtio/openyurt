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
	"reflect"
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

type approver struct {
	sync.Mutex
	reqKeyToNames                      map[string]sets.String
	configMapSynced                    cache.InformerSynced
	supportedResourceAndVerbsForFilter map[string]map[string]sets.String
	defaultReqKeyToNames               map[string]sets.String
	stopCh                             chan struct{}
}

var (
	// defaultBlackListRequests is used for requests that don't need to be filtered.
	defaultBlackListRequests = sets.NewString(reqKey(projectinfo.GetHubName(), "configmaps", "list"), reqKey(projectinfo.GetHubName(), "configmaps", "watch"))
)

func NewApprover(sharedFactory informers.SharedInformerFactory, filterSupportedResAndVerbs map[string]map[string]sets.String) Approver {
	configMapInformer := sharedFactory.Core().V1().ConfigMaps().Informer()
	na := &approver{
		reqKeyToNames:                      make(map[string]sets.String),
		configMapSynced:                    configMapInformer.HasSynced,
		supportedResourceAndVerbsForFilter: filterSupportedResAndVerbs,
		defaultReqKeyToNames:               make(map[string]sets.String),
		stopCh:                             make(chan struct{}),
	}

	for name, setting := range SupportedComponentsForFilter {
		for _, key := range na.parseRequestSetting(name, setting) {
			if _, ok := na.defaultReqKeyToNames[key]; !ok {
				na.defaultReqKeyToNames[key] = sets.NewString()
			}
			na.defaultReqKeyToNames[key].Insert(name)
		}
	}

	na.merge("init", na.defaultReqKeyToNames)
	configMapInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    na.addConfigMap,
		UpdateFunc: na.updateConfigMap,
		DeleteFunc: na.deleteConfigMap,
	})
	return na
}

func (a *approver) Approve(req *http.Request) (bool, []string) {
	filterNames := make([]string, 0)
	key := getKeyByRequest(req)
	if len(key) == 0 {
		return false, filterNames
	}

	if defaultBlackListRequests.Has(key) {
		return false, filterNames
	}

	if ok := cache.WaitForCacheSync(a.stopCh, a.configMapSynced); !ok {
		return false, filterNames
	}

	a.Lock()
	defer a.Unlock()
	if nameSetting, ok := a.reqKeyToNames[key]; ok {
		return true, nameSetting.UnsortedList()
	}

	return false, filterNames
}

func (a *approver) addConfigMap(obj interface{}) {
	cm, ok := obj.(*corev1.ConfigMap)
	if !ok {
		return
	}

	// get reqKeyToNames of user request setting from configmap
	reqKeyToNamesFromCM := make(map[string]sets.String)
	for key, setting := range cm.Data {
		if filterName, ok := a.hasFilterName(key); ok {
			for _, requestKey := range a.parseRequestSetting(filterName, setting) {
				if _, ok := reqKeyToNamesFromCM[requestKey]; !ok {
					reqKeyToNamesFromCM[requestKey] = sets.NewString()
				}
				reqKeyToNamesFromCM[requestKey].Insert(filterName)
			}
		}
	}

	// update reqKeyToNames by merging user setting
	a.merge("add", reqKeyToNamesFromCM)
}

func (a *approver) updateConfigMap(oldObj, newObj interface{}) {
	oldCM, ok := oldObj.(*corev1.ConfigMap)
	if !ok {
		return
	}

	newCM, ok := newObj.(*corev1.ConfigMap)
	if !ok {
		return
	}

	// request settings are changed or not
	needUpdated := a.requestSettingsUpdated(oldCM.Data, newCM.Data)
	if !needUpdated {
		return
	}

	// get reqKeyToNames of user request setting from new configmap
	reqKeyToNamesFromCM := make(map[string]sets.String)
	for key, setting := range newCM.Data {
		if filterName, ok := a.hasFilterName(key); ok {
			for _, requestKey := range a.parseRequestSetting(filterName, setting) {
				if _, ok := reqKeyToNamesFromCM[requestKey]; !ok {
					reqKeyToNamesFromCM[requestKey] = sets.NewString()
				}
				reqKeyToNamesFromCM[requestKey].Insert(filterName)
			}
		}
	}

	// update reqKeyToName by merging user setting
	a.merge("update", reqKeyToNamesFromCM)
}

func (a *approver) deleteConfigMap(obj interface{}) {
	_, ok := obj.(*corev1.ConfigMap)
	if !ok {
		return
	}

	// update reqKeyToName by merging user setting
	a.merge("delete", map[string]sets.String{})
}

// merge is used to add specified setting into reqKeyToNames map.
func (a *approver) merge(action string, keyToNamesSetting map[string]sets.String) {
	a.Lock()
	defer a.Unlock()
	// remove current user setting from reqKeyToNames and left default setting
	for key, currentNames := range a.reqKeyToNames {
		if defaultNames, ok := a.defaultReqKeyToNames[key]; !ok {
			delete(a.reqKeyToNames, key)
		} else {
			notDefaultNames := currentNames.Difference(defaultNames).UnsortedList()
			a.reqKeyToNames[key].Delete(notDefaultNames...)
		}
	}

	// merge new user setting
	for key, names := range keyToNamesSetting {
		if _, ok := a.reqKeyToNames[key]; !ok {
			a.reqKeyToNames[key] = sets.NewString()
		}
		a.reqKeyToNames[key].Insert(names.UnsortedList()...)
	}
	klog.Infof("current filter setting: %v after %s", a.reqKeyToNames, action)
}

// parseRequestSetting extract comp info from setting, and make up request keys.
// requestSetting format as following(take servicetopology for example):
// servicetopology: "comp1,comp2"
func (a *approver) parseRequestSetting(name, setting string) []string {
	reqKeys := make([]string, 0)
	resourceAndVerbs, ok := a.supportedResourceAndVerbsForFilter[name]
	if !ok {
		return reqKeys
	}

	for _, comp := range strings.Split(setting, ",") {
		if strings.Contains(comp, "/") {
			comp = strings.Split(comp, "/")[0]
		}
		for resource, verbSet := range resourceAndVerbs {
			comp = strings.TrimSpace(comp)
			resource = strings.TrimSpace(resource)
			verbs := verbSet.List()

			if len(comp) != 0 && len(resource) != 0 && len(verbs) != 0 {
				for i := range verbs {
					reqKeys = append(reqKeys, reqKey(comp, resource, strings.TrimSpace(verbs[i])))
				}
			}
		}
	}
	return reqKeys
}

// hasFilterName check the key that includes a filter name or not.
// and return filter name and check result.
func (a *approver) hasFilterName(key string) (string, bool) {
	name := strings.TrimSpace(key)
	if strings.HasPrefix(name, "filter_") {
		name = strings.TrimSpace(strings.TrimPrefix(name, "filter_"))
	}

	if _, ok := a.supportedResourceAndVerbsForFilter[name]; ok {
		return name, true
	}

	return "", false
}

// requestSettingsUpdated is used to verify filter setting is changed or not.
func (a *approver) requestSettingsUpdated(old, new map[string]string) bool {
	for key := range old {
		if _, ok := a.hasFilterName(key); !ok {
			delete(old, key)
		}
	}

	for key := range new {
		if _, ok := a.hasFilterName(key); !ok {
			delete(new, key)
		}
	}

	// if filter setting of old and new equal, return false.
	// vice versa, return true.
	return !reflect.DeepEqual(old, new)
}

// getKeyByRequest returns reqKey for specified request.
func getKeyByRequest(req *http.Request) string {
	var key string
	ctx := req.Context()
	comp, ok := util.ClientComponentFrom(ctx)
	if !ok {
		return key
	}

	info, ok := apirequest.RequestInfoFrom(ctx)
	if !ok {
		return key
	}

	return reqKey(comp, info.Resource, info.Verb)
}

// reqKey is made up by comp and resource, verb
func reqKey(comp, resource, verb string) string {
	if len(comp) == 0 || len(resource) == 0 || len(verb) == 0 {
		return ""
	}
	return fmt.Sprintf("%s/%s/%s", comp, resource, verb)
}
