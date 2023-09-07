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

package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openyurtio/openyurt/pkg/apis/raven"
)

// SetDefaultsGateway set default values for Gateway.
func SetDefaultsGateway(obj *Gateway) {
	// Set default value for Gateway
	obj.Spec.NodeSelector = &metav1.LabelSelector{
		MatchLabels: map[string]string{
			raven.LabelCurrentGateway: obj.Name,
		},
	}
	for idx, val := range obj.Spec.Endpoints {
		if val.Port == 0 {
			switch val.Type {
			case Proxy:
				obj.Spec.Endpoints[idx].Port = DefaultProxyServerExposedPort
			case Tunnel:
				obj.Spec.Endpoints[idx].Port = DefaultTunnelServerExposedPort
			}
		}
	}
}
