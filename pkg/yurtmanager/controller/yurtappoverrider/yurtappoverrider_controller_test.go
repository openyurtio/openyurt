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
package yurtappoverrider

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	"k8s.io/apimachinery/pkg/types"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var overrider = &v1alpha1.YurtAppOverrider{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "demo",
		Namespace: "default",
	},
	Subject: v1alpha1.Subject{
		Name: "demo",
		TypeMeta: metav1.TypeMeta{
			Kind:       "test",
			APIVersion: "apps.openyurt.io/v1alpha1",
		},
	},
	Entries: []v1alpha1.Entry{
		{
			Pools: []string{"*"},
		},
	},
}

func TestReconcile(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Logf("failed to add yurt custom resource")
		return
	}
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Logf("failed to add kubernetes clint-go custom resource")
		return
	}
	reconciler := ReconcileYurtAppOverrider{
		Client: fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(overrider).Build(),
	}
	_, err := reconciler.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "demo",
		},
	})
	if err != nil {
		t.Logf("fail to call Reconcile: %v", err)
	}
}
