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
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
)

var initialReplicas int32 = 2

var testPatchDeployment = &appsv1.Deployment{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "test",
		Namespace: "default",
		OwnerReferences: []metav1.OwnerReference{{
			APIVersion: "apps.openyurt.io/v1alpha1",
			Kind:       "YurtAppSet",
			Name:       "yurtappset-patch",
		}},
	},
	Status: appsv1.DeploymentStatus{},
	Spec: appsv1.DeploymentSpec{
		Replicas: &initialReplicas,
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

var patchControl = PatchControl{
	patches: []v1alpha1.Patch{
		{
			Operation: v1alpha1.REPLACE,
			Path:      "/spec/template/spec/containers/0/image",
			Value: apiextensionsv1.JSON{
				Raw: []byte(`"tomcat:1.18"`),
			},
		},
		{
			Operation: v1alpha1.ADD,
			Path:      "/spec/replicas",
			Value: apiextensionsv1.JSON{
				Raw: []byte("5"),
			},
		},
	},
	patchObject: testPatchDeployment,
	dataStruct:  appsv1.Deployment{},
}

func TestJsonMergePatch(t *testing.T) {
	sample := v1alpha1.Patch{
		Operation: v1alpha1.ADD,
		Path:      "/spec/template/spec/containers/0/image",
		Value:     apiextensionsv1.JSON{Raw: []byte(`"tomcat"`)},
	}
	if err := patchControl.jsonMergePatch([]v1alpha1.Patch{sample}); err != nil {
		t.Fatalf("fail to call jsonMergePatch")
	}
	t.Logf("image:%v", testPatchDeployment.Spec.Template.Spec.Containers[0].Name)
}

func TestUpdatePatches(t *testing.T) {
	if err := patchControl.updatePatches(); err != nil {
		t.Fatalf("fail to call updatePatches: %v", err)
	}
}
