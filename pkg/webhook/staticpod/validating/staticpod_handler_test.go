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

package validating

import (
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"

	appsv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
)

func Test_validateStaticPodSpec(t *testing.T) {

	tests := []struct {
		name string
		spec *appsv1alpha1.StaticPodSpec
		want field.ErrorList
	}{
		{
			"validate success",
			&appsv1alpha1.StaticPodSpec{
				StaticPodNamespace: metav1.NamespaceDefault,
				StaticPodName:      "nginx",
				StaticPodManifest:  "nginx",
				UpgradeStrategy: appsv1alpha1.StaticPodUpgradeStrategy{
					Type: appsv1alpha1.OTAStaticPodUpgradeStrategyType,
				},
			},
			nil,
		},
		{
			"miss namespace",
			&appsv1alpha1.StaticPodSpec{
				StaticPodName:     "nginx",
				StaticPodManifest: "nginx",
				UpgradeStrategy: appsv1alpha1.StaticPodUpgradeStrategy{
					Type: appsv1alpha1.OTAStaticPodUpgradeStrategyType,
				},
			},
			field.ErrorList{field.Required(field.NewPath("spec").Child("Namespace"),
				"Namespace is required")},
		},
		{
			"miss name",
			&appsv1alpha1.StaticPodSpec{
				StaticPodNamespace: metav1.NamespaceDefault,
				StaticPodManifest:  "nginx",
				UpgradeStrategy: appsv1alpha1.StaticPodUpgradeStrategy{
					Type: appsv1alpha1.OTAStaticPodUpgradeStrategyType,
				},
			},
			field.ErrorList{field.Required(field.NewPath("spec").Child("StaticPodName"),
				"StaticPodName is required")},
		},
		{
			"miss manifest",
			&appsv1alpha1.StaticPodSpec{
				StaticPodName:      "nginx",
				StaticPodNamespace: metav1.NamespaceDefault,
				UpgradeStrategy: appsv1alpha1.StaticPodUpgradeStrategy{
					Type: appsv1alpha1.OTAStaticPodUpgradeStrategyType,
				},
			},
			field.ErrorList{field.Required(field.NewPath("spec").Child("StaticPodManifest"),
				"StaticPodManifest is required")},
		},
		{
			"miss upgrade strategy unavailable",
			&appsv1alpha1.StaticPodSpec{
				StaticPodName:      "nginx",
				StaticPodNamespace: metav1.NamespaceDefault,
				StaticPodManifest:  "nginx",
				UpgradeStrategy:    appsv1alpha1.StaticPodUpgradeStrategy{Type: appsv1alpha1.AutoStaticPodUpgradeStrategyType},
			},
			field.ErrorList{field.Required(field.NewPath("spec").Child("upgradeStrategy"),
				"max-unavailable is required in auto mode")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validateStaticPodSpec(tt.spec); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("validateStaticPodSpec() = %v, want %v", got, tt.want)
			}
		})
	}
}
