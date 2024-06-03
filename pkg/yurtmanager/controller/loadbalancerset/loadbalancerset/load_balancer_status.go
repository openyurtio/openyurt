/*
Copyright 2024 The OpenYurt Authors.

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

package loadbalancerset

import (
	"reflect"
	"sort"

	corev1 "k8s.io/api/core/v1"

	netv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/network/v1alpha1"
)

func aggregateLbStatus(poolServices []netv1alpha1.PoolService) corev1.LoadBalancerStatus {
	var lbIngress []corev1.LoadBalancerIngress
	for _, cps := range poolServices {
		lbIngress = append(lbIngress, cps.Status.LoadBalancer.Ingress...)
	}

	sort.Slice(lbIngress, func(i, j int) bool {
		return lbIngress[i].IP < lbIngress[j].IP
	})

	return corev1.LoadBalancerStatus{
		Ingress: lbIngress,
	}
}

func compareAndUpdateServiceLbStatus(svc *corev1.Service, lbStatus corev1.LoadBalancerStatus) bool {
	if !reflect.DeepEqual(svc.Status.LoadBalancer, lbStatus) {
		svc.Status.LoadBalancer = lbStatus
		return true
	}
	return false
}
