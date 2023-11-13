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

package controllers

import (
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	edgexCli "github.com/openyurtio/openyurt/pkg/yurtiotdock/clients/edgex-foundry"
)

func genFirstUpdateFilter(objKind string) predicate.Predicate {
	return predicate.Funcs{
		// ignore the update event that is generated due to a
		// new deviceprofile being added to the Edgex Foundry
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldDp, ok := e.ObjectOld.(edgexCli.EdgeXObject)
			if !ok {
				klog.Infof("could not assert object to deviceprofile, object kind is %s", objKind)
				return false
			}
			newDp, ok := e.ObjectNew.(edgexCli.EdgeXObject)
			if !ok {
				klog.Infof("could not assert object to deviceprofile, object kind is %s", objKind)
				return false
			}
			if !oldDp.IsAddedToEdgeX() && newDp.IsAddedToEdgeX() {
				return false
			}
			return true
		},
	}
}
