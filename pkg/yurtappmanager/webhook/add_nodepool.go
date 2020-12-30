/*
Copyright 2020 The Openyurt Authors.

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

package webhook

import (
	appsv1alpha1 "github.com/alibaba/openyurt/pkg/yurtappmanager/apis/apps/v1alpha1"
	"github.com/alibaba/openyurt/pkg/yurtappmanager/util/gate"
	"github.com/alibaba/openyurt/pkg/yurtappmanager/webhook/nodepool/mutating"
	"github.com/alibaba/openyurt/pkg/yurtappmanager/webhook/nodepool/validating"
)

func init() {
	if !gate.ResourceEnabled(&appsv1alpha1.NodePool{}) {
		return
	}
	addHandlers(mutating.HandlerMap)
	addHandlers(validating.HandlerMap)
}
