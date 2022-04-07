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
	reqKeyToName    map[string]string
	configMapSynced cache.InformerSynced
	stopCh          chan struct{}
}

var (
	supportedVerbs           = sets.NewString("get", "list", "watch")
	defaultWhiteListRequests = sets.NewString(reqKey(projectinfo.GetHubName(), "configmaps", "list"), reqKey(projectinfo.GetHubName(), "configmaps", "watch"))
	defaultReqKeyToName      = map[string]string{
		reqKey("kubelet", "services", "list"):                    MasterServiceFilterName,
		reqKey("kubelet", "services", "watch"):                   MasterServiceFilterName,
		reqKey("nginx-ingress-controller", "endpoints", "list"):  EndpointsFilterName,
		reqKey("nginx-ingress-controller", "endpoints", "watch"): EndpointsFilterName,
		reqKey("kube-proxy", "services", "list"):                 DiscardCloudServiceFilterName,
		reqKey("kube-proxy", "services", "watch"):                DiscardCloudServiceFilterName,
		reqKey("kube-proxy", "endpointslices", "list"):           ServiceTopologyFilterName,
		reqKey("kube-proxy", "endpointslices", "watch"):          ServiceTopologyFilterName,
	}
)

func newApprover(sharedFactory informers.SharedInformerFactory) *approver {
	configMapInformer := sharedFactory.Core().V1().ConfigMaps().Informer()
	na := &approver{
		reqKeyToName:    make(map[string]string),
		configMapSynced: configMapInformer.HasSynced,
		stopCh:          make(chan struct{}),
	}
	na.merge("init", defaultReqKeyToName)
	configMapInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    na.addConfigMap,
		UpdateFunc: na.updateConfigMap,
		DeleteFunc: na.deleteConfigMap,
	})
	return na
}

func (a *approver) Approve(req *http.Request) bool {
	if isWhitelistReq(req) {
		return false
	}
	if ok := cache.WaitForCacheSync(a.stopCh, a.configMapSynced); !ok {
		return false
	}

	key := getKeyByRequest(req)
	if len(key) == 0 {
		return false
	}

	a.Lock()
	defer a.Unlock()
	if _, ok := a.reqKeyToName[key]; ok {
		return true
	}

	return false
}

func (a *approver) GetFilterName(req *http.Request) string {
	key := getKeyByRequest(req)
	if len(key) == 0 {
		return ""
	}

	a.Lock()
	defer a.Unlock()
	return a.reqKeyToName[key]
}

// Determine whether it is a whitelist resource
func isWhitelistReq(req *http.Request) bool {
	key := getKeyByRequest(req)
	if ok := defaultWhiteListRequests.Has(key); ok {
		return true
	}
	return false
}

func (a *approver) addConfigMap(obj interface{}) {
	cm, ok := obj.(*corev1.ConfigMap)
	if !ok {
		return
	}

	// get reqKeyToName of user request setting from configmap
	reqKeyToNameFromCM := make(map[string]string)
	for key, setting := range cm.Data {
		if name, ok := hasFilterName(key); ok {
			for _, key := range parseRequestSetting(setting) {
				reqKeyToNameFromCM[key] = name
			}
		}
	}

	// update reqKeyToName by merging user setting
	a.merge("add", reqKeyToNameFromCM)
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
	needUpdated := requestSettingsUpdated(oldCM.Data, newCM.Data)
	if !needUpdated {
		return
	}

	// get reqKeyToName of user request setting from new configmap
	reqKeyToNameFromCM := make(map[string]string)
	for key, setting := range newCM.Data {
		if name, ok := hasFilterName(key); ok {
			for _, key := range parseRequestSetting(setting) {
				reqKeyToNameFromCM[key] = name
			}
		}
	}

	// update reqKeyToName by merging user setting
	a.merge("update", reqKeyToNameFromCM)
}

func (a *approver) deleteConfigMap(obj interface{}) {
	_, ok := obj.(*corev1.ConfigMap)
	if !ok {
		return
	}

	// update reqKeyToName by merging user setting
	a.merge("delete", map[string]string{})
}

// merge is used to add specified setting into reqKeyToName map.
func (a *approver) merge(action string, keyToNameSetting map[string]string) {
	a.Lock()
	defer a.Unlock()
	// remove current user setting from reqKeyToName and left default setting only
	for key := range a.reqKeyToName {
		if _, ok := defaultReqKeyToName[key]; !ok {
			delete(a.reqKeyToName, key)
		}
	}

	// merge new user setting
	for key, name := range keyToNameSetting {
		// if filter setting is duplicated, only recognize the first setting.
		if currentName, ok := a.reqKeyToName[key]; !ok {
			a.reqKeyToName[key] = name
		} else {
			klog.Warningf("request %s has already used filter %s, so skip filter %s setting", key, currentName, name)
		}
	}
	klog.Infof("current filter setting: %v after %s", a.reqKeyToName, action)
}

// parseRequestSetting extract comp, resource, verbs from setting, and
// make up request keys.
func parseRequestSetting(setting string) []string {
	reqKeys := make([]string, 0)
	for _, reqSetting := range strings.Split(setting, ",") {
		parts := strings.Split(reqSetting, "#")
		if len(parts) != 2 {
			continue
		}

		items := strings.Split(parts[0], "/")
		if len(items) != 2 {
			continue
		}
		comp := strings.TrimSpace(items[0])
		resource := strings.TrimSpace(items[1])
		verbs := strings.Split(parts[1], ";")

		if len(comp) != 0 && len(resource) != 0 && len(verbs) != 0 {
			for i := range verbs {
				verb := strings.TrimSpace(verbs[i])
				if ok := supportedVerbs.Has(verb); ok {
					reqKeys = append(reqKeys, reqKey(comp, resource, verb))
				}
			}
		}
	}
	return reqKeys
}

// hasFilterName check the key that includes a filter name or not.
// and return filter name and check result.
func hasFilterName(key string) (string, bool) {
	if strings.HasPrefix(key, "filter_") {
		name := strings.TrimSpace(strings.TrimPrefix(key, "filter_"))
		return name, len(name) != 0
	}

	return "", false
}

// requestSettingsUpdated is used to verify filter setting is changed or not.
func requestSettingsUpdated(old, new map[string]string) bool {
	for key := range old {
		if _, ok := hasFilterName(key); !ok {
			delete(old, key)
		}
	}

	for key := range new {
		if _, ok := hasFilterName(key); !ok {
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
