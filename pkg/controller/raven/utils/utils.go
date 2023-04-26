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

package utils

import (
	"net"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openyurtio/openyurt/pkg/apis/raven/v1alpha1"
)

// GetNodeInternalIP returns internal ip of the given `node`.
func GetNodeInternalIP(node corev1.Node) string {
	var ip string
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeInternalIP && net.ParseIP(addr.Address) != nil {
			ip = addr.Address
			break
		}
	}
	return ip
}

func IsGatewayExposeByLB(gateway *v1alpha1.Gateway) bool {
	return gateway.Spec.ExposeType == v1alpha1.ExposeTypeLoadBalancer
}

// AddGatewayToWorkQueue adds the Gateway the reconciler's workqueue
func AddGatewayToWorkQueue(gwName string,
	q workqueue.RateLimitingInterface) {
	if gwName != "" {
		q.Add(reconcile.Request{
			NamespacedName: types.NamespacedName{Name: gwName},
		})
	}
}
