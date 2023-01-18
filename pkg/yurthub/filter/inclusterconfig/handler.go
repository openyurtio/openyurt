/*
Copyright 2023 The OpenYurt Authors.

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

package inclusterconfig

import (
	"fmt"
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
)

const (
	KubeProxyConfigMapNamespace = "kube-system"
	KubeProxyConfigMapName      = "kube-proxy"
	KubeProxyDataKey            = "config.conf"
	KubeProxyKubeConfigStr      = "kubeconfig"
)

type inClusterConfigFilterHandler struct{}

func NewInClusterConfigFilterHandler() filter.ObjectHandler {
	return &inClusterConfigFilterHandler{}
}

// RuntimeObjectFilter comments kubeconfig in kube-system/kube-proxy configmap in the response object
func (fh *inClusterConfigFilterHandler) RuntimeObjectFilter(obj runtime.Object) (runtime.Object, bool) {
	switch v := obj.(type) {
	case *v1.ConfigMapList:
		for i := range v.Items {
			newCM, mutated := mutateKubeProxyConfigMap(&v.Items[i])
			if mutated {
				v.Items[i] = *newCM
				break
			}
		}
		return v, false
	case *v1.ConfigMap:
		cm, _ := mutateKubeProxyConfigMap(v)
		return cm, false
	default:
		return v, false
	}
}

func mutateKubeProxyConfigMap(cm *v1.ConfigMap) (*v1.ConfigMap, bool) {
	mutated := false
	if cm.Namespace == KubeProxyConfigMapNamespace && cm.Name == KubeProxyConfigMapName {
		if cm.Data != nil && len(cm.Data[KubeProxyDataKey]) != 0 {
			parts := make([]string, 0)
			for _, line := range strings.Split(cm.Data[KubeProxyDataKey], "\n") {
				items := strings.Split(strings.Trim(line, " "), ":")
				if len(items) == 2 && items[0] == KubeProxyKubeConfigStr {
					parts = append(parts, strings.Replace(line, KubeProxyKubeConfigStr, fmt.Sprintf("#%s", KubeProxyKubeConfigStr), 1))
					mutated = true
				} else {
					parts = append(parts, line)
				}
			}
			if mutated {
				cm.Data[KubeProxyDataKey] = strings.Join(parts, "\n")
				klog.Infof("kubeconfig in configmap(%s/%s) has been commented, new config.conf: \n%s\n", cm.Namespace, cm.Name, cm.Data[KubeProxyDataKey])
			}
		}
	}

	return cm, mutated
}
