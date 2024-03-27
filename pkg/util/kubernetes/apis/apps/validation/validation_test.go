/*
Copyright 2016 The Kubernetes Authors.

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

package validation

import (
	"strings"
	"testing"

	"github.com/davecgh/go-spew/spew"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation/field"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"

	"github.com/openyurtio/openyurt/pkg/util/kubernetes/apis/apps"
	api "github.com/openyurtio/openyurt/pkg/util/kubernetes/apis/core"
	corevalidation "github.com/openyurtio/openyurt/pkg/util/kubernetes/apis/core/validation"
	"github.com/openyurtio/openyurt/pkg/util/kubernetes/features"
)

func validDeployment() *apps.Deployment {
	return &apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "abc",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: apps.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name": "abc",
				},
			},
			Strategy: apps.DeploymentStrategy{
				Type: apps.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &apps.RollingUpdateDeployment{
					MaxSurge:       intstr.FromInt(1),
					MaxUnavailable: intstr.FromInt(1),
				},
			},
			Template: api.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "abc",
					Namespace: metav1.NamespaceDefault,
					Labels: map[string]string{
						"name": "abc",
					},
				},
				Spec: api.PodSpec{
					RestartPolicy: api.RestartPolicyAlways,
					DNSPolicy:     api.DNSDefault,
					Containers: []api.Container{
						{
							Name:                     "nginx",
							Image:                    "image",
							ImagePullPolicy:          api.PullNever,
							TerminationMessagePolicy: api.TerminationMessageReadFile,
						},
					},
				},
			},
			RollbackTo: &apps.RollbackConfig{
				Revision: 1,
			},
		},
	}
}

func TestValidateDeployment(t *testing.T) {
	defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EphemeralContainers, true)()

	successCases := []*apps.Deployment{
		validDeployment(),
	}
	for _, successCase := range successCases {
		if errs := ValidateDeployment(successCase, corevalidation.PodValidationOptions{}); len(errs) != 0 {
			t.Errorf("expected success: %v", errs)
		}
	}

	errorCases := map[string]*apps.Deployment{}
	errorCases["metadata.name: Required value"] = &apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: metav1.NamespaceDefault,
		},
	}
	// selector should match the labels in pod template.
	invalidSelectorDeployment := validDeployment()
	invalidSelectorDeployment.Spec.Selector = &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"name": "def",
		},
	}
	errorCases["`selector` does not match template `labels`"] = invalidSelectorDeployment

	// RestartPolicy should be always.
	invalidRestartPolicyDeployment := validDeployment()
	invalidRestartPolicyDeployment.Spec.Template.Spec.RestartPolicy = api.RestartPolicyNever
	errorCases["Unsupported value: \"Never\""] = invalidRestartPolicyDeployment

	// must have valid strategy type
	invalidStrategyDeployment := validDeployment()
	invalidStrategyDeployment.Spec.Strategy.Type = apps.DeploymentStrategyType("randomType")
	errorCases[`supported values: "Recreate", "RollingUpdate"`] = invalidStrategyDeployment

	// rollingUpdate should be nil for recreate.
	invalidRecreateDeployment := validDeployment()
	invalidRecreateDeployment.Spec.Strategy = apps.DeploymentStrategy{
		Type:          apps.RecreateDeploymentStrategyType,
		RollingUpdate: &apps.RollingUpdateDeployment{},
	}
	errorCases["may not be specified when strategy `type` is 'Recreate'"] = invalidRecreateDeployment

	// MaxSurge should be in the form of 20%.
	invalidMaxSurgeDeployment := validDeployment()
	invalidMaxSurgeDeployment.Spec.Strategy = apps.DeploymentStrategy{
		Type: apps.RollingUpdateDeploymentStrategyType,
		RollingUpdate: &apps.RollingUpdateDeployment{
			MaxSurge: intstr.FromString("20Percent"),
		},
	}
	errorCases["a valid percent string must be"] = invalidMaxSurgeDeployment

	// MaxSurge and MaxUnavailable cannot both be zero.
	invalidRollingUpdateDeployment := validDeployment()
	invalidRollingUpdateDeployment.Spec.Strategy = apps.DeploymentStrategy{
		Type: apps.RollingUpdateDeploymentStrategyType,
		RollingUpdate: &apps.RollingUpdateDeployment{
			MaxSurge:       intstr.FromString("0%"),
			MaxUnavailable: intstr.FromInt(0),
		},
	}
	errorCases["may not be 0 when `maxSurge` is 0"] = invalidRollingUpdateDeployment

	// MaxUnavailable should not be more than 100%.
	invalidMaxUnavailableDeployment := validDeployment()
	invalidMaxUnavailableDeployment.Spec.Strategy = apps.DeploymentStrategy{
		Type: apps.RollingUpdateDeploymentStrategyType,
		RollingUpdate: &apps.RollingUpdateDeployment{
			MaxUnavailable: intstr.FromString("110%"),
		},
	}
	errorCases["must not be greater than 100%"] = invalidMaxUnavailableDeployment

	// Rollback.Revision must be non-negative
	invalidRollbackRevisionDeployment := validDeployment()
	invalidRollbackRevisionDeployment.Spec.RollbackTo.Revision = -3
	errorCases["must be greater than or equal to 0"] = invalidRollbackRevisionDeployment

	// ProgressDeadlineSeconds should be greater than MinReadySeconds
	invalidProgressDeadlineDeployment := validDeployment()
	seconds := int32(600)
	invalidProgressDeadlineDeployment.Spec.ProgressDeadlineSeconds = &seconds
	invalidProgressDeadlineDeployment.Spec.MinReadySeconds = seconds
	errorCases["must be greater than minReadySeconds"] = invalidProgressDeadlineDeployment

	// Must not have ephemeral containers
	invalidEphemeralContainersDeployment := validDeployment()
	invalidEphemeralContainersDeployment.Spec.Template.Spec.EphemeralContainers = []api.EphemeralContainer{{
		EphemeralContainerCommon: api.EphemeralContainerCommon{
			Name:                     "ec",
			Image:                    "image",
			ImagePullPolicy:          "IfNotPresent",
			TerminationMessagePolicy: "File"},
	}}
	errorCases["ephemeral containers not allowed"] = invalidEphemeralContainersDeployment

	for k, v := range errorCases {
		errs := ValidateDeployment(v, corevalidation.PodValidationOptions{})
		if len(errs) == 0 {
			t.Errorf("[%s] expected failure", k)
		} else if !strings.Contains(errs[0].Error(), k) {
			t.Errorf("unexpected error: %q, expected: %q", errs[0].Error(), k)
		}
	}
}

func TestValidateDeploymentStatus(t *testing.T) {
	collisionCount := int32(-3)
	tests := []struct {
		name string

		replicas           int32
		updatedReplicas    int32
		readyReplicas      int32
		availableReplicas  int32
		observedGeneration int64
		collisionCount     *int32

		expectedErr bool
	}{
		{
			name:               "valid status",
			replicas:           3,
			updatedReplicas:    3,
			readyReplicas:      2,
			availableReplicas:  1,
			observedGeneration: 2,
			expectedErr:        false,
		},
		{
			name:               "invalid replicas",
			replicas:           -1,
			updatedReplicas:    2,
			readyReplicas:      2,
			availableReplicas:  1,
			observedGeneration: 2,
			expectedErr:        true,
		},
		{
			name:               "invalid updatedReplicas",
			replicas:           2,
			updatedReplicas:    -1,
			readyReplicas:      2,
			availableReplicas:  1,
			observedGeneration: 2,
			expectedErr:        true,
		},
		{
			name:               "invalid readyReplicas",
			replicas:           3,
			readyReplicas:      -1,
			availableReplicas:  1,
			observedGeneration: 2,
			expectedErr:        true,
		},
		{
			name:               "invalid availableReplicas",
			replicas:           3,
			readyReplicas:      3,
			availableReplicas:  -1,
			observedGeneration: 2,
			expectedErr:        true,
		},
		{
			name:               "invalid observedGeneration",
			replicas:           3,
			readyReplicas:      3,
			availableReplicas:  3,
			observedGeneration: -1,
			expectedErr:        true,
		},
		{
			name:               "updatedReplicas greater than replicas",
			replicas:           3,
			updatedReplicas:    4,
			readyReplicas:      3,
			availableReplicas:  3,
			observedGeneration: 1,
			expectedErr:        true,
		},
		{
			name:               "readyReplicas greater than replicas",
			replicas:           3,
			readyReplicas:      4,
			availableReplicas:  3,
			observedGeneration: 1,
			expectedErr:        true,
		},
		{
			name:               "availableReplicas greater than replicas",
			replicas:           3,
			readyReplicas:      3,
			availableReplicas:  4,
			observedGeneration: 1,
			expectedErr:        true,
		},
		{
			name:               "availableReplicas greater than readyReplicas",
			replicas:           3,
			readyReplicas:      2,
			availableReplicas:  3,
			observedGeneration: 1,
			expectedErr:        true,
		},
		{
			name:               "invalid collisionCount",
			replicas:           3,
			observedGeneration: 1,
			collisionCount:     &collisionCount,
			expectedErr:        true,
		},
	}

	for _, test := range tests {
		status := apps.DeploymentStatus{
			Replicas:           test.replicas,
			UpdatedReplicas:    test.updatedReplicas,
			ReadyReplicas:      test.readyReplicas,
			AvailableReplicas:  test.availableReplicas,
			ObservedGeneration: test.observedGeneration,
			CollisionCount:     test.collisionCount,
		}

		errs := ValidateDeploymentStatus(&status, field.NewPath("status"))
		if hasErr := len(errs) > 0; hasErr != test.expectedErr {
			errString := spew.Sprintf("%#v", errs)
			t.Errorf("%s: expected error: %t, got error: %t\nerrors: %s", test.name, test.expectedErr, hasErr, errString)
		}
	}
}
