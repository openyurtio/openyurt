/*
Copyright 2021 The Openyurt Authors.

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

package validating

import (
	webhookutil "github.com/alibaba/openyurt/pkg/yurtappmanager/webhook/util"
)

// +kubebuilder:webhook:verbs=create;update;delete,path=/validate-apps-openyurt-io-v1alpha1-nodepool,mutating=false,failurePolicy=fail,groups=apps.openyurt.io,resources=nodepools,versions=v1alpha1,name=vnodepool.kb.io

var (
	// HandlerMap contains admission webhook handlers
	HandlerMap = map[string]webhookutil.Handler{
		"validate-apps-openyurt-io-v1alpha1-nodepool": &NodePoolCreateUpdateHandler{},
	}
)
