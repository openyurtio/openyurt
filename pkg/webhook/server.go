/*
Copyright 2022 The OpenYurt Authors.

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
	"github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
)

var webhookAddFuncs []func(ctrl.Manager) error

func init() {
	webhookAddFuncs = append(webhookAddFuncs, v1beta1.AddSample)
}

func SetupWebhookWithManager(mgr ctrl.Manager) error {
	for _, f := range webhookAddFuncs {
		if err := f(mgr); err != nil {
			if kindMatchErr, ok := err.(*meta.NoKindMatchError); ok {
				klog.Infof("CRD %v is not installed, its webhook will perform noops!", kindMatchErr.GroupKind)
				continue
			}
			return err
		}
	}
	return nil
}
