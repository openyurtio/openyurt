/*
Copyright 2020 The OpenYurt Authors.
Copyright 2020 The Kruise Authors.

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
	unitv1alpha1 "github.com/alibaba/openyurt/pkg/yurtappmanager/apis/apps/v1alpha1"
	"github.com/alibaba/openyurt/pkg/yurtappmanager/util/gate"
	"github.com/alibaba/openyurt/pkg/yurtappmanager/webhook/uniteddeployment/mutating"
	"github.com/alibaba/openyurt/pkg/yurtappmanager/webhook/uniteddeployment/validating"
)

func init() {
	if !gate.ResourceEnabled(&unitv1alpha1.UnitedDeployment{}) {
		return
	}
	addHandlers(mutating.HandlerMap)
	addHandlers(validating.HandlerMap)
}
