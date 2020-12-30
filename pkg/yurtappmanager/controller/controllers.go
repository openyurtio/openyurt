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

package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/alibaba/openyurt/pkg/yurtappmanager/controller/nodepool"
	"github.com/alibaba/openyurt/pkg/yurtappmanager/controller/uniteddeployment"
)

var controllerAddFuncs []func(manager.Manager, context.Context) error

func init() {
	controllerAddFuncs = append(controllerAddFuncs, uniteddeployment.Add, nodepool.Add)
}

func SetupWithManager(m manager.Manager, ctx context.Context) error {
	for _, f := range controllerAddFuncs {
		if err := f(m, ctx); err != nil {
			if kindMatchErr, ok := err.(*meta.NoKindMatchError); ok {
				klog.Infof("CRD %v is not installed, its controller will perform noops!", kindMatchErr.GroupKind)
				continue
			}
			return err
		}
	}
	return nil
}
