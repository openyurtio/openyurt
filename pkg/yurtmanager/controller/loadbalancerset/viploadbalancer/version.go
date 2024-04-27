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

package viploadbalancer

import (
	netv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/network/v1alpha1"
)

const VipAgentName = "yurt-lb-agent"
const VipAgentImage = "openyurt/yurt-lb-agent"
const VipAgentControlPlane = "vip-loadbalance-controller"

func DefaultVersion(poolService netv1alpha1.PoolService) (string, string, error) {
	var (
		ver string
		ns  string
	)
	ver = "latest"
	ns = poolService.Namespace
	return ver, ns, nil
}
