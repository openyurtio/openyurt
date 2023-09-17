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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
)

var (
	replica int32 = 3
)

var yurtAppDaemon = &v1alpha1.YurtAppDaemon{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "yurtappdaemon",
		Namespace: "default",
	},
	Spec: v1alpha1.YurtAppDaemonSpec{
		NodePoolSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{
				"app": "test",
			},
		},
		Selector: &metav1.LabelSelector{
			MatchLabels: map[string]string{
				"app": "test",
			},
		},
	},
}

var daemonDeployment = &appsv1.Deployment{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "test",
		Namespace: "default",
		OwnerReferences: []metav1.OwnerReference{{
			APIVersion: "apps.openyurt.io/v1alpha1",
			Kind:       "YurtAppDaemon",
			Name:       "yurtappdaemon",
		}},
		Labels: map[string]string{
			"apps.openyurt.io/pool-name": "nodepool-test",
		},
	},
	Status: appsv1.DeploymentStatus{},
	Spec: appsv1.DeploymentSpec{
		Replicas: &replica,
		Selector: &metav1.LabelSelector{
			MatchLabels: map[string]string{
				"app": "test",
			},
		},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					"app": "test",
				},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "nginx",
						Image: "nginx",
					},
				},
			},
		},
	},
}

var overrider = &v1alpha1.YurtAppOverrider{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "demo",
		Namespace: "default",
	},
	Subject: v1alpha1.Subject{
		Name: "yurtappdaemon",
		TypeMeta: metav1.TypeMeta{
			Kind:       "YurtAppDaemon",
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
		Client:            fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(daemonDeployment, overrider, yurtAppDaemon).Build(),
		CacheOverriderMap: make(map[string]*v1alpha1.YurtAppOverrider),
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
