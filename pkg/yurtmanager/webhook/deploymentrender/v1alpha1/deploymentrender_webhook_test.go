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
package v1alpha1

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
)

var (
	replica int32 = 3
)

var defaultAppSet = &v1alpha1.YurtAppSet{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "yurtappset-patch",
		Namespace: "default",
	},
	Spec: v1alpha1.YurtAppSetSpec{
		Topology: v1alpha1.Topology{
			Pools: []v1alpha1.Pool{{
				Name:     "nodepool-test",
				Replicas: &replica}},
		},
		Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "test"}},
		WorkloadTemplate: v1alpha1.WorkloadTemplate{
			DeploymentTemplate: &v1alpha1.DeploymentTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "test"},
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "test"}},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "test"},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{Name: "nginx", Image: "nginx"},
							},
							Volumes: []corev1.Volume{
								{
									Name: "config",
									VolumeSource: corev1.VolumeSource{
										ConfigMap: &corev1.ConfigMapVolumeSource{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: "configMapSource-nodepool-test",
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	},
}

var defaultNodePool = &v1alpha1.NodePool{
	ObjectMeta: metav1.ObjectMeta{
		Name: "nodepool-test",
	},
	Spec: v1alpha1.NodePoolSpec{},
}

var defaultAppDaemon = &v1alpha1.YurtAppDaemon{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "yurtappdaemon",
		Namespace: "default",
	},
	Spec: v1alpha1.YurtAppDaemonSpec{
		Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "test"}},
		WorkloadTemplate: v1alpha1.WorkloadTemplate{
			DeploymentTemplate: &v1alpha1.DeploymentTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "test"},
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "test"}},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "test"},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{Name: "nginx", Image: "nginx"},
							},
						},
					},
				},
			},
		},
	},
}

var deploymentByYasv1beta1 = &appsv1.Deployment{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "test1",
		Namespace: "default",
		OwnerReferences: []metav1.OwnerReference{{
			APIVersion: "apps.openyurt.io/v1beta1",
			Kind:       "YurtAppSet",
			Name:       "yurtappset-patch",
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

var defaultDeployment = &appsv1.Deployment{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "test1",
		Namespace: "default",
		OwnerReferences: []metav1.OwnerReference{{
			APIVersion: "apps.openyurt.io/v1alpha1",
			Kind:       "YurtAppSet",
			Name:       "yurtappset-patch",
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

var daemonDeployment = &appsv1.Deployment{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "test2",
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

var overrider1 = &v1alpha1.YurtAppOverrider{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "demo",
		Namespace: "default",
	},
	Subject: v1alpha1.Subject{
		Name: "yurtappset-patch",
		TypeMeta: metav1.TypeMeta{
			Kind:       "YurtAppSet",
			APIVersion: "apps.openyurt.io/v1alpha1",
		},
	},
	Entries: []v1alpha1.Entry{
		{
			Pools: []string{"nodepool-test"},
			Items: []v1alpha1.Item{
				{
					Image: &v1alpha1.ImageItem{
						ContainerName: "nginx",
						ImageClaim:    "nginx:1.18",
					},
				},
			},
			Patches: []v1alpha1.Patch{
				{
					Operation: v1alpha1.REPLACE,
					Path:      "/spec/replicas",
					Value: apiextensionsv1.JSON{
						Raw: []byte("3"),
					},
				},
			},
		},
	},
}

var overrider2 = &v1alpha1.YurtAppOverrider{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "demo",
		Namespace: "default",
	},
	Subject: v1alpha1.Subject{
		Name: "yurtappset-patch",
		TypeMeta: metav1.TypeMeta{
			Kind:       "YurtAppSet",
			APIVersion: "apps.openyurt.io/v1alpha1",
		},
	},
	Entries: []v1alpha1.Entry{
		{
			Pools: []string{"*"},
			Patches: []v1alpha1.Patch{
				{
					Operation: v1alpha1.ADD,
					Path:      "/spec/template/spec/volumes/-",
					Value: apiextensionsv1.JSON{
						Raw: []byte(`{"name":"configmap-{{nodepool}}","configMap":{"name":"demo","items":[{"key": "game.properities","path": "game.properities"}]}}`),
					},
				},
			},
		},
	},
}

var overrider3 = &v1alpha1.YurtAppOverrider{
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

var overrider4 = &v1alpha1.YurtAppOverrider{
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
			Pools: []string{"*", "-nodepool-test"},
		},
	},
}

func TestDeploymentRenderHandler_Default(t *testing.T) {
	tcases := []struct {
		overrider *v1alpha1.YurtAppOverrider
	}{
		{overrider1},
		{overrider2},
		{overrider3},
		{overrider4},
	}
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Logf("could not add yurt custom resource")
		return
	}
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Logf("could not add kubernetes clint-go custom resource")
		return
	}
	for _, tcase := range tcases {
		t.Run("", func(t *testing.T) {
			webhook := &DeploymentRenderHandler{
				Client: fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(defaultAppSet, daemonDeployment, defaultNodePool, defaultDeployment, defaultAppDaemon, tcase.overrider).Build(),
				Scheme: scheme,
			}
			if err := webhook.Default(context.TODO(), defaultDeployment); err != nil {
				t.Fatal(err)
			}
			if err := webhook.Default(context.TODO(), daemonDeployment); err != nil {
				t.Fatal(err)
			}
			if err := webhook.Default(context.TODO(), deploymentByYasv1beta1); err != nil {
				t.Fatal(err)
			}
		})
	}
}
