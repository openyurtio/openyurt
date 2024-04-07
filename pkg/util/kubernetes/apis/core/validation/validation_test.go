/*
Copyright 2014 The Kubernetes Authors.

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
	"bytes"
	"math"
	"reflect"
	"strings"
	"testing"

	asserttestify "github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	utilpointer "k8s.io/utils/pointer"

	"github.com/openyurtio/openyurt/pkg/util/kubernetes/apis/core"
	"github.com/openyurtio/openyurt/pkg/util/kubernetes/capabilities"
	"github.com/openyurtio/openyurt/pkg/util/kubernetes/features"
)

const (
	dnsLabelErrMsg          = "a lowercase RFC 1123 label must consist of"
	dnsSubdomainLabelErrMsg = "a lowercase RFC 1123 subdomain"
	envVarNameErrMsg        = "a valid environment variable name must consist of"
)

func newHostPathType(pathType string) *core.HostPathType {
	hostPathType := new(core.HostPathType)
	*hostPathType = core.HostPathType(pathType)
	return hostPathType
}

func testVolume(name string, namespace string, spec core.PersistentVolumeSpec) *core.PersistentVolume {
	objMeta := metav1.ObjectMeta{Name: name}
	if namespace != "" {
		objMeta.Namespace = namespace
	}

	return &core.PersistentVolume{
		ObjectMeta: objMeta,
		Spec:       spec,
	}
}

func TestValidatePersistentVolumes(t *testing.T) {
	validMode := core.PersistentVolumeFilesystem
	invalidMode := core.PersistentVolumeMode("fakeVolumeMode")
	scenarios := map[string]struct {
		isExpectedFailure      bool
		enableReadWriteOncePod bool
		volume                 *core.PersistentVolume
	}{
		"good-volume": {
			isExpectedFailure: false,
			volume: testVolume("foo", "", core.PersistentVolumeSpec{
				Capacity: core.ResourceList{
					core.ResourceName(core.ResourceStorage): resource.MustParse("10G"),
				},
				AccessModes: []core.PersistentVolumeAccessMode{core.ReadWriteOnce},
				PersistentVolumeSource: core.PersistentVolumeSource{
					HostPath: &core.HostPathVolumeSource{
						Path: "/foo",
						Type: newHostPathType(string(core.HostPathDirectory)),
					},
				},
			}),
		},
		"good-volume-with-capacity-unit": {
			isExpectedFailure: false,
			volume: testVolume("foo", "", core.PersistentVolumeSpec{
				Capacity: core.ResourceList{
					core.ResourceName(core.ResourceStorage): resource.MustParse("10Gi"),
				},
				AccessModes: []core.PersistentVolumeAccessMode{core.ReadWriteOnce},
				PersistentVolumeSource: core.PersistentVolumeSource{
					HostPath: &core.HostPathVolumeSource{
						Path: "/foo",
						Type: newHostPathType(string(core.HostPathDirectory)),
					},
				},
			}),
		},
		"good-volume-without-capacity-unit": {
			isExpectedFailure: false,
			volume: testVolume("foo", "", core.PersistentVolumeSpec{
				Capacity: core.ResourceList{
					core.ResourceName(core.ResourceStorage): resource.MustParse("10"),
				},
				AccessModes: []core.PersistentVolumeAccessMode{core.ReadWriteOnce},
				PersistentVolumeSource: core.PersistentVolumeSource{
					HostPath: &core.HostPathVolumeSource{
						Path: "/foo",
						Type: newHostPathType(string(core.HostPathDirectory)),
					},
				},
			}),
		},
		"good-volume-with-storage-class": {
			isExpectedFailure: false,
			volume: testVolume("foo", "", core.PersistentVolumeSpec{
				Capacity: core.ResourceList{
					core.ResourceName(core.ResourceStorage): resource.MustParse("10G"),
				},
				AccessModes: []core.PersistentVolumeAccessMode{core.ReadWriteOnce},
				PersistentVolumeSource: core.PersistentVolumeSource{
					HostPath: &core.HostPathVolumeSource{
						Path: "/foo",
						Type: newHostPathType(string(core.HostPathDirectory)),
					},
				},
				StorageClassName: "valid",
			}),
		},
		"good-volume-with-retain-policy": {
			isExpectedFailure: false,
			volume: testVolume("foo", "", core.PersistentVolumeSpec{
				Capacity: core.ResourceList{
					core.ResourceName(core.ResourceStorage): resource.MustParse("10G"),
				},
				AccessModes: []core.PersistentVolumeAccessMode{core.ReadWriteOnce},
				PersistentVolumeSource: core.PersistentVolumeSource{
					HostPath: &core.HostPathVolumeSource{
						Path: "/foo",
						Type: newHostPathType(string(core.HostPathDirectory)),
					},
				},
				PersistentVolumeReclaimPolicy: core.PersistentVolumeReclaimRetain,
			}),
		},
		"good-volume-with-volume-mode": {
			isExpectedFailure: false,
			volume: testVolume("foo", "", core.PersistentVolumeSpec{
				Capacity: core.ResourceList{
					core.ResourceName(core.ResourceStorage): resource.MustParse("10G"),
				},
				AccessModes: []core.PersistentVolumeAccessMode{core.ReadWriteOnce},
				PersistentVolumeSource: core.PersistentVolumeSource{
					HostPath: &core.HostPathVolumeSource{
						Path: "/foo",
						Type: newHostPathType(string(core.HostPathDirectory)),
					},
				},
				VolumeMode: &validMode,
			}),
		},
		"invalid-accessmode": {
			isExpectedFailure: true,
			volume: testVolume("foo", "", core.PersistentVolumeSpec{
				Capacity: core.ResourceList{
					core.ResourceName(core.ResourceStorage): resource.MustParse("10G"),
				},
				AccessModes: []core.PersistentVolumeAccessMode{"fakemode"},
				PersistentVolumeSource: core.PersistentVolumeSource{
					HostPath: &core.HostPathVolumeSource{
						Path: "/foo",
						Type: newHostPathType(string(core.HostPathDirectory)),
					},
				},
			}),
		},
		"invalid-reclaimpolicy": {
			isExpectedFailure: true,
			volume: testVolume("foo", "", core.PersistentVolumeSpec{
				Capacity: core.ResourceList{
					core.ResourceName(core.ResourceStorage): resource.MustParse("10G"),
				},
				AccessModes: []core.PersistentVolumeAccessMode{core.ReadWriteOnce},
				PersistentVolumeSource: core.PersistentVolumeSource{
					HostPath: &core.HostPathVolumeSource{
						Path: "/foo",
						Type: newHostPathType(string(core.HostPathDirectory)),
					},
				},
				PersistentVolumeReclaimPolicy: "fakeReclaimPolicy",
			}),
		},
		"invalid-volume-mode": {
			isExpectedFailure: true,
			volume: testVolume("foo", "", core.PersistentVolumeSpec{
				Capacity: core.ResourceList{
					core.ResourceName(core.ResourceStorage): resource.MustParse("10G"),
				},
				AccessModes: []core.PersistentVolumeAccessMode{core.ReadWriteOnce},
				PersistentVolumeSource: core.PersistentVolumeSource{
					HostPath: &core.HostPathVolumeSource{
						Path: "/foo",
						Type: newHostPathType(string(core.HostPathDirectory)),
					},
				},
				VolumeMode: &invalidMode,
			}),
		},
		"with-read-write-once-pod-feature-gate-enabled": {
			isExpectedFailure:      false,
			enableReadWriteOncePod: true,
			volume: testVolume("foo", "", core.PersistentVolumeSpec{
				Capacity: core.ResourceList{
					core.ResourceName(core.ResourceStorage): resource.MustParse("10G"),
				},
				AccessModes: []core.PersistentVolumeAccessMode{"ReadWriteOncePod"},
				PersistentVolumeSource: core.PersistentVolumeSource{
					HostPath: &core.HostPathVolumeSource{
						Path: "/foo",
						Type: newHostPathType(string(core.HostPathDirectory)),
					},
				},
			}),
		},
		"with-read-write-once-pod-feature-gate-disabled": {
			isExpectedFailure:      true,
			enableReadWriteOncePod: false,
			volume: testVolume("foo", "", core.PersistentVolumeSpec{
				Capacity: core.ResourceList{
					core.ResourceName(core.ResourceStorage): resource.MustParse("10G"),
				},
				AccessModes: []core.PersistentVolumeAccessMode{"ReadWriteOncePod"},
				PersistentVolumeSource: core.PersistentVolumeSource{
					HostPath: &core.HostPathVolumeSource{
						Path: "/foo",
						Type: newHostPathType(string(core.HostPathDirectory)),
					},
				},
			}),
		},
		"with-read-write-once-pod-and-others-feature-gate-enabled": {
			isExpectedFailure:      true,
			enableReadWriteOncePod: true,
			volume: testVolume("foo", "", core.PersistentVolumeSpec{
				Capacity: core.ResourceList{
					core.ResourceName(core.ResourceStorage): resource.MustParse("10G"),
				},
				AccessModes: []core.PersistentVolumeAccessMode{"ReadWriteOncePod", "ReadWriteMany"},
				PersistentVolumeSource: core.PersistentVolumeSource{
					HostPath: &core.HostPathVolumeSource{
						Path: "/foo",
						Type: newHostPathType(string(core.HostPathDirectory)),
					},
				},
			}),
		},
		"unexpected-namespace": {
			isExpectedFailure: true,
			volume: testVolume("foo", "unexpected-namespace", core.PersistentVolumeSpec{
				Capacity: core.ResourceList{
					core.ResourceName(core.ResourceStorage): resource.MustParse("10G"),
				},
				AccessModes: []core.PersistentVolumeAccessMode{core.ReadWriteOnce},
				PersistentVolumeSource: core.PersistentVolumeSource{
					HostPath: &core.HostPathVolumeSource{
						Path: "/foo",
						Type: newHostPathType(string(core.HostPathDirectory)),
					},
				},
			}),
		},
		"missing-volume-source": {
			isExpectedFailure: true,
			volume: testVolume("foo", "", core.PersistentVolumeSpec{
				Capacity: core.ResourceList{
					core.ResourceName(core.ResourceStorage): resource.MustParse("10G"),
				},
				AccessModes: []core.PersistentVolumeAccessMode{core.ReadWriteOnce},
			}),
		},
		"bad-name": {
			isExpectedFailure: true,
			volume: testVolume("123*Bad(Name", "unexpected-namespace", core.PersistentVolumeSpec{
				Capacity: core.ResourceList{
					core.ResourceName(core.ResourceStorage): resource.MustParse("10G"),
				},
				AccessModes: []core.PersistentVolumeAccessMode{core.ReadWriteOnce},
				PersistentVolumeSource: core.PersistentVolumeSource{
					HostPath: &core.HostPathVolumeSource{
						Path: "/foo",
						Type: newHostPathType(string(core.HostPathDirectory)),
					},
				},
			}),
		},
		"missing-name": {
			isExpectedFailure: true,
			volume: testVolume("", "", core.PersistentVolumeSpec{
				Capacity: core.ResourceList{
					core.ResourceName(core.ResourceStorage): resource.MustParse("10G"),
				},
				AccessModes: []core.PersistentVolumeAccessMode{core.ReadWriteOnce},
				PersistentVolumeSource: core.PersistentVolumeSource{
					HostPath: &core.HostPathVolumeSource{
						Path: "/foo",
						Type: newHostPathType(string(core.HostPathDirectory)),
					},
				},
			}),
		},
		"missing-capacity": {
			isExpectedFailure: true,
			volume: testVolume("foo", "", core.PersistentVolumeSpec{
				AccessModes: []core.PersistentVolumeAccessMode{core.ReadWriteOnce},
				PersistentVolumeSource: core.PersistentVolumeSource{
					HostPath: &core.HostPathVolumeSource{
						Path: "/foo",
						Type: newHostPathType(string(core.HostPathDirectory)),
					},
				},
			}),
		},
		"bad-volume-zero-capacity": {
			isExpectedFailure: true,
			volume: testVolume("foo", "", core.PersistentVolumeSpec{
				Capacity: core.ResourceList{
					core.ResourceName(core.ResourceStorage): resource.MustParse("0"),
				},
				AccessModes: []core.PersistentVolumeAccessMode{core.ReadWriteOnce},
				PersistentVolumeSource: core.PersistentVolumeSource{
					HostPath: &core.HostPathVolumeSource{
						Path: "/foo",
						Type: newHostPathType(string(core.HostPathDirectory)),
					},
				},
			}),
		},
		"missing-accessmodes": {
			isExpectedFailure: true,
			volume: testVolume("goodname", "missing-accessmodes", core.PersistentVolumeSpec{
				Capacity: core.ResourceList{
					core.ResourceName(core.ResourceStorage): resource.MustParse("10G"),
				},
				PersistentVolumeSource: core.PersistentVolumeSource{
					HostPath: &core.HostPathVolumeSource{
						Path: "/foo",
						Type: newHostPathType(string(core.HostPathDirectory)),
					},
				},
			}),
		},
		"too-many-sources": {
			isExpectedFailure: true,
			volume: testVolume("foo", "", core.PersistentVolumeSpec{
				Capacity: core.ResourceList{
					core.ResourceName(core.ResourceStorage): resource.MustParse("5G"),
				},
				PersistentVolumeSource: core.PersistentVolumeSource{
					HostPath: &core.HostPathVolumeSource{
						Path: "/foo",
						Type: newHostPathType(string(core.HostPathDirectory)),
					},
					GCEPersistentDisk: &core.GCEPersistentDiskVolumeSource{PDName: "foo", FSType: "ext4"},
				},
			}),
		},
		"host mount of / with recycle reclaim policy": {
			isExpectedFailure: true,
			volume: testVolume("bad-recycle-do-not-want", "", core.PersistentVolumeSpec{
				Capacity: core.ResourceList{
					core.ResourceName(core.ResourceStorage): resource.MustParse("10G"),
				},
				AccessModes: []core.PersistentVolumeAccessMode{core.ReadWriteOnce},
				PersistentVolumeSource: core.PersistentVolumeSource{
					HostPath: &core.HostPathVolumeSource{
						Path: "/",
						Type: newHostPathType(string(core.HostPathDirectory)),
					},
				},
				PersistentVolumeReclaimPolicy: core.PersistentVolumeReclaimRecycle,
			}),
		},
		"host mount of / with recycle reclaim policy 2": {
			isExpectedFailure: true,
			volume: testVolume("bad-recycle-do-not-want", "", core.PersistentVolumeSpec{
				Capacity: core.ResourceList{
					core.ResourceName(core.ResourceStorage): resource.MustParse("10G"),
				},
				AccessModes: []core.PersistentVolumeAccessMode{core.ReadWriteOnce},
				PersistentVolumeSource: core.PersistentVolumeSource{
					HostPath: &core.HostPathVolumeSource{
						Path: "/a/..",
						Type: newHostPathType(string(core.HostPathDirectory)),
					},
				},
				PersistentVolumeReclaimPolicy: core.PersistentVolumeReclaimRecycle,
			}),
		},
		"invalid-storage-class-name": {
			isExpectedFailure: true,
			volume: testVolume("invalid-storage-class-name", "", core.PersistentVolumeSpec{
				Capacity: core.ResourceList{
					core.ResourceName(core.ResourceStorage): resource.MustParse("10G"),
				},
				AccessModes: []core.PersistentVolumeAccessMode{core.ReadWriteOnce},
				PersistentVolumeSource: core.PersistentVolumeSource{
					HostPath: &core.HostPathVolumeSource{
						Path: "/foo",
						Type: newHostPathType(string(core.HostPathDirectory)),
					},
				},
				StorageClassName: "-invalid-",
			}),
		},
		"bad-hostpath-volume-backsteps": {
			isExpectedFailure: true,
			volume: testVolume("foo", "", core.PersistentVolumeSpec{
				Capacity: core.ResourceList{
					core.ResourceName(core.ResourceStorage): resource.MustParse("10G"),
				},
				AccessModes: []core.PersistentVolumeAccessMode{core.ReadWriteOnce},
				PersistentVolumeSource: core.PersistentVolumeSource{
					HostPath: &core.HostPathVolumeSource{
						Path: "/foo/..",
						Type: newHostPathType(string(core.HostPathDirectory)),
					},
				},
				StorageClassName: "backstep-hostpath",
			}),
		},
		"volume-node-affinity": {
			isExpectedFailure: false,
			volume:            testVolumeWithNodeAffinity(simpleVolumeNodeAffinity("foo", "bar")),
		},
		"volume-empty-node-affinity": {
			isExpectedFailure: true,
			volume:            testVolumeWithNodeAffinity(&core.VolumeNodeAffinity{}),
		},
		"volume-bad-node-affinity": {
			isExpectedFailure: true,
			volume: testVolumeWithNodeAffinity(
				&core.VolumeNodeAffinity{
					Required: &core.NodeSelector{
						NodeSelectorTerms: []core.NodeSelectorTerm{
							{
								MatchExpressions: []core.NodeSelectorRequirement{
									{
										Operator: core.NodeSelectorOpIn,
										Values:   []string{"test-label-value"},
									},
								},
							},
						},
					},
				}),
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.ReadWriteOncePod, scenario.enableReadWriteOncePod)()

			opts := ValidationOptionsForPersistentVolume(scenario.volume, nil)
			errs := ValidatePersistentVolume(scenario.volume, opts)
			if len(errs) == 0 && scenario.isExpectedFailure {
				t.Errorf("Unexpected success for scenario: %s", name)
			}
			if len(errs) > 0 && !scenario.isExpectedFailure {
				t.Errorf("Unexpected failure for scenario: %s - %+v", name, errs)
			}
		})
	}

}

func TestValidatePersistentVolumeSpec(t *testing.T) {
	fsmode := core.PersistentVolumeFilesystem
	blockmode := core.PersistentVolumeBlock
	scenarios := map[string]struct {
		isExpectedFailure bool
		isInlineSpec      bool
		pvSpec            *core.PersistentVolumeSpec
	}{
		"pv-pvspec-valid": {
			isExpectedFailure: false,
			isInlineSpec:      false,
			pvSpec: &core.PersistentVolumeSpec{
				Capacity: core.ResourceList{
					core.ResourceName(core.ResourceStorage): resource.MustParse("10G"),
				},
				StorageClassName:              "testclass",
				PersistentVolumeReclaimPolicy: core.PersistentVolumeReclaimRecycle,
				AccessModes:                   []core.PersistentVolumeAccessMode{core.ReadWriteOnce},
				PersistentVolumeSource: core.PersistentVolumeSource{
					HostPath: &core.HostPathVolumeSource{
						Path: "/foo",
						Type: newHostPathType(string(core.HostPathDirectory)),
					},
				},
				VolumeMode:   &fsmode,
				NodeAffinity: simpleVolumeNodeAffinity("foo", "bar"),
			},
		},
		"inline-pvspec-with-capacity": {
			isExpectedFailure: true,
			isInlineSpec:      true,
			pvSpec: &core.PersistentVolumeSpec{
				Capacity: core.ResourceList{
					core.ResourceName(core.ResourceStorage): resource.MustParse("10G"),
				},
				PersistentVolumeSource: core.PersistentVolumeSource{
					CSI: &core.CSIPersistentVolumeSource{Driver: "test-driver", VolumeHandle: "test-123", ReadOnly: true},
				},
				AccessModes: []core.PersistentVolumeAccessMode{core.ReadWriteOnce},
			},
		},
		"inline-pvspec-with-sc": {
			isExpectedFailure: true,
			isInlineSpec:      true,
			pvSpec: &core.PersistentVolumeSpec{
				PersistentVolumeSource: core.PersistentVolumeSource{
					CSI: &core.CSIPersistentVolumeSource{Driver: "test-driver", VolumeHandle: "test-123", ReadOnly: true},
				},
				AccessModes:      []core.PersistentVolumeAccessMode{core.ReadWriteOnce},
				StorageClassName: "testclass",
			},
		},
		"inline-pvspec-with-non-fs-volume-mode": {
			isExpectedFailure: true,
			isInlineSpec:      true,
			pvSpec: &core.PersistentVolumeSpec{
				PersistentVolumeSource: core.PersistentVolumeSource{
					CSI: &core.CSIPersistentVolumeSource{Driver: "test-driver", VolumeHandle: "test-123", ReadOnly: true},
				},
				AccessModes: []core.PersistentVolumeAccessMode{core.ReadWriteOnce},
				VolumeMode:  &blockmode,
			},
		},
		"inline-pvspec-with-non-retain-reclaim-policy": {
			isExpectedFailure: true,
			isInlineSpec:      true,
			pvSpec: &core.PersistentVolumeSpec{
				PersistentVolumeReclaimPolicy: core.PersistentVolumeReclaimRecycle,
				PersistentVolumeSource: core.PersistentVolumeSource{
					CSI: &core.CSIPersistentVolumeSource{Driver: "test-driver", VolumeHandle: "test-123", ReadOnly: true},
				},
				AccessModes: []core.PersistentVolumeAccessMode{core.ReadWriteOnce},
			},
		},
		"inline-pvspec-with-node-affinity": {
			isExpectedFailure: true,
			isInlineSpec:      true,
			pvSpec: &core.PersistentVolumeSpec{
				PersistentVolumeSource: core.PersistentVolumeSource{
					CSI: &core.CSIPersistentVolumeSource{Driver: "test-driver", VolumeHandle: "test-123", ReadOnly: true},
				},
				AccessModes:  []core.PersistentVolumeAccessMode{core.ReadWriteOnce},
				NodeAffinity: simpleVolumeNodeAffinity("foo", "bar"),
			},
		},
		"inline-pvspec-with-non-csi-source": {
			isExpectedFailure: true,
			isInlineSpec:      true,
			pvSpec: &core.PersistentVolumeSpec{
				PersistentVolumeSource: core.PersistentVolumeSource{
					HostPath: &core.HostPathVolumeSource{
						Path: "/foo",
						Type: newHostPathType(string(core.HostPathDirectory)),
					},
				},
				AccessModes: []core.PersistentVolumeAccessMode{core.ReadWriteOnce},
			},
		},
		"inline-pvspec-valid-with-access-modes-and-mount-options": {
			isExpectedFailure: false,
			isInlineSpec:      true,
			pvSpec: &core.PersistentVolumeSpec{
				PersistentVolumeSource: core.PersistentVolumeSource{
					CSI: &core.CSIPersistentVolumeSource{Driver: "test-driver", VolumeHandle: "test-123", ReadOnly: true},
				},
				AccessModes:  []core.PersistentVolumeAccessMode{core.ReadWriteOnce},
				MountOptions: []string{"soft", "read-write"},
			},
		},
		"inline-pvspec-valid-with-access-modes": {
			isExpectedFailure: false,
			isInlineSpec:      true,
			pvSpec: &core.PersistentVolumeSpec{
				PersistentVolumeSource: core.PersistentVolumeSource{
					CSI: &core.CSIPersistentVolumeSource{Driver: "test-driver", VolumeHandle: "test-123", ReadOnly: true},
				},
				AccessModes: []core.PersistentVolumeAccessMode{core.ReadWriteOnce},
			},
		},
		"inline-pvspec-with-missing-acess-modes": {
			isExpectedFailure: true,
			isInlineSpec:      true,
			pvSpec: &core.PersistentVolumeSpec{
				PersistentVolumeSource: core.PersistentVolumeSource{
					CSI: &core.CSIPersistentVolumeSource{Driver: "test-driver", VolumeHandle: "test-123", ReadOnly: true},
				},
				MountOptions: []string{"soft", "read-write"},
			},
		},
	}
	for name, scenario := range scenarios {
		opts := PersistentVolumeSpecValidationOptions{}
		errs := ValidatePersistentVolumeSpec(scenario.pvSpec, "", scenario.isInlineSpec, field.NewPath("field"), opts)
		if len(errs) == 0 && scenario.isExpectedFailure {
			t.Errorf("Unexpected success for scenario: %s", name)
		}
		if len(errs) > 0 && !scenario.isExpectedFailure {
			t.Errorf("Unexpected failure for scenario: %s - %+v", name, errs)
		}
	}
}

func TestValidatePersistentVolumeSourceUpdate(t *testing.T) {
	validVolume := testVolume("foo", "", core.PersistentVolumeSpec{
		Capacity: core.ResourceList{
			core.ResourceName(core.ResourceStorage): resource.MustParse("1G"),
		},
		AccessModes: []core.PersistentVolumeAccessMode{core.ReadWriteOnce},
		PersistentVolumeSource: core.PersistentVolumeSource{
			HostPath: &core.HostPathVolumeSource{
				Path: "/foo",
				Type: newHostPathType(string(core.HostPathDirectory)),
			},
		},
		StorageClassName: "valid",
	})
	validPvSourceNoUpdate := validVolume.DeepCopy()
	invalidPvSourceUpdateType := validVolume.DeepCopy()
	invalidPvSourceUpdateType.Spec.PersistentVolumeSource = core.PersistentVolumeSource{
		FlexVolume: &core.FlexPersistentVolumeSource{
			Driver: "kubernetes.io/blue",
			FSType: "ext4",
		},
	}
	invalidPvSourceUpdateDeep := validVolume.DeepCopy()
	invalidPvSourceUpdateDeep.Spec.PersistentVolumeSource = core.PersistentVolumeSource{
		HostPath: &core.HostPathVolumeSource{
			Path: "/updated",
			Type: newHostPathType(string(core.HostPathDirectory)),
		},
	}

	validCSIVolume := testVolume("csi-volume", "", core.PersistentVolumeSpec{
		Capacity: core.ResourceList{
			core.ResourceName(core.ResourceStorage): resource.MustParse("1G"),
		},
		AccessModes: []core.PersistentVolumeAccessMode{core.ReadWriteOnce},
		PersistentVolumeSource: core.PersistentVolumeSource{
			CSI: &core.CSIPersistentVolumeSource{
				Driver:       "come.google.gcepd",
				VolumeHandle: "foobar",
			},
		},
		StorageClassName: "gp2",
	})

	expandSecretRef := &core.SecretReference{
		Name:      "expansion-secret",
		Namespace: "default",
	}

	scenarios := map[string]struct {
		isExpectedFailure   bool
		csiExpansionEnabled bool
		oldVolume           *core.PersistentVolume
		newVolume           *core.PersistentVolume
	}{
		"condition-no-update": {
			isExpectedFailure: false,
			oldVolume:         validVolume,
			newVolume:         validPvSourceNoUpdate,
		},
		"condition-update-source-type": {
			isExpectedFailure: true,
			oldVolume:         validVolume,
			newVolume:         invalidPvSourceUpdateType,
		},
		"condition-update-source-deep": {
			isExpectedFailure: true,
			oldVolume:         validVolume,
			newVolume:         invalidPvSourceUpdateDeep,
		},
		"csi-expansion-enabled-with-pv-secret": {
			csiExpansionEnabled: true,
			isExpectedFailure:   false,
			oldVolume:           validCSIVolume,
			newVolume:           getCSIVolumeWithSecret(validCSIVolume, expandSecretRef),
		},
		"csi-expansion-enabled-with-old-pv-secret": {
			csiExpansionEnabled: true,
			isExpectedFailure:   true,
			oldVolume:           getCSIVolumeWithSecret(validCSIVolume, expandSecretRef),
			newVolume: getCSIVolumeWithSecret(validCSIVolume, &core.SecretReference{
				Name:      "foo-secret",
				Namespace: "default",
			}),
		},
	}
	for name, scenario := range scenarios {
		opts := ValidationOptionsForPersistentVolume(scenario.newVolume, scenario.oldVolume)
		errs := ValidatePersistentVolumeUpdate(scenario.newVolume, scenario.oldVolume, opts)
		if len(errs) == 0 && scenario.isExpectedFailure {
			t.Errorf("Unexpected success for scenario: %s", name)
		}
		if len(errs) > 0 && !scenario.isExpectedFailure {
			t.Errorf("Unexpected failure for scenario: %s - %+v", name, errs)
		}
	}
}

func TestValidationOptionsForPersistentVolume(t *testing.T) {
	tests := map[string]struct {
		oldPv                  *core.PersistentVolume
		enableReadWriteOncePod bool
		expectValidationOpts   PersistentVolumeSpecValidationOptions
	}{
		"nil old pv": {
			oldPv:                  nil,
			enableReadWriteOncePod: true,
			expectValidationOpts: PersistentVolumeSpecValidationOptions{
				AllowReadWriteOncePod: true,
			},
		},
		"rwop allowed because feature enabled": {
			oldPv:                  pvWithAccessModes([]core.PersistentVolumeAccessMode{core.ReadWriteOnce}),
			enableReadWriteOncePod: true,
			expectValidationOpts: PersistentVolumeSpecValidationOptions{
				AllowReadWriteOncePod: true,
			},
		},
		"rwop not allowed because not used and feature disabled": {
			oldPv:                  pvWithAccessModes([]core.PersistentVolumeAccessMode{core.ReadWriteOnce}),
			enableReadWriteOncePod: false,
			expectValidationOpts: PersistentVolumeSpecValidationOptions{
				AllowReadWriteOncePod: false,
			},
		},
		"rwop allowed because used and feature enabled": {
			oldPv:                  pvWithAccessModes([]core.PersistentVolumeAccessMode{core.ReadWriteOncePod}),
			enableReadWriteOncePod: true,
			expectValidationOpts: PersistentVolumeSpecValidationOptions{
				AllowReadWriteOncePod: true,
			},
		},
		"rwop allowed because used and feature disabled": {
			oldPv:                  pvWithAccessModes([]core.PersistentVolumeAccessMode{core.ReadWriteOncePod}),
			enableReadWriteOncePod: false,
			expectValidationOpts: PersistentVolumeSpecValidationOptions{
				AllowReadWriteOncePod: true,
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.ReadWriteOncePod, tc.enableReadWriteOncePod)()

			opts := ValidationOptionsForPersistentVolume(nil, tc.oldPv)
			if opts != tc.expectValidationOpts {
				t.Errorf("Expected opts: %+v, received: %+v", opts, tc.expectValidationOpts)
			}
		})
	}
}

func pvWithAccessModes(accessModes []core.PersistentVolumeAccessMode) *core.PersistentVolume {
	return &core.PersistentVolume{
		Spec: core.PersistentVolumeSpec{
			AccessModes: accessModes,
		},
	}
}

func pvcWithAccessModes(accessModes []core.PersistentVolumeAccessMode) *core.PersistentVolumeClaim {
	return &core.PersistentVolumeClaim{
		Spec: core.PersistentVolumeClaimSpec{
			AccessModes: accessModes,
		},
	}
}

func pvcTemplateWithAccessModes(accessModes []core.PersistentVolumeAccessMode) *core.PersistentVolumeClaimTemplate {
	return &core.PersistentVolumeClaimTemplate{
		Spec: core.PersistentVolumeClaimSpec{
			AccessModes: accessModes,
		},
	}
}

func getCSIVolumeWithSecret(pv *core.PersistentVolume, secret *core.SecretReference) *core.PersistentVolume {
	pvCopy := pv.DeepCopy()
	if secret != nil {
		pvCopy.Spec.CSI.ControllerExpandSecretRef = secret
	}
	return pvCopy
}

func testLocalVolume(path string, affinity *core.VolumeNodeAffinity) core.PersistentVolumeSpec {
	return core.PersistentVolumeSpec{
		Capacity: core.ResourceList{
			core.ResourceName(core.ResourceStorage): resource.MustParse("10G"),
		},
		AccessModes: []core.PersistentVolumeAccessMode{core.ReadWriteOnce},
		PersistentVolumeSource: core.PersistentVolumeSource{
			Local: &core.LocalVolumeSource{
				Path: path,
			},
		},
		NodeAffinity:     affinity,
		StorageClassName: "test-storage-class",
	}
}

func TestValidateLocalVolumes(t *testing.T) {
	scenarios := map[string]struct {
		isExpectedFailure bool
		volume            *core.PersistentVolume
	}{
		"alpha invalid local volume nil annotations": {
			isExpectedFailure: true,
			volume: testVolume(
				"invalid-local-volume-nil-annotations",
				"",
				testLocalVolume("/foo", nil)),
		},
		"valid local volume": {
			isExpectedFailure: false,
			volume: testVolume("valid-local-volume", "",
				testLocalVolume("/foo", simpleVolumeNodeAffinity("foo", "bar"))),
		},
		"invalid local volume no node affinity": {
			isExpectedFailure: true,
			volume: testVolume("invalid-local-volume-no-node-affinity", "",
				testLocalVolume("/foo", nil)),
		},
		"invalid local volume empty path": {
			isExpectedFailure: true,
			volume: testVolume("invalid-local-volume-empty-path", "",
				testLocalVolume("", simpleVolumeNodeAffinity("foo", "bar"))),
		},
		"invalid-local-volume-backsteps": {
			isExpectedFailure: true,
			volume: testVolume("foo", "",
				testLocalVolume("/foo/..", simpleVolumeNodeAffinity("foo", "bar"))),
		},
		"valid-local-volume-relative-path": {
			isExpectedFailure: false,
			volume: testVolume("foo", "",
				testLocalVolume("foo", simpleVolumeNodeAffinity("foo", "bar"))),
		},
	}

	for name, scenario := range scenarios {
		opts := ValidationOptionsForPersistentVolume(scenario.volume, nil)
		errs := ValidatePersistentVolume(scenario.volume, opts)
		if len(errs) == 0 && scenario.isExpectedFailure {
			t.Errorf("Unexpected success for scenario: %s", name)
		}
		if len(errs) > 0 && !scenario.isExpectedFailure {
			t.Errorf("Unexpected failure for scenario: %s - %+v", name, errs)
		}
	}
}

func testVolumeWithNodeAffinity(affinity *core.VolumeNodeAffinity) *core.PersistentVolume {
	return testVolume("test-affinity-volume", "",
		core.PersistentVolumeSpec{
			Capacity: core.ResourceList{
				core.ResourceName(core.ResourceStorage): resource.MustParse("10G"),
			},
			AccessModes: []core.PersistentVolumeAccessMode{core.ReadWriteOnce},
			PersistentVolumeSource: core.PersistentVolumeSource{
				GCEPersistentDisk: &core.GCEPersistentDiskVolumeSource{
					PDName: "foo",
				},
			},
			StorageClassName: "test-storage-class",
			NodeAffinity:     affinity,
		})
}

func simpleVolumeNodeAffinity(key, value string) *core.VolumeNodeAffinity {
	return &core.VolumeNodeAffinity{
		Required: &core.NodeSelector{
			NodeSelectorTerms: []core.NodeSelectorTerm{
				{
					MatchExpressions: []core.NodeSelectorRequirement{
						{
							Key:      key,
							Operator: core.NodeSelectorOpIn,
							Values:   []string{value},
						},
					},
				},
			},
		},
	}
}

func TestValidateVolumeNodeAffinityUpdate(t *testing.T) {
	scenarios := map[string]struct {
		isExpectedFailure bool
		oldPV             *core.PersistentVolume
		newPV             *core.PersistentVolume
	}{
		"nil-nothing-changed": {
			isExpectedFailure: false,
			oldPV:             testVolumeWithNodeAffinity(nil),
			newPV:             testVolumeWithNodeAffinity(nil),
		},
		"affinity-nothing-changed": {
			isExpectedFailure: false,
			oldPV:             testVolumeWithNodeAffinity(simpleVolumeNodeAffinity("foo", "bar")),
			newPV:             testVolumeWithNodeAffinity(simpleVolumeNodeAffinity("foo", "bar")),
		},
		"affinity-changed": {
			isExpectedFailure: true,
			oldPV:             testVolumeWithNodeAffinity(simpleVolumeNodeAffinity("foo", "bar")),
			newPV:             testVolumeWithNodeAffinity(simpleVolumeNodeAffinity("foo", "bar2")),
		},
		"nil-to-obj": {
			isExpectedFailure: false,
			oldPV:             testVolumeWithNodeAffinity(nil),
			newPV:             testVolumeWithNodeAffinity(simpleVolumeNodeAffinity("foo", "bar")),
		},
		"obj-to-nil": {
			isExpectedFailure: true,
			oldPV:             testVolumeWithNodeAffinity(simpleVolumeNodeAffinity("foo", "bar")),
			newPV:             testVolumeWithNodeAffinity(nil),
		},
	}

	for name, scenario := range scenarios {
		opts := ValidationOptionsForPersistentVolume(scenario.newPV, scenario.oldPV)
		errs := ValidatePersistentVolumeUpdate(scenario.newPV, scenario.oldPV, opts)
		if len(errs) == 0 && scenario.isExpectedFailure {
			t.Errorf("Unexpected success for scenario: %s", name)
		}
		if len(errs) > 0 && !scenario.isExpectedFailure {
			t.Errorf("Unexpected failure for scenario: %s - %+v", name, errs)
		}
	}
}

func testVolumeClaim(name string, namespace string, spec core.PersistentVolumeClaimSpec) *core.PersistentVolumeClaim {
	return &core.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec:       spec,
	}
}

func testVolumeClaimWithStatus(
	name, namespace string,
	spec core.PersistentVolumeClaimSpec,
	status core.PersistentVolumeClaimStatus) *core.PersistentVolumeClaim {
	return &core.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec:       spec,
		Status:     status,
	}
}

func testVolumeClaimStorageClass(name string, namespace string, annval string, spec core.PersistentVolumeClaimSpec) *core.PersistentVolumeClaim {
	annotations := map[string]string{
		v1.BetaStorageClassAnnotation: annval,
	}

	return &core.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Annotations: annotations,
		},
		Spec: spec,
	}
}

func testVolumeClaimAnnotation(name string, namespace string, ann string, annval string, spec core.PersistentVolumeClaimSpec) *core.PersistentVolumeClaim {
	annotations := map[string]string{
		ann: annval,
	}

	return &core.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Annotations: annotations,
		},
		Spec: spec,
	}
}

func testVolumeClaimStorageClassInSpec(name, namespace, scName string, spec core.PersistentVolumeClaimSpec) *core.PersistentVolumeClaim {
	spec.StorageClassName = &scName
	return &core.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: spec,
	}
}

func testVolumeSnapshotDataSourceInSpec(name string, kind string, apiGroup string) *core.PersistentVolumeClaimSpec {
	scName := "csi-plugin"
	dataSourceInSpec := core.PersistentVolumeClaimSpec{
		AccessModes: []core.PersistentVolumeAccessMode{
			core.ReadOnlyMany,
		},
		Resources: core.ResourceRequirements{
			Requests: core.ResourceList{
				core.ResourceName(core.ResourceStorage): resource.MustParse("10G"),
			},
		},
		StorageClassName: &scName,
		DataSource: &core.TypedLocalObjectReference{
			APIGroup: &apiGroup,
			Kind:     kind,
			Name:     name,
		},
	}

	return &dataSourceInSpec
}

func TestAlphaVolumeSnapshotDataSource(t *testing.T) {
	successTestCases := []core.PersistentVolumeClaimSpec{
		*testVolumeSnapshotDataSourceInSpec("test_snapshot", "VolumeSnapshot", "snapshot.storage.k8s.io"),
	}
	failedTestCases := []core.PersistentVolumeClaimSpec{
		*testVolumeSnapshotDataSourceInSpec("", "VolumeSnapshot", "snapshot.storage.k8s.io"),
		*testVolumeSnapshotDataSourceInSpec("test_snapshot", "", "snapshot.storage.k8s.io"),
	}

	for _, tc := range successTestCases {
		opts := PersistentVolumeClaimSpecValidationOptions{}
		if errs := ValidatePersistentVolumeClaimSpec(&tc, field.NewPath("spec"), opts); len(errs) != 0 {
			t.Errorf("expected success: %v", errs)
		}
	}
	for _, tc := range failedTestCases {
		opts := PersistentVolumeClaimSpecValidationOptions{}
		if errs := ValidatePersistentVolumeClaimSpec(&tc, field.NewPath("spec"), opts); len(errs) == 0 {
			t.Errorf("expected failure: %v", errs)
		}
	}
}

func testVolumeClaimStorageClassInAnnotationAndSpec(name, namespace, scNameInAnn, scName string, spec core.PersistentVolumeClaimSpec) *core.PersistentVolumeClaim {
	spec.StorageClassName = &scName
	return &core.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Annotations: map[string]string{v1.BetaStorageClassAnnotation: scNameInAnn},
		},
		Spec: spec,
	}
}

func testValidatePVC(t *testing.T, ephemeral bool) {
	invalidClassName := "-invalid-"
	validClassName := "valid"
	invalidMode := core.PersistentVolumeMode("fakeVolumeMode")
	validMode := core.PersistentVolumeFilesystem
	goodName := "foo"
	goodNS := "ns"
	if ephemeral {
		// Must be empty for ephemeral inline volumes.
		goodName = ""
		goodNS = ""
	}
	goodClaimSpec := core.PersistentVolumeClaimSpec{
		Selector: &metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{
					Key:      "key2",
					Operator: "Exists",
				},
			},
		},
		AccessModes: []core.PersistentVolumeAccessMode{
			core.ReadWriteOnce,
			core.ReadOnlyMany,
		},
		Resources: core.ResourceRequirements{
			Requests: core.ResourceList{
				core.ResourceName(core.ResourceStorage): resource.MustParse("10G"),
			},
		},
		StorageClassName: &validClassName,
		VolumeMode:       &validMode,
	}
	now := metav1.Now()
	ten := int64(10)

	scenarios := map[string]struct {
		isExpectedFailure      bool
		enableReadWriteOncePod bool
		claim                  *core.PersistentVolumeClaim
	}{
		"good-claim": {
			isExpectedFailure: false,
			claim:             testVolumeClaim(goodName, goodNS, goodClaimSpec),
		},
		"missing-name": {
			isExpectedFailure: !ephemeral,
			claim:             testVolumeClaim("", goodNS, goodClaimSpec),
		},
		"missing-namespace": {
			isExpectedFailure: !ephemeral,
			claim:             testVolumeClaim(goodName, "", goodClaimSpec),
		},
		"with-generate-name": {
			isExpectedFailure: ephemeral,
			claim: func() *core.PersistentVolumeClaim {
				claim := testVolumeClaim(goodName, goodNS, goodClaimSpec)
				claim.GenerateName = "pvc-"
				return claim
			}(),
		},
		"with-uid": {
			isExpectedFailure: ephemeral,
			claim: func() *core.PersistentVolumeClaim {
				claim := testVolumeClaim(goodName, goodNS, goodClaimSpec)
				claim.UID = "ac051fac-2ead-46d9-b8b4-4e0fbeb7455d"
				return claim
			}(),
		},
		"with-resource-version": {
			isExpectedFailure: ephemeral,
			claim: func() *core.PersistentVolumeClaim {
				claim := testVolumeClaim(goodName, goodNS, goodClaimSpec)
				claim.ResourceVersion = "1"
				return claim
			}(),
		},
		"with-generation": {
			isExpectedFailure: ephemeral,
			claim: func() *core.PersistentVolumeClaim {
				claim := testVolumeClaim(goodName, goodNS, goodClaimSpec)
				claim.Generation = 100
				return claim
			}(),
		},
		"with-creation-timestamp": {
			isExpectedFailure: ephemeral,
			claim: func() *core.PersistentVolumeClaim {
				claim := testVolumeClaim(goodName, goodNS, goodClaimSpec)
				claim.CreationTimestamp = now
				return claim
			}(),
		},
		"with-deletion-grace-period-seconds": {
			isExpectedFailure: ephemeral,
			claim: func() *core.PersistentVolumeClaim {
				claim := testVolumeClaim(goodName, goodNS, goodClaimSpec)
				claim.DeletionGracePeriodSeconds = &ten
				return claim
			}(),
		},
		"with-owner-references": {
			isExpectedFailure: ephemeral,
			claim: func() *core.PersistentVolumeClaim {
				claim := testVolumeClaim(goodName, goodNS, goodClaimSpec)
				claim.OwnerReferences = []metav1.OwnerReference{
					{
						APIVersion: "v1",
						Kind:       "pod",
						Name:       "foo",
						UID:        "ac051fac-2ead-46d9-b8b4-4e0fbeb7455d",
					},
				}
				return claim
			}(),
		},
		"with-finalizers": {
			isExpectedFailure: ephemeral,
			claim: func() *core.PersistentVolumeClaim {
				claim := testVolumeClaim(goodName, goodNS, goodClaimSpec)
				claim.Finalizers = []string{
					"example.com/foo",
				}
				return claim
			}(),
		},
		"with-cluster-name": {
			isExpectedFailure: ephemeral,
			claim: func() *core.PersistentVolumeClaim {
				claim := testVolumeClaim(goodName, goodNS, goodClaimSpec)
				claim.ClusterName = "foo"
				return claim
			}(),
		},
		"with-managed-fields": {
			isExpectedFailure: ephemeral,
			claim: func() *core.PersistentVolumeClaim {
				claim := testVolumeClaim(goodName, goodNS, goodClaimSpec)
				claim.ManagedFields = []metav1.ManagedFieldsEntry{
					{
						FieldsType: "FieldsV1",
						Operation:  "Apply",
						APIVersion: "apps/v1",
						Manager:    "foo",
					},
				}
				return claim
			}(),
		},
		"with-good-labels": {
			claim: func() *core.PersistentVolumeClaim {
				claim := testVolumeClaim(goodName, goodNS, goodClaimSpec)
				claim.Labels = map[string]string{
					"apps.kubernetes.io/name": "test",
				}
				return claim
			}(),
		},
		"with-bad-labels": {
			isExpectedFailure: true,
			claim: func() *core.PersistentVolumeClaim {
				claim := testVolumeClaim(goodName, goodNS, goodClaimSpec)
				claim.Labels = map[string]string{
					"hello-world": "hyphen not allowed",
				}
				return claim
			}(),
		},
		"with-good-annotations": {
			claim: func() *core.PersistentVolumeClaim {
				claim := testVolumeClaim(goodName, goodNS, goodClaimSpec)
				claim.Labels = map[string]string{
					"foo": "bar",
				}
				return claim
			}(),
		},
		"with-bad-annotations": {
			isExpectedFailure: true,
			claim: func() *core.PersistentVolumeClaim {
				claim := testVolumeClaim(goodName, goodNS, goodClaimSpec)
				claim.Labels = map[string]string{
					"hello-world": "hyphen not allowed",
				}
				return claim
			}(),
		},
		"with-read-write-once-pod-feature-gate-enabled": {
			isExpectedFailure:      false,
			enableReadWriteOncePod: true,
			claim: testVolumeClaim(goodName, goodNS, core.PersistentVolumeClaimSpec{
				AccessModes: []core.PersistentVolumeAccessMode{"ReadWriteOncePod"},
				Resources: core.ResourceRequirements{
					Requests: core.ResourceList{
						core.ResourceName(core.ResourceStorage): resource.MustParse("10G"),
					},
				},
			}),
		},
		"with-read-write-once-pod-feature-gate-disabled": {
			isExpectedFailure:      true,
			enableReadWriteOncePod: false,
			claim: testVolumeClaim(goodName, goodNS, core.PersistentVolumeClaimSpec{
				AccessModes: []core.PersistentVolumeAccessMode{"ReadWriteOncePod"},
				Resources: core.ResourceRequirements{
					Requests: core.ResourceList{
						core.ResourceName(core.ResourceStorage): resource.MustParse("10G"),
					},
				},
			}),
		},
		"with-read-write-once-pod-and-others-feature-gate-enabled": {
			isExpectedFailure:      true,
			enableReadWriteOncePod: true,
			claim: testVolumeClaim(goodName, goodNS, core.PersistentVolumeClaimSpec{
				AccessModes: []core.PersistentVolumeAccessMode{"ReadWriteOncePod", "ReadWriteMany"},
				Resources: core.ResourceRequirements{
					Requests: core.ResourceList{
						core.ResourceName(core.ResourceStorage): resource.MustParse("10G"),
					},
				},
			}),
		},
		"invalid-claim-zero-capacity": {
			isExpectedFailure: true,
			claim: testVolumeClaim(goodName, goodNS, core.PersistentVolumeClaimSpec{
				Selector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "key2",
							Operator: "Exists",
						},
					},
				},
				AccessModes: []core.PersistentVolumeAccessMode{
					core.ReadWriteOnce,
					core.ReadOnlyMany,
				},
				Resources: core.ResourceRequirements{
					Requests: core.ResourceList{
						core.ResourceName(core.ResourceStorage): resource.MustParse("0G"),
					},
				},
				StorageClassName: &validClassName,
			}),
		},
		"invalid-label-selector": {
			isExpectedFailure: true,
			claim: testVolumeClaim(goodName, goodNS, core.PersistentVolumeClaimSpec{
				Selector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "key2",
							Operator: "InvalidOp",
							Values:   []string{"value1", "value2"},
						},
					},
				},
				AccessModes: []core.PersistentVolumeAccessMode{
					core.ReadWriteOnce,
					core.ReadOnlyMany,
				},
				Resources: core.ResourceRequirements{
					Requests: core.ResourceList{
						core.ResourceName(core.ResourceStorage): resource.MustParse("10G"),
					},
				},
			}),
		},
		"invalid-accessmode": {
			isExpectedFailure: true,
			claim: testVolumeClaim(goodName, goodNS, core.PersistentVolumeClaimSpec{
				AccessModes: []core.PersistentVolumeAccessMode{"fakemode"},
				Resources: core.ResourceRequirements{
					Requests: core.ResourceList{
						core.ResourceName(core.ResourceStorage): resource.MustParse("10G"),
					},
				},
			}),
		},
		"no-access-modes": {
			isExpectedFailure: true,
			claim: testVolumeClaim(goodName, goodNS, core.PersistentVolumeClaimSpec{
				Resources: core.ResourceRequirements{
					Requests: core.ResourceList{
						core.ResourceName(core.ResourceStorage): resource.MustParse("10G"),
					},
				},
			}),
		},
		"no-resource-requests": {
			isExpectedFailure: true,
			claim: testVolumeClaim(goodName, goodNS, core.PersistentVolumeClaimSpec{
				AccessModes: []core.PersistentVolumeAccessMode{
					core.ReadWriteOnce,
				},
			}),
		},
		"invalid-resource-requests": {
			isExpectedFailure: true,
			claim: testVolumeClaim(goodName, goodNS, core.PersistentVolumeClaimSpec{
				AccessModes: []core.PersistentVolumeAccessMode{
					core.ReadWriteOnce,
				},
				Resources: core.ResourceRequirements{
					Requests: core.ResourceList{
						core.ResourceName(core.ResourceMemory): resource.MustParse("10G"),
					},
				},
			}),
		},
		"negative-storage-request": {
			isExpectedFailure: true,
			claim: testVolumeClaim(goodName, goodNS, core.PersistentVolumeClaimSpec{
				Selector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "key2",
							Operator: "Exists",
						},
					},
				},
				AccessModes: []core.PersistentVolumeAccessMode{
					core.ReadWriteOnce,
					core.ReadOnlyMany,
				},
				Resources: core.ResourceRequirements{
					Requests: core.ResourceList{
						core.ResourceName(core.ResourceStorage): resource.MustParse("-10G"),
					},
				},
			}),
		},
		"zero-storage-request": {
			isExpectedFailure: true,
			claim: testVolumeClaim(goodName, goodNS, core.PersistentVolumeClaimSpec{
				Selector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "key2",
							Operator: "Exists",
						},
					},
				},
				AccessModes: []core.PersistentVolumeAccessMode{
					core.ReadWriteOnce,
					core.ReadOnlyMany,
				},
				Resources: core.ResourceRequirements{
					Requests: core.ResourceList{
						core.ResourceName(core.ResourceStorage): resource.MustParse("0G"),
					},
				},
			}),
		},
		"invalid-storage-class-name": {
			isExpectedFailure: true,
			claim: testVolumeClaim(goodName, goodNS, core.PersistentVolumeClaimSpec{
				Selector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "key2",
							Operator: "Exists",
						},
					},
				},
				AccessModes: []core.PersistentVolumeAccessMode{
					core.ReadWriteOnce,
					core.ReadOnlyMany,
				},
				Resources: core.ResourceRequirements{
					Requests: core.ResourceList{
						core.ResourceName(core.ResourceStorage): resource.MustParse("10G"),
					},
				},
				StorageClassName: &invalidClassName,
			}),
		},
		"invalid-volume-mode": {
			isExpectedFailure: true,
			claim: testVolumeClaim(goodName, goodNS, core.PersistentVolumeClaimSpec{
				AccessModes: []core.PersistentVolumeAccessMode{
					core.ReadWriteOnce,
					core.ReadOnlyMany,
				},
				Resources: core.ResourceRequirements{
					Requests: core.ResourceList{
						core.ResourceName(core.ResourceStorage): resource.MustParse("10G"),
					},
				},
				VolumeMode: &invalidMode,
			}),
		},
		"mismatch-data-source-and-ref": {
			isExpectedFailure: true,
			claim: testVolumeClaim(goodName, goodNS, core.PersistentVolumeClaimSpec{
				AccessModes: []core.PersistentVolumeAccessMode{
					core.ReadWriteOnce,
				},
				Resources: core.ResourceRequirements{
					Requests: core.ResourceList{
						core.ResourceName(core.ResourceStorage): resource.MustParse("10G"),
					},
				},
				DataSource: &core.TypedLocalObjectReference{
					Kind: "PersistentVolumeClaim",
					Name: "pvc1",
				},
				DataSourceRef: &core.TypedLocalObjectReference{
					Kind: "PersistentVolumeClaim",
					Name: "pvc2",
				},
			}),
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.ReadWriteOncePod, scenario.enableReadWriteOncePod)()

			var errs field.ErrorList
			if ephemeral {
				volumes := []core.Volume{
					{
						Name: "foo",
						VolumeSource: core.VolumeSource{
							Ephemeral: &core.EphemeralVolumeSource{
								VolumeClaimTemplate: &core.PersistentVolumeClaimTemplate{
									ObjectMeta: scenario.claim.ObjectMeta,
									Spec:       scenario.claim.Spec,
								},
							},
						},
					},
				}
				opts := PodValidationOptions{}
				_, errs = ValidateVolumes(volumes, nil, field.NewPath(""), opts)
			} else {
				opts := ValidationOptionsForPersistentVolumeClaim(scenario.claim, nil)
				errs = ValidatePersistentVolumeClaim(scenario.claim, opts)
			}
			if len(errs) == 0 && scenario.isExpectedFailure {
				t.Error("Unexpected success for scenario")
			}
			if len(errs) > 0 && !scenario.isExpectedFailure {
				t.Errorf("Unexpected failure: %+v", errs)
			}
		})
	}
}

func TestValidatePersistentVolumeClaim(t *testing.T) {
	testValidatePVC(t, false)
}

func TestValidateEphemeralVolume(t *testing.T) {
	testValidatePVC(t, true)
}

func TestAlphaPVVolumeModeUpdate(t *testing.T) {
	block := core.PersistentVolumeBlock
	file := core.PersistentVolumeFilesystem

	scenarios := map[string]struct {
		isExpectedFailure bool
		oldPV             *core.PersistentVolume
		newPV             *core.PersistentVolume
	}{
		"valid-update-volume-mode-block-to-block": {
			isExpectedFailure: false,
			oldPV:             createTestVolModePV(&block),
			newPV:             createTestVolModePV(&block),
		},
		"valid-update-volume-mode-file-to-file": {
			isExpectedFailure: false,
			oldPV:             createTestVolModePV(&file),
			newPV:             createTestVolModePV(&file),
		},
		"invalid-update-volume-mode-to-block": {
			isExpectedFailure: true,
			oldPV:             createTestVolModePV(&file),
			newPV:             createTestVolModePV(&block),
		},
		"invalid-update-volume-mode-to-file": {
			isExpectedFailure: true,
			oldPV:             createTestVolModePV(&block),
			newPV:             createTestVolModePV(&file),
		},
		"invalid-update-volume-mode-nil-to-file": {
			isExpectedFailure: true,
			oldPV:             createTestVolModePV(nil),
			newPV:             createTestVolModePV(&file),
		},
		"invalid-update-volume-mode-nil-to-block": {
			isExpectedFailure: true,
			oldPV:             createTestVolModePV(nil),
			newPV:             createTestVolModePV(&block),
		},
		"invalid-update-volume-mode-file-to-nil": {
			isExpectedFailure: true,
			oldPV:             createTestVolModePV(&file),
			newPV:             createTestVolModePV(nil),
		},
		"invalid-update-volume-mode-block-to-nil": {
			isExpectedFailure: true,
			oldPV:             createTestVolModePV(&block),
			newPV:             createTestVolModePV(nil),
		},
		"invalid-update-volume-mode-nil-to-nil": {
			isExpectedFailure: false,
			oldPV:             createTestVolModePV(nil),
			newPV:             createTestVolModePV(nil),
		},
		"invalid-update-volume-mode-empty-to-mode": {
			isExpectedFailure: true,
			oldPV:             createTestPV(),
			newPV:             createTestVolModePV(&block),
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			opts := ValidationOptionsForPersistentVolume(scenario.newPV, scenario.oldPV)
			// ensure we have a resource version specified for updates
			errs := ValidatePersistentVolumeUpdate(scenario.newPV, scenario.oldPV, opts)
			if len(errs) == 0 && scenario.isExpectedFailure {
				t.Errorf("Unexpected success for scenario: %s", name)
			}
			if len(errs) > 0 && !scenario.isExpectedFailure {
				t.Errorf("Unexpected failure for scenario: %s - %+v", name, errs)
			}
		})
	}
}

func TestValidatePersistentVolumeClaimUpdate(t *testing.T) {
	block := core.PersistentVolumeBlock
	file := core.PersistentVolumeFilesystem

	validClaim := testVolumeClaimWithStatus("foo", "ns", core.PersistentVolumeClaimSpec{
		AccessModes: []core.PersistentVolumeAccessMode{
			core.ReadWriteOnce,
			core.ReadOnlyMany,
		},
		Resources: core.ResourceRequirements{
			Requests: core.ResourceList{
				core.ResourceName(core.ResourceStorage): resource.MustParse("10G"),
			},
		},
	}, core.PersistentVolumeClaimStatus{
		Phase: core.ClaimBound,
	})

	validClaimStorageClass := testVolumeClaimStorageClass("foo", "ns", "fast", core.PersistentVolumeClaimSpec{
		AccessModes: []core.PersistentVolumeAccessMode{
			core.ReadOnlyMany,
		},
		Resources: core.ResourceRequirements{
			Requests: core.ResourceList{
				core.ResourceName(core.ResourceStorage): resource.MustParse("10G"),
			},
		},
	})
	validClaimAnnotation := testVolumeClaimAnnotation("foo", "ns", "description", "foo-description", core.PersistentVolumeClaimSpec{
		AccessModes: []core.PersistentVolumeAccessMode{
			core.ReadOnlyMany,
		},
		Resources: core.ResourceRequirements{
			Requests: core.ResourceList{
				core.ResourceName(core.ResourceStorage): resource.MustParse("10G"),
			},
		},
	})
	validUpdateClaim := testVolumeClaim("foo", "ns", core.PersistentVolumeClaimSpec{
		AccessModes: []core.PersistentVolumeAccessMode{
			core.ReadWriteOnce,
			core.ReadOnlyMany,
		},
		Resources: core.ResourceRequirements{
			Requests: core.ResourceList{
				core.ResourceName(core.ResourceStorage): resource.MustParse("10G"),
			},
		},
		VolumeName: "volume",
	})
	invalidUpdateClaimResources := testVolumeClaim("foo", "ns", core.PersistentVolumeClaimSpec{
		AccessModes: []core.PersistentVolumeAccessMode{
			core.ReadWriteOnce,
			core.ReadOnlyMany,
		},
		Resources: core.ResourceRequirements{
			Requests: core.ResourceList{
				core.ResourceName(core.ResourceStorage): resource.MustParse("20G"),
			},
		},
		VolumeName: "volume",
	})
	invalidUpdateClaimAccessModes := testVolumeClaim("foo", "ns", core.PersistentVolumeClaimSpec{
		AccessModes: []core.PersistentVolumeAccessMode{
			core.ReadWriteOnce,
		},
		Resources: core.ResourceRequirements{
			Requests: core.ResourceList{
				core.ResourceName(core.ResourceStorage): resource.MustParse("10G"),
			},
		},
		VolumeName: "volume",
	})
	validClaimVolumeModeFile := testVolumeClaim("foo", "ns", core.PersistentVolumeClaimSpec{
		AccessModes: []core.PersistentVolumeAccessMode{
			core.ReadWriteOnce,
		},
		VolumeMode: &file,
		Resources: core.ResourceRequirements{
			Requests: core.ResourceList{
				core.ResourceName(core.ResourceStorage): resource.MustParse("10G"),
			},
		},
		VolumeName: "volume",
	})
	validClaimVolumeModeBlock := testVolumeClaim("foo", "ns", core.PersistentVolumeClaimSpec{
		AccessModes: []core.PersistentVolumeAccessMode{
			core.ReadWriteOnce,
		},
		VolumeMode: &block,
		Resources: core.ResourceRequirements{
			Requests: core.ResourceList{
				core.ResourceName(core.ResourceStorage): resource.MustParse("10G"),
			},
		},
		VolumeName: "volume",
	})
	invalidClaimVolumeModeNil := testVolumeClaim("foo", "ns", core.PersistentVolumeClaimSpec{
		AccessModes: []core.PersistentVolumeAccessMode{
			core.ReadWriteOnce,
		},
		VolumeMode: nil,
		Resources: core.ResourceRequirements{
			Requests: core.ResourceList{
				core.ResourceName(core.ResourceStorage): resource.MustParse("10G"),
			},
		},
		VolumeName: "volume",
	})
	invalidUpdateClaimStorageClass := testVolumeClaimStorageClass("foo", "ns", "fast2", core.PersistentVolumeClaimSpec{
		AccessModes: []core.PersistentVolumeAccessMode{
			core.ReadOnlyMany,
		},
		Resources: core.ResourceRequirements{
			Requests: core.ResourceList{
				core.ResourceName(core.ResourceStorage): resource.MustParse("10G"),
			},
		},
		VolumeName: "volume",
	})
	validUpdateClaimMutableAnnotation := testVolumeClaimAnnotation("foo", "ns", "description", "updated-or-added-foo-description", core.PersistentVolumeClaimSpec{
		AccessModes: []core.PersistentVolumeAccessMode{
			core.ReadOnlyMany,
		},
		Resources: core.ResourceRequirements{
			Requests: core.ResourceList{
				core.ResourceName(core.ResourceStorage): resource.MustParse("10G"),
			},
		},
		VolumeName: "volume",
	})
	validAddClaimAnnotation := testVolumeClaimAnnotation("foo", "ns", "description", "updated-or-added-foo-description", core.PersistentVolumeClaimSpec{
		AccessModes: []core.PersistentVolumeAccessMode{
			core.ReadWriteOnce,
			core.ReadOnlyMany,
		},
		Resources: core.ResourceRequirements{
			Requests: core.ResourceList{
				core.ResourceName(core.ResourceStorage): resource.MustParse("10G"),
			},
		},
		VolumeName: "volume",
	})
	validSizeUpdate := testVolumeClaimWithStatus("foo", "ns", core.PersistentVolumeClaimSpec{
		AccessModes: []core.PersistentVolumeAccessMode{
			core.ReadWriteOnce,
			core.ReadOnlyMany,
		},
		Resources: core.ResourceRequirements{
			Requests: core.ResourceList{
				core.ResourceName(core.ResourceStorage): resource.MustParse("15G"),
			},
		},
	}, core.PersistentVolumeClaimStatus{
		Phase: core.ClaimBound,
	})

	invalidSizeUpdate := testVolumeClaimWithStatus("foo", "ns", core.PersistentVolumeClaimSpec{
		AccessModes: []core.PersistentVolumeAccessMode{
			core.ReadWriteOnce,
			core.ReadOnlyMany,
		},
		Resources: core.ResourceRequirements{
			Requests: core.ResourceList{
				core.ResourceName(core.ResourceStorage): resource.MustParse("5G"),
			},
		},
	}, core.PersistentVolumeClaimStatus{
		Phase: core.ClaimBound,
	})

	unboundSizeUpdate := testVolumeClaimWithStatus("foo", "ns", core.PersistentVolumeClaimSpec{
		AccessModes: []core.PersistentVolumeAccessMode{
			core.ReadWriteOnce,
			core.ReadOnlyMany,
		},
		Resources: core.ResourceRequirements{
			Requests: core.ResourceList{
				core.ResourceName(core.ResourceStorage): resource.MustParse("12G"),
			},
		},
	}, core.PersistentVolumeClaimStatus{
		Phase: core.ClaimPending,
	})

	validClaimStorageClassInSpec := testVolumeClaimStorageClassInSpec("foo", "ns", "fast", core.PersistentVolumeClaimSpec{
		AccessModes: []core.PersistentVolumeAccessMode{
			core.ReadOnlyMany,
		},
		Resources: core.ResourceRequirements{
			Requests: core.ResourceList{
				core.ResourceName(core.ResourceStorage): resource.MustParse("10G"),
			},
		},
	})

	invalidClaimStorageClassInSpec := testVolumeClaimStorageClassInSpec("foo", "ns", "fast2", core.PersistentVolumeClaimSpec{
		AccessModes: []core.PersistentVolumeAccessMode{
			core.ReadOnlyMany,
		},
		Resources: core.ResourceRequirements{
			Requests: core.ResourceList{
				core.ResourceName(core.ResourceStorage): resource.MustParse("10G"),
			},
		},
	})

	validClaimStorageClassInAnnotationAndSpec := testVolumeClaimStorageClassInAnnotationAndSpec(
		"foo", "ns", "fast", "fast", core.PersistentVolumeClaimSpec{
			AccessModes: []core.PersistentVolumeAccessMode{
				core.ReadOnlyMany,
			},
			Resources: core.ResourceRequirements{
				Requests: core.ResourceList{
					core.ResourceName(core.ResourceStorage): resource.MustParse("10G"),
				},
			},
		})

	invalidClaimStorageClassInAnnotationAndSpec := testVolumeClaimStorageClassInAnnotationAndSpec(
		"foo", "ns", "fast2", "fast", core.PersistentVolumeClaimSpec{
			AccessModes: []core.PersistentVolumeAccessMode{
				core.ReadOnlyMany,
			},
			Resources: core.ResourceRequirements{
				Requests: core.ResourceList{
					core.ResourceName(core.ResourceStorage): resource.MustParse("10G"),
				},
			},
		})

	validClaimRWOPAccessMode := testVolumeClaim("foo", "ns", core.PersistentVolumeClaimSpec{
		AccessModes: []core.PersistentVolumeAccessMode{
			core.ReadWriteOncePod,
		},
		Resources: core.ResourceRequirements{
			Requests: core.ResourceList{
				core.ResourceName(core.ResourceStorage): resource.MustParse("10G"),
			},
		},
		VolumeName: "volume",
	})

	validClaimRWOPAccessModeAddAnnotation := testVolumeClaimAnnotation("foo", "ns", "description", "updated-or-added-foo-description", core.PersistentVolumeClaimSpec{
		AccessModes: []core.PersistentVolumeAccessMode{
			core.ReadWriteOncePod,
		},
		Resources: core.ResourceRequirements{
			Requests: core.ResourceList{
				core.ResourceName(core.ResourceStorage): resource.MustParse("10G"),
			},
		},
		VolumeName: "volume",
	})

	scenarios := map[string]struct {
		isExpectedFailure bool
		oldClaim          *core.PersistentVolumeClaim
		newClaim          *core.PersistentVolumeClaim
		enableResize      bool
	}{
		"valid-update-volumeName-only": {
			isExpectedFailure: false,
			oldClaim:          validClaim,
			newClaim:          validUpdateClaim,
			enableResize:      false,
		},
		"valid-no-op-update": {
			isExpectedFailure: false,
			oldClaim:          validUpdateClaim,
			newClaim:          validUpdateClaim,
			enableResize:      false,
		},
		"invalid-update-change-resources-on-bound-claim": {
			isExpectedFailure: true,
			oldClaim:          validUpdateClaim,
			newClaim:          invalidUpdateClaimResources,
			enableResize:      false,
		},
		"invalid-update-change-access-modes-on-bound-claim": {
			isExpectedFailure: true,
			oldClaim:          validUpdateClaim,
			newClaim:          invalidUpdateClaimAccessModes,
			enableResize:      false,
		},
		"valid-update-volume-mode-block-to-block": {
			isExpectedFailure: false,
			oldClaim:          validClaimVolumeModeBlock,
			newClaim:          validClaimVolumeModeBlock,
			enableResize:      false,
		},
		"valid-update-volume-mode-file-to-file": {
			isExpectedFailure: false,
			oldClaim:          validClaimVolumeModeFile,
			newClaim:          validClaimVolumeModeFile,
			enableResize:      false,
		},
		"invalid-update-volume-mode-to-block": {
			isExpectedFailure: true,
			oldClaim:          validClaimVolumeModeFile,
			newClaim:          validClaimVolumeModeBlock,
			enableResize:      false,
		},
		"invalid-update-volume-mode-to-file": {
			isExpectedFailure: true,
			oldClaim:          validClaimVolumeModeBlock,
			newClaim:          validClaimVolumeModeFile,
			enableResize:      false,
		},
		"invalid-update-volume-mode-nil-to-file": {
			isExpectedFailure: true,
			oldClaim:          invalidClaimVolumeModeNil,
			newClaim:          validClaimVolumeModeFile,
			enableResize:      false,
		},
		"invalid-update-volume-mode-nil-to-block": {
			isExpectedFailure: true,
			oldClaim:          invalidClaimVolumeModeNil,
			newClaim:          validClaimVolumeModeBlock,
			enableResize:      false,
		},
		"invalid-update-volume-mode-block-to-nil": {
			isExpectedFailure: true,
			oldClaim:          validClaimVolumeModeBlock,
			newClaim:          invalidClaimVolumeModeNil,
			enableResize:      false,
		},
		"invalid-update-volume-mode-file-to-nil": {
			isExpectedFailure: true,
			oldClaim:          validClaimVolumeModeFile,
			newClaim:          invalidClaimVolumeModeNil,
			enableResize:      false,
		},
		"invalid-update-volume-mode-empty-to-mode": {
			isExpectedFailure: true,
			oldClaim:          validClaim,
			newClaim:          validClaimVolumeModeBlock,
			enableResize:      false,
		},
		"invalid-update-volume-mode-mode-to-empty": {
			isExpectedFailure: true,
			oldClaim:          validClaimVolumeModeBlock,
			newClaim:          validClaim,
			enableResize:      false,
		},
		"invalid-update-change-storage-class-annotation-after-creation": {
			isExpectedFailure: true,
			oldClaim:          validClaimStorageClass,
			newClaim:          invalidUpdateClaimStorageClass,
			enableResize:      false,
		},
		"valid-update-mutable-annotation": {
			isExpectedFailure: false,
			oldClaim:          validClaimAnnotation,
			newClaim:          validUpdateClaimMutableAnnotation,
			enableResize:      false,
		},
		"valid-update-add-annotation": {
			isExpectedFailure: false,
			oldClaim:          validClaim,
			newClaim:          validAddClaimAnnotation,
			enableResize:      false,
		},
		"valid-size-update-resize-disabled": {
			isExpectedFailure: true,
			oldClaim:          validClaim,
			newClaim:          validSizeUpdate,
			enableResize:      false,
		},
		"valid-size-update-resize-enabled": {
			isExpectedFailure: false,
			oldClaim:          validClaim,
			newClaim:          validSizeUpdate,
			enableResize:      true,
		},
		"invalid-size-update-resize-enabled": {
			isExpectedFailure: true,
			oldClaim:          validClaim,
			newClaim:          invalidSizeUpdate,
			enableResize:      true,
		},
		"unbound-size-update-resize-enabled": {
			isExpectedFailure: true,
			oldClaim:          validClaim,
			newClaim:          unboundSizeUpdate,
			enableResize:      true,
		},
		"valid-upgrade-storage-class-annotation-to-spec": {
			isExpectedFailure: false,
			oldClaim:          validClaimStorageClass,
			newClaim:          validClaimStorageClassInSpec,
			enableResize:      false,
		},
		"invalid-upgrade-storage-class-annotation-to-spec": {
			isExpectedFailure: true,
			oldClaim:          validClaimStorageClass,
			newClaim:          invalidClaimStorageClassInSpec,
			enableResize:      false,
		},
		"valid-upgrade-storage-class-annotation-to-annotation-and-spec": {
			isExpectedFailure: false,
			oldClaim:          validClaimStorageClass,
			newClaim:          validClaimStorageClassInAnnotationAndSpec,
			enableResize:      false,
		},
		"invalid-upgrade-storage-class-annotation-to-annotation-and-spec": {
			isExpectedFailure: true,
			oldClaim:          validClaimStorageClass,
			newClaim:          invalidClaimStorageClassInAnnotationAndSpec,
			enableResize:      false,
		},
		"invalid-upgrade-storage-class-in-spec": {
			isExpectedFailure: true,
			oldClaim:          validClaimStorageClassInSpec,
			newClaim:          invalidClaimStorageClassInSpec,
			enableResize:      false,
		},
		"invalid-downgrade-storage-class-spec-to-annotation": {
			isExpectedFailure: true,
			oldClaim:          validClaimStorageClassInSpec,
			newClaim:          validClaimStorageClass,
			enableResize:      false,
		},
		"valid-update-rwop-used-and-rwop-feature-disabled": {
			isExpectedFailure: false,
			oldClaim:          validClaimRWOPAccessMode,
			newClaim:          validClaimRWOPAccessModeAddAnnotation,
			enableResize:      false,
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			// ensure we have a resource version specified for updates
			defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.ExpandPersistentVolumes, scenario.enableResize)()
			scenario.oldClaim.ResourceVersion = "1"
			scenario.newClaim.ResourceVersion = "1"
			opts := ValidationOptionsForPersistentVolumeClaim(scenario.newClaim, scenario.oldClaim)
			errs := ValidatePersistentVolumeClaimUpdate(scenario.newClaim, scenario.oldClaim, opts)
			if len(errs) == 0 && scenario.isExpectedFailure {
				t.Errorf("Unexpected success for scenario: %s", name)
			}
			if len(errs) > 0 && !scenario.isExpectedFailure {
				t.Errorf("Unexpected failure for scenario: %s - %+v", name, errs)
			}
		})
	}
}

func TestValidationOptionsForPersistentVolumeClaim(t *testing.T) {
	tests := map[string]struct {
		oldPvc                 *core.PersistentVolumeClaim
		enableReadWriteOncePod bool
		expectValidationOpts   PersistentVolumeClaimSpecValidationOptions
	}{
		"nil pv": {
			oldPvc:                 nil,
			enableReadWriteOncePod: true,
			expectValidationOpts: PersistentVolumeClaimSpecValidationOptions{
				AllowReadWriteOncePod: true,
			},
		},
		"rwop allowed because feature enabled": {
			oldPvc:                 pvcWithAccessModes([]core.PersistentVolumeAccessMode{core.ReadWriteOnce}),
			enableReadWriteOncePod: true,
			expectValidationOpts: PersistentVolumeClaimSpecValidationOptions{
				AllowReadWriteOncePod: true,
			},
		},
		"rwop not allowed because not used and feature disabled": {
			oldPvc:                 pvcWithAccessModes([]core.PersistentVolumeAccessMode{core.ReadWriteOnce}),
			enableReadWriteOncePod: false,
			expectValidationOpts: PersistentVolumeClaimSpecValidationOptions{
				AllowReadWriteOncePod: false,
			},
		},
		"rwop allowed because used and feature enabled": {
			oldPvc:                 pvcWithAccessModes([]core.PersistentVolumeAccessMode{core.ReadWriteOncePod}),
			enableReadWriteOncePod: true,
			expectValidationOpts: PersistentVolumeClaimSpecValidationOptions{
				AllowReadWriteOncePod: true,
			},
		},
		"rwop allowed because used and feature disabled": {
			oldPvc:                 pvcWithAccessModes([]core.PersistentVolumeAccessMode{core.ReadWriteOncePod}),
			enableReadWriteOncePod: false,
			expectValidationOpts: PersistentVolumeClaimSpecValidationOptions{
				AllowReadWriteOncePod: true,
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.ReadWriteOncePod, tc.enableReadWriteOncePod)()

			opts := ValidationOptionsForPersistentVolumeClaim(nil, tc.oldPvc)
			if opts != tc.expectValidationOpts {
				t.Errorf("Expected opts: %+v, received: %+v", opts, tc.expectValidationOpts)
			}
		})
	}
}

func TestValidationOptionsForPersistentVolumeClaimTemplate(t *testing.T) {
	tests := map[string]struct {
		oldPvcTemplate         *core.PersistentVolumeClaimTemplate
		enableReadWriteOncePod bool
		expectValidationOpts   PersistentVolumeClaimSpecValidationOptions
	}{
		"nil pv": {
			oldPvcTemplate:         nil,
			enableReadWriteOncePod: true,
			expectValidationOpts: PersistentVolumeClaimSpecValidationOptions{
				AllowReadWriteOncePod: true,
			},
		},
		"rwop allowed because feature enabled": {
			oldPvcTemplate:         pvcTemplateWithAccessModes([]core.PersistentVolumeAccessMode{core.ReadWriteOnce}),
			enableReadWriteOncePod: true,
			expectValidationOpts: PersistentVolumeClaimSpecValidationOptions{
				AllowReadWriteOncePod: true,
			},
		},
		"rwop not allowed because not used and feature disabled": {
			oldPvcTemplate:         pvcTemplateWithAccessModes([]core.PersistentVolumeAccessMode{core.ReadWriteOnce}),
			enableReadWriteOncePod: false,
			expectValidationOpts: PersistentVolumeClaimSpecValidationOptions{
				AllowReadWriteOncePod: false,
			},
		},
		"rwop allowed because used and feature enabled": {
			oldPvcTemplate:         pvcTemplateWithAccessModes([]core.PersistentVolumeAccessMode{core.ReadWriteOncePod}),
			enableReadWriteOncePod: true,
			expectValidationOpts: PersistentVolumeClaimSpecValidationOptions{
				AllowReadWriteOncePod: true,
			},
		},
		"rwop allowed because used and feature disabled": {
			oldPvcTemplate:         pvcTemplateWithAccessModes([]core.PersistentVolumeAccessMode{core.ReadWriteOncePod}),
			enableReadWriteOncePod: false,
			expectValidationOpts: PersistentVolumeClaimSpecValidationOptions{
				AllowReadWriteOncePod: true,
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.ReadWriteOncePod, tc.enableReadWriteOncePod)()

			opts := ValidationOptionsForPersistentVolumeClaimTemplate(nil, tc.oldPvcTemplate)
			if opts != tc.expectValidationOpts {
				t.Errorf("Expected opts: %+v, received: %+v", opts, tc.expectValidationOpts)
			}
		})
	}
}

func TestValidateKeyToPath(t *testing.T) {
	testCases := []struct {
		kp      core.KeyToPath
		ok      bool
		errtype field.ErrorType
	}{
		{
			kp: core.KeyToPath{Key: "k", Path: "p"},
			ok: true,
		},
		{
			kp: core.KeyToPath{Key: "k", Path: "p/p/p/p"},
			ok: true,
		},
		{
			kp: core.KeyToPath{Key: "k", Path: "p/..p/p../p..p"},
			ok: true,
		},
		{
			kp: core.KeyToPath{Key: "k", Path: "p", Mode: utilpointer.Int32Ptr(0644)},
			ok: true,
		},
		{
			kp:      core.KeyToPath{Key: "", Path: "p"},
			ok:      false,
			errtype: field.ErrorTypeRequired,
		},
		{
			kp:      core.KeyToPath{Key: "k", Path: ""},
			ok:      false,
			errtype: field.ErrorTypeRequired,
		},
		{
			kp:      core.KeyToPath{Key: "k", Path: "..p"},
			ok:      false,
			errtype: field.ErrorTypeInvalid,
		},
		{
			kp:      core.KeyToPath{Key: "k", Path: "../p"},
			ok:      false,
			errtype: field.ErrorTypeInvalid,
		},
		{
			kp:      core.KeyToPath{Key: "k", Path: "p/../p"},
			ok:      false,
			errtype: field.ErrorTypeInvalid,
		},
		{
			kp:      core.KeyToPath{Key: "k", Path: "p/.."},
			ok:      false,
			errtype: field.ErrorTypeInvalid,
		},
		{
			kp:      core.KeyToPath{Key: "k", Path: "p", Mode: utilpointer.Int32Ptr(01000)},
			ok:      false,
			errtype: field.ErrorTypeInvalid,
		},
		{
			kp:      core.KeyToPath{Key: "k", Path: "p", Mode: utilpointer.Int32Ptr(-1)},
			ok:      false,
			errtype: field.ErrorTypeInvalid,
		},
	}

	for i, tc := range testCases {
		errs := validateKeyToPath(&tc.kp, field.NewPath("field"))
		if tc.ok && len(errs) > 0 {
			t.Errorf("[%d] unexpected errors: %v", i, errs)
		} else if !tc.ok && len(errs) == 0 {
			t.Errorf("[%d] expected error type %v", i, tc.errtype)
		} else if len(errs) > 1 {
			t.Errorf("[%d] expected only one error, got %d", i, len(errs))
		} else if !tc.ok {
			if errs[0].Type != tc.errtype {
				t.Errorf("[%d] expected error type %v, got %v", i, tc.errtype, errs[0].Type)
			}
		}
	}
}

func TestValidateNFSVolumeSource(t *testing.T) {
	testCases := []struct {
		name      string
		nfs       *core.NFSVolumeSource
		errtype   field.ErrorType
		errfield  string
		errdetail string
	}{
		{
			name:     "missing server",
			nfs:      &core.NFSVolumeSource{Server: "", Path: "/tmp"},
			errtype:  field.ErrorTypeRequired,
			errfield: "server",
		},
		{
			name:     "missing path",
			nfs:      &core.NFSVolumeSource{Server: "my-server", Path: ""},
			errtype:  field.ErrorTypeRequired,
			errfield: "path",
		},
		{
			name:      "abs path",
			nfs:       &core.NFSVolumeSource{Server: "my-server", Path: "tmp"},
			errtype:   field.ErrorTypeInvalid,
			errfield:  "path",
			errdetail: "must be an absolute path",
		},
	}

	for i, tc := range testCases {
		errs := validateNFSVolumeSource(tc.nfs, field.NewPath("field"))

		if len(errs) > 0 && tc.errtype == "" {
			t.Errorf("[%d: %q] unexpected error(s): %v", i, tc.name, errs)
		} else if len(errs) == 0 && tc.errtype != "" {
			t.Errorf("[%d: %q] expected error type %v", i, tc.name, tc.errtype)
		} else if len(errs) >= 1 {
			if errs[0].Type != tc.errtype {
				t.Errorf("[%d: %q] expected error type %v, got %v", i, tc.name, tc.errtype, errs[0].Type)
			} else if !strings.HasSuffix(errs[0].Field, "."+tc.errfield) {
				t.Errorf("[%d: %q] expected error on field %q, got %q", i, tc.name, tc.errfield, errs[0].Field)
			} else if !strings.Contains(errs[0].Detail, tc.errdetail) {
				t.Errorf("[%d: %q] expected error detail %q, got %q", i, tc.name, tc.errdetail, errs[0].Detail)
			}
		}
	}
}

func TestValidateGlusterfs(t *testing.T) {
	testCases := []struct {
		name     string
		gfs      *core.GlusterfsVolumeSource
		errtype  field.ErrorType
		errfield string
	}{
		{
			name:     "missing endpointname",
			gfs:      &core.GlusterfsVolumeSource{EndpointsName: "", Path: "/tmp"},
			errtype:  field.ErrorTypeRequired,
			errfield: "endpoints",
		},
		{
			name:     "missing path",
			gfs:      &core.GlusterfsVolumeSource{EndpointsName: "my-endpoint", Path: ""},
			errtype:  field.ErrorTypeRequired,
			errfield: "path",
		},
		{
			name:     "missing endpointname and path",
			gfs:      &core.GlusterfsVolumeSource{EndpointsName: "", Path: ""},
			errtype:  field.ErrorTypeRequired,
			errfield: "endpoints",
		},
	}

	for i, tc := range testCases {
		errs := validateGlusterfsVolumeSource(tc.gfs, field.NewPath("field"))

		if len(errs) > 0 && tc.errtype == "" {
			t.Errorf("[%d: %q] unexpected error(s): %v", i, tc.name, errs)
		} else if len(errs) == 0 && tc.errtype != "" {
			t.Errorf("[%d: %q] expected error type %v", i, tc.name, tc.errtype)
		} else if len(errs) >= 1 {
			if errs[0].Type != tc.errtype {
				t.Errorf("[%d: %q] expected error type %v, got %v", i, tc.name, tc.errtype, errs[0].Type)
			} else if !strings.HasSuffix(errs[0].Field, "."+tc.errfield) {
				t.Errorf("[%d: %q] expected error on field %q, got %q", i, tc.name, tc.errfield, errs[0].Field)
			}
		}
	}
}

func TestValidateGlusterfsPersistentVolumeSource(t *testing.T) {
	var epNs *string
	namespace := ""
	epNs = &namespace

	testCases := []struct {
		name     string
		gfs      *core.GlusterfsPersistentVolumeSource
		errtype  field.ErrorType
		errfield string
	}{
		{
			name:     "missing endpointname",
			gfs:      &core.GlusterfsPersistentVolumeSource{EndpointsName: "", Path: "/tmp"},
			errtype:  field.ErrorTypeRequired,
			errfield: "endpoints",
		},
		{
			name:     "missing path",
			gfs:      &core.GlusterfsPersistentVolumeSource{EndpointsName: "my-endpoint", Path: ""},
			errtype:  field.ErrorTypeRequired,
			errfield: "path",
		},
		{
			name:     "non null endpointnamespace with empty string",
			gfs:      &core.GlusterfsPersistentVolumeSource{EndpointsName: "my-endpoint", Path: "/tmp", EndpointsNamespace: epNs},
			errtype:  field.ErrorTypeInvalid,
			errfield: "endpointsNamespace",
		},
		{
			name:     "missing endpointname and path",
			gfs:      &core.GlusterfsPersistentVolumeSource{EndpointsName: "", Path: ""},
			errtype:  field.ErrorTypeRequired,
			errfield: "endpoints",
		},
	}

	for i, tc := range testCases {
		errs := validateGlusterfsPersistentVolumeSource(tc.gfs, field.NewPath("field"))

		if len(errs) > 0 && tc.errtype == "" {
			t.Errorf("[%d: %q] unexpected error(s): %v", i, tc.name, errs)
		} else if len(errs) == 0 && tc.errtype != "" {
			t.Errorf("[%d: %q] expected error type %v", i, tc.name, tc.errtype)
		} else if len(errs) >= 1 {
			if errs[0].Type != tc.errtype {
				t.Errorf("[%d: %q] expected error type %v, got %v", i, tc.name, tc.errtype, errs[0].Type)
			} else if !strings.HasSuffix(errs[0].Field, "."+tc.errfield) {
				t.Errorf("[%d: %q] expected error on field %q, got %q", i, tc.name, tc.errfield, errs[0].Field)
			}
		}
	}
}

func TestValidateCSIVolumeSource(t *testing.T) {
	testCases := []struct {
		name     string
		csi      *core.CSIPersistentVolumeSource
		errtype  field.ErrorType
		errfield string
	}{
		{
			name: "all required fields ok",
			csi:  &core.CSIPersistentVolumeSource{Driver: "test-driver", VolumeHandle: "test-123", ReadOnly: true},
		},
		{
			name: "with default values ok",
			csi:  &core.CSIPersistentVolumeSource{Driver: "test-driver", VolumeHandle: "test-123"},
		},
		{
			name:     "missing driver name",
			csi:      &core.CSIPersistentVolumeSource{VolumeHandle: "test-123"},
			errtype:  field.ErrorTypeRequired,
			errfield: "driver",
		},
		{
			name:     "missing volume handle",
			csi:      &core.CSIPersistentVolumeSource{Driver: "my-driver"},
			errtype:  field.ErrorTypeRequired,
			errfield: "volumeHandle",
		},
		{
			name: "driver name: ok no punctuations",
			csi:  &core.CSIPersistentVolumeSource{Driver: "comgooglestoragecsigcepd", VolumeHandle: "test-123"},
		},
		{
			name: "driver name: ok dot only",
			csi:  &core.CSIPersistentVolumeSource{Driver: "io.kubernetes.storage.csi.flex", VolumeHandle: "test-123"},
		},
		{
			name: "driver name: ok dash only",
			csi:  &core.CSIPersistentVolumeSource{Driver: "io-kubernetes-storage-csi-flex", VolumeHandle: "test-123"},
		},
		{
			name:     "driver name: invalid underscore",
			csi:      &core.CSIPersistentVolumeSource{Driver: "io_kubernetes_storage_csi_flex", VolumeHandle: "test-123"},
			errtype:  field.ErrorTypeInvalid,
			errfield: "driver",
		},
		{
			name:     "driver name: invalid dot underscores",
			csi:      &core.CSIPersistentVolumeSource{Driver: "io.kubernetes.storage_csi.flex", VolumeHandle: "test-123"},
			errtype:  field.ErrorTypeInvalid,
			errfield: "driver",
		},
		{
			name: "driver name: ok beginnin with number",
			csi:  &core.CSIPersistentVolumeSource{Driver: "2io.kubernetes.storage-csi.flex", VolumeHandle: "test-123"},
		},
		{
			name: "driver name: ok ending with number",
			csi:  &core.CSIPersistentVolumeSource{Driver: "io.kubernetes.storage-csi.flex2", VolumeHandle: "test-123"},
		},
		{
			name:     "driver name: invalid dot dash underscores",
			csi:      &core.CSIPersistentVolumeSource{Driver: "io.kubernetes-storage.csi_flex", VolumeHandle: "test-123"},
			errtype:  field.ErrorTypeInvalid,
			errfield: "driver",
		},
		{
			name:     "driver name: invalid length 0",
			csi:      &core.CSIPersistentVolumeSource{Driver: "", VolumeHandle: "test-123"},
			errtype:  field.ErrorTypeRequired,
			errfield: "driver",
		},
		{
			name: "driver name: ok length 1",
			csi:  &core.CSIPersistentVolumeSource{Driver: "a", VolumeHandle: "test-123"},
		},
		{
			name:     "driver name: invalid length > 63",
			csi:      &core.CSIPersistentVolumeSource{Driver: "comgooglestoragecsigcepdcomgooglestoragecsigcepdcomgooglestoragecsigcepdcomgooglestoragecsigcepd", VolumeHandle: "test-123"},
			errtype:  field.ErrorTypeTooLong,
			errfield: "driver",
		},
		{
			name:     "driver name: invalid start char",
			csi:      &core.CSIPersistentVolumeSource{Driver: "_comgooglestoragecsigcepd", VolumeHandle: "test-123"},
			errtype:  field.ErrorTypeInvalid,
			errfield: "driver",
		},
		{
			name:     "driver name: invalid end char",
			csi:      &core.CSIPersistentVolumeSource{Driver: "comgooglestoragecsigcepd/", VolumeHandle: "test-123"},
			errtype:  field.ErrorTypeInvalid,
			errfield: "driver",
		},
		{
			name:     "driver name: invalid separators",
			csi:      &core.CSIPersistentVolumeSource{Driver: "com/google/storage/csi~gcepd", VolumeHandle: "test-123"},
			errtype:  field.ErrorTypeInvalid,
			errfield: "driver",
		},
		{
			name:     "controllerExpandSecretRef: invalid name missing",
			csi:      &core.CSIPersistentVolumeSource{Driver: "com.google.gcepd", VolumeHandle: "foobar", ControllerExpandSecretRef: &core.SecretReference{Namespace: "default"}},
			errtype:  field.ErrorTypeRequired,
			errfield: "controllerExpandSecretRef.name",
		},
		{
			name:     "controllerExpandSecretRef: invalid namespace missing",
			csi:      &core.CSIPersistentVolumeSource{Driver: "com.google.gcepd", VolumeHandle: "foobar", ControllerExpandSecretRef: &core.SecretReference{Name: "foobar"}},
			errtype:  field.ErrorTypeRequired,
			errfield: "controllerExpandSecretRef.namespace",
		},
		{
			name: "valid controllerExpandSecretRef",
			csi:  &core.CSIPersistentVolumeSource{Driver: "com.google.gcepd", VolumeHandle: "foobar", ControllerExpandSecretRef: &core.SecretReference{Name: "foobar", Namespace: "default"}},
		},
	}

	for i, tc := range testCases {
		errs := validateCSIPersistentVolumeSource(tc.csi, field.NewPath("field"))

		if len(errs) > 0 && tc.errtype == "" {
			t.Errorf("[%d: %q] unexpected error(s): %v", i, tc.name, errs)
		} else if len(errs) == 0 && tc.errtype != "" {
			t.Errorf("[%d: %q] expected error type %v", i, tc.name, tc.errtype)
		} else if len(errs) >= 1 {
			if errs[0].Type != tc.errtype {
				t.Errorf("[%d: %q] expected error type %v, got %v", i, tc.name, tc.errtype, errs[0].Type)
			} else if !strings.HasSuffix(errs[0].Field, "."+tc.errfield) {
				t.Errorf("[%d: %q] expected error on field %q, got %q", i, tc.name, tc.errfield, errs[0].Field)
			}
		}
	}
}

// This test is a little too top-to-bottom.  Ideally we would test each volume
// type on its own, but we want to also make sure that the logic works through
// the one-of wrapper, so we just do it all in one place.
func TestValidateVolumes(t *testing.T) {
	validInitiatorName := "iqn.2015-02.example.com:init"
	invalidInitiatorName := "2015-02.example.com:init"

	type verr struct {
		etype  field.ErrorType
		field  string
		detail string
	}

	testCases := []struct {
		name string
		vol  core.Volume
		errs []verr
		opts PodValidationOptions
	}{
		// EmptyDir and basic volume names
		{
			name: "valid alpha name",
			vol: core.Volume{
				Name: "empty",
				VolumeSource: core.VolumeSource{
					EmptyDir: &core.EmptyDirVolumeSource{},
				},
			},
		},
		{
			name: "valid num name",
			vol: core.Volume{
				Name: "123",
				VolumeSource: core.VolumeSource{
					EmptyDir: &core.EmptyDirVolumeSource{},
				},
			},
		},
		{
			name: "valid alphanum name",
			vol: core.Volume{
				Name: "empty-123",
				VolumeSource: core.VolumeSource{
					EmptyDir: &core.EmptyDirVolumeSource{},
				},
			},
		},
		{
			name: "valid numalpha name",
			vol: core.Volume{
				Name: "123-empty",
				VolumeSource: core.VolumeSource{
					EmptyDir: &core.EmptyDirVolumeSource{},
				},
			},
		},
		{
			name: "zero-length name",
			vol: core.Volume{
				Name:         "",
				VolumeSource: core.VolumeSource{EmptyDir: &core.EmptyDirVolumeSource{}},
			},
			errs: []verr{{
				etype: field.ErrorTypeRequired,
				field: "name",
			}},
		},
		{
			name: "name > 63 characters",
			vol: core.Volume{
				Name:         strings.Repeat("a", 64),
				VolumeSource: core.VolumeSource{EmptyDir: &core.EmptyDirVolumeSource{}},
			},
			errs: []verr{{
				etype:  field.ErrorTypeInvalid,
				field:  "name",
				detail: "must be no more than",
			}},
		},
		{
			name: "name not a DNS label",
			vol: core.Volume{
				Name:         "a.b.c",
				VolumeSource: core.VolumeSource{EmptyDir: &core.EmptyDirVolumeSource{}},
			},
			errs: []verr{{
				etype:  field.ErrorTypeInvalid,
				field:  "name",
				detail: dnsLabelErrMsg,
			}},
		},
		// More than one source field specified.
		{
			name: "more than one source",
			vol: core.Volume{
				Name: "dups",
				VolumeSource: core.VolumeSource{
					EmptyDir: &core.EmptyDirVolumeSource{},
					HostPath: &core.HostPathVolumeSource{
						Path: "/mnt/path",
						Type: newHostPathType(string(core.HostPathDirectory)),
					},
				},
			},
			errs: []verr{{
				etype:  field.ErrorTypeForbidden,
				field:  "hostPath",
				detail: "may not specify more than 1 volume",
			}},
		},
		// HostPath Default
		{
			name: "default HostPath",
			vol: core.Volume{
				Name: "hostpath",
				VolumeSource: core.VolumeSource{
					HostPath: &core.HostPathVolumeSource{
						Path: "/mnt/path",
						Type: newHostPathType(string(core.HostPathDirectory)),
					},
				},
			},
		},
		// HostPath Supported
		{
			name: "valid HostPath",
			vol: core.Volume{
				Name: "hostpath",
				VolumeSource: core.VolumeSource{
					HostPath: &core.HostPathVolumeSource{
						Path: "/mnt/path",
						Type: newHostPathType(string(core.HostPathSocket)),
					},
				},
			},
		},
		// HostPath Invalid
		{
			name: "invalid HostPath",
			vol: core.Volume{
				Name: "hostpath",
				VolumeSource: core.VolumeSource{
					HostPath: &core.HostPathVolumeSource{
						Path: "/mnt/path",
						Type: newHostPathType("invalid"),
					},
				},
			},
			errs: []verr{{
				etype: field.ErrorTypeNotSupported,
				field: "type",
			}},
		},
		{
			name: "invalid HostPath backsteps",
			vol: core.Volume{
				Name: "hostpath",
				VolumeSource: core.VolumeSource{
					HostPath: &core.HostPathVolumeSource{
						Path: "/mnt/path/..",
						Type: newHostPathType(string(core.HostPathDirectory)),
					},
				},
			},
			errs: []verr{{
				etype:  field.ErrorTypeInvalid,
				field:  "path",
				detail: "must not contain '..'",
			}},
		},
		// GcePersistentDisk
		{
			name: "valid GcePersistentDisk",
			vol: core.Volume{
				Name: "gce-pd",
				VolumeSource: core.VolumeSource{
					GCEPersistentDisk: &core.GCEPersistentDiskVolumeSource{
						PDName:    "my-PD",
						FSType:    "ext4",
						Partition: 1,
						ReadOnly:  false,
					},
				},
			},
		},
		// AWSElasticBlockStore
		{
			name: "valid AWSElasticBlockStore",
			vol: core.Volume{
				Name: "aws-ebs",
				VolumeSource: core.VolumeSource{
					AWSElasticBlockStore: &core.AWSElasticBlockStoreVolumeSource{
						VolumeID:  "my-PD",
						FSType:    "ext4",
						Partition: 1,
						ReadOnly:  false,
					},
				},
			},
		},
		// GitRepo
		{
			name: "valid GitRepo",
			vol: core.Volume{
				Name: "git-repo",
				VolumeSource: core.VolumeSource{
					GitRepo: &core.GitRepoVolumeSource{
						Repository: "my-repo",
						Revision:   "hashstring",
						Directory:  "target",
					},
				},
			},
		},
		{
			name: "valid GitRepo in .",
			vol: core.Volume{
				Name: "git-repo-dot",
				VolumeSource: core.VolumeSource{
					GitRepo: &core.GitRepoVolumeSource{
						Repository: "my-repo",
						Directory:  ".",
					},
				},
			},
		},
		{
			name: "valid GitRepo with .. in name",
			vol: core.Volume{
				Name: "git-repo-dot-dot-foo",
				VolumeSource: core.VolumeSource{
					GitRepo: &core.GitRepoVolumeSource{
						Repository: "my-repo",
						Directory:  "..foo",
					},
				},
			},
		},
		{
			name: "GitRepo starts with ../",
			vol: core.Volume{
				Name: "gitrepo",
				VolumeSource: core.VolumeSource{
					GitRepo: &core.GitRepoVolumeSource{
						Repository: "foo",
						Directory:  "../dots/bar",
					},
				},
			},
			errs: []verr{{
				etype:  field.ErrorTypeInvalid,
				field:  "gitRepo.directory",
				detail: `must not contain '..'`,
			}},
		},
		{
			name: "GitRepo contains ..",
			vol: core.Volume{
				Name: "gitrepo",
				VolumeSource: core.VolumeSource{
					GitRepo: &core.GitRepoVolumeSource{
						Repository: "foo",
						Directory:  "dots/../bar",
					},
				},
			},
			errs: []verr{{
				etype:  field.ErrorTypeInvalid,
				field:  "gitRepo.directory",
				detail: `must not contain '..'`,
			}},
		},
		{
			name: "GitRepo absolute target",
			vol: core.Volume{
				Name: "gitrepo",
				VolumeSource: core.VolumeSource{
					GitRepo: &core.GitRepoVolumeSource{
						Repository: "foo",
						Directory:  "/abstarget",
					},
				},
			},
			errs: []verr{{
				etype: field.ErrorTypeInvalid,
				field: "gitRepo.directory",
			}},
		},
		// ISCSI
		{
			name: "valid ISCSI",
			vol: core.Volume{
				Name: "iscsi",
				VolumeSource: core.VolumeSource{
					ISCSI: &core.ISCSIVolumeSource{
						TargetPortal: "127.0.0.1",
						IQN:          "iqn.2015-02.example.com:test",
						Lun:          1,
						FSType:       "ext4",
						ReadOnly:     false,
					},
				},
			},
		},
		{
			name: "valid IQN: eui format",
			vol: core.Volume{
				Name: "iscsi",
				VolumeSource: core.VolumeSource{
					ISCSI: &core.ISCSIVolumeSource{
						TargetPortal: "127.0.0.1",
						IQN:          "eui.0123456789ABCDEF",
						Lun:          1,
						FSType:       "ext4",
						ReadOnly:     false,
					},
				},
			},
		},
		{
			name: "valid IQN: naa format",
			vol: core.Volume{
				Name: "iscsi",
				VolumeSource: core.VolumeSource{
					ISCSI: &core.ISCSIVolumeSource{
						TargetPortal: "127.0.0.1",
						IQN:          "naa.62004567BA64678D0123456789ABCDEF",
						Lun:          1,
						FSType:       "ext4",
						ReadOnly:     false,
					},
				},
			},
		},
		{
			name: "empty portal",
			vol: core.Volume{
				Name: "iscsi",
				VolumeSource: core.VolumeSource{
					ISCSI: &core.ISCSIVolumeSource{
						TargetPortal: "",
						IQN:          "iqn.2015-02.example.com:test",
						Lun:          1,
						FSType:       "ext4",
						ReadOnly:     false,
					},
				},
			},
			errs: []verr{{
				etype: field.ErrorTypeRequired,
				field: "iscsi.targetPortal",
			}},
		},
		{
			name: "empty iqn",
			vol: core.Volume{
				Name: "iscsi",
				VolumeSource: core.VolumeSource{
					ISCSI: &core.ISCSIVolumeSource{
						TargetPortal: "127.0.0.1",
						IQN:          "",
						Lun:          1,
						FSType:       "ext4",
						ReadOnly:     false,
					},
				},
			},
			errs: []verr{{
				etype: field.ErrorTypeRequired,
				field: "iscsi.iqn",
			}},
		},
		{
			name: "invalid IQN: iqn format",
			vol: core.Volume{
				Name: "iscsi",
				VolumeSource: core.VolumeSource{
					ISCSI: &core.ISCSIVolumeSource{
						TargetPortal: "127.0.0.1",
						IQN:          "iqn.2015-02.example.com:test;ls;",
						Lun:          1,
						FSType:       "ext4",
						ReadOnly:     false,
					},
				},
			},
			errs: []verr{{
				etype: field.ErrorTypeInvalid,
				field: "iscsi.iqn",
			}},
		},
		{
			name: "invalid IQN: eui format",
			vol: core.Volume{
				Name: "iscsi",
				VolumeSource: core.VolumeSource{
					ISCSI: &core.ISCSIVolumeSource{
						TargetPortal: "127.0.0.1",
						IQN:          "eui.0123456789ABCDEFGHIJ",
						Lun:          1,
						FSType:       "ext4",
						ReadOnly:     false,
					},
				},
			},
			errs: []verr{{
				etype: field.ErrorTypeInvalid,
				field: "iscsi.iqn",
			}},
		},
		{
			name: "invalid IQN: naa format",
			vol: core.Volume{
				Name: "iscsi",
				VolumeSource: core.VolumeSource{
					ISCSI: &core.ISCSIVolumeSource{
						TargetPortal: "127.0.0.1",
						IQN:          "naa.62004567BA_4-78D.123456789ABCDEF",
						Lun:          1,
						FSType:       "ext4",
						ReadOnly:     false,
					},
				},
			},
			errs: []verr{{
				etype: field.ErrorTypeInvalid,
				field: "iscsi.iqn",
			}},
		},
		{
			name: "valid initiatorName",
			vol: core.Volume{
				Name: "iscsi",
				VolumeSource: core.VolumeSource{
					ISCSI: &core.ISCSIVolumeSource{
						TargetPortal:  "127.0.0.1",
						IQN:           "iqn.2015-02.example.com:test",
						Lun:           1,
						InitiatorName: &validInitiatorName,
						FSType:        "ext4",
						ReadOnly:      false,
					},
				},
			},
		},
		{
			name: "invalid initiatorName",
			vol: core.Volume{
				Name: "iscsi",
				VolumeSource: core.VolumeSource{
					ISCSI: &core.ISCSIVolumeSource{
						TargetPortal:  "127.0.0.1",
						IQN:           "iqn.2015-02.example.com:test",
						Lun:           1,
						InitiatorName: &invalidInitiatorName,
						FSType:        "ext4",
						ReadOnly:      false,
					},
				},
			},
			errs: []verr{{
				etype: field.ErrorTypeInvalid,
				field: "iscsi.initiatorname",
			}},
		},
		{
			name: "empty secret",
			vol: core.Volume{
				Name: "iscsi",
				VolumeSource: core.VolumeSource{
					ISCSI: &core.ISCSIVolumeSource{
						TargetPortal:      "127.0.0.1",
						IQN:               "iqn.2015-02.example.com:test",
						Lun:               1,
						FSType:            "ext4",
						ReadOnly:          false,
						DiscoveryCHAPAuth: true,
					},
				},
			},
			errs: []verr{{
				etype: field.ErrorTypeRequired,
				field: "iscsi.secretRef",
			}},
		},
		{
			name: "empty secret",
			vol: core.Volume{
				Name: "iscsi",
				VolumeSource: core.VolumeSource{
					ISCSI: &core.ISCSIVolumeSource{
						TargetPortal:    "127.0.0.1",
						IQN:             "iqn.2015-02.example.com:test",
						Lun:             1,
						FSType:          "ext4",
						ReadOnly:        false,
						SessionCHAPAuth: true,
					},
				},
			},
			errs: []verr{{
				etype: field.ErrorTypeRequired,
				field: "iscsi.secretRef",
			}},
		},
		// Secret
		{
			name: "valid Secret",
			vol: core.Volume{
				Name: "secret",
				VolumeSource: core.VolumeSource{
					Secret: &core.SecretVolumeSource{
						SecretName: "my-secret",
					},
				},
			},
		},
		{
			name: "valid Secret with defaultMode",
			vol: core.Volume{
				Name: "secret",
				VolumeSource: core.VolumeSource{
					Secret: &core.SecretVolumeSource{
						SecretName:  "my-secret",
						DefaultMode: utilpointer.Int32Ptr(0644),
					},
				},
			},
		},
		{
			name: "valid Secret with projection and mode",
			vol: core.Volume{
				Name: "secret",
				VolumeSource: core.VolumeSource{
					Secret: &core.SecretVolumeSource{
						SecretName: "my-secret",
						Items: []core.KeyToPath{{
							Key:  "key",
							Path: "filename",
							Mode: utilpointer.Int32Ptr(0644),
						}},
					},
				},
			},
		},
		{
			name: "valid Secret with subdir projection",
			vol: core.Volume{
				Name: "secret",
				VolumeSource: core.VolumeSource{
					Secret: &core.SecretVolumeSource{
						SecretName: "my-secret",
						Items: []core.KeyToPath{{
							Key:  "key",
							Path: "dir/filename",
						}},
					},
				},
			},
		},
		{
			name: "secret with missing path",
			vol: core.Volume{
				Name: "secret",
				VolumeSource: core.VolumeSource{
					Secret: &core.SecretVolumeSource{
						SecretName: "s",
						Items:      []core.KeyToPath{{Key: "key", Path: ""}},
					},
				},
			},
			errs: []verr{{
				etype: field.ErrorTypeRequired,
				field: "secret.items[0].path",
			}},
		},
		{
			name: "secret with leading ..",
			vol: core.Volume{
				Name: "secret",
				VolumeSource: core.VolumeSource{
					Secret: &core.SecretVolumeSource{
						SecretName: "s",
						Items:      []core.KeyToPath{{Key: "key", Path: "../foo"}},
					},
				},
			},
			errs: []verr{{
				etype: field.ErrorTypeInvalid,
				field: "secret.items[0].path",
			}},
		},
		{
			name: "secret with .. inside",
			vol: core.Volume{
				Name: "secret",
				VolumeSource: core.VolumeSource{
					Secret: &core.SecretVolumeSource{
						SecretName: "s",
						Items:      []core.KeyToPath{{Key: "key", Path: "foo/../bar"}},
					},
				},
			},
			errs: []verr{{
				etype: field.ErrorTypeInvalid,
				field: "secret.items[0].path",
			}},
		},
		{
			name: "secret with invalid positive defaultMode",
			vol: core.Volume{
				Name: "secret",
				VolumeSource: core.VolumeSource{
					Secret: &core.SecretVolumeSource{
						SecretName:  "s",
						DefaultMode: utilpointer.Int32Ptr(01000),
					},
				},
			},
			errs: []verr{{
				etype: field.ErrorTypeInvalid,
				field: "secret.defaultMode",
			}},
		},
		{
			name: "secret with invalid negative defaultMode",
			vol: core.Volume{
				Name: "secret",
				VolumeSource: core.VolumeSource{
					Secret: &core.SecretVolumeSource{
						SecretName:  "s",
						DefaultMode: utilpointer.Int32Ptr(-1),
					},
				},
			},
			errs: []verr{{
				etype: field.ErrorTypeInvalid,
				field: "secret.defaultMode",
			}},
		},
		// ConfigMap
		{
			name: "valid ConfigMap",
			vol: core.Volume{
				Name: "cfgmap",
				VolumeSource: core.VolumeSource{
					ConfigMap: &core.ConfigMapVolumeSource{
						LocalObjectReference: core.LocalObjectReference{
							Name: "my-cfgmap",
						},
					},
				},
			},
		},
		{
			name: "valid ConfigMap with defaultMode",
			vol: core.Volume{
				Name: "cfgmap",
				VolumeSource: core.VolumeSource{
					ConfigMap: &core.ConfigMapVolumeSource{
						LocalObjectReference: core.LocalObjectReference{
							Name: "my-cfgmap",
						},
						DefaultMode: utilpointer.Int32Ptr(0644),
					},
				},
			},
		},
		{
			name: "valid ConfigMap with projection and mode",
			vol: core.Volume{
				Name: "cfgmap",
				VolumeSource: core.VolumeSource{
					ConfigMap: &core.ConfigMapVolumeSource{
						LocalObjectReference: core.LocalObjectReference{
							Name: "my-cfgmap"},
						Items: []core.KeyToPath{{
							Key:  "key",
							Path: "filename",
							Mode: utilpointer.Int32Ptr(0644),
						}},
					},
				},
			},
		},
		{
			name: "valid ConfigMap with subdir projection",
			vol: core.Volume{
				Name: "cfgmap",
				VolumeSource: core.VolumeSource{
					ConfigMap: &core.ConfigMapVolumeSource{
						LocalObjectReference: core.LocalObjectReference{
							Name: "my-cfgmap"},
						Items: []core.KeyToPath{{
							Key:  "key",
							Path: "dir/filename",
						}},
					},
				},
			},
		},
		{
			name: "configmap with missing path",
			vol: core.Volume{
				Name: "cfgmap",
				VolumeSource: core.VolumeSource{
					ConfigMap: &core.ConfigMapVolumeSource{
						LocalObjectReference: core.LocalObjectReference{Name: "c"},
						Items:                []core.KeyToPath{{Key: "key", Path: ""}},
					},
				},
			},
			errs: []verr{{
				etype: field.ErrorTypeRequired,
				field: "configMap.items[0].path",
			}},
		},
		{
			name: "configmap with leading ..",
			vol: core.Volume{
				Name: "cfgmap",
				VolumeSource: core.VolumeSource{
					ConfigMap: &core.ConfigMapVolumeSource{
						LocalObjectReference: core.LocalObjectReference{Name: "c"},
						Items:                []core.KeyToPath{{Key: "key", Path: "../foo"}},
					},
				},
			},
			errs: []verr{{
				etype: field.ErrorTypeInvalid,
				field: "configMap.items[0].path",
			}},
		},
		{
			name: "configmap with .. inside",
			vol: core.Volume{
				Name: "cfgmap",
				VolumeSource: core.VolumeSource{
					ConfigMap: &core.ConfigMapVolumeSource{
						LocalObjectReference: core.LocalObjectReference{Name: "c"},
						Items:                []core.KeyToPath{{Key: "key", Path: "foo/../bar"}},
					},
				},
			},
			errs: []verr{{
				etype: field.ErrorTypeInvalid,
				field: "configMap.items[0].path",
			}},
		},
		{
			name: "configmap with invalid positive defaultMode",
			vol: core.Volume{
				Name: "cfgmap",
				VolumeSource: core.VolumeSource{
					ConfigMap: &core.ConfigMapVolumeSource{
						LocalObjectReference: core.LocalObjectReference{Name: "c"},
						DefaultMode:          utilpointer.Int32Ptr(01000),
					},
				},
			},
			errs: []verr{{
				etype: field.ErrorTypeInvalid,
				field: "configMap.defaultMode",
			}},
		},
		{
			name: "configmap with invalid negative defaultMode",
			vol: core.Volume{
				Name: "cfgmap",
				VolumeSource: core.VolumeSource{
					ConfigMap: &core.ConfigMapVolumeSource{
						LocalObjectReference: core.LocalObjectReference{Name: "c"},
						DefaultMode:          utilpointer.Int32Ptr(-1),
					},
				},
			},
			errs: []verr{{
				etype: field.ErrorTypeInvalid,
				field: "configMap.defaultMode",
			}},
		},
		// Glusterfs
		{
			name: "valid Glusterfs",
			vol: core.Volume{
				Name: "glusterfs",
				VolumeSource: core.VolumeSource{
					Glusterfs: &core.GlusterfsVolumeSource{
						EndpointsName: "host1",
						Path:          "path",
						ReadOnly:      false,
					},
				},
			},
		},
		{
			name: "empty hosts",
			vol: core.Volume{
				Name: "glusterfs",
				VolumeSource: core.VolumeSource{
					Glusterfs: &core.GlusterfsVolumeSource{
						EndpointsName: "",
						Path:          "path",
						ReadOnly:      false,
					},
				},
			},
			errs: []verr{{
				etype: field.ErrorTypeRequired,
				field: "glusterfs.endpoints",
			}},
		},
		{
			name: "empty path",
			vol: core.Volume{
				Name: "glusterfs",
				VolumeSource: core.VolumeSource{
					Glusterfs: &core.GlusterfsVolumeSource{
						EndpointsName: "host",
						Path:          "",
						ReadOnly:      false,
					},
				},
			},
			errs: []verr{{
				etype: field.ErrorTypeRequired,
				field: "glusterfs.path",
			}},
		},
		// Flocker
		{
			name: "valid Flocker -- datasetUUID",
			vol: core.Volume{
				Name: "flocker",
				VolumeSource: core.VolumeSource{
					Flocker: &core.FlockerVolumeSource{
						DatasetUUID: "d846b09d-223d-43df-ab5b-d6db2206a0e4",
					},
				},
			},
		},
		{
			name: "valid Flocker -- datasetName",
			vol: core.Volume{
				Name: "flocker",
				VolumeSource: core.VolumeSource{
					Flocker: &core.FlockerVolumeSource{
						DatasetName: "datasetName",
					},
				},
			},
		},
		{
			name: "both empty",
			vol: core.Volume{
				Name: "flocker",
				VolumeSource: core.VolumeSource{
					Flocker: &core.FlockerVolumeSource{
						DatasetName: "",
					},
				},
			},
			errs: []verr{{
				etype: field.ErrorTypeRequired,
				field: "flocker",
			}},
		},
		{
			name: "both specified",
			vol: core.Volume{
				Name: "flocker",
				VolumeSource: core.VolumeSource{
					Flocker: &core.FlockerVolumeSource{
						DatasetName: "datasetName",
						DatasetUUID: "d846b09d-223d-43df-ab5b-d6db2206a0e4",
					},
				},
			},
			errs: []verr{{
				etype: field.ErrorTypeInvalid,
				field: "flocker",
			}},
		},
		{
			name: "slash in flocker datasetName",
			vol: core.Volume{
				Name: "flocker",
				VolumeSource: core.VolumeSource{
					Flocker: &core.FlockerVolumeSource{
						DatasetName: "foo/bar",
					},
				},
			},
			errs: []verr{{
				etype:  field.ErrorTypeInvalid,
				field:  "flocker.datasetName",
				detail: "must not contain '/'",
			}},
		},
		// RBD
		{
			name: "valid RBD",
			vol: core.Volume{
				Name: "rbd",
				VolumeSource: core.VolumeSource{
					RBD: &core.RBDVolumeSource{
						CephMonitors: []string{"foo"},
						RBDImage:     "bar",
						FSType:       "ext4",
					},
				},
			},
		},
		{
			name: "empty rbd monitors",
			vol: core.Volume{
				Name: "rbd",
				VolumeSource: core.VolumeSource{
					RBD: &core.RBDVolumeSource{
						CephMonitors: []string{},
						RBDImage:     "bar",
						FSType:       "ext4",
					},
				},
			},
			errs: []verr{{
				etype: field.ErrorTypeRequired,
				field: "rbd.monitors",
			}},
		},
		{
			name: "empty image",
			vol: core.Volume{
				Name: "rbd",
				VolumeSource: core.VolumeSource{
					RBD: &core.RBDVolumeSource{
						CephMonitors: []string{"foo"},
						RBDImage:     "",
						FSType:       "ext4",
					},
				},
			},
			errs: []verr{{
				etype: field.ErrorTypeRequired,
				field: "rbd.image",
			}},
		},
		// Cinder
		{
			name: "valid Cinder",
			vol: core.Volume{
				Name: "cinder",
				VolumeSource: core.VolumeSource{
					Cinder: &core.CinderVolumeSource{
						VolumeID: "29ea5088-4f60-4757-962e-dba678767887",
						FSType:   "ext4",
						ReadOnly: false,
					},
				},
			},
		},
		// CephFS
		{
			name: "valid CephFS",
			vol: core.Volume{
				Name: "cephfs",
				VolumeSource: core.VolumeSource{
					CephFS: &core.CephFSVolumeSource{
						Monitors: []string{"foo"},
					},
				},
			},
		},
		{
			name: "empty cephfs monitors",
			vol: core.Volume{
				Name: "cephfs",
				VolumeSource: core.VolumeSource{
					CephFS: &core.CephFSVolumeSource{
						Monitors: []string{},
					},
				},
			},
			errs: []verr{{
				etype: field.ErrorTypeRequired,
				field: "cephfs.monitors",
			}},
		},
		// DownwardAPI
		{
			name: "valid DownwardAPI",
			vol: core.Volume{
				Name: "downwardapi",
				VolumeSource: core.VolumeSource{
					DownwardAPI: &core.DownwardAPIVolumeSource{
						Items: []core.DownwardAPIVolumeFile{
							{
								Path: "labels",
								FieldRef: &core.ObjectFieldSelector{
									APIVersion: "v1",
									FieldPath:  "metadata.labels",
								},
							},
							{
								Path: "labels with subscript",
								FieldRef: &core.ObjectFieldSelector{
									APIVersion: "v1",
									FieldPath:  "metadata.labels['key']",
								},
							},
							{
								Path: "labels with complex subscript",
								FieldRef: &core.ObjectFieldSelector{
									APIVersion: "v1",
									FieldPath:  "metadata.labels['test.example.com/key']",
								},
							},
							{
								Path: "annotations",
								FieldRef: &core.ObjectFieldSelector{
									APIVersion: "v1",
									FieldPath:  "metadata.annotations",
								},
							},
							{
								Path: "annotations with subscript",
								FieldRef: &core.ObjectFieldSelector{
									APIVersion: "v1",
									FieldPath:  "metadata.annotations['key']",
								},
							},
							{
								Path: "annotations with complex subscript",
								FieldRef: &core.ObjectFieldSelector{
									APIVersion: "v1",
									FieldPath:  "metadata.annotations['TEST.EXAMPLE.COM/key']",
								},
							},
							{
								Path: "namespace",
								FieldRef: &core.ObjectFieldSelector{
									APIVersion: "v1",
									FieldPath:  "metadata.namespace",
								},
							},
							{
								Path: "name",
								FieldRef: &core.ObjectFieldSelector{
									APIVersion: "v1",
									FieldPath:  "metadata.name",
								},
							},
							{
								Path: "path/with/subdirs",
								FieldRef: &core.ObjectFieldSelector{
									APIVersion: "v1",
									FieldPath:  "metadata.labels",
								},
							},
							{
								Path: "path/./withdot",
								FieldRef: &core.ObjectFieldSelector{
									APIVersion: "v1",
									FieldPath:  "metadata.labels",
								},
							},
							{
								Path: "path/with/embedded..dotdot",
								FieldRef: &core.ObjectFieldSelector{
									APIVersion: "v1",
									FieldPath:  "metadata.labels",
								},
							},
							{
								Path: "path/with/leading/..dotdot",
								FieldRef: &core.ObjectFieldSelector{
									APIVersion: "v1",
									FieldPath:  "metadata.labels",
								},
							},
							{
								Path: "cpu_limit",
								ResourceFieldRef: &core.ResourceFieldSelector{
									ContainerName: "test-container",
									Resource:      "limits.cpu",
								},
							},
							{
								Path: "cpu_request",
								ResourceFieldRef: &core.ResourceFieldSelector{
									ContainerName: "test-container",
									Resource:      "requests.cpu",
								},
							},
							{
								Path: "memory_limit",
								ResourceFieldRef: &core.ResourceFieldSelector{
									ContainerName: "test-container",
									Resource:      "limits.memory",
								},
							},
							{
								Path: "memory_request",
								ResourceFieldRef: &core.ResourceFieldSelector{
									ContainerName: "test-container",
									Resource:      "requests.memory",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "hugepages-downwardAPI-enabled",
			vol: core.Volume{
				Name: "downwardapi",
				VolumeSource: core.VolumeSource{
					DownwardAPI: &core.DownwardAPIVolumeSource{
						Items: []core.DownwardAPIVolumeFile{
							{
								Path: "hugepages_request",
								ResourceFieldRef: &core.ResourceFieldSelector{
									ContainerName: "test-container",
									Resource:      "requests.hugepages-2Mi",
								},
							},
							{
								Path: "hugepages_limit",
								ResourceFieldRef: &core.ResourceFieldSelector{
									ContainerName: "test-container",
									Resource:      "limits.hugepages-2Mi",
								},
							},
						},
					},
				},
			},
			opts: PodValidationOptions{AllowDownwardAPIHugePages: true},
		},
		{
			name: "hugepages-downwardAPI-requests-disabled",
			vol: core.Volume{
				Name: "downwardapi",
				VolumeSource: core.VolumeSource{
					DownwardAPI: &core.DownwardAPIVolumeSource{
						Items: []core.DownwardAPIVolumeFile{
							{
								Path: "hugepages_request",
								ResourceFieldRef: &core.ResourceFieldSelector{
									ContainerName: "test-container",
									Resource:      "requests.hugepages-2Mi",
								},
							},
						},
					},
				},
			},
			errs: []verr{{
				etype: field.ErrorTypeNotSupported,
				field: "downwardAPI.resourceFieldRef.resource",
			}},
		},
		{
			name: "hugepages-downwardAPI-limits-disabled",
			vol: core.Volume{
				Name: "downwardapi",
				VolumeSource: core.VolumeSource{
					DownwardAPI: &core.DownwardAPIVolumeSource{
						Items: []core.DownwardAPIVolumeFile{
							{
								Path: "hugepages_limit",
								ResourceFieldRef: &core.ResourceFieldSelector{
									ContainerName: "test-container",
									Resource:      "limits.hugepages-2Mi",
								},
							},
						},
					},
				},
			},
			errs: []verr{{
				etype: field.ErrorTypeNotSupported,
				field: "downwardAPI.resourceFieldRef.resource",
			}},
		},
		{
			name: "downapi valid defaultMode",
			vol: core.Volume{
				Name: "downapi",
				VolumeSource: core.VolumeSource{
					DownwardAPI: &core.DownwardAPIVolumeSource{
						DefaultMode: utilpointer.Int32Ptr(0644),
					},
				},
			},
		},
		{
			name: "downapi valid item mode",
			vol: core.Volume{
				Name: "downapi",
				VolumeSource: core.VolumeSource{
					DownwardAPI: &core.DownwardAPIVolumeSource{
						Items: []core.DownwardAPIVolumeFile{{
							Mode: utilpointer.Int32Ptr(0644),
							Path: "path",
							FieldRef: &core.ObjectFieldSelector{
								APIVersion: "v1",
								FieldPath:  "metadata.labels",
							},
						}},
					},
				},
			},
		},
		{
			name: "downapi invalid positive item mode",
			vol: core.Volume{
				Name: "downapi",
				VolumeSource: core.VolumeSource{
					DownwardAPI: &core.DownwardAPIVolumeSource{
						Items: []core.DownwardAPIVolumeFile{{
							Mode: utilpointer.Int32Ptr(01000),
							Path: "path",
							FieldRef: &core.ObjectFieldSelector{
								APIVersion: "v1",
								FieldPath:  "metadata.labels",
							},
						}},
					},
				},
			},
			errs: []verr{{
				etype: field.ErrorTypeInvalid,
				field: "downwardAPI.mode",
			}},
		},
		{
			name: "downapi invalid negative item mode",
			vol: core.Volume{
				Name: "downapi",
				VolumeSource: core.VolumeSource{
					DownwardAPI: &core.DownwardAPIVolumeSource{
						Items: []core.DownwardAPIVolumeFile{{
							Mode: utilpointer.Int32Ptr(-1),
							Path: "path",
							FieldRef: &core.ObjectFieldSelector{
								APIVersion: "v1",
								FieldPath:  "metadata.labels",
							},
						}},
					},
				},
			},
			errs: []verr{{
				etype: field.ErrorTypeInvalid,
				field: "downwardAPI.mode",
			}},
		},
		{
			name: "downapi empty metatada path",
			vol: core.Volume{
				Name: "downapi",
				VolumeSource: core.VolumeSource{
					DownwardAPI: &core.DownwardAPIVolumeSource{
						Items: []core.DownwardAPIVolumeFile{{
							Path: "",
							FieldRef: &core.ObjectFieldSelector{
								APIVersion: "v1",
								FieldPath:  "metadata.labels",
							},
						}},
					},
				},
			},
			errs: []verr{{
				etype: field.ErrorTypeRequired,
				field: "downwardAPI.path",
			}},
		},
		{
			name: "downapi absolute path",
			vol: core.Volume{
				Name: "downapi",
				VolumeSource: core.VolumeSource{
					DownwardAPI: &core.DownwardAPIVolumeSource{
						Items: []core.DownwardAPIVolumeFile{{
							Path: "/absolutepath",
							FieldRef: &core.ObjectFieldSelector{
								APIVersion: "v1",
								FieldPath:  "metadata.labels",
							},
						}},
					},
				},
			},
			errs: []verr{{
				etype: field.ErrorTypeInvalid,
				field: "downwardAPI.path",
			}},
		},
		{
			name: "downapi dot dot path",
			vol: core.Volume{
				Name: "downapi",
				VolumeSource: core.VolumeSource{
					DownwardAPI: &core.DownwardAPIVolumeSource{
						Items: []core.DownwardAPIVolumeFile{{
							Path: "../../passwd",
							FieldRef: &core.ObjectFieldSelector{
								APIVersion: "v1",
								FieldPath:  "metadata.labels",
							},
						}},
					},
				},
			},
			errs: []verr{{
				etype:  field.ErrorTypeInvalid,
				field:  "downwardAPI.path",
				detail: `must not contain '..'`,
			}},
		},
		{
			name: "downapi dot dot file name",
			vol: core.Volume{
				Name: "downapi",
				VolumeSource: core.VolumeSource{
					DownwardAPI: &core.DownwardAPIVolumeSource{
						Items: []core.DownwardAPIVolumeFile{{
							Path: "..badFileName",
							FieldRef: &core.ObjectFieldSelector{
								APIVersion: "v1",
								FieldPath:  "metadata.labels",
							},
						}},
					},
				},
			},
			errs: []verr{{
				etype:  field.ErrorTypeInvalid,
				field:  "downwardAPI.path",
				detail: `must not start with '..'`,
			}},
		},
		{
			name: "downapi dot dot first level dirent",
			vol: core.Volume{
				Name: "downapi",
				VolumeSource: core.VolumeSource{
					DownwardAPI: &core.DownwardAPIVolumeSource{
						Items: []core.DownwardAPIVolumeFile{{
							Path: "..badDirName/goodFileName",
							FieldRef: &core.ObjectFieldSelector{
								APIVersion: "v1",
								FieldPath:  "metadata.labels",
							},
						}},
					},
				},
			},
			errs: []verr{{
				etype:  field.ErrorTypeInvalid,
				field:  "downwardAPI.path",
				detail: `must not start with '..'`,
			}},
		},
		{
			name: "downapi fieldRef and ResourceFieldRef together",
			vol: core.Volume{
				Name: "downapi",
				VolumeSource: core.VolumeSource{
					DownwardAPI: &core.DownwardAPIVolumeSource{
						Items: []core.DownwardAPIVolumeFile{{
							Path: "test",
							FieldRef: &core.ObjectFieldSelector{
								APIVersion: "v1",
								FieldPath:  "metadata.labels",
							},
							ResourceFieldRef: &core.ResourceFieldSelector{
								ContainerName: "test-container",
								Resource:      "requests.memory",
							},
						}},
					},
				},
			},
			errs: []verr{{
				etype:  field.ErrorTypeInvalid,
				field:  "downwardAPI",
				detail: "fieldRef and resourceFieldRef can not be specified simultaneously",
			}},
		},
		{
			name: "downapi invalid positive defaultMode",
			vol: core.Volume{
				Name: "downapi",
				VolumeSource: core.VolumeSource{
					DownwardAPI: &core.DownwardAPIVolumeSource{
						DefaultMode: utilpointer.Int32Ptr(01000),
					},
				},
			},
			errs: []verr{{
				etype: field.ErrorTypeInvalid,
				field: "downwardAPI.defaultMode",
			}},
		},
		{
			name: "downapi invalid negative defaultMode",
			vol: core.Volume{
				Name: "downapi",
				VolumeSource: core.VolumeSource{
					DownwardAPI: &core.DownwardAPIVolumeSource{
						DefaultMode: utilpointer.Int32Ptr(-1),
					},
				},
			},
			errs: []verr{{
				etype: field.ErrorTypeInvalid,
				field: "downwardAPI.defaultMode",
			}},
		},
		// FC
		{
			name: "FC valid targetWWNs and lun",
			vol: core.Volume{
				Name: "fc",
				VolumeSource: core.VolumeSource{
					FC: &core.FCVolumeSource{
						TargetWWNs: []string{"some_wwn"},
						Lun:        utilpointer.Int32Ptr(1),
						FSType:     "ext4",
						ReadOnly:   false,
					},
				},
			},
		},
		{
			name: "FC valid wwids",
			vol: core.Volume{
				Name: "fc",
				VolumeSource: core.VolumeSource{
					FC: &core.FCVolumeSource{
						WWIDs:    []string{"some_wwid"},
						FSType:   "ext4",
						ReadOnly: false,
					},
				},
			},
		},
		{
			name: "FC empty targetWWNs and wwids",
			vol: core.Volume{
				Name: "fc",
				VolumeSource: core.VolumeSource{
					FC: &core.FCVolumeSource{
						TargetWWNs: []string{},
						Lun:        utilpointer.Int32Ptr(1),
						WWIDs:      []string{},
						FSType:     "ext4",
						ReadOnly:   false,
					},
				},
			},
			errs: []verr{{
				etype:  field.ErrorTypeRequired,
				field:  "fc.targetWWNs",
				detail: "must specify either targetWWNs or wwids",
			}},
		},
		{
			name: "FC invalid: both targetWWNs and wwids simultaneously",
			vol: core.Volume{
				Name: "fc",
				VolumeSource: core.VolumeSource{
					FC: &core.FCVolumeSource{
						TargetWWNs: []string{"some_wwn"},
						Lun:        utilpointer.Int32Ptr(1),
						WWIDs:      []string{"some_wwid"},
						FSType:     "ext4",
						ReadOnly:   false,
					},
				},
			},
			errs: []verr{{
				etype:  field.ErrorTypeInvalid,
				field:  "fc.targetWWNs",
				detail: "targetWWNs and wwids can not be specified simultaneously",
			}},
		},
		{
			name: "FC valid targetWWNs and empty lun",
			vol: core.Volume{
				Name: "fc",
				VolumeSource: core.VolumeSource{
					FC: &core.FCVolumeSource{
						TargetWWNs: []string{"wwn"},
						Lun:        nil,
						FSType:     "ext4",
						ReadOnly:   false,
					},
				},
			},
			errs: []verr{{
				etype:  field.ErrorTypeRequired,
				field:  "fc.lun",
				detail: "lun is required if targetWWNs is specified",
			}},
		},
		{
			name: "FC valid targetWWNs and invalid lun",
			vol: core.Volume{
				Name: "fc",
				VolumeSource: core.VolumeSource{
					FC: &core.FCVolumeSource{
						TargetWWNs: []string{"wwn"},
						Lun:        utilpointer.Int32Ptr(256),
						FSType:     "ext4",
						ReadOnly:   false,
					},
				},
			},
			errs: []verr{{
				etype:  field.ErrorTypeInvalid,
				field:  "fc.lun",
				detail: validation.InclusiveRangeError(0, 255),
			}},
		},
		// FlexVolume
		{
			name: "valid FlexVolume",
			vol: core.Volume{
				Name: "flex-volume",
				VolumeSource: core.VolumeSource{
					FlexVolume: &core.FlexVolumeSource{
						Driver: "kubernetes.io/blue",
						FSType: "ext4",
					},
				},
			},
		},
		// AzureFile
		{
			name: "valid AzureFile",
			vol: core.Volume{
				Name: "azure-file",
				VolumeSource: core.VolumeSource{
					AzureFile: &core.AzureFileVolumeSource{
						SecretName: "key",
						ShareName:  "share",
						ReadOnly:   false,
					},
				},
			},
		},
		{
			name: "AzureFile empty secret",
			vol: core.Volume{
				Name: "azure-file",
				VolumeSource: core.VolumeSource{
					AzureFile: &core.AzureFileVolumeSource{
						SecretName: "",
						ShareName:  "share",
						ReadOnly:   false,
					},
				},
			},
			errs: []verr{{
				etype: field.ErrorTypeRequired,
				field: "azureFile.secretName",
			}},
		},
		{
			name: "AzureFile empty share",
			vol: core.Volume{
				Name: "azure-file",
				VolumeSource: core.VolumeSource{
					AzureFile: &core.AzureFileVolumeSource{
						SecretName: "name",
						ShareName:  "",
						ReadOnly:   false,
					},
				},
			},
			errs: []verr{{
				etype: field.ErrorTypeRequired,
				field: "azureFile.shareName",
			}},
		},
		// Quobyte
		{
			name: "valid Quobyte",
			vol: core.Volume{
				Name: "quobyte",
				VolumeSource: core.VolumeSource{
					Quobyte: &core.QuobyteVolumeSource{
						Registry: "registry:7861",
						Volume:   "volume",
						ReadOnly: false,
						User:     "root",
						Group:    "root",
						Tenant:   "ThisIsSomeTenantUUID",
					},
				},
			},
		},
		{
			name: "empty registry quobyte",
			vol: core.Volume{
				Name: "quobyte",
				VolumeSource: core.VolumeSource{
					Quobyte: &core.QuobyteVolumeSource{
						Volume: "/test",
						Tenant: "ThisIsSomeTenantUUID",
					},
				},
			},
			errs: []verr{{
				etype: field.ErrorTypeRequired,
				field: "quobyte.registry",
			}},
		},
		{
			name: "wrong format registry quobyte",
			vol: core.Volume{
				Name: "quobyte",
				VolumeSource: core.VolumeSource{
					Quobyte: &core.QuobyteVolumeSource{
						Registry: "registry7861",
						Volume:   "/test",
						Tenant:   "ThisIsSomeTenantUUID",
					},
				},
			},
			errs: []verr{{
				etype: field.ErrorTypeInvalid,
				field: "quobyte.registry",
			}},
		},
		{
			name: "wrong format multiple registries quobyte",
			vol: core.Volume{
				Name: "quobyte",
				VolumeSource: core.VolumeSource{
					Quobyte: &core.QuobyteVolumeSource{
						Registry: "registry:7861,reg2",
						Volume:   "/test",
						Tenant:   "ThisIsSomeTenantUUID",
					},
				},
			},
			errs: []verr{{
				etype: field.ErrorTypeInvalid,
				field: "quobyte.registry",
			}},
		},
		{
			name: "empty volume quobyte",
			vol: core.Volume{
				Name: "quobyte",
				VolumeSource: core.VolumeSource{
					Quobyte: &core.QuobyteVolumeSource{
						Registry: "registry:7861",
						Tenant:   "ThisIsSomeTenantUUID",
					},
				},
			},
			errs: []verr{{
				etype: field.ErrorTypeRequired,
				field: "quobyte.volume",
			}},
		},
		{
			name: "empty tenant quobyte",
			vol: core.Volume{
				Name: "quobyte",
				VolumeSource: core.VolumeSource{
					Quobyte: &core.QuobyteVolumeSource{
						Registry: "registry:7861",
						Volume:   "/test",
						Tenant:   "",
					},
				},
			},
		},
		{
			name: "too long tenant quobyte",
			vol: core.Volume{
				Name: "quobyte",
				VolumeSource: core.VolumeSource{
					Quobyte: &core.QuobyteVolumeSource{
						Registry: "registry:7861",
						Volume:   "/test",
						Tenant:   "this is too long to be a valid uuid so this test has to fail on the maximum length validation of the tenant.",
					},
				},
			},
			errs: []verr{{
				etype: field.ErrorTypeRequired,
				field: "quobyte.tenant",
			}},
		},
		// AzureDisk
		{
			name: "valid AzureDisk",
			vol: core.Volume{
				Name: "azure-disk",
				VolumeSource: core.VolumeSource{
					AzureDisk: &core.AzureDiskVolumeSource{
						DiskName:    "foo",
						DataDiskURI: "https://blob/vhds/bar.vhd",
					},
				},
			},
		},
		{
			name: "AzureDisk empty disk name",
			vol: core.Volume{
				Name: "azure-disk",
				VolumeSource: core.VolumeSource{
					AzureDisk: &core.AzureDiskVolumeSource{
						DiskName:    "",
						DataDiskURI: "https://blob/vhds/bar.vhd",
					},
				},
			},
			errs: []verr{{
				etype: field.ErrorTypeRequired,
				field: "azureDisk.diskName",
			}},
		},
		{
			name: "AzureDisk empty disk uri",
			vol: core.Volume{
				Name: "azure-disk",
				VolumeSource: core.VolumeSource{
					AzureDisk: &core.AzureDiskVolumeSource{
						DiskName:    "foo",
						DataDiskURI: "",
					},
				},
			},
			errs: []verr{{
				etype: field.ErrorTypeRequired,
				field: "azureDisk.diskURI",
			}},
		},
		// ScaleIO
		{
			name: "valid scaleio volume",
			vol: core.Volume{
				Name: "scaleio-volume",
				VolumeSource: core.VolumeSource{
					ScaleIO: &core.ScaleIOVolumeSource{
						Gateway:    "http://abcd/efg",
						System:     "test-system",
						VolumeName: "test-vol-1",
					},
				},
			},
		},
		{
			name: "ScaleIO with empty name",
			vol: core.Volume{
				Name: "scaleio-volume",
				VolumeSource: core.VolumeSource{
					ScaleIO: &core.ScaleIOVolumeSource{
						Gateway:    "http://abcd/efg",
						System:     "test-system",
						VolumeName: "",
					},
				},
			},
			errs: []verr{{
				etype: field.ErrorTypeRequired,
				field: "scaleIO.volumeName",
			}},
		},
		{
			name: "ScaleIO with empty gateway",
			vol: core.Volume{
				Name: "scaleio-volume",
				VolumeSource: core.VolumeSource{
					ScaleIO: &core.ScaleIOVolumeSource{
						Gateway:    "",
						System:     "test-system",
						VolumeName: "test-vol-1",
					},
				},
			},
			errs: []verr{{
				etype: field.ErrorTypeRequired,
				field: "scaleIO.gateway",
			}},
		},
		{
			name: "ScaleIO with empty system",
			vol: core.Volume{
				Name: "scaleio-volume",
				VolumeSource: core.VolumeSource{
					ScaleIO: &core.ScaleIOVolumeSource{
						Gateway:    "http://agc/efg/gateway",
						System:     "",
						VolumeName: "test-vol-1",
					},
				},
			},
			errs: []verr{{
				etype: field.ErrorTypeRequired,
				field: "scaleIO.system",
			}},
		},
		// ProjectedVolumeSource
		{
			name: "ProjectedVolumeSource more than one projection in a source",
			vol: core.Volume{
				Name: "projected-volume",
				VolumeSource: core.VolumeSource{
					Projected: &core.ProjectedVolumeSource{
						Sources: []core.VolumeProjection{
							{
								Secret: &core.SecretProjection{
									LocalObjectReference: core.LocalObjectReference{
										Name: "foo",
									},
								},
							},
							{
								Secret: &core.SecretProjection{
									LocalObjectReference: core.LocalObjectReference{
										Name: "foo",
									},
								},
								DownwardAPI: &core.DownwardAPIProjection{},
							},
						},
					},
				},
			},
			errs: []verr{{
				etype: field.ErrorTypeForbidden,
				field: "projected.sources[1]",
			}},
		},
		{
			name: "ProjectedVolumeSource more than one projection in a source",
			vol: core.Volume{
				Name: "projected-volume",
				VolumeSource: core.VolumeSource{
					Projected: &core.ProjectedVolumeSource{
						Sources: []core.VolumeProjection{
							{
								Secret: &core.SecretProjection{},
							},
							{
								Secret:      &core.SecretProjection{},
								DownwardAPI: &core.DownwardAPIProjection{},
							},
						},
					},
				},
			},
			errs: []verr{
				{
					etype: field.ErrorTypeRequired,
					field: "projected.sources[0].secret.name",
				},
				{
					etype: field.ErrorTypeRequired,
					field: "projected.sources[1].secret.name",
				},
				{
					etype: field.ErrorTypeForbidden,
					field: "projected.sources[1]",
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			names, errs := ValidateVolumes([]core.Volume{tc.vol}, nil, field.NewPath("field"), tc.opts)
			if len(errs) != len(tc.errs) {
				t.Fatalf("unexpected error(s): got %d, want %d: %v", len(tc.errs), len(errs), errs)
			}
			if len(errs) == 0 && (len(names) > 1 || !IsMatchedVolume(tc.vol.Name, names)) {
				t.Errorf("wrong names result: %v", names)
			}
			for i, err := range errs {
				expErr := tc.errs[i]
				if err.Type != expErr.etype {
					t.Errorf("unexpected error type: got %v, want %v", expErr.etype, err.Type)
				}
				if !strings.HasSuffix(err.Field, "."+expErr.field) {
					t.Errorf("unexpected error field: got %v, want %v", expErr.field, err.Field)
				}
				if !strings.Contains(err.Detail, expErr.detail) {
					t.Errorf("unexpected error detail: got %v, want %v", expErr.detail, err.Detail)
				}
			}
		})
	}

	dupsCase := []core.Volume{
		{Name: "abc", VolumeSource: core.VolumeSource{EmptyDir: &core.EmptyDirVolumeSource{}}},
		{Name: "abc", VolumeSource: core.VolumeSource{EmptyDir: &core.EmptyDirVolumeSource{}}},
	}
	_, errs := ValidateVolumes(dupsCase, nil, field.NewPath("field"), PodValidationOptions{})
	if len(errs) == 0 {
		t.Errorf("expected error")
	} else if len(errs) != 1 {
		t.Errorf("expected 1 error, got %d: %v", len(errs), errs)
	} else if errs[0].Type != field.ErrorTypeDuplicate {
		t.Errorf("expected error type %v, got %v", field.ErrorTypeDuplicate, errs[0].Type)
	}

	// Validate HugePages medium type for EmptyDir
	hugePagesCase := core.VolumeSource{EmptyDir: &core.EmptyDirVolumeSource{Medium: core.StorageMediumHugePages}}

	// Enable HugePages
	if errs := validateVolumeSource(&hugePagesCase, field.NewPath("field").Index(0), "working", nil, PodValidationOptions{}); len(errs) != 0 {
		t.Errorf("Unexpected error when HugePages feature is enabled.")
	}

}

func TestHugePagesIsolation(t *testing.T) {
	testCases := map[string]struct {
		pod         *core.Pod
		expectError bool
	}{
		"Valid: request hugepages-2Mi": {
			pod: &core.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "123", Namespace: "ns"},
				Spec: core.PodSpec{
					Containers: []core.Container{
						{
							Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File",
							Resources: core.ResourceRequirements{
								Requests: core.ResourceList{
									core.ResourceName(core.ResourceCPU):    resource.MustParse("10"),
									core.ResourceName(core.ResourceMemory): resource.MustParse("10G"),
									core.ResourceName("hugepages-2Mi"):     resource.MustParse("1Gi"),
								},
								Limits: core.ResourceList{
									core.ResourceName(core.ResourceCPU):    resource.MustParse("10"),
									core.ResourceName(core.ResourceMemory): resource.MustParse("10G"),
									core.ResourceName("hugepages-2Mi"):     resource.MustParse("1Gi"),
								},
							},
						},
					},
					RestartPolicy: core.RestartPolicyAlways,
					DNSPolicy:     core.DNSClusterFirst,
				},
			},
		},
		"Valid: request more than one hugepages size": {
			pod: &core.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "hugepages-shared", Namespace: "ns"},
				Spec: core.PodSpec{
					Containers: []core.Container{
						{
							Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File",
							Resources: core.ResourceRequirements{
								Requests: core.ResourceList{
									core.ResourceName(core.ResourceCPU):    resource.MustParse("10"),
									core.ResourceName(core.ResourceMemory): resource.MustParse("10G"),
									core.ResourceName("hugepages-2Mi"):     resource.MustParse("1Gi"),
									core.ResourceName("hugepages-1Gi"):     resource.MustParse("2Gi"),
								},
								Limits: core.ResourceList{
									core.ResourceName(core.ResourceCPU):    resource.MustParse("10"),
									core.ResourceName(core.ResourceMemory): resource.MustParse("10G"),
									core.ResourceName("hugepages-2Mi"):     resource.MustParse("1Gi"),
									core.ResourceName("hugepages-1Gi"):     resource.MustParse("2Gi"),
								},
							},
						},
					},
					RestartPolicy: core.RestartPolicyAlways,
					DNSPolicy:     core.DNSClusterFirst,
				},
			},
			expectError: false,
		},
		"Valid: request hugepages-1Gi, limit hugepages-2Mi and hugepages-1Gi": {
			pod: &core.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "hugepages-multiple", Namespace: "ns"},
				Spec: core.PodSpec{
					Containers: []core.Container{
						{
							Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File",
							Resources: core.ResourceRequirements{
								Requests: core.ResourceList{
									core.ResourceName(core.ResourceCPU):    resource.MustParse("10"),
									core.ResourceName(core.ResourceMemory): resource.MustParse("10G"),
									core.ResourceName("hugepages-2Mi"):     resource.MustParse("1Gi"),
									core.ResourceName("hugepages-1Gi"):     resource.MustParse("2Gi"),
								},
								Limits: core.ResourceList{
									core.ResourceName(core.ResourceCPU):    resource.MustParse("10"),
									core.ResourceName(core.ResourceMemory): resource.MustParse("10G"),
									core.ResourceName("hugepages-2Mi"):     resource.MustParse("1Gi"),
									core.ResourceName("hugepages-1Gi"):     resource.MustParse("2Gi"),
								},
							},
						},
					},
					RestartPolicy: core.RestartPolicyAlways,
					DNSPolicy:     core.DNSClusterFirst,
				},
			},
		},
		"Invalid: not requesting cpu and memory": {
			pod: &core.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "hugepages-requireCpuOrMemory", Namespace: "ns"},
				Spec: core.PodSpec{
					Containers: []core.Container{
						{
							Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File",
							Resources: core.ResourceRequirements{
								Requests: core.ResourceList{
									core.ResourceName("hugepages-2Mi"): resource.MustParse("1Gi"),
								},
								Limits: core.ResourceList{
									core.ResourceName("hugepages-2Mi"): resource.MustParse("1Gi"),
								},
							},
						},
					},
					RestartPolicy: core.RestartPolicyAlways,
					DNSPolicy:     core.DNSClusterFirst,
				},
			},
			expectError: true,
		},
		"Invalid: request 1Gi hugepages-2Mi but limit 2Gi": {
			pod: &core.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "hugepages-shared", Namespace: "ns"},
				Spec: core.PodSpec{
					Containers: []core.Container{
						{
							Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File",
							Resources: core.ResourceRequirements{
								Requests: core.ResourceList{
									core.ResourceName(core.ResourceCPU):    resource.MustParse("10"),
									core.ResourceName(core.ResourceMemory): resource.MustParse("10G"),
									core.ResourceName("hugepages-2Mi"):     resource.MustParse("1Gi"),
								},
								Limits: core.ResourceList{
									core.ResourceName(core.ResourceCPU):    resource.MustParse("10"),
									core.ResourceName(core.ResourceMemory): resource.MustParse("10G"),
									core.ResourceName("hugepages-2Mi"):     resource.MustParse("2Gi"),
								},
							},
						},
					},
					RestartPolicy: core.RestartPolicyAlways,
					DNSPolicy:     core.DNSClusterFirst,
				},
			},
			expectError: true,
		},
	}
	for tcName, tc := range testCases {
		t.Run(tcName, func(t *testing.T) {
			errs := ValidatePodCreate(tc.pod, PodValidationOptions{})
			if tc.expectError && len(errs) == 0 {
				t.Errorf("Unexpected success")
			}
			if !tc.expectError && len(errs) != 0 {
				t.Errorf("Unexpected error(s): %v", errs)
			}
		})
	}
}

func TestPVCVolumeMode(t *testing.T) {
	block := core.PersistentVolumeBlock
	file := core.PersistentVolumeFilesystem
	fake := core.PersistentVolumeMode("fake")
	empty := core.PersistentVolumeMode("")

	// Success Cases
	successCasesPVC := map[string]*core.PersistentVolumeClaim{
		"valid block value":      createTestVolModePVC(&block),
		"valid filesystem value": createTestVolModePVC(&file),
		"valid nil value":        createTestVolModePVC(nil),
	}
	for k, v := range successCasesPVC {
		opts := ValidationOptionsForPersistentVolumeClaim(v, nil)
		if errs := ValidatePersistentVolumeClaim(v, opts); len(errs) != 0 {
			t.Errorf("expected success for %s", k)
		}
	}

	// Error Cases
	errorCasesPVC := map[string]*core.PersistentVolumeClaim{
		"invalid value": createTestVolModePVC(&fake),
		"empty value":   createTestVolModePVC(&empty),
	}
	for k, v := range errorCasesPVC {
		opts := ValidationOptionsForPersistentVolumeClaim(v, nil)
		if errs := ValidatePersistentVolumeClaim(v, opts); len(errs) == 0 {
			t.Errorf("expected failure for %s", k)
		}
	}
}

func TestPVVolumeMode(t *testing.T) {
	block := core.PersistentVolumeBlock
	file := core.PersistentVolumeFilesystem
	fake := core.PersistentVolumeMode("fake")
	empty := core.PersistentVolumeMode("")

	// Success Cases
	successCasesPV := map[string]*core.PersistentVolume{
		"valid block value":      createTestVolModePV(&block),
		"valid filesystem value": createTestVolModePV(&file),
		"valid nil value":        createTestVolModePV(nil),
	}
	for k, v := range successCasesPV {
		opts := ValidationOptionsForPersistentVolume(v, nil)
		if errs := ValidatePersistentVolume(v, opts); len(errs) != 0 {
			t.Errorf("expected success for %s", k)
		}
	}

	// Error Cases
	errorCasesPV := map[string]*core.PersistentVolume{
		"invalid value": createTestVolModePV(&fake),
		"empty value":   createTestVolModePV(&empty),
	}
	for k, v := range errorCasesPV {
		opts := ValidationOptionsForPersistentVolume(v, nil)
		if errs := ValidatePersistentVolume(v, opts); len(errs) == 0 {
			t.Errorf("expected failure for %s", k)
		}
	}
}

func createTestVolModePVC(vmode *core.PersistentVolumeMode) *core.PersistentVolumeClaim {
	validName := "valid-storage-class"

	pvc := core.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: core.PersistentVolumeClaimSpec{
			Resources: core.ResourceRequirements{
				Requests: core.ResourceList{
					core.ResourceName(core.ResourceStorage): resource.MustParse("10G"),
				},
			},
			AccessModes:      []core.PersistentVolumeAccessMode{core.ReadWriteOnce},
			StorageClassName: &validName,
			VolumeMode:       vmode,
		},
	}
	return &pvc
}

func createTestVolModePV(vmode *core.PersistentVolumeMode) *core.PersistentVolume {

	// PersistentVolume with VolumeMode set (valid and invalid)
	pv := core.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "",
		},
		Spec: core.PersistentVolumeSpec{
			Capacity: core.ResourceList{
				core.ResourceName(core.ResourceStorage): resource.MustParse("10G"),
			},
			AccessModes: []core.PersistentVolumeAccessMode{core.ReadWriteOnce},
			PersistentVolumeSource: core.PersistentVolumeSource{
				HostPath: &core.HostPathVolumeSource{
					Path: "/foo",
					Type: newHostPathType(string(core.HostPathDirectory)),
				},
			},
			StorageClassName: "test-storage-class",
			VolumeMode:       vmode,
		},
	}
	return &pv
}

func createTestPV() *core.PersistentVolume {

	// PersistentVolume with VolumeMode set (valid and invalid)
	pv := core.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "",
		},
		Spec: core.PersistentVolumeSpec{
			Capacity: core.ResourceList{
				core.ResourceName(core.ResourceStorage): resource.MustParse("10G"),
			},
			AccessModes: []core.PersistentVolumeAccessMode{core.ReadWriteOnce},
			PersistentVolumeSource: core.PersistentVolumeSource{
				HostPath: &core.HostPathVolumeSource{
					Path: "/foo",
					Type: newHostPathType(string(core.HostPathDirectory)),
				},
			},
			StorageClassName: "test-storage-class",
		},
	}
	return &pv
}

func TestAlphaLocalStorageCapacityIsolation(t *testing.T) {

	testCases := []core.VolumeSource{
		{EmptyDir: &core.EmptyDirVolumeSource{SizeLimit: resource.NewQuantity(int64(5), resource.BinarySI)}},
	}

	for _, tc := range testCases {
		if errs := validateVolumeSource(&tc, field.NewPath("spec"), "tmpvol", nil, PodValidationOptions{}); len(errs) != 0 {
			t.Errorf("expected success: %v", errs)
		}
	}

	containerLimitCase := core.ResourceRequirements{
		Limits: core.ResourceList{
			core.ResourceEphemeralStorage: *resource.NewMilliQuantity(
				int64(40000),
				resource.BinarySI),
		},
	}
	if errs := ValidateResourceRequirements(&containerLimitCase, field.NewPath("resources"), PodValidationOptions{}); len(errs) != 0 {
		t.Errorf("expected success: %v", errs)
	}
}

func TestValidateResourceQuotaWithAlphaLocalStorageCapacityIsolation(t *testing.T) {
	spec := core.ResourceQuotaSpec{
		Hard: core.ResourceList{
			core.ResourceCPU:                      resource.MustParse("100"),
			core.ResourceMemory:                   resource.MustParse("10000"),
			core.ResourceRequestsCPU:              resource.MustParse("100"),
			core.ResourceRequestsMemory:           resource.MustParse("10000"),
			core.ResourceLimitsCPU:                resource.MustParse("100"),
			core.ResourceLimitsMemory:             resource.MustParse("10000"),
			core.ResourcePods:                     resource.MustParse("10"),
			core.ResourceServices:                 resource.MustParse("0"),
			core.ResourceReplicationControllers:   resource.MustParse("10"),
			core.ResourceQuotas:                   resource.MustParse("10"),
			core.ResourceConfigMaps:               resource.MustParse("10"),
			core.ResourceSecrets:                  resource.MustParse("10"),
			core.ResourceEphemeralStorage:         resource.MustParse("10000"),
			core.ResourceRequestsEphemeralStorage: resource.MustParse("10000"),
			core.ResourceLimitsEphemeralStorage:   resource.MustParse("10000"),
		},
	}
	resourceQuota := &core.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "abc",
			Namespace: "foo",
		},
		Spec: spec,
	}

	if errs := ValidateResourceQuota(resourceQuota, ResourceQuotaValidationOptions{}); len(errs) != 0 {
		t.Errorf("expected success: %v", errs)
	}
}

func TestValidatePorts(t *testing.T) {
	successCase := []core.ContainerPort{
		{Name: "abc", ContainerPort: 80, HostPort: 80, Protocol: "TCP"},
		{Name: "easy", ContainerPort: 82, Protocol: "TCP"},
		{Name: "as", ContainerPort: 83, Protocol: "UDP"},
		{Name: "do-re-me", ContainerPort: 84, Protocol: "SCTP"},
		{ContainerPort: 85, Protocol: "TCP"},
	}
	if errs := validateContainerPorts(successCase, field.NewPath("field")); len(errs) != 0 {
		t.Errorf("expected success: %v", errs)
	}

	nonCanonicalCase := []core.ContainerPort{
		{ContainerPort: 80, Protocol: "TCP"},
	}
	if errs := validateContainerPorts(nonCanonicalCase, field.NewPath("field")); len(errs) != 0 {
		t.Errorf("expected success: %v", errs)
	}

	errorCases := map[string]struct {
		P []core.ContainerPort
		T field.ErrorType
		F string
		D string
	}{
		"name > 15 characters": {
			[]core.ContainerPort{{Name: strings.Repeat("a", 16), ContainerPort: 80, Protocol: "TCP"}},
			field.ErrorTypeInvalid,
			"name", "15",
		},
		"name contains invalid characters": {
			[]core.ContainerPort{{Name: "a.b.c", ContainerPort: 80, Protocol: "TCP"}},
			field.ErrorTypeInvalid,
			"name", "alpha-numeric",
		},
		"name is a number": {
			[]core.ContainerPort{{Name: "80", ContainerPort: 80, Protocol: "TCP"}},
			field.ErrorTypeInvalid,
			"name", "at least one letter",
		},
		"name not unique": {
			[]core.ContainerPort{
				{Name: "abc", ContainerPort: 80, Protocol: "TCP"},
				{Name: "abc", ContainerPort: 81, Protocol: "TCP"},
			},
			field.ErrorTypeDuplicate,
			"[1].name", "",
		},
		"zero container port": {
			[]core.ContainerPort{{ContainerPort: 0, Protocol: "TCP"}},
			field.ErrorTypeRequired,
			"containerPort", "",
		},
		"invalid container port": {
			[]core.ContainerPort{{ContainerPort: 65536, Protocol: "TCP"}},
			field.ErrorTypeInvalid,
			"containerPort", "between",
		},
		"invalid host port": {
			[]core.ContainerPort{{ContainerPort: 80, HostPort: 65536, Protocol: "TCP"}},
			field.ErrorTypeInvalid,
			"hostPort", "between",
		},
		"invalid protocol case": {
			[]core.ContainerPort{{ContainerPort: 80, Protocol: "tcp"}},
			field.ErrorTypeNotSupported,
			"protocol", `supported values: "SCTP", "TCP", "UDP"`,
		},
		"invalid protocol": {
			[]core.ContainerPort{{ContainerPort: 80, Protocol: "ICMP"}},
			field.ErrorTypeNotSupported,
			"protocol", `supported values: "SCTP", "TCP", "UDP"`,
		},
		"protocol required": {
			[]core.ContainerPort{{Name: "abc", ContainerPort: 80}},
			field.ErrorTypeRequired,
			"protocol", "",
		},
	}
	for k, v := range errorCases {
		errs := validateContainerPorts(v.P, field.NewPath("field"))
		if len(errs) == 0 {
			t.Errorf("expected failure for %s", k)
		}
		for i := range errs {
			if errs[i].Type != v.T {
				t.Errorf("%s: expected error to have type %q: %q", k, v.T, errs[i].Type)
			}
			if !strings.Contains(errs[i].Field, v.F) {
				t.Errorf("%s: expected error field %q: %q", k, v.F, errs[i].Field)
			}
			if !strings.Contains(errs[i].Detail, v.D) {
				t.Errorf("%s: expected error detail %q, got %q", k, v.D, errs[i].Detail)
			}
		}
	}
}

func TestLocalStorageEnvWithFeatureGate(t *testing.T) {
	testCases := []core.EnvVar{
		{
			Name: "ephemeral-storage-limits",
			ValueFrom: &core.EnvVarSource{
				ResourceFieldRef: &core.ResourceFieldSelector{
					ContainerName: "test-container",
					Resource:      "limits.ephemeral-storage",
				},
			},
		},
		{
			Name: "ephemeral-storage-requests",
			ValueFrom: &core.EnvVarSource{
				ResourceFieldRef: &core.ResourceFieldSelector{
					ContainerName: "test-container",
					Resource:      "requests.ephemeral-storage",
				},
			},
		},
	}
	for _, testCase := range testCases {
		if errs := validateEnvVarValueFrom(testCase, field.NewPath("field"), PodValidationOptions{}); len(errs) != 0 {
			t.Errorf("expected success, got: %v", errs)
		}
	}
}

func TestHugePagesEnv(t *testing.T) {
	testCases := []core.EnvVar{
		{
			Name: "hugepages-limits",
			ValueFrom: &core.EnvVarSource{
				ResourceFieldRef: &core.ResourceFieldSelector{
					ContainerName: "test-container",
					Resource:      "limits.hugepages-2Mi",
				},
			},
		},
		{
			Name: "hugepages-requests",
			ValueFrom: &core.EnvVarSource{
				ResourceFieldRef: &core.ResourceFieldSelector{
					ContainerName: "test-container",
					Resource:      "requests.hugepages-2Mi",
				},
			},
		},
	}
	// enable gate
	for _, testCase := range testCases {
		t.Run(testCase.Name, func(t *testing.T) {
			defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.DownwardAPIHugePages, true)()
			opts := PodValidationOptions{AllowDownwardAPIHugePages: true}
			if errs := validateEnvVarValueFrom(testCase, field.NewPath("field"), opts); len(errs) != 0 {
				t.Errorf("expected success, got: %v", errs)
			}
		})
	}
	// disable gate
	for _, testCase := range testCases {
		t.Run(testCase.Name, func(t *testing.T) {
			defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.DownwardAPIHugePages, false)()
			opts := PodValidationOptions{AllowDownwardAPIHugePages: false}
			if errs := validateEnvVarValueFrom(testCase, field.NewPath("field"), opts); len(errs) == 0 {
				t.Errorf("expected failure")
			}
		})
	}
}

func TestValidateEnv(t *testing.T) {
	successCase := []core.EnvVar{
		{Name: "abc", Value: "value"},
		{Name: "ABC", Value: "value"},
		{Name: "AbC_123", Value: "value"},
		{Name: "abc", Value: ""},
		{Name: "a.b.c", Value: "value"},
		{Name: "a-b-c", Value: "value"},
		{
			Name: "abc",
			ValueFrom: &core.EnvVarSource{
				FieldRef: &core.ObjectFieldSelector{
					APIVersion: "v1",
					FieldPath:  "metadata.annotations['key']",
				},
			},
		},
		{
			Name: "abc",
			ValueFrom: &core.EnvVarSource{
				FieldRef: &core.ObjectFieldSelector{
					APIVersion: "v1",
					FieldPath:  "metadata.labels['key']",
				},
			},
		},
		{
			Name: "abc",
			ValueFrom: &core.EnvVarSource{
				FieldRef: &core.ObjectFieldSelector{
					APIVersion: "v1",
					FieldPath:  "metadata.name",
				},
			},
		},
		{
			Name: "abc",
			ValueFrom: &core.EnvVarSource{
				FieldRef: &core.ObjectFieldSelector{
					APIVersion: "v1",
					FieldPath:  "metadata.namespace",
				},
			},
		},
		{
			Name: "abc",
			ValueFrom: &core.EnvVarSource{
				FieldRef: &core.ObjectFieldSelector{
					APIVersion: "v1",
					FieldPath:  "metadata.uid",
				},
			},
		},
		{
			Name: "abc",
			ValueFrom: &core.EnvVarSource{
				FieldRef: &core.ObjectFieldSelector{
					APIVersion: "v1",
					FieldPath:  "spec.nodeName",
				},
			},
		},
		{
			Name: "abc",
			ValueFrom: &core.EnvVarSource{
				FieldRef: &core.ObjectFieldSelector{
					APIVersion: "v1",
					FieldPath:  "spec.serviceAccountName",
				},
			},
		},
		{
			Name: "abc",
			ValueFrom: &core.EnvVarSource{
				FieldRef: &core.ObjectFieldSelector{
					APIVersion: "v1",
					FieldPath:  "status.hostIP",
				},
			},
		},
		{
			Name: "abc",
			ValueFrom: &core.EnvVarSource{
				FieldRef: &core.ObjectFieldSelector{
					APIVersion: "v1",
					FieldPath:  "status.podIP",
				},
			},
		},
		{
			Name: "abc",
			ValueFrom: &core.EnvVarSource{
				FieldRef: &core.ObjectFieldSelector{
					APIVersion: "v1",
					FieldPath:  "status.podIPs",
				},
			},
		},
		{
			Name: "secret_value",
			ValueFrom: &core.EnvVarSource{
				SecretKeyRef: &core.SecretKeySelector{
					LocalObjectReference: core.LocalObjectReference{
						Name: "some-secret",
					},
					Key: "secret-key",
				},
			},
		},
		{
			Name: "ENV_VAR_1",
			ValueFrom: &core.EnvVarSource{
				ConfigMapKeyRef: &core.ConfigMapKeySelector{
					LocalObjectReference: core.LocalObjectReference{
						Name: "some-config-map",
					},
					Key: "some-key",
				},
			},
		},
	}
	if errs := ValidateEnv(successCase, field.NewPath("field"), PodValidationOptions{}); len(errs) != 0 {
		t.Errorf("expected success, got: %v", errs)
	}

	errorCases := []struct {
		name          string
		envs          []core.EnvVar
		expectedError string
	}{
		{
			name:          "zero-length name",
			envs:          []core.EnvVar{{Name: ""}},
			expectedError: "[0].name: Required value",
		},
		{
			name:          "illegal character",
			envs:          []core.EnvVar{{Name: "a!b"}},
			expectedError: `[0].name: Invalid value: "a!b": ` + envVarNameErrMsg,
		},
		{
			name:          "dot only",
			envs:          []core.EnvVar{{Name: "."}},
			expectedError: `[0].name: Invalid value: ".": must not be`,
		},
		{
			name:          "double dots only",
			envs:          []core.EnvVar{{Name: ".."}},
			expectedError: `[0].name: Invalid value: "..": must not be`,
		},
		{
			name:          "leading double dots",
			envs:          []core.EnvVar{{Name: "..abc"}},
			expectedError: `[0].name: Invalid value: "..abc": must not start with`,
		},
		{
			name: "value and valueFrom specified",
			envs: []core.EnvVar{{
				Name:  "abc",
				Value: "foo",
				ValueFrom: &core.EnvVarSource{
					FieldRef: &core.ObjectFieldSelector{
						APIVersion: "v1",
						FieldPath:  "metadata.name",
					},
				},
			}},
			expectedError: "[0].valueFrom: Invalid value: \"\": may not be specified when `value` is not empty",
		},
		{
			name: "valueFrom without a source",
			envs: []core.EnvVar{{
				Name:      "abc",
				ValueFrom: &core.EnvVarSource{},
			}},
			expectedError: "[0].valueFrom: Invalid value: \"\": must specify one of: `fieldRef`, `resourceFieldRef`, `configMapKeyRef` or `secretKeyRef`",
		},
		{
			name: "valueFrom.fieldRef and valueFrom.secretKeyRef specified",
			envs: []core.EnvVar{{
				Name: "abc",
				ValueFrom: &core.EnvVarSource{
					FieldRef: &core.ObjectFieldSelector{
						APIVersion: "v1",
						FieldPath:  "metadata.name",
					},
					SecretKeyRef: &core.SecretKeySelector{
						LocalObjectReference: core.LocalObjectReference{
							Name: "a-secret",
						},
						Key: "a-key",
					},
				},
			}},
			expectedError: "[0].valueFrom: Invalid value: \"\": may not have more than one field specified at a time",
		},
		{
			name: "valueFrom.fieldRef and valueFrom.configMapKeyRef set",
			envs: []core.EnvVar{{
				Name: "some_var_name",
				ValueFrom: &core.EnvVarSource{
					FieldRef: &core.ObjectFieldSelector{
						APIVersion: "v1",
						FieldPath:  "metadata.name",
					},
					ConfigMapKeyRef: &core.ConfigMapKeySelector{
						LocalObjectReference: core.LocalObjectReference{
							Name: "some-config-map",
						},
						Key: "some-key",
					},
				},
			}},
			expectedError: `[0].valueFrom: Invalid value: "": may not have more than one field specified at a time`,
		},
		{
			name: "valueFrom.fieldRef and valueFrom.secretKeyRef specified",
			envs: []core.EnvVar{{
				Name: "abc",
				ValueFrom: &core.EnvVarSource{
					FieldRef: &core.ObjectFieldSelector{
						APIVersion: "v1",
						FieldPath:  "metadata.name",
					},
					SecretKeyRef: &core.SecretKeySelector{
						LocalObjectReference: core.LocalObjectReference{
							Name: "a-secret",
						},
						Key: "a-key",
					},
					ConfigMapKeyRef: &core.ConfigMapKeySelector{
						LocalObjectReference: core.LocalObjectReference{
							Name: "some-config-map",
						},
						Key: "some-key",
					},
				},
			}},
			expectedError: `[0].valueFrom: Invalid value: "": may not have more than one field specified at a time`,
		},
		{
			name: "valueFrom.secretKeyRef.name invalid",
			envs: []core.EnvVar{{
				Name: "abc",
				ValueFrom: &core.EnvVarSource{
					SecretKeyRef: &core.SecretKeySelector{
						LocalObjectReference: core.LocalObjectReference{
							Name: "$%^&*#",
						},
						Key: "a-key",
					},
				},
			}},
		},
		{
			name: "valueFrom.configMapKeyRef.name invalid",
			envs: []core.EnvVar{{
				Name: "abc",
				ValueFrom: &core.EnvVarSource{
					ConfigMapKeyRef: &core.ConfigMapKeySelector{
						LocalObjectReference: core.LocalObjectReference{
							Name: "$%^&*#",
						},
						Key: "some-key",
					},
				},
			}},
		},
		{
			name: "missing FieldPath on ObjectFieldSelector",
			envs: []core.EnvVar{{
				Name: "abc",
				ValueFrom: &core.EnvVarSource{
					FieldRef: &core.ObjectFieldSelector{
						APIVersion: "v1",
					},
				},
			}},
			expectedError: `[0].valueFrom.fieldRef.fieldPath: Required value`,
		},
		{
			name: "missing APIVersion on ObjectFieldSelector",
			envs: []core.EnvVar{{
				Name: "abc",
				ValueFrom: &core.EnvVarSource{
					FieldRef: &core.ObjectFieldSelector{
						FieldPath: "metadata.name",
					},
				},
			}},
			expectedError: `[0].valueFrom.fieldRef.apiVersion: Required value`,
		},
		{
			name: "invalid fieldPath",
			envs: []core.EnvVar{{
				Name: "abc",
				ValueFrom: &core.EnvVarSource{
					FieldRef: &core.ObjectFieldSelector{
						FieldPath:  "metadata.whoops",
						APIVersion: "v1",
					},
				},
			}},
			expectedError: `[0].valueFrom.fieldRef.fieldPath: Invalid value: "metadata.whoops": error converting fieldPath`,
		},
		{
			name: "metadata.name with subscript",
			envs: []core.EnvVar{{
				Name: "labels",
				ValueFrom: &core.EnvVarSource{
					FieldRef: &core.ObjectFieldSelector{
						FieldPath:  "metadata.name['key']",
						APIVersion: "v1",
					},
				},
			}},
			expectedError: `[0].valueFrom.fieldRef.fieldPath: Invalid value: "metadata.name['key']": error converting fieldPath: field label does not support subscript`,
		},
		{
			name: "metadata.labels without subscript",
			envs: []core.EnvVar{{
				Name: "labels",
				ValueFrom: &core.EnvVarSource{
					FieldRef: &core.ObjectFieldSelector{
						FieldPath:  "metadata.labels",
						APIVersion: "v1",
					},
				},
			}},
			expectedError: `[0].valueFrom.fieldRef.fieldPath: Unsupported value: "metadata.labels": supported values: "metadata.name", "metadata.namespace", "metadata.uid", "spec.nodeName", "spec.serviceAccountName", "status.hostIP", "status.podIP", "status.podIPs"`,
		},
		{
			name: "metadata.annotations without subscript",
			envs: []core.EnvVar{{
				Name: "abc",
				ValueFrom: &core.EnvVarSource{
					FieldRef: &core.ObjectFieldSelector{
						FieldPath:  "metadata.annotations",
						APIVersion: "v1",
					},
				},
			}},
			expectedError: `[0].valueFrom.fieldRef.fieldPath: Unsupported value: "metadata.annotations": supported values: "metadata.name", "metadata.namespace", "metadata.uid", "spec.nodeName", "spec.serviceAccountName", "status.hostIP", "status.podIP", "status.podIPs"`,
		},
		{
			name: "metadata.annotations with invalid key",
			envs: []core.EnvVar{{
				Name: "abc",
				ValueFrom: &core.EnvVarSource{
					FieldRef: &core.ObjectFieldSelector{
						FieldPath:  "metadata.annotations['invalid~key']",
						APIVersion: "v1",
					},
				},
			}},
			expectedError: `field[0].valueFrom.fieldRef: Invalid value: "invalid~key"`,
		},
		{
			name: "metadata.labels with invalid key",
			envs: []core.EnvVar{{
				Name: "abc",
				ValueFrom: &core.EnvVarSource{
					FieldRef: &core.ObjectFieldSelector{
						FieldPath:  "metadata.labels['Www.k8s.io/test']",
						APIVersion: "v1",
					},
				},
			}},
			expectedError: `field[0].valueFrom.fieldRef: Invalid value: "Www.k8s.io/test"`,
		},
		{
			name: "unsupported fieldPath",
			envs: []core.EnvVar{{
				Name: "abc",
				ValueFrom: &core.EnvVarSource{
					FieldRef: &core.ObjectFieldSelector{
						FieldPath:  "status.phase",
						APIVersion: "v1",
					},
				},
			}},
			expectedError: `valueFrom.fieldRef.fieldPath: Unsupported value: "status.phase": supported values: "metadata.name", "metadata.namespace", "metadata.uid", "spec.nodeName", "spec.serviceAccountName", "status.hostIP", "status.podIP", "status.podIPs"`,
		},
	}
	for _, tc := range errorCases {
		if errs := ValidateEnv(tc.envs, field.NewPath("field"), PodValidationOptions{}); len(errs) == 0 {
			t.Errorf("expected failure for %s", tc.name)
		} else {
			for i := range errs {
				str := errs[i].Error()
				if str != "" && !strings.Contains(str, tc.expectedError) {
					t.Errorf("%s: expected error detail either empty or %q, got %q", tc.name, tc.expectedError, str)
				}
			}
		}
	}
}

func TestValidateEnvFrom(t *testing.T) {
	successCase := []core.EnvFromSource{
		{
			ConfigMapRef: &core.ConfigMapEnvSource{
				LocalObjectReference: core.LocalObjectReference{Name: "abc"},
			},
		},
		{
			Prefix: "pre_",
			ConfigMapRef: &core.ConfigMapEnvSource{
				LocalObjectReference: core.LocalObjectReference{Name: "abc"},
			},
		},
		{
			Prefix: "a.b",
			ConfigMapRef: &core.ConfigMapEnvSource{
				LocalObjectReference: core.LocalObjectReference{Name: "abc"},
			},
		},
		{
			SecretRef: &core.SecretEnvSource{
				LocalObjectReference: core.LocalObjectReference{Name: "abc"},
			},
		},
		{
			Prefix: "pre_",
			SecretRef: &core.SecretEnvSource{
				LocalObjectReference: core.LocalObjectReference{Name: "abc"},
			},
		},
		{
			Prefix: "a.b",
			SecretRef: &core.SecretEnvSource{
				LocalObjectReference: core.LocalObjectReference{Name: "abc"},
			},
		},
	}
	if errs := ValidateEnvFrom(successCase, field.NewPath("field")); len(errs) != 0 {
		t.Errorf("expected success: %v", errs)
	}

	errorCases := []struct {
		name          string
		envs          []core.EnvFromSource
		expectedError string
	}{
		{
			name: "zero-length name",
			envs: []core.EnvFromSource{
				{
					ConfigMapRef: &core.ConfigMapEnvSource{
						LocalObjectReference: core.LocalObjectReference{Name: ""}},
				},
			},
			expectedError: "field[0].configMapRef.name: Required value",
		},
		{
			name: "invalid name",
			envs: []core.EnvFromSource{
				{
					ConfigMapRef: &core.ConfigMapEnvSource{
						LocalObjectReference: core.LocalObjectReference{Name: "$"}},
				},
			},
			expectedError: "field[0].configMapRef.name: Invalid value",
		},
		{
			name: "invalid prefix",
			envs: []core.EnvFromSource{
				{
					Prefix: "a!b",
					ConfigMapRef: &core.ConfigMapEnvSource{
						LocalObjectReference: core.LocalObjectReference{Name: "abc"}},
				},
			},
			expectedError: `field[0].prefix: Invalid value: "a!b": ` + envVarNameErrMsg,
		},
		{
			name: "zero-length name",
			envs: []core.EnvFromSource{
				{
					SecretRef: &core.SecretEnvSource{
						LocalObjectReference: core.LocalObjectReference{Name: ""}},
				},
			},
			expectedError: "field[0].secretRef.name: Required value",
		},
		{
			name: "invalid name",
			envs: []core.EnvFromSource{
				{
					SecretRef: &core.SecretEnvSource{
						LocalObjectReference: core.LocalObjectReference{Name: "&"}},
				},
			},
			expectedError: "field[0].secretRef.name: Invalid value",
		},
		{
			name: "invalid prefix",
			envs: []core.EnvFromSource{
				{
					Prefix: "a!b",
					SecretRef: &core.SecretEnvSource{
						LocalObjectReference: core.LocalObjectReference{Name: "abc"}},
				},
			},
			expectedError: `field[0].prefix: Invalid value: "a!b": ` + envVarNameErrMsg,
		},
		{
			name: "no refs",
			envs: []core.EnvFromSource{
				{},
			},
			expectedError: "field: Invalid value: \"\": must specify one of: `configMapRef` or `secretRef`",
		},
		{
			name: "multiple refs",
			envs: []core.EnvFromSource{
				{
					SecretRef: &core.SecretEnvSource{
						LocalObjectReference: core.LocalObjectReference{Name: "abc"}},
					ConfigMapRef: &core.ConfigMapEnvSource{
						LocalObjectReference: core.LocalObjectReference{Name: "abc"}},
				},
			},
			expectedError: "field: Invalid value: \"\": may not have more than one field specified at a time",
		},
		{
			name: "invalid secret ref name",
			envs: []core.EnvFromSource{
				{
					SecretRef: &core.SecretEnvSource{
						LocalObjectReference: core.LocalObjectReference{Name: "$%^&*#"}},
				},
			},
			expectedError: "field[0].secretRef.name: Invalid value: \"$%^&*#\": " + dnsSubdomainLabelErrMsg,
		},
		{
			name: "invalid config ref name",
			envs: []core.EnvFromSource{
				{
					ConfigMapRef: &core.ConfigMapEnvSource{
						LocalObjectReference: core.LocalObjectReference{Name: "$%^&*#"}},
				},
			},
			expectedError: "field[0].configMapRef.name: Invalid value: \"$%^&*#\": " + dnsSubdomainLabelErrMsg,
		},
	}
	for _, tc := range errorCases {
		if errs := ValidateEnvFrom(tc.envs, field.NewPath("field")); len(errs) == 0 {
			t.Errorf("expected failure for %s", tc.name)
		} else {
			for i := range errs {
				str := errs[i].Error()
				if str != "" && !strings.Contains(str, tc.expectedError) {
					t.Errorf("%s: expected error detail either empty or %q, got %q", tc.name, tc.expectedError, str)
				}
			}
		}
	}
}

func TestValidateVolumeMounts(t *testing.T) {
	volumes := []core.Volume{
		{Name: "abc", VolumeSource: core.VolumeSource{PersistentVolumeClaim: &core.PersistentVolumeClaimVolumeSource{ClaimName: "testclaim1"}}},
		{Name: "abc-123", VolumeSource: core.VolumeSource{PersistentVolumeClaim: &core.PersistentVolumeClaimVolumeSource{ClaimName: "testclaim2"}}},
		{Name: "123", VolumeSource: core.VolumeSource{HostPath: &core.HostPathVolumeSource{Path: "/foo/baz", Type: newHostPathType(string(core.HostPathUnset))}}},
	}
	vols, v1err := ValidateVolumes(volumes, nil, field.NewPath("field"), PodValidationOptions{})
	if len(v1err) > 0 {
		t.Errorf("Invalid test volume - expected success %v", v1err)
		return
	}
	container := core.Container{
		SecurityContext: nil,
	}
	propagation := core.MountPropagationBidirectional

	successCase := []core.VolumeMount{
		{Name: "abc", MountPath: "/foo"},
		{Name: "123", MountPath: "/bar"},
		{Name: "abc-123", MountPath: "/baz"},
		{Name: "abc-123", MountPath: "/baa", SubPath: ""},
		{Name: "abc-123", MountPath: "/bab", SubPath: "baz"},
		{Name: "abc-123", MountPath: "d:", SubPath: ""},
		{Name: "abc-123", MountPath: "F:", SubPath: ""},
		{Name: "abc-123", MountPath: "G:\\mount", SubPath: ""},
		{Name: "abc-123", MountPath: "/bac", SubPath: ".baz"},
		{Name: "abc-123", MountPath: "/bad", SubPath: "..baz"},
	}
	goodVolumeDevices := []core.VolumeDevice{
		{Name: "xyz", DevicePath: "/foofoo"},
		{Name: "uvw", DevicePath: "/foofoo/share/test"},
	}
	if errs := ValidateVolumeMounts(successCase, GetVolumeDeviceMap(goodVolumeDevices), vols, &container, field.NewPath("field")); len(errs) != 0 {
		t.Errorf("expected success: %v", errs)
	}

	errorCases := map[string][]core.VolumeMount{
		"empty name":                             {{Name: "", MountPath: "/foo"}},
		"name not found":                         {{Name: "", MountPath: "/foo"}},
		"empty mountpath":                        {{Name: "abc", MountPath: ""}},
		"mountpath collision":                    {{Name: "foo", MountPath: "/path/a"}, {Name: "bar", MountPath: "/path/a"}},
		"absolute subpath":                       {{Name: "abc", MountPath: "/bar", SubPath: "/baz"}},
		"subpath in ..":                          {{Name: "abc", MountPath: "/bar", SubPath: "../baz"}},
		"subpath contains ..":                    {{Name: "abc", MountPath: "/bar", SubPath: "baz/../bat"}},
		"subpath ends in ..":                     {{Name: "abc", MountPath: "/bar", SubPath: "./.."}},
		"disabled MountPropagation feature gate": {{Name: "abc", MountPath: "/bar", MountPropagation: &propagation}},
		"name exists in volumeDevice":            {{Name: "xyz", MountPath: "/bar"}},
		"mountpath exists in volumeDevice":       {{Name: "uvw", MountPath: "/mnt/exists"}},
		"both exist in volumeDevice":             {{Name: "xyz", MountPath: "/mnt/exists"}},
	}
	badVolumeDevice := []core.VolumeDevice{
		{Name: "xyz", DevicePath: "/mnt/exists"},
	}

	for k, v := range errorCases {
		if errs := ValidateVolumeMounts(v, GetVolumeDeviceMap(badVolumeDevice), vols, &container, field.NewPath("field")); len(errs) == 0 {
			t.Errorf("expected failure for %s", k)
		}
	}
}

func TestValidateDisabledSubpath(t *testing.T) {
	defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.VolumeSubpath, false)()

	volumes := []core.Volume{
		{Name: "abc", VolumeSource: core.VolumeSource{PersistentVolumeClaim: &core.PersistentVolumeClaimVolumeSource{ClaimName: "testclaim1"}}},
		{Name: "abc-123", VolumeSource: core.VolumeSource{PersistentVolumeClaim: &core.PersistentVolumeClaimVolumeSource{ClaimName: "testclaim2"}}},
		{Name: "123", VolumeSource: core.VolumeSource{HostPath: &core.HostPathVolumeSource{Path: "/foo/baz", Type: newHostPathType(string(core.HostPathUnset))}}},
	}
	vols, v1err := ValidateVolumes(volumes, nil, field.NewPath("field"), PodValidationOptions{})
	if len(v1err) > 0 {
		t.Errorf("Invalid test volume - expected success %v", v1err)
		return
	}

	container := core.Container{
		SecurityContext: nil,
	}

	goodVolumeDevices := []core.VolumeDevice{
		{Name: "xyz", DevicePath: "/foofoo"},
		{Name: "uvw", DevicePath: "/foofoo/share/test"},
	}

	cases := map[string]struct {
		mounts      []core.VolumeMount
		expectError bool
	}{
		"subpath not specified": {
			[]core.VolumeMount{
				{
					Name:      "abc-123",
					MountPath: "/bab",
				},
			},
			false,
		},
		"subpath specified": {
			[]core.VolumeMount{
				{
					Name:      "abc-123",
					MountPath: "/bab",
					SubPath:   "baz",
				},
			},
			false, // validation should not fail, dropping the field is handled in PrepareForCreate/PrepareForUpdate
		},
	}

	for name, test := range cases {
		errs := ValidateVolumeMounts(test.mounts, GetVolumeDeviceMap(goodVolumeDevices), vols, &container, field.NewPath("field"))

		if len(errs) != 0 && !test.expectError {
			t.Errorf("test %v failed: %+v", name, errs)
		}

		if len(errs) == 0 && test.expectError {
			t.Errorf("test %v failed, expected error", name)
		}
	}
}

func TestValidateSubpathMutuallyExclusive(t *testing.T) {
	// Enable feature VolumeSubpath
	defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.VolumeSubpath, true)()

	volumes := []core.Volume{
		{Name: "abc", VolumeSource: core.VolumeSource{PersistentVolumeClaim: &core.PersistentVolumeClaimVolumeSource{ClaimName: "testclaim1"}}},
		{Name: "abc-123", VolumeSource: core.VolumeSource{PersistentVolumeClaim: &core.PersistentVolumeClaimVolumeSource{ClaimName: "testclaim2"}}},
		{Name: "123", VolumeSource: core.VolumeSource{HostPath: &core.HostPathVolumeSource{Path: "/foo/baz", Type: newHostPathType(string(core.HostPathUnset))}}},
	}
	vols, v1err := ValidateVolumes(volumes, nil, field.NewPath("field"), PodValidationOptions{})
	if len(v1err) > 0 {
		t.Errorf("Invalid test volume - expected success %v", v1err)
		return
	}

	container := core.Container{
		SecurityContext: nil,
	}

	goodVolumeDevices := []core.VolumeDevice{
		{Name: "xyz", DevicePath: "/foofoo"},
		{Name: "uvw", DevicePath: "/foofoo/share/test"},
	}

	cases := map[string]struct {
		mounts      []core.VolumeMount
		expectError bool
	}{
		"subpath and subpathexpr not specified": {
			[]core.VolumeMount{
				{
					Name:      "abc-123",
					MountPath: "/bab",
				},
			},
			false,
		},
		"subpath expr specified": {
			[]core.VolumeMount{
				{
					Name:        "abc-123",
					MountPath:   "/bab",
					SubPathExpr: "$(POD_NAME)",
				},
			},
			false,
		},
		"subpath specified": {
			[]core.VolumeMount{
				{
					Name:      "abc-123",
					MountPath: "/bab",
					SubPath:   "baz",
				},
			},
			false,
		},
		"subpath and subpathexpr specified": {
			[]core.VolumeMount{
				{
					Name:        "abc-123",
					MountPath:   "/bab",
					SubPath:     "baz",
					SubPathExpr: "$(POD_NAME)",
				},
			},
			true,
		},
	}

	for name, test := range cases {
		errs := ValidateVolumeMounts(test.mounts, GetVolumeDeviceMap(goodVolumeDevices), vols, &container, field.NewPath("field"))

		if len(errs) != 0 && !test.expectError {
			t.Errorf("test %v failed: %+v", name, errs)
		}

		if len(errs) == 0 && test.expectError {
			t.Errorf("test %v failed, expected error", name)
		}
	}
}

func TestValidateDisabledSubpathExpr(t *testing.T) {

	volumes := []core.Volume{
		{Name: "abc", VolumeSource: core.VolumeSource{PersistentVolumeClaim: &core.PersistentVolumeClaimVolumeSource{ClaimName: "testclaim1"}}},
		{Name: "abc-123", VolumeSource: core.VolumeSource{PersistentVolumeClaim: &core.PersistentVolumeClaimVolumeSource{ClaimName: "testclaim2"}}},
		{Name: "123", VolumeSource: core.VolumeSource{HostPath: &core.HostPathVolumeSource{Path: "/foo/baz", Type: newHostPathType(string(core.HostPathUnset))}}},
	}
	vols, v1err := ValidateVolumes(volumes, nil, field.NewPath("field"), PodValidationOptions{})
	if len(v1err) > 0 {
		t.Errorf("Invalid test volume - expected success %v", v1err)
		return
	}

	container := core.Container{
		SecurityContext: nil,
	}

	goodVolumeDevices := []core.VolumeDevice{
		{Name: "xyz", DevicePath: "/foofoo"},
		{Name: "uvw", DevicePath: "/foofoo/share/test"},
	}

	cases := map[string]struct {
		mounts      []core.VolumeMount
		expectError bool
	}{
		"subpath expr not specified": {
			[]core.VolumeMount{
				{
					Name:      "abc-123",
					MountPath: "/bab",
				},
			},
			false,
		},
		"subpath expr specified": {
			[]core.VolumeMount{
				{
					Name:        "abc-123",
					MountPath:   "/bab",
					SubPathExpr: "$(POD_NAME)",
				},
			},
			false,
		},
	}

	for name, test := range cases {
		errs := ValidateVolumeMounts(test.mounts, GetVolumeDeviceMap(goodVolumeDevices), vols, &container, field.NewPath("field"))

		if len(errs) != 0 && !test.expectError {
			t.Errorf("test %v failed: %+v", name, errs)
		}

		if len(errs) == 0 && test.expectError {
			t.Errorf("test %v failed, expected error", name)
		}
	}

	// Repeat with subpath feature gate off
	defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.VolumeSubpath, false)()
	cases = map[string]struct {
		mounts      []core.VolumeMount
		expectError bool
	}{
		"subpath expr not specified": {
			[]core.VolumeMount{
				{
					Name:      "abc-123",
					MountPath: "/bab",
				},
			},
			false,
		},
		"subpath expr specified": {
			[]core.VolumeMount{
				{
					Name:        "abc-123",
					MountPath:   "/bab",
					SubPathExpr: "$(POD_NAME)",
				},
			},
			false, // validation should not fail, dropping the field is handled in PrepareForCreate/PrepareForUpdate
		},
	}

	for name, test := range cases {
		errs := ValidateVolumeMounts(test.mounts, GetVolumeDeviceMap(goodVolumeDevices), vols, &container, field.NewPath("field"))

		if len(errs) != 0 && !test.expectError {
			t.Errorf("test %v failed: %+v", name, errs)
		}

		if len(errs) == 0 && test.expectError {
			t.Errorf("test %v failed, expected error", name)
		}
	}
}

func TestValidateMountPropagation(t *testing.T) {
	bTrue := true
	bFalse := false
	privilegedContainer := &core.Container{
		SecurityContext: &core.SecurityContext{
			Privileged: &bTrue,
		},
	}
	nonPrivilegedContainer := &core.Container{
		SecurityContext: &core.SecurityContext{
			Privileged: &bFalse,
		},
	}
	defaultContainer := &core.Container{}

	propagationBidirectional := core.MountPropagationBidirectional
	propagationHostToContainer := core.MountPropagationHostToContainer
	propagationNone := core.MountPropagationNone
	propagationInvalid := core.MountPropagationMode("invalid")

	tests := []struct {
		mount       core.VolumeMount
		container   *core.Container
		expectError bool
	}{
		{
			// implicitly non-privileged container + no propagation
			core.VolumeMount{Name: "foo", MountPath: "/foo"},
			defaultContainer,
			false,
		},
		{
			// implicitly non-privileged container + HostToContainer
			core.VolumeMount{Name: "foo", MountPath: "/foo", MountPropagation: &propagationHostToContainer},
			defaultContainer,
			false,
		},
		{
			// non-privileged container + None
			core.VolumeMount{Name: "foo", MountPath: "/foo", MountPropagation: &propagationNone},
			defaultContainer,
			false,
		},
		{
			// error: implicitly non-privileged container + Bidirectional
			core.VolumeMount{Name: "foo", MountPath: "/foo", MountPropagation: &propagationBidirectional},
			defaultContainer,
			true,
		},
		{
			// explicitly non-privileged container + no propagation
			core.VolumeMount{Name: "foo", MountPath: "/foo"},
			nonPrivilegedContainer,
			false,
		},
		{
			// explicitly non-privileged container + HostToContainer
			core.VolumeMount{Name: "foo", MountPath: "/foo", MountPropagation: &propagationHostToContainer},
			nonPrivilegedContainer,
			false,
		},
		{
			// explicitly non-privileged container + HostToContainer
			core.VolumeMount{Name: "foo", MountPath: "/foo", MountPropagation: &propagationBidirectional},
			nonPrivilegedContainer,
			true,
		},
		{
			// privileged container + no propagation
			core.VolumeMount{Name: "foo", MountPath: "/foo"},
			privilegedContainer,
			false,
		},
		{
			// privileged container + HostToContainer
			core.VolumeMount{Name: "foo", MountPath: "/foo", MountPropagation: &propagationHostToContainer},
			privilegedContainer,
			false,
		},
		{
			// privileged container + Bidirectional
			core.VolumeMount{Name: "foo", MountPath: "/foo", MountPropagation: &propagationBidirectional},
			privilegedContainer,
			false,
		},
		{
			// error: privileged container + invalid mount propagation
			core.VolumeMount{Name: "foo", MountPath: "/foo", MountPropagation: &propagationInvalid},
			privilegedContainer,
			true,
		},
		{
			// no container + Bidirectional
			core.VolumeMount{Name: "foo", MountPath: "/foo", MountPropagation: &propagationBidirectional},
			nil,
			false,
		},
	}

	volumes := []core.Volume{
		{Name: "foo", VolumeSource: core.VolumeSource{HostPath: &core.HostPathVolumeSource{Path: "/foo/baz", Type: newHostPathType(string(core.HostPathUnset))}}},
	}
	vols2, v2err := ValidateVolumes(volumes, nil, field.NewPath("field"), PodValidationOptions{})
	if len(v2err) > 0 {
		t.Errorf("Invalid test volume - expected success %v", v2err)
		return
	}
	for i, test := range tests {
		errs := ValidateVolumeMounts([]core.VolumeMount{test.mount}, nil, vols2, test.container, field.NewPath("field"))
		if test.expectError && len(errs) == 0 {
			t.Errorf("test %d expected error, got none", i)
		}
		if !test.expectError && len(errs) != 0 {
			t.Errorf("test %d expected success, got error: %v", i, errs)
		}
	}
}

func TestAlphaValidateVolumeDevices(t *testing.T) {
	volumes := []core.Volume{
		{Name: "abc", VolumeSource: core.VolumeSource{PersistentVolumeClaim: &core.PersistentVolumeClaimVolumeSource{ClaimName: "testclaim1"}}},
		{Name: "abc-123", VolumeSource: core.VolumeSource{PersistentVolumeClaim: &core.PersistentVolumeClaimVolumeSource{ClaimName: "testclaim2"}}},
		{Name: "def", VolumeSource: core.VolumeSource{HostPath: &core.HostPathVolumeSource{Path: "/foo/baz", Type: newHostPathType(string(core.HostPathUnset))}}},
	}

	vols, v1err := ValidateVolumes(volumes, nil, field.NewPath("field"), PodValidationOptions{})
	if len(v1err) > 0 {
		t.Errorf("Invalid test volumes - expected success %v", v1err)
		return
	}

	successCase := []core.VolumeDevice{
		{Name: "abc", DevicePath: "/foo"},
		{Name: "abc-123", DevicePath: "/usr/share/test"},
	}
	goodVolumeMounts := []core.VolumeMount{
		{Name: "xyz", MountPath: "/foofoo"},
		{Name: "ghi", MountPath: "/foo/usr/share/test"},
	}

	errorCases := map[string][]core.VolumeDevice{
		"empty name":                    {{Name: "", DevicePath: "/foo"}},
		"duplicate name":                {{Name: "abc", DevicePath: "/foo"}, {Name: "abc", DevicePath: "/foo/bar"}},
		"name not found":                {{Name: "not-found", DevicePath: "/usr/share/test"}},
		"name found but invalid source": {{Name: "def", DevicePath: "/usr/share/test"}},
		"empty devicepath":              {{Name: "abc", DevicePath: ""}},
		"relative devicepath":           {{Name: "abc-123", DevicePath: "baz"}},
		"duplicate devicepath":          {{Name: "abc", DevicePath: "/foo"}, {Name: "abc-123", DevicePath: "/foo"}},
		"no backsteps":                  {{Name: "def", DevicePath: "/baz/../"}},
		"name exists in volumemounts":   {{Name: "abc", DevicePath: "/baz/../"}},
		"path exists in volumemounts":   {{Name: "xyz", DevicePath: "/this/path/exists"}},
		"both exist in volumemounts":    {{Name: "abc", DevicePath: "/this/path/exists"}},
	}
	badVolumeMounts := []core.VolumeMount{
		{Name: "abc", MountPath: "/foo"},
		{Name: "abc-123", MountPath: "/this/path/exists"},
	}

	// Success Cases:
	// Validate normal success cases - only PVC volumeSource
	if errs := ValidateVolumeDevices(successCase, GetVolumeMountMap(goodVolumeMounts), vols, field.NewPath("field")); len(errs) != 0 {
		t.Errorf("expected success: %v", errs)
	}

	// Error Cases:
	// Validate normal error cases - only PVC volumeSource
	for k, v := range errorCases {
		if errs := ValidateVolumeDevices(v, GetVolumeMountMap(badVolumeMounts), vols, field.NewPath("field")); len(errs) == 0 {
			t.Errorf("expected failure for %s", k)
		}
	}
}

func TestValidateProbe(t *testing.T) {
	handler := core.Handler{Exec: &core.ExecAction{Command: []string{"echo"}}}
	// These fields must be positive.
	positiveFields := [...]string{"InitialDelaySeconds", "TimeoutSeconds", "PeriodSeconds", "SuccessThreshold", "FailureThreshold"}
	successCases := []*core.Probe{nil}
	for _, field := range positiveFields {
		probe := &core.Probe{Handler: handler}
		reflect.ValueOf(probe).Elem().FieldByName(field).SetInt(10)
		successCases = append(successCases, probe)
	}

	for _, p := range successCases {
		if errs := validateProbe(p, field.NewPath("field")); len(errs) != 0 {
			t.Errorf("expected success: %v", errs)
		}
	}

	errorCases := []*core.Probe{{TimeoutSeconds: 10, InitialDelaySeconds: 10}}
	for _, field := range positiveFields {
		probe := &core.Probe{Handler: handler}
		reflect.ValueOf(probe).Elem().FieldByName(field).SetInt(-10)
		errorCases = append(errorCases, probe)
	}
	for _, p := range errorCases {
		if errs := validateProbe(p, field.NewPath("field")); len(errs) == 0 {
			t.Errorf("expected failure for %v", p)
		}
	}
}

func Test_validateProbe(t *testing.T) {
	defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.ProbeTerminationGracePeriod, true)()

	fldPath := field.NewPath("test")
	type args struct {
		probe   *core.Probe
		fldPath *field.Path
	}
	tests := []struct {
		name string
		args args
		want field.ErrorList
	}{
		{
			args: args{
				probe:   &core.Probe{},
				fldPath: fldPath,
			},
			want: field.ErrorList{field.Required(fldPath, "must specify a handler type")},
		},
		{
			args: args{
				probe: &core.Probe{
					Handler: core.Handler{Exec: &core.ExecAction{Command: []string{"echo"}}},
				},
				fldPath: fldPath,
			},
			want: field.ErrorList{},
		},
		{
			args: args{
				probe: &core.Probe{
					Handler:             core.Handler{Exec: &core.ExecAction{Command: []string{"echo"}}},
					InitialDelaySeconds: -1,
				},
				fldPath: fldPath,
			},
			want: field.ErrorList{field.Invalid(fldPath.Child("initialDelaySeconds"), -1, "must be greater than or equal to 0")},
		},
		{
			args: args{
				probe: &core.Probe{
					Handler:        core.Handler{Exec: &core.ExecAction{Command: []string{"echo"}}},
					TimeoutSeconds: -1,
				},
				fldPath: fldPath,
			},
			want: field.ErrorList{field.Invalid(fldPath.Child("timeoutSeconds"), -1, "must be greater than or equal to 0")},
		},
		{
			args: args{
				probe: &core.Probe{
					Handler:       core.Handler{Exec: &core.ExecAction{Command: []string{"echo"}}},
					PeriodSeconds: -1,
				},
				fldPath: fldPath,
			},
			want: field.ErrorList{field.Invalid(fldPath.Child("periodSeconds"), -1, "must be greater than or equal to 0")},
		},
		{
			args: args{
				probe: &core.Probe{
					Handler:          core.Handler{Exec: &core.ExecAction{Command: []string{"echo"}}},
					SuccessThreshold: -1,
				},
				fldPath: fldPath,
			},
			want: field.ErrorList{field.Invalid(fldPath.Child("successThreshold"), -1, "must be greater than or equal to 0")},
		},
		{
			args: args{
				probe: &core.Probe{
					Handler:          core.Handler{Exec: &core.ExecAction{Command: []string{"echo"}}},
					FailureThreshold: -1,
				},
				fldPath: fldPath,
			},
			want: field.ErrorList{field.Invalid(fldPath.Child("failureThreshold"), -1, "must be greater than or equal to 0")},
		},
		{
			args: args{
				probe: &core.Probe{
					Handler:                       core.Handler{Exec: &core.ExecAction{Command: []string{"echo"}}},
					TerminationGracePeriodSeconds: utilpointer.Int64(-1),
				},
				fldPath: fldPath,
			},
			want: field.ErrorList{field.Invalid(fldPath.Child("terminationGracePeriodSeconds"), -1, "must be greater than 0")},
		},
		{
			args: args{
				probe: &core.Probe{
					Handler:                       core.Handler{Exec: &core.ExecAction{Command: []string{"echo"}}},
					TerminationGracePeriodSeconds: utilpointer.Int64(0),
				},
				fldPath: fldPath,
			},
			want: field.ErrorList{field.Invalid(fldPath.Child("terminationGracePeriodSeconds"), 0, "must be greater than 0")},
		},
		{
			args: args{
				probe: &core.Probe{
					Handler:                       core.Handler{Exec: &core.ExecAction{Command: []string{"echo"}}},
					TerminationGracePeriodSeconds: utilpointer.Int64(1),
				},
				fldPath: fldPath,
			},
			want: field.ErrorList{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validateProbe(tt.args.probe, tt.args.fldPath)
			if len(got) != len(tt.want) {
				t.Errorf("validateProbe() = %v, want %v", got, tt.want)
				return
			}
			for i := range got {
				if got[i].Type != tt.want[i].Type ||
					got[i].Field != tt.want[i].Field {
					t.Errorf("validateProbe()[%d] = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestValidateHandler(t *testing.T) {
	successCases := []core.Handler{
		{Exec: &core.ExecAction{Command: []string{"echo"}}},
		{HTTPGet: &core.HTTPGetAction{Path: "/", Port: intstr.FromInt(1), Host: "", Scheme: "HTTP"}},
		{HTTPGet: &core.HTTPGetAction{Path: "/foo", Port: intstr.FromInt(65535), Host: "host", Scheme: "HTTP"}},
		{HTTPGet: &core.HTTPGetAction{Path: "/", Port: intstr.FromString("port"), Host: "", Scheme: "HTTP"}},
		{HTTPGet: &core.HTTPGetAction{Path: "/", Port: intstr.FromString("port"), Host: "", Scheme: "HTTP", HTTPHeaders: []core.HTTPHeader{{Name: "Host", Value: "foo.example.com"}}}},
		{HTTPGet: &core.HTTPGetAction{Path: "/", Port: intstr.FromString("port"), Host: "", Scheme: "HTTP", HTTPHeaders: []core.HTTPHeader{{Name: "X-Forwarded-For", Value: "1.2.3.4"}, {Name: "X-Forwarded-For", Value: "5.6.7.8"}}}},
	}
	for _, h := range successCases {
		if errs := validateHandler(&h, field.NewPath("field")); len(errs) != 0 {
			t.Errorf("expected success: %v", errs)
		}
	}

	errorCases := []core.Handler{
		{},
		{Exec: &core.ExecAction{Command: []string{}}},
		{HTTPGet: &core.HTTPGetAction{Path: "", Port: intstr.FromInt(0), Host: ""}},
		{HTTPGet: &core.HTTPGetAction{Path: "/foo", Port: intstr.FromInt(65536), Host: "host"}},
		{HTTPGet: &core.HTTPGetAction{Path: "", Port: intstr.FromString(""), Host: ""}},
		{HTTPGet: &core.HTTPGetAction{Path: "/", Port: intstr.FromString("port"), Host: "", Scheme: "HTTP", HTTPHeaders: []core.HTTPHeader{{Name: "Host:", Value: "foo.example.com"}}}},
		{HTTPGet: &core.HTTPGetAction{Path: "/", Port: intstr.FromString("port"), Host: "", Scheme: "HTTP", HTTPHeaders: []core.HTTPHeader{{Name: "X_Forwarded_For", Value: "foo.example.com"}}}},
	}
	for _, h := range errorCases {
		if errs := validateHandler(&h, field.NewPath("field")); len(errs) == 0 {
			t.Errorf("expected failure for %#v", h)
		}
	}
}

func TestValidatePullPolicy(t *testing.T) {
	type T struct {
		Container      core.Container
		ExpectedPolicy core.PullPolicy
	}
	testCases := map[string]T{
		"NotPresent1": {
			core.Container{Name: "abc", Image: "image:latest", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"},
			core.PullIfNotPresent,
		},
		"NotPresent2": {
			core.Container{Name: "abc1", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"},
			core.PullIfNotPresent,
		},
		"Always1": {
			core.Container{Name: "123", Image: "image:latest", ImagePullPolicy: "Always"},
			core.PullAlways,
		},
		"Always2": {
			core.Container{Name: "1234", Image: "image", ImagePullPolicy: "Always"},
			core.PullAlways,
		},
		"Never1": {
			core.Container{Name: "abc-123", Image: "image:latest", ImagePullPolicy: "Never"},
			core.PullNever,
		},
		"Never2": {
			core.Container{Name: "abc-1234", Image: "image", ImagePullPolicy: "Never"},
			core.PullNever,
		},
	}
	for k, v := range testCases {
		ctr := &v.Container
		errs := validatePullPolicy(ctr.ImagePullPolicy, field.NewPath("field"))
		if len(errs) != 0 {
			t.Errorf("case[%s] expected success, got %#v", k, errs)
		}
		if ctr.ImagePullPolicy != v.ExpectedPolicy {
			t.Errorf("case[%s] expected policy %v, got %v", k, v.ExpectedPolicy, ctr.ImagePullPolicy)
		}
	}
}

func getResourceLimits(cpu, memory string) core.ResourceList {
	res := core.ResourceList{}
	res[core.ResourceCPU] = resource.MustParse(cpu)
	res[core.ResourceMemory] = resource.MustParse(memory)
	return res
}

func TestValidateEphemeralContainers(t *testing.T) {
	containers := []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}}
	initContainers := []core.Container{{Name: "ictr", Image: "iimage", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}}
	vols := map[string]core.VolumeSource{"vol": {EmptyDir: &core.EmptyDirVolumeSource{}}}

	// Success Cases
	for title, ephemeralContainers := range map[string][]core.EphemeralContainer{
		"Empty Ephemeral Containers": {},
		"Single Container": {
			{EphemeralContainerCommon: core.EphemeralContainerCommon{Name: "debug", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
		},
		"Multiple Containers": {
			{EphemeralContainerCommon: core.EphemeralContainerCommon{Name: "debug1", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
			{EphemeralContainerCommon: core.EphemeralContainerCommon{Name: "debug2", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
		},
		"Single Container with Target": {
			{
				EphemeralContainerCommon: core.EphemeralContainerCommon{Name: "debug", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"},
				TargetContainerName:      "ctr",
			},
		},
		"All allowed Fields": {
			{
				EphemeralContainerCommon: core.EphemeralContainerCommon{

					Name:       "debug",
					Image:      "image",
					Command:    []string{"bash"},
					Args:       []string{"bash"},
					WorkingDir: "/",
					EnvFrom: []core.EnvFromSource{
						{
							ConfigMapRef: &core.ConfigMapEnvSource{
								LocalObjectReference: core.LocalObjectReference{Name: "dummy"},
								Optional:             &[]bool{true}[0],
							},
						},
					},
					Env: []core.EnvVar{
						{Name: "TEST", Value: "TRUE"},
					},
					VolumeMounts: []core.VolumeMount{
						{Name: "vol", MountPath: "/vol"},
					},
					TerminationMessagePath:   "/dev/termination-log",
					TerminationMessagePolicy: "File",
					ImagePullPolicy:          "IfNotPresent",
					SecurityContext: &core.SecurityContext{
						Capabilities: &core.Capabilities{
							Add: []core.Capability{"SYS_ADMIN"},
						},
					},
					Stdin:     true,
					StdinOnce: true,
					TTY:       true,
				},
			},
		},
	} {
		if errs := validateEphemeralContainers(ephemeralContainers, containers, initContainers, vols, field.NewPath("ephemeralContainers"), PodValidationOptions{}); len(errs) != 0 {
			t.Errorf("expected success for '%s' but got errors: %v", title, errs)
		}
	}

	// Failure Cases
	tcs := []struct {
		title               string
		ephemeralContainers []core.EphemeralContainer
		expectedError       field.Error
	}{

		{
			"Name Collision with Container.Containers",
			[]core.EphemeralContainer{
				{EphemeralContainerCommon: core.EphemeralContainerCommon{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
				{EphemeralContainerCommon: core.EphemeralContainerCommon{Name: "debug1", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
			},
			field.Error{Type: field.ErrorTypeDuplicate, Field: "ephemeralContainers[0].name"},
		},
		{
			"Name Collision with Container.InitContainers",
			[]core.EphemeralContainer{
				{EphemeralContainerCommon: core.EphemeralContainerCommon{Name: "ictr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
				{EphemeralContainerCommon: core.EphemeralContainerCommon{Name: "debug1", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
			},
			field.Error{Type: field.ErrorTypeDuplicate, Field: "ephemeralContainers[0].name"},
		},
		{
			"Name Collision with EphemeralContainers",
			[]core.EphemeralContainer{
				{EphemeralContainerCommon: core.EphemeralContainerCommon{Name: "debug1", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
				{EphemeralContainerCommon: core.EphemeralContainerCommon{Name: "debug1", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
			},
			field.Error{Type: field.ErrorTypeDuplicate, Field: "ephemeralContainers[1].name"},
		},
		{
			"empty Container Container",
			[]core.EphemeralContainer{
				{EphemeralContainerCommon: core.EphemeralContainerCommon{}},
			},
			field.Error{Type: field.ErrorTypeRequired, Field: "ephemeralContainers[0]"},
		},
		{
			"empty Container Name",
			[]core.EphemeralContainer{
				{EphemeralContainerCommon: core.EphemeralContainerCommon{Name: "", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
			},
			field.Error{Type: field.ErrorTypeRequired, Field: "ephemeralContainers[0]"},
		},
		{
			"whitespace padded image name",
			[]core.EphemeralContainer{
				{EphemeralContainerCommon: core.EphemeralContainerCommon{Name: "debug", Image: " image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
			},
			field.Error{Type: field.ErrorTypeInvalid, Field: "ephemeralContainers[0][0].image"},
		},
		{
			"TargetContainerName doesn't exist",
			[]core.EphemeralContainer{
				{
					EphemeralContainerCommon: core.EphemeralContainerCommon{Name: "debug", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"},
					TargetContainerName:      "bogus",
				},
			},
			field.Error{Type: field.ErrorTypeNotFound, Field: "ephemeralContainers[0].targetContainerName"},
		},
		{
			"Container uses disallowed field: Lifecycle",
			[]core.EphemeralContainer{
				{
					EphemeralContainerCommon: core.EphemeralContainerCommon{
						Name:                     "debug",
						Image:                    "image",
						ImagePullPolicy:          "IfNotPresent",
						TerminationMessagePolicy: "File",
						Lifecycle: &core.Lifecycle{
							PreStop: &core.Handler{
								Exec: &core.ExecAction{Command: []string{"ls", "-l"}},
							},
						},
					},
				},
			},
			field.Error{Type: field.ErrorTypeForbidden, Field: "ephemeralContainers[0].lifecycle"},
		},
		{
			"Container uses disallowed field: LivenessProbe",
			[]core.EphemeralContainer{
				{
					EphemeralContainerCommon: core.EphemeralContainerCommon{
						Name:                     "debug",
						Image:                    "image",
						ImagePullPolicy:          "IfNotPresent",
						TerminationMessagePolicy: "File",
						LivenessProbe: &core.Probe{
							Handler: core.Handler{
								TCPSocket: &core.TCPSocketAction{Port: intstr.FromInt(80)},
							},
							SuccessThreshold: 1,
						},
					},
				},
			},
			field.Error{Type: field.ErrorTypeForbidden, Field: "ephemeralContainers[0].livenessProbe"},
		},
		{
			"Container uses disallowed field: Ports",
			[]core.EphemeralContainer{
				{
					EphemeralContainerCommon: core.EphemeralContainerCommon{
						Name:                     "debug",
						Image:                    "image",
						ImagePullPolicy:          "IfNotPresent",
						TerminationMessagePolicy: "File",
						Ports: []core.ContainerPort{
							{Protocol: "TCP", ContainerPort: 80},
						},
					},
				},
			},
			field.Error{Type: field.ErrorTypeForbidden, Field: "ephemeralContainers[0].ports"},
		},
		{
			"Container uses disallowed field: ReadinessProbe",
			[]core.EphemeralContainer{
				{
					EphemeralContainerCommon: core.EphemeralContainerCommon{
						Name:                     "debug",
						Image:                    "image",
						ImagePullPolicy:          "IfNotPresent",
						TerminationMessagePolicy: "File",
						ReadinessProbe: &core.Probe{
							Handler: core.Handler{
								TCPSocket: &core.TCPSocketAction{Port: intstr.FromInt(80)},
							},
						},
					},
				},
			},
			field.Error{Type: field.ErrorTypeForbidden, Field: "ephemeralContainers[0].readinessProbe"},
		},
		{
			"Container uses disallowed field: Resources",
			[]core.EphemeralContainer{
				{
					EphemeralContainerCommon: core.EphemeralContainerCommon{
						Name:                     "debug",
						Image:                    "image",
						ImagePullPolicy:          "IfNotPresent",
						TerminationMessagePolicy: "File",
						Resources: core.ResourceRequirements{
							Limits: core.ResourceList{
								core.ResourceName(core.ResourceCPU): resource.MustParse("10"),
							},
						},
					},
				},
			},
			field.Error{Type: field.ErrorTypeForbidden, Field: "ephemeralContainers[0].resources"},
		},
	}

	for _, tc := range tcs {
		errs := validateEphemeralContainers(tc.ephemeralContainers, containers, initContainers, vols, field.NewPath("ephemeralContainers"), PodValidationOptions{})

		if len(errs) == 0 {
			t.Errorf("for test %q, expected error but received none", tc.title)
		} else if len(errs) > 1 {
			t.Errorf("for test %q, expected 1 error but received %d: %q", tc.title, len(errs), errs)
		} else {
			if errs[0].Type != tc.expectedError.Type {
				t.Errorf("for test %q, expected error type %q but received %q: %q", tc.title, string(tc.expectedError.Type), string(errs[0].Type), errs)
			}
			if errs[0].Field != tc.expectedError.Field {
				t.Errorf("for test %q, expected error for field %q but received error for field %q: %q", tc.title, tc.expectedError.Field, errs[0].Field, errs)
			}
		}
	}
}

func TestValidateContainers(t *testing.T) {
	volumeDevices := make(map[string]core.VolumeSource)
	capabilities.SetForTests(capabilities.Capabilities{
		AllowPrivileged: true,
	})

	successCase := []core.Container{
		{Name: "abc", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"},
		// backwards compatibility to ensure containers in pod template spec do not check for this
		{Name: "def", Image: " ", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"},
		{Name: "ghi", Image: " some  ", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"},
		{Name: "123", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"},
		{Name: "abc-123", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"},
		{
			Name:  "life-123",
			Image: "image",
			Lifecycle: &core.Lifecycle{
				PreStop: &core.Handler{
					Exec: &core.ExecAction{Command: []string{"ls", "-l"}},
				},
			},
			ImagePullPolicy:          "IfNotPresent",
			TerminationMessagePolicy: "File",
		},
		{
			Name:  "resources-test",
			Image: "image",
			Resources: core.ResourceRequirements{
				Limits: core.ResourceList{
					core.ResourceName(core.ResourceCPU):    resource.MustParse("10"),
					core.ResourceName(core.ResourceMemory): resource.MustParse("10G"),
					core.ResourceName("my.org/resource"):   resource.MustParse("10"),
				},
			},
			ImagePullPolicy:          "IfNotPresent",
			TerminationMessagePolicy: "File",
		},
		{
			Name:  "resources-test-with-request-and-limit",
			Image: "image",
			Resources: core.ResourceRequirements{
				Requests: core.ResourceList{
					core.ResourceName(core.ResourceCPU):    resource.MustParse("10"),
					core.ResourceName(core.ResourceMemory): resource.MustParse("10G"),
				},
				Limits: core.ResourceList{
					core.ResourceName(core.ResourceCPU):    resource.MustParse("10"),
					core.ResourceName(core.ResourceMemory): resource.MustParse("10G"),
				},
			},
			ImagePullPolicy:          "IfNotPresent",
			TerminationMessagePolicy: "File",
		},
		{
			Name:  "resources-request-limit-simple",
			Image: "image",
			Resources: core.ResourceRequirements{
				Requests: core.ResourceList{
					core.ResourceName(core.ResourceCPU): resource.MustParse("8"),
				},
				Limits: core.ResourceList{
					core.ResourceName(core.ResourceCPU): resource.MustParse("10"),
				},
			},
			ImagePullPolicy:          "IfNotPresent",
			TerminationMessagePolicy: "File",
		},
		{
			Name:  "resources-request-limit-edge",
			Image: "image",
			Resources: core.ResourceRequirements{
				Requests: core.ResourceList{
					core.ResourceName(core.ResourceCPU):    resource.MustParse("10"),
					core.ResourceName(core.ResourceMemory): resource.MustParse("10G"),
					core.ResourceName("my.org/resource"):   resource.MustParse("10"),
				},
				Limits: core.ResourceList{
					core.ResourceName(core.ResourceCPU):    resource.MustParse("10"),
					core.ResourceName(core.ResourceMemory): resource.MustParse("10G"),
					core.ResourceName("my.org/resource"):   resource.MustParse("10"),
				},
			},
			ImagePullPolicy:          "IfNotPresent",
			TerminationMessagePolicy: "File",
		},
		{
			Name:  "resources-request-limit-partials",
			Image: "image",
			Resources: core.ResourceRequirements{
				Requests: core.ResourceList{
					core.ResourceName(core.ResourceCPU):    resource.MustParse("9.5"),
					core.ResourceName(core.ResourceMemory): resource.MustParse("10G"),
				},
				Limits: core.ResourceList{
					core.ResourceName(core.ResourceCPU):  resource.MustParse("10"),
					core.ResourceName("my.org/resource"): resource.MustParse("10"),
				},
			},
			ImagePullPolicy:          "IfNotPresent",
			TerminationMessagePolicy: "File",
		},
		{
			Name:  "resources-request",
			Image: "image",
			Resources: core.ResourceRequirements{
				Requests: core.ResourceList{
					core.ResourceName(core.ResourceCPU):    resource.MustParse("9.5"),
					core.ResourceName(core.ResourceMemory): resource.MustParse("10G"),
				},
			},
			ImagePullPolicy:          "IfNotPresent",
			TerminationMessagePolicy: "File",
		},
		{
			Name:  "same-host-port-different-protocol",
			Image: "image",
			Ports: []core.ContainerPort{
				{ContainerPort: 80, HostPort: 80, Protocol: "TCP"},
				{ContainerPort: 80, HostPort: 80, Protocol: "UDP"},
			},
			ImagePullPolicy:          "IfNotPresent",
			TerminationMessagePolicy: "File",
		},
		{
			Name:                     "fallback-to-logs-termination-message",
			Image:                    "image",
			ImagePullPolicy:          "IfNotPresent",
			TerminationMessagePolicy: "FallbackToLogsOnError",
		},
		{
			Name:                     "file-termination-message",
			Image:                    "image",
			ImagePullPolicy:          "IfNotPresent",
			TerminationMessagePolicy: "File",
		},
		{
			Name:                     "env-from-source",
			Image:                    "image",
			ImagePullPolicy:          "IfNotPresent",
			TerminationMessagePolicy: "File",
			EnvFrom: []core.EnvFromSource{
				{
					ConfigMapRef: &core.ConfigMapEnvSource{
						LocalObjectReference: core.LocalObjectReference{
							Name: "test",
						},
					},
				},
			},
		},
		{Name: "abc-1234", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File", SecurityContext: fakeValidSecurityContext(true)},
	}
	if errs := validateContainers(successCase, false, volumeDevices, field.NewPath("field"), PodValidationOptions{}); len(errs) != 0 {
		t.Errorf("expected success: %v", errs)
	}

	capabilities.SetForTests(capabilities.Capabilities{
		AllowPrivileged: false,
	})
	errorCases := map[string][]core.Container{
		"zero-length name":     {{Name: "", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
		"zero-length-image":    {{Name: "abc", Image: "", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
		"name > 63 characters": {{Name: strings.Repeat("a", 64), Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
		"name not a DNS label": {{Name: "a.b.c", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
		"name not unique": {
			{Name: "abc", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"},
			{Name: "abc", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"},
		},
		"zero-length image": {{Name: "abc", Image: "", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
		"host port not unique": {
			{Name: "abc", Image: "image", Ports: []core.ContainerPort{{ContainerPort: 80, HostPort: 80, Protocol: "TCP"}},
				ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"},
			{Name: "def", Image: "image", Ports: []core.ContainerPort{{ContainerPort: 81, HostPort: 80, Protocol: "TCP"}},
				ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"},
		},
		"invalid env var name": {
			{Name: "abc", Image: "image", Env: []core.EnvVar{{Name: "ev!1"}}, ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"},
		},
		"unknown volume name": {
			{Name: "abc", Image: "image", VolumeMounts: []core.VolumeMount{{Name: "anything", MountPath: "/foo"}},
				ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"},
		},
		"invalid lifecycle, no exec command.": {
			{
				Name:  "life-123",
				Image: "image",
				Lifecycle: &core.Lifecycle{
					PreStop: &core.Handler{
						Exec: &core.ExecAction{},
					},
				},
				ImagePullPolicy:          "IfNotPresent",
				TerminationMessagePolicy: "File",
			},
		},
		"invalid lifecycle, no http path.": {
			{
				Name:  "life-123",
				Image: "image",
				Lifecycle: &core.Lifecycle{
					PreStop: &core.Handler{
						HTTPGet: &core.HTTPGetAction{},
					},
				},
				ImagePullPolicy:          "IfNotPresent",
				TerminationMessagePolicy: "File",
			},
		},
		"invalid lifecycle, no tcp socket port.": {
			{
				Name:  "life-123",
				Image: "image",
				Lifecycle: &core.Lifecycle{
					PreStop: &core.Handler{
						TCPSocket: &core.TCPSocketAction{},
					},
				},
				ImagePullPolicy:          "IfNotPresent",
				TerminationMessagePolicy: "File",
			},
		},
		"invalid lifecycle, zero tcp socket port.": {
			{
				Name:  "life-123",
				Image: "image",
				Lifecycle: &core.Lifecycle{
					PreStop: &core.Handler{
						TCPSocket: &core.TCPSocketAction{
							Port: intstr.FromInt(0),
						},
					},
				},
				ImagePullPolicy:          "IfNotPresent",
				TerminationMessagePolicy: "File",
			},
		},
		"invalid lifecycle, no action.": {
			{
				Name:  "life-123",
				Image: "image",
				Lifecycle: &core.Lifecycle{
					PreStop: &core.Handler{},
				},
				ImagePullPolicy:          "IfNotPresent",
				TerminationMessagePolicy: "File",
			},
		},
		"invalid readiness probe, terminationGracePeriodSeconds set.": {
			{
				Name:  "life-123",
				Image: "image",
				ReadinessProbe: &core.Probe{
					Handler: core.Handler{
						TCPSocket: &core.TCPSocketAction{},
					},
					TerminationGracePeriodSeconds: utilpointer.Int64Ptr(10),
				},
				ImagePullPolicy:          "IfNotPresent",
				TerminationMessagePolicy: "File",
			},
		},
		"invalid liveness probe, no tcp socket port.": {
			{
				Name:  "life-123",
				Image: "image",
				LivenessProbe: &core.Probe{
					Handler: core.Handler{
						TCPSocket: &core.TCPSocketAction{},
					},
				},
				ImagePullPolicy:          "IfNotPresent",
				TerminationMessagePolicy: "File",
			},
		},
		"invalid liveness probe, no action.": {
			{
				Name:  "life-123",
				Image: "image",
				LivenessProbe: &core.Probe{
					Handler: core.Handler{},
				},
				ImagePullPolicy:          "IfNotPresent",
				TerminationMessagePolicy: "File",
			},
		},
		"invalid message termination policy": {
			{
				Name:                     "life-123",
				Image:                    "image",
				ImagePullPolicy:          "IfNotPresent",
				TerminationMessagePolicy: "Unknown",
			},
		},
		"empty message termination policy": {
			{
				Name:                     "life-123",
				Image:                    "image",
				ImagePullPolicy:          "IfNotPresent",
				TerminationMessagePolicy: "",
			},
		},
		"privilege disabled": {
			{Name: "abc", Image: "image", SecurityContext: fakeValidSecurityContext(true)},
		},
		"invalid compute resource": {
			{
				Name:  "abc-123",
				Image: "image",
				Resources: core.ResourceRequirements{
					Limits: core.ResourceList{
						"disk": resource.MustParse("10G"),
					},
				},
				ImagePullPolicy:          "IfNotPresent",
				TerminationMessagePolicy: "File",
			},
		},
		"Resource CPU invalid": {
			{
				Name:  "abc-123",
				Image: "image",
				Resources: core.ResourceRequirements{
					Limits: getResourceLimits("-10", "0"),
				},
				ImagePullPolicy:          "IfNotPresent",
				TerminationMessagePolicy: "File",
			},
		},
		"Resource Requests CPU invalid": {
			{
				Name:  "abc-123",
				Image: "image",
				Resources: core.ResourceRequirements{
					Requests: getResourceLimits("-10", "0"),
				},
				ImagePullPolicy:          "IfNotPresent",
				TerminationMessagePolicy: "File",
			},
		},
		"Resource Memory invalid": {
			{
				Name:  "abc-123",
				Image: "image",
				Resources: core.ResourceRequirements{
					Limits: getResourceLimits("0", "-10"),
				},
				ImagePullPolicy:          "IfNotPresent",
				TerminationMessagePolicy: "File",
			},
		},
		"Request limit simple invalid": {
			{
				Name:  "abc-123",
				Image: "image",
				Resources: core.ResourceRequirements{
					Limits:   getResourceLimits("5", "3"),
					Requests: getResourceLimits("6", "3"),
				},
				ImagePullPolicy:          "IfNotPresent",
				TerminationMessagePolicy: "File",
			},
		},
		"Invalid storage limit request": {
			{
				Name:  "abc-123",
				Image: "image",
				Resources: core.ResourceRequirements{
					Limits: core.ResourceList{
						core.ResourceName("attachable-volumes-aws-ebs"): *resource.NewQuantity(10, resource.DecimalSI),
					},
				},
				ImagePullPolicy:          "IfNotPresent",
				TerminationMessagePolicy: "File",
			},
		},
		"Request limit multiple invalid": {
			{
				Name:  "abc-123",
				Image: "image",
				Resources: core.ResourceRequirements{
					Limits:   getResourceLimits("5", "3"),
					Requests: getResourceLimits("6", "4"),
				},
				ImagePullPolicy:          "IfNotPresent",
				TerminationMessagePolicy: "File",
			},
		},
		"Invalid env from": {
			{
				Name:                     "env-from-source",
				Image:                    "image",
				ImagePullPolicy:          "IfNotPresent",
				TerminationMessagePolicy: "File",
				EnvFrom: []core.EnvFromSource{
					{
						ConfigMapRef: &core.ConfigMapEnvSource{
							LocalObjectReference: core.LocalObjectReference{
								Name: "$%^&*#",
							},
						},
					},
				},
			},
		},
	}
	for k, v := range errorCases {
		if errs := validateContainers(v, false, volumeDevices, field.NewPath("field"), PodValidationOptions{}); len(errs) == 0 {
			t.Errorf("expected failure for %s", k)
		}
	}
}

func TestValidateInitContainers(t *testing.T) {
	volumeDevices := make(map[string]core.VolumeSource)
	capabilities.SetForTests(capabilities.Capabilities{
		AllowPrivileged: true,
	})

	successCase := []core.Container{
		{
			Name:  "container-1-same-host-port-different-protocol",
			Image: "image",
			Ports: []core.ContainerPort{
				{ContainerPort: 80, HostPort: 80, Protocol: "TCP"},
				{ContainerPort: 80, HostPort: 80, Protocol: "UDP"},
			},
			ImagePullPolicy:          "IfNotPresent",
			TerminationMessagePolicy: "File",
		},
		{
			Name:  "container-2-same-host-port-different-protocol",
			Image: "image",
			Ports: []core.ContainerPort{
				{ContainerPort: 80, HostPort: 80, Protocol: "TCP"},
				{ContainerPort: 80, HostPort: 80, Protocol: "UDP"},
			},
			ImagePullPolicy:          "IfNotPresent",
			TerminationMessagePolicy: "File",
		},
	}
	if errs := validateContainers(successCase, true, volumeDevices, field.NewPath("field"), PodValidationOptions{}); len(errs) != 0 {
		t.Errorf("expected success: %v", errs)
	}

	capabilities.SetForTests(capabilities.Capabilities{
		AllowPrivileged: false,
	})
	errorCases := map[string][]core.Container{
		"duplicate ports": {
			{
				Name:  "abc",
				Image: "image",
				Ports: []core.ContainerPort{
					{
						ContainerPort: 8080, HostPort: 8080, Protocol: "TCP",
					},
					{
						ContainerPort: 8080, HostPort: 8080, Protocol: "TCP",
					},
				},
				ImagePullPolicy:          "IfNotPresent",
				TerminationMessagePolicy: "File",
			},
		},
	}
	for k, v := range errorCases {
		if errs := validateContainers(v, true, volumeDevices, field.NewPath("field"), PodValidationOptions{}); len(errs) == 0 {
			t.Errorf("expected failure for %s", k)
		}
	}
}

func TestValidateRestartPolicy(t *testing.T) {
	successCases := []core.RestartPolicy{
		core.RestartPolicyAlways,
		core.RestartPolicyOnFailure,
		core.RestartPolicyNever,
	}
	for _, policy := range successCases {
		if errs := validateRestartPolicy(&policy, field.NewPath("field")); len(errs) != 0 {
			t.Errorf("expected success: %v", errs)
		}
	}

	errorCases := []core.RestartPolicy{"", "newpolicy"}

	for k, policy := range errorCases {
		if errs := validateRestartPolicy(&policy, field.NewPath("field")); len(errs) == 0 {
			t.Errorf("expected failure for %d", k)
		}
	}
}

func TestValidateDNSPolicy(t *testing.T) {
	successCases := []core.DNSPolicy{core.DNSClusterFirst, core.DNSDefault, core.DNSClusterFirstWithHostNet, core.DNSNone}
	for _, policy := range successCases {
		if errs := validateDNSPolicy(&policy, field.NewPath("field")); len(errs) != 0 {
			t.Errorf("expected success: %v", errs)
		}
	}

	errorCases := []core.DNSPolicy{core.DNSPolicy("invalid"), core.DNSPolicy("")}
	for _, policy := range errorCases {
		if errs := validateDNSPolicy(&policy, field.NewPath("field")); len(errs) == 0 {
			t.Errorf("expected failure for %v", policy)
		}
	}
}

func TestValidatePodDNSConfig(t *testing.T) {
	generateTestSearchPathFunc := func(numChars int) string {
		res := ""
		for i := 0; i < numChars; i++ {
			res = res + "a"
		}
		return res
	}
	testOptionValue := "2"
	testDNSNone := core.DNSNone
	testDNSClusterFirst := core.DNSClusterFirst

	testCases := []struct {
		desc          string
		dnsConfig     *core.PodDNSConfig
		dnsPolicy     *core.DNSPolicy
		opts          PodValidationOptions
		expectedError bool
	}{
		{
			desc:          "valid: empty DNSConfig",
			dnsConfig:     &core.PodDNSConfig{},
			expectedError: false,
		},
		{
			desc: "valid: 1 option",
			dnsConfig: &core.PodDNSConfig{
				Options: []core.PodDNSConfigOption{
					{Name: "ndots", Value: &testOptionValue},
				},
			},
			expectedError: false,
		},
		{
			desc: "valid: 1 nameserver",
			dnsConfig: &core.PodDNSConfig{
				Nameservers: []string{"127.0.0.1"},
			},
			expectedError: false,
		},
		{
			desc: "valid: DNSNone with 1 nameserver",
			dnsConfig: &core.PodDNSConfig{
				Nameservers: []string{"127.0.0.1"},
			},
			dnsPolicy:     &testDNSNone,
			expectedError: false,
		},
		{
			desc: "valid: 1 search path",
			dnsConfig: &core.PodDNSConfig{
				Searches: []string{"custom"},
			},
			expectedError: false,
		},
		{
			desc: "valid: 1 search path with trailing period",
			dnsConfig: &core.PodDNSConfig{
				Searches: []string{"custom."},
			},
			expectedError: false,
		},
		{
			desc: "valid: 3 nameservers and 6 search paths(legacy)",
			dnsConfig: &core.PodDNSConfig{
				Nameservers: []string{"127.0.0.1", "10.0.0.10", "8.8.8.8"},
				Searches:    []string{"custom", "mydomain.com", "local", "cluster.local", "svc.cluster.local", "default.svc.cluster.local."},
			},
			expectedError: false,
		},
		{
			desc: "valid: 3 nameservers and 32 search paths",
			dnsConfig: &core.PodDNSConfig{
				Nameservers: []string{"127.0.0.1", "10.0.0.10", "8.8.8.8"},
				Searches:    []string{"custom", "mydomain.com", "local", "cluster.local", "svc.cluster.local", "default.svc.cluster.local.", "7", "8", "9", "10", "11", "12", "13", "14", "15", "16", "17", "18", "19", "20", "21", "22", "23", "24", "25", "26", "27", "28", "29", "30", "31", "32"},
			},
			opts: PodValidationOptions{
				AllowExpandedDNSConfig: true,
			},
			expectedError: false,
		},
		{
			desc: "valid: 256 characters in search path list(legacy)",
			dnsConfig: &core.PodDNSConfig{
				// We can have 256 - (6 - 1) = 251 characters in total for 6 search paths.
				Searches: []string{
					generateTestSearchPathFunc(1),
					generateTestSearchPathFunc(50),
					generateTestSearchPathFunc(50),
					generateTestSearchPathFunc(50),
					generateTestSearchPathFunc(50),
					generateTestSearchPathFunc(50),
				},
			},
			expectedError: false,
		},
		{
			desc: "valid: 2048 characters in search path list",
			dnsConfig: &core.PodDNSConfig{
				// We can have 2048 - (32 - 1) = 2017 characters in total for 32 search paths.
				Searches: []string{
					generateTestSearchPathFunc(64),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
				},
			},
			opts: PodValidationOptions{
				AllowExpandedDNSConfig: true,
			},
			expectedError: false,
		},
		{
			desc: "valid: ipv6 nameserver",
			dnsConfig: &core.PodDNSConfig{
				Nameservers: []string{"FE80::0202:B3FF:FE1E:8329"},
			},
			expectedError: false,
		},
		{
			desc: "invalid: 4 nameservers",
			dnsConfig: &core.PodDNSConfig{
				Nameservers: []string{"127.0.0.1", "10.0.0.10", "8.8.8.8", "1.2.3.4"},
			},
			expectedError: true,
		},
		{
			desc: "invalid: 7 search paths(legacy)",
			dnsConfig: &core.PodDNSConfig{
				Searches: []string{"custom", "mydomain.com", "local", "cluster.local", "svc.cluster.local", "default.svc.cluster.local", "exceeded"},
			},
			expectedError: true,
		},
		{
			desc: "invalid: 33 search paths",
			dnsConfig: &core.PodDNSConfig{
				Searches: []string{"custom", "mydomain.com", "local", "cluster.local", "svc.cluster.local", "default.svc.cluster.local.", "7", "8", "9", "10", "11", "12", "13", "14", "15", "16", "17", "18", "19", "20", "21", "22", "23", "24", "25", "26", "27", "28", "29", "30", "31", "32", "33"},
			},
			opts: PodValidationOptions{
				AllowExpandedDNSConfig: true,
			},
			expectedError: true,
		},
		{
			desc: "invalid: 257 characters in search path list",
			dnsConfig: &core.PodDNSConfig{
				// We can have 256 - (6 - 1) = 251 characters in total for 6 search paths.
				Searches: []string{
					generateTestSearchPathFunc(2),
					generateTestSearchPathFunc(50),
					generateTestSearchPathFunc(50),
					generateTestSearchPathFunc(50),
					generateTestSearchPathFunc(50),
					generateTestSearchPathFunc(50),
				},
			},
			expectedError: true,
		},
		{
			desc: "invalid: 2049 characters in search path list",
			dnsConfig: &core.PodDNSConfig{
				// We can have 2048 - (32 - 1) = 2017 characters in total for 32 search paths.
				Searches: []string{
					generateTestSearchPathFunc(65),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
					generateTestSearchPathFunc(63),
				},
			},
			opts: PodValidationOptions{
				AllowExpandedDNSConfig: true,
			},
			expectedError: true,
		},
		{
			desc: "invalid search path",
			dnsConfig: &core.PodDNSConfig{
				Searches: []string{"custom?"},
			},
			expectedError: true,
		},
		{
			desc: "invalid nameserver",
			dnsConfig: &core.PodDNSConfig{
				Nameservers: []string{"invalid"},
			},
			expectedError: true,
		},
		{
			desc: "invalid empty option name",
			dnsConfig: &core.PodDNSConfig{
				Options: []core.PodDNSConfigOption{
					{Value: &testOptionValue},
				},
			},
			expectedError: true,
		},
		{
			desc: "invalid: DNSNone with 0 nameserver",
			dnsConfig: &core.PodDNSConfig{
				Searches: []string{"custom"},
			},
			dnsPolicy:     &testDNSNone,
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		if tc.dnsPolicy == nil {
			tc.dnsPolicy = &testDNSClusterFirst
		}

		errs := validatePodDNSConfig(tc.dnsConfig, tc.dnsPolicy, field.NewPath("dnsConfig"), tc.opts)
		if len(errs) != 0 && !tc.expectedError {
			t.Errorf("%v: validatePodDNSConfig(%v) = %v, want nil", tc.desc, tc.dnsConfig, errs)
		} else if len(errs) == 0 && tc.expectedError {
			t.Errorf("%v: validatePodDNSConfig(%v) = nil, want error", tc.desc, tc.dnsConfig)
		}
	}
}

func TestValidatePodReadinessGates(t *testing.T) {
	successCases := []struct {
		desc           string
		readinessGates []core.PodReadinessGate
	}{
		{
			"no gate",
			[]core.PodReadinessGate{},
		},
		{
			"one readiness gate",
			[]core.PodReadinessGate{
				{
					ConditionType: core.PodConditionType("example.com/condition"),
				},
			},
		},
		{
			"two readiness gates",
			[]core.PodReadinessGate{
				{
					ConditionType: core.PodConditionType("example.com/condition1"),
				},
				{
					ConditionType: core.PodConditionType("example.com/condition2"),
				},
			},
		},
	}
	for _, tc := range successCases {
		if errs := validateReadinessGates(tc.readinessGates, field.NewPath("field")); len(errs) != 0 {
			t.Errorf("expect tc %q to success: %v", tc.desc, errs)
		}
	}

	errorCases := []struct {
		desc           string
		readinessGates []core.PodReadinessGate
	}{
		{
			"invalid condition type",
			[]core.PodReadinessGate{
				{
					ConditionType: core.PodConditionType("invalid/condition/type"),
				},
			},
		},
	}
	for _, tc := range errorCases {
		if errs := validateReadinessGates(tc.readinessGates, field.NewPath("field")); len(errs) == 0 {
			t.Errorf("expected tc %q to fail", tc.desc)
		}
	}
}

func TestValidatePodConditions(t *testing.T) {
	successCases := []struct {
		desc          string
		podConditions []core.PodCondition
	}{
		{
			"no condition",
			[]core.PodCondition{},
		},
		{
			"one system condition",
			[]core.PodCondition{
				{
					Type:   core.PodReady,
					Status: core.ConditionTrue,
				},
			},
		},
		{
			"one system condition and one custom condition",
			[]core.PodCondition{
				{
					Type:   core.PodReady,
					Status: core.ConditionTrue,
				},
				{
					Type:   core.PodConditionType("example.com/condition"),
					Status: core.ConditionFalse,
				},
			},
		},
		{
			"two custom condition",
			[]core.PodCondition{
				{
					Type:   core.PodConditionType("foobar"),
					Status: core.ConditionTrue,
				},
				{
					Type:   core.PodConditionType("example.com/condition"),
					Status: core.ConditionFalse,
				},
			},
		},
	}

	for _, tc := range successCases {
		if errs := validatePodConditions(tc.podConditions, field.NewPath("field")); len(errs) != 0 {
			t.Errorf("expected tc %q to success, but got: %v", tc.desc, errs)
		}
	}

	errorCases := []struct {
		desc          string
		podConditions []core.PodCondition
	}{
		{
			"one system condition and a invalid custom condition",
			[]core.PodCondition{
				{
					Type:   core.PodReady,
					Status: core.ConditionStatus("True"),
				},
				{
					Type:   core.PodConditionType("invalid/custom/condition"),
					Status: core.ConditionStatus("True"),
				},
			},
		},
	}
	for _, tc := range errorCases {
		if errs := validatePodConditions(tc.podConditions, field.NewPath("field")); len(errs) == 0 {
			t.Errorf("expected tc %q to fail", tc.desc)
		}
	}
}

func TestValidatePodSpec(t *testing.T) {
	activeDeadlineSeconds := int64(30)
	activeDeadlineSecondsMax := int64(math.MaxInt32)

	minUserID := int64(0)
	maxUserID := int64(2147483647)
	minGroupID := int64(0)
	maxGroupID := int64(2147483647)
	goodfsGroupChangePolicy := core.FSGroupChangeAlways
	badfsGroupChangePolicy1 := core.PodFSGroupChangePolicy("invalid")
	badfsGroupChangePolicy2 := core.PodFSGroupChangePolicy("")

	successCases := map[string]core.PodSpec{
		"populate basic fields, leave defaults for most": {
			Volumes:       []core.Volume{{Name: "vol", VolumeSource: core.VolumeSource{EmptyDir: &core.EmptyDirVolumeSource{}}}},
			Containers:    []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
			RestartPolicy: core.RestartPolicyAlways,
			DNSPolicy:     core.DNSClusterFirst,
		},
		"populate all fields": {
			Volumes: []core.Volume{
				{Name: "vol", VolumeSource: core.VolumeSource{EmptyDir: &core.EmptyDirVolumeSource{}}},
			},
			Containers:     []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
			InitContainers: []core.Container{{Name: "ictr", Image: "iimage", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
			RestartPolicy:  core.RestartPolicyAlways,
			NodeSelector: map[string]string{
				"key": "value",
			},
			NodeName:              "foobar",
			DNSPolicy:             core.DNSClusterFirst,
			ActiveDeadlineSeconds: &activeDeadlineSeconds,
			ServiceAccountName:    "acct",
		},
		"populate all fields with larger active deadline": {
			Volumes: []core.Volume{
				{Name: "vol", VolumeSource: core.VolumeSource{EmptyDir: &core.EmptyDirVolumeSource{}}},
			},
			Containers:     []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
			InitContainers: []core.Container{{Name: "ictr", Image: "iimage", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
			RestartPolicy:  core.RestartPolicyAlways,
			NodeSelector: map[string]string{
				"key": "value",
			},
			NodeName:              "foobar",
			DNSPolicy:             core.DNSClusterFirst,
			ActiveDeadlineSeconds: &activeDeadlineSecondsMax,
			ServiceAccountName:    "acct",
		},
		"populate HostNetwork": {
			Containers: []core.Container{
				{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File",
					Ports: []core.ContainerPort{
						{HostPort: 8080, ContainerPort: 8080, Protocol: "TCP"}},
				},
			},
			SecurityContext: &core.PodSecurityContext{
				HostNetwork: true,
			},
			RestartPolicy: core.RestartPolicyAlways,
			DNSPolicy:     core.DNSClusterFirst,
		},
		"populate RunAsUser SupplementalGroups FSGroup with minID 0": {
			Containers: []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
			SecurityContext: &core.PodSecurityContext{
				SupplementalGroups: []int64{minGroupID},
				RunAsUser:          &minUserID,
				FSGroup:            &minGroupID,
			},
			RestartPolicy: core.RestartPolicyAlways,
			DNSPolicy:     core.DNSClusterFirst,
		},
		"populate RunAsUser SupplementalGroups FSGroup with maxID 2147483647": {
			Containers: []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
			SecurityContext: &core.PodSecurityContext{
				SupplementalGroups: []int64{maxGroupID},
				RunAsUser:          &maxUserID,
				FSGroup:            &maxGroupID,
			},
			RestartPolicy: core.RestartPolicyAlways,
			DNSPolicy:     core.DNSClusterFirst,
		},
		"populate HostIPC": {
			SecurityContext: &core.PodSecurityContext{
				HostIPC: true,
			},
			Volumes:       []core.Volume{{Name: "vol", VolumeSource: core.VolumeSource{EmptyDir: &core.EmptyDirVolumeSource{}}}},
			Containers:    []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
			RestartPolicy: core.RestartPolicyAlways,
			DNSPolicy:     core.DNSClusterFirst,
		},
		"populate HostPID": {
			SecurityContext: &core.PodSecurityContext{
				HostPID: true,
			},
			Volumes:       []core.Volume{{Name: "vol", VolumeSource: core.VolumeSource{EmptyDir: &core.EmptyDirVolumeSource{}}}},
			Containers:    []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
			RestartPolicy: core.RestartPolicyAlways,
			DNSPolicy:     core.DNSClusterFirst,
		},
		"populate Affinity": {
			Volumes:       []core.Volume{{Name: "vol", VolumeSource: core.VolumeSource{EmptyDir: &core.EmptyDirVolumeSource{}}}},
			Containers:    []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
			RestartPolicy: core.RestartPolicyAlways,
			DNSPolicy:     core.DNSClusterFirst,
		},
		"populate HostAliases": {
			HostAliases:   []core.HostAlias{{IP: "12.34.56.78", Hostnames: []string{"host1", "host2"}}},
			Volumes:       []core.Volume{{Name: "vol", VolumeSource: core.VolumeSource{EmptyDir: &core.EmptyDirVolumeSource{}}}},
			Containers:    []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
			RestartPolicy: core.RestartPolicyAlways,
			DNSPolicy:     core.DNSClusterFirst,
		},
		"populate HostAliases with `foo.bar` hostnames": {
			HostAliases:   []core.HostAlias{{IP: "12.34.56.78", Hostnames: []string{"host1.foo", "host2.bar"}}},
			Volumes:       []core.Volume{{Name: "vol", VolumeSource: core.VolumeSource{EmptyDir: &core.EmptyDirVolumeSource{}}}},
			Containers:    []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
			RestartPolicy: core.RestartPolicyAlways,
			DNSPolicy:     core.DNSClusterFirst,
		},
		"populate HostAliases with HostNetwork": {
			HostAliases: []core.HostAlias{{IP: "12.34.56.78", Hostnames: []string{"host1.foo", "host2.bar"}}},
			Containers:  []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
			SecurityContext: &core.PodSecurityContext{
				HostNetwork: true,
			},
			RestartPolicy: core.RestartPolicyAlways,
			DNSPolicy:     core.DNSClusterFirst,
		},
		"populate PriorityClassName": {
			Volumes:           []core.Volume{{Name: "vol", VolumeSource: core.VolumeSource{EmptyDir: &core.EmptyDirVolumeSource{}}}},
			Containers:        []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
			RestartPolicy:     core.RestartPolicyAlways,
			DNSPolicy:         core.DNSClusterFirst,
			PriorityClassName: "valid-name",
		},
		"populate ShareProcessNamespace": {
			Volumes:       []core.Volume{{Name: "vol", VolumeSource: core.VolumeSource{EmptyDir: &core.EmptyDirVolumeSource{}}}},
			Containers:    []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
			RestartPolicy: core.RestartPolicyAlways,
			DNSPolicy:     core.DNSClusterFirst,
			SecurityContext: &core.PodSecurityContext{
				ShareProcessNamespace: &[]bool{true}[0],
			},
		},
		"populate RuntimeClassName": {
			Containers:       []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
			RestartPolicy:    core.RestartPolicyAlways,
			DNSPolicy:        core.DNSClusterFirst,
			RuntimeClassName: utilpointer.StringPtr("valid-sandbox"),
		},
		"populate Overhead": {
			Containers:       []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
			RestartPolicy:    core.RestartPolicyAlways,
			DNSPolicy:        core.DNSClusterFirst,
			RuntimeClassName: utilpointer.StringPtr("valid-sandbox"),
			Overhead:         core.ResourceList{},
		},
		"populate DNSPolicy": {
			Containers: []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
			SecurityContext: &core.PodSecurityContext{
				FSGroupChangePolicy: &goodfsGroupChangePolicy,
			},
			RestartPolicy: core.RestartPolicyAlways,
			DNSPolicy:     core.DNSClusterFirst,
		},
	}
	for k, v := range successCases {
		t.Run(k, func(t *testing.T) {
			if errs := ValidatePodSpec(&v, nil, field.NewPath("field"), PodValidationOptions{}); len(errs) != 0 {
				t.Errorf("expected success: %v", errs)
			}
		})
	}

	activeDeadlineSeconds = int64(0)
	activeDeadlineSecondsTooLarge := int64(math.MaxInt32 + 1)

	minUserID = int64(-1)
	maxUserID = int64(2147483648)
	minGroupID = int64(-1)
	maxGroupID = int64(2147483648)

	failureCases := map[string]core.PodSpec{
		"bad volume": {
			Volumes:       []core.Volume{{}},
			RestartPolicy: core.RestartPolicyAlways,
			DNSPolicy:     core.DNSClusterFirst,
			Containers:    []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
		},
		"no containers": {
			RestartPolicy: core.RestartPolicyAlways,
			DNSPolicy:     core.DNSClusterFirst,
		},
		"bad container": {
			Containers:    []core.Container{{}},
			RestartPolicy: core.RestartPolicyAlways,
			DNSPolicy:     core.DNSClusterFirst,
		},
		"bad init container": {
			Containers:     []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
			InitContainers: []core.Container{{}},
			RestartPolicy:  core.RestartPolicyAlways,
			DNSPolicy:      core.DNSClusterFirst,
		},
		"bad DNS policy": {
			DNSPolicy:     core.DNSPolicy("invalid"),
			RestartPolicy: core.RestartPolicyAlways,
			Containers:    []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
		},
		"bad service account name": {
			Containers:         []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
			RestartPolicy:      core.RestartPolicyAlways,
			DNSPolicy:          core.DNSClusterFirst,
			ServiceAccountName: "invalidName",
		},
		"bad restart policy": {
			RestartPolicy: "UnknowPolicy",
			DNSPolicy:     core.DNSClusterFirst,
			Containers:    []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
		},
		"with hostNetwork hostPort not equal to containerPort": {
			Containers: []core.Container{
				{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", Ports: []core.ContainerPort{
					{HostPort: 8080, ContainerPort: 2600, Protocol: "TCP"}},
				},
			},
			SecurityContext: &core.PodSecurityContext{
				HostNetwork: true,
			},
			RestartPolicy: core.RestartPolicyAlways,
			DNSPolicy:     core.DNSClusterFirst,
		},
		"with hostAliases with invalid IP": {
			SecurityContext: &core.PodSecurityContext{
				HostNetwork: false,
			},
			HostAliases: []core.HostAlias{{IP: "999.999.999.999", Hostnames: []string{"host1", "host2"}}},
		},
		"with hostAliases with invalid hostname": {
			SecurityContext: &core.PodSecurityContext{
				HostNetwork: false,
			},
			HostAliases: []core.HostAlias{{IP: "12.34.56.78", Hostnames: []string{"@#$^#@#$"}}},
		},
		"bad supplementalGroups large than math.MaxInt32": {
			Containers: []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
			SecurityContext: &core.PodSecurityContext{
				HostNetwork:        false,
				SupplementalGroups: []int64{maxGroupID, 1234},
			},
			RestartPolicy: core.RestartPolicyAlways,
			DNSPolicy:     core.DNSClusterFirst,
		},
		"bad supplementalGroups less than 0": {
			Containers: []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
			SecurityContext: &core.PodSecurityContext{
				HostNetwork:        false,
				SupplementalGroups: []int64{minGroupID, 1234},
			},
			RestartPolicy: core.RestartPolicyAlways,
			DNSPolicy:     core.DNSClusterFirst,
		},
		"bad runAsUser large than math.MaxInt32": {
			Containers: []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
			SecurityContext: &core.PodSecurityContext{
				HostNetwork: false,
				RunAsUser:   &maxUserID,
			},
			RestartPolicy: core.RestartPolicyAlways,
			DNSPolicy:     core.DNSClusterFirst,
		},
		"bad runAsUser less than 0": {
			Containers: []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
			SecurityContext: &core.PodSecurityContext{
				HostNetwork: false,
				RunAsUser:   &minUserID,
			},
			RestartPolicy: core.RestartPolicyAlways,
			DNSPolicy:     core.DNSClusterFirst,
		},
		"bad fsGroup large than math.MaxInt32": {
			Containers: []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
			SecurityContext: &core.PodSecurityContext{
				HostNetwork: false,
				FSGroup:     &maxGroupID,
			},
			RestartPolicy: core.RestartPolicyAlways,
			DNSPolicy:     core.DNSClusterFirst,
		},
		"bad fsGroup less than 0": {
			Containers: []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
			SecurityContext: &core.PodSecurityContext{
				HostNetwork: false,
				FSGroup:     &minGroupID,
			},
			RestartPolicy: core.RestartPolicyAlways,
			DNSPolicy:     core.DNSClusterFirst,
		},
		"bad-active-deadline-seconds": {
			Volumes: []core.Volume{
				{Name: "vol", VolumeSource: core.VolumeSource{EmptyDir: &core.EmptyDirVolumeSource{}}},
			},
			Containers:    []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
			RestartPolicy: core.RestartPolicyAlways,
			NodeSelector: map[string]string{
				"key": "value",
			},
			NodeName:              "foobar",
			DNSPolicy:             core.DNSClusterFirst,
			ActiveDeadlineSeconds: &activeDeadlineSeconds,
		},
		"active-deadline-seconds-too-large": {
			Volumes: []core.Volume{
				{Name: "vol", VolumeSource: core.VolumeSource{EmptyDir: &core.EmptyDirVolumeSource{}}},
			},
			Containers:    []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
			RestartPolicy: core.RestartPolicyAlways,
			NodeSelector: map[string]string{
				"key": "value",
			},
			NodeName:              "foobar",
			DNSPolicy:             core.DNSClusterFirst,
			ActiveDeadlineSeconds: &activeDeadlineSecondsTooLarge,
		},
		"bad nodeName": {
			NodeName:      "node name",
			Volumes:       []core.Volume{{Name: "vol", VolumeSource: core.VolumeSource{EmptyDir: &core.EmptyDirVolumeSource{}}}},
			Containers:    []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
			RestartPolicy: core.RestartPolicyAlways,
			DNSPolicy:     core.DNSClusterFirst,
		},
		"bad PriorityClassName": {
			Volumes:           []core.Volume{{Name: "vol", VolumeSource: core.VolumeSource{EmptyDir: &core.EmptyDirVolumeSource{}}}},
			Containers:        []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
			RestartPolicy:     core.RestartPolicyAlways,
			DNSPolicy:         core.DNSClusterFirst,
			PriorityClassName: "InvalidName",
		},
		"ShareProcessNamespace and HostPID both set": {
			Volumes:       []core.Volume{{Name: "vol", VolumeSource: core.VolumeSource{EmptyDir: &core.EmptyDirVolumeSource{}}}},
			Containers:    []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
			RestartPolicy: core.RestartPolicyAlways,
			DNSPolicy:     core.DNSClusterFirst,
			SecurityContext: &core.PodSecurityContext{
				HostPID:               true,
				ShareProcessNamespace: &[]bool{true}[0],
			},
		},
		"bad RuntimeClassName": {
			Containers:       []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
			RestartPolicy:    core.RestartPolicyAlways,
			DNSPolicy:        core.DNSClusterFirst,
			RuntimeClassName: utilpointer.StringPtr("invalid/sandbox"),
		},
		"bad empty fsGroupchangepolicy": {
			Containers: []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
			SecurityContext: &core.PodSecurityContext{
				FSGroupChangePolicy: &badfsGroupChangePolicy2,
			},
			RestartPolicy: core.RestartPolicyAlways,
			DNSPolicy:     core.DNSClusterFirst,
		},
		"bad invalid fsgroupchangepolicy": {
			Containers: []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
			SecurityContext: &core.PodSecurityContext{
				FSGroupChangePolicy: &badfsGroupChangePolicy1,
			},
			RestartPolicy: core.RestartPolicyAlways,
			DNSPolicy:     core.DNSClusterFirst,
		},
	}
	for k, v := range failureCases {
		if errs := ValidatePodSpec(&v, nil, field.NewPath("field"), PodValidationOptions{}); len(errs) == 0 {
			t.Errorf("expected failure for %q", k)
		}
	}
}

func extendPodSpecwithTolerations(in core.PodSpec, tolerations []core.Toleration) core.PodSpec {
	var out core.PodSpec
	out.Containers = in.Containers
	out.RestartPolicy = in.RestartPolicy
	out.DNSPolicy = in.DNSPolicy
	out.Tolerations = tolerations
	return out
}

func TestValidatePod(t *testing.T) {
	validPodSpec := func(affinity *core.Affinity) core.PodSpec {
		spec := core.PodSpec{
			Containers:    []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
			RestartPolicy: core.RestartPolicyAlways,
			DNSPolicy:     core.DNSClusterFirst,
		}
		if affinity != nil {
			spec.Affinity = affinity
		}
		return spec
	}
	validPVCSpec := core.PersistentVolumeClaimSpec{
		AccessModes: []core.PersistentVolumeAccessMode{
			core.ReadWriteOnce,
		},
		Resources: core.ResourceRequirements{
			Requests: core.ResourceList{
				core.ResourceName(core.ResourceStorage): resource.MustParse("10G"),
			},
		},
	}
	validPVCTemplate := core.PersistentVolumeClaimTemplate{
		Spec: validPVCSpec,
	}
	longPodName := strings.Repeat("a", 200)
	longVolName := strings.Repeat("b", 60)

	successCases := map[string]core.Pod{
		"basic fields": {
			ObjectMeta: metav1.ObjectMeta{Name: "123", Namespace: "ns"},
			Spec: core.PodSpec{
				Volumes:       []core.Volume{{Name: "vol", VolumeSource: core.VolumeSource{EmptyDir: &core.EmptyDirVolumeSource{}}}},
				Containers:    []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
				RestartPolicy: core.RestartPolicyAlways,
				DNSPolicy:     core.DNSClusterFirst,
			},
		},
		"just about everything": {
			ObjectMeta: metav1.ObjectMeta{Name: "abc.123.do-re-mi", Namespace: "ns"},
			Spec: core.PodSpec{
				Volumes: []core.Volume{
					{Name: "vol", VolumeSource: core.VolumeSource{EmptyDir: &core.EmptyDirVolumeSource{}}},
				},
				Containers:    []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
				RestartPolicy: core.RestartPolicyAlways,
				DNSPolicy:     core.DNSClusterFirst,
				NodeSelector: map[string]string{
					"key": "value",
				},
				NodeName: "foobar",
			},
		},
		"serialized node affinity requirements": {
			ObjectMeta: metav1.ObjectMeta{
				Name:      "123",
				Namespace: "ns",
			},
			Spec: validPodSpec(
				// TODO: Uncomment and move this block and move inside NodeAffinity once
				// RequiredDuringSchedulingRequiredDuringExecution is implemented
				//		RequiredDuringSchedulingRequiredDuringExecution: &core.NodeSelector{
				//			NodeSelectorTerms: []core.NodeSelectorTerm{
				//				{
				//					MatchExpressions: []core.NodeSelectorRequirement{
				//						{
				//							Key: "key1",
				//							Operator: core.NodeSelectorOpExists
				//						},
				//					},
				//				},
				//			},
				//		},
				&core.Affinity{
					NodeAffinity: &core.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &core.NodeSelector{
							NodeSelectorTerms: []core.NodeSelectorTerm{
								{
									MatchExpressions: []core.NodeSelectorRequirement{
										{
											Key:      "key2",
											Operator: core.NodeSelectorOpIn,
											Values:   []string{"value1", "value2"},
										},
									},
									MatchFields: []core.NodeSelectorRequirement{
										{
											Key:      "metadata.name",
											Operator: core.NodeSelectorOpIn,
											Values:   []string{"host1"},
										},
									},
								},
							},
						},
						PreferredDuringSchedulingIgnoredDuringExecution: []core.PreferredSchedulingTerm{
							{
								Weight: 10,
								Preference: core.NodeSelectorTerm{
									MatchExpressions: []core.NodeSelectorRequirement{
										{
											Key:      "foo",
											Operator: core.NodeSelectorOpIn,
											Values:   []string{"bar"},
										},
									},
								},
							},
						},
					},
				},
			),
		},
		"serialized node affinity requirements, II": {
			ObjectMeta: metav1.ObjectMeta{
				Name:      "123",
				Namespace: "ns",
			},
			Spec: validPodSpec(
				// TODO: Uncomment and move this block and move inside NodeAffinity once
				// RequiredDuringSchedulingRequiredDuringExecution is implemented
				//		RequiredDuringSchedulingRequiredDuringExecution: &core.NodeSelector{
				//			NodeSelectorTerms: []core.NodeSelectorTerm{
				//				{
				//					MatchExpressions: []core.NodeSelectorRequirement{
				//						{
				//							Key: "key1",
				//							Operator: core.NodeSelectorOpExists
				//						},
				//					},
				//				},
				//			},
				//		},
				&core.Affinity{
					NodeAffinity: &core.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &core.NodeSelector{
							NodeSelectorTerms: []core.NodeSelectorTerm{
								{
									MatchExpressions: []core.NodeSelectorRequirement{},
								},
							},
						},
						PreferredDuringSchedulingIgnoredDuringExecution: []core.PreferredSchedulingTerm{
							{
								Weight: 10,
								Preference: core.NodeSelectorTerm{
									MatchExpressions: []core.NodeSelectorRequirement{},
								},
							},
						},
					},
				},
			),
		},
		"serialized pod affinity in affinity requirements in annotations": {
			ObjectMeta: metav1.ObjectMeta{
				Name:      "123",
				Namespace: "ns",
				// TODO: Uncomment and move this block into Annotations map once
				// RequiredDuringSchedulingRequiredDuringExecution is implemented
				//		"requiredDuringSchedulingRequiredDuringExecution": [{
				//			"labelSelector": {
				//				"matchExpressions": [{
				//					"key": "key2",
				//					"operator": "In",
				//					"values": ["value1", "value2"]
				//				}]
				//			},
				//			"namespaces":["ns"],
				//			"topologyKey": "zone"
				//		}]
			},
			Spec: validPodSpec(&core.Affinity{
				PodAffinity: &core.PodAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: []core.PodAffinityTerm{
						{
							LabelSelector: &metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      "key2",
										Operator: metav1.LabelSelectorOpIn,
										Values:   []string{"value1", "value2"},
									},
								},
							},
							TopologyKey: "zone",
							Namespaces:  []string{"ns"},
							NamespaceSelector: &metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      "key",
										Operator: metav1.LabelSelectorOpIn,
										Values:   []string{"value1", "value2"},
									},
								},
							},
						},
					},
					PreferredDuringSchedulingIgnoredDuringExecution: []core.WeightedPodAffinityTerm{
						{
							Weight: 10,
							PodAffinityTerm: core.PodAffinityTerm{
								LabelSelector: &metav1.LabelSelector{
									MatchExpressions: []metav1.LabelSelectorRequirement{
										{
											Key:      "key2",
											Operator: metav1.LabelSelectorOpNotIn,
											Values:   []string{"value1", "value2"},
										},
									},
								},
								Namespaces:  []string{"ns"},
								TopologyKey: "region",
							},
						},
					},
				},
			}),
		},
		"serialized pod anti affinity with different Label Operators in affinity requirements in annotations": {
			ObjectMeta: metav1.ObjectMeta{
				Name:      "123",
				Namespace: "ns",
				// TODO: Uncomment and move this block into Annotations map once
				// RequiredDuringSchedulingRequiredDuringExecution is implemented
				//		"requiredDuringSchedulingRequiredDuringExecution": [{
				//			"labelSelector": {
				//				"matchExpressions": [{
				//					"key": "key2",
				//					"operator": "In",
				//					"values": ["value1", "value2"]
				//				}]
				//			},
				//			"namespaces":["ns"],
				//			"topologyKey": "zone"
				//		}]
			},
			Spec: validPodSpec(&core.Affinity{
				PodAntiAffinity: &core.PodAntiAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: []core.PodAffinityTerm{
						{
							LabelSelector: &metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      "key2",
										Operator: metav1.LabelSelectorOpExists,
									},
								},
							},
							TopologyKey: "zone",
							Namespaces:  []string{"ns"},
						},
					},
					PreferredDuringSchedulingIgnoredDuringExecution: []core.WeightedPodAffinityTerm{
						{
							Weight: 10,
							PodAffinityTerm: core.PodAffinityTerm{
								LabelSelector: &metav1.LabelSelector{
									MatchExpressions: []metav1.LabelSelectorRequirement{
										{
											Key:      "key2",
											Operator: metav1.LabelSelectorOpDoesNotExist,
										},
									},
								},
								Namespaces:  []string{"ns"},
								TopologyKey: "region",
							},
						},
					},
				},
			}),
		},
		"populate forgiveness tolerations with exists operator in annotations.": {
			ObjectMeta: metav1.ObjectMeta{
				Name:      "123",
				Namespace: "ns",
			},
			Spec: extendPodSpecwithTolerations(validPodSpec(nil), []core.Toleration{{Key: "foo", Operator: "Exists", Value: "", Effect: "NoExecute", TolerationSeconds: &[]int64{60}[0]}}),
		},
		"populate forgiveness tolerations with equal operator in annotations.": {
			ObjectMeta: metav1.ObjectMeta{
				Name:      "123",
				Namespace: "ns",
			},
			Spec: extendPodSpecwithTolerations(validPodSpec(nil), []core.Toleration{{Key: "foo", Operator: "Equal", Value: "bar", Effect: "NoExecute", TolerationSeconds: &[]int64{60}[0]}}),
		},
		"populate tolerations equal operator in annotations.": {
			ObjectMeta: metav1.ObjectMeta{
				Name:      "123",
				Namespace: "ns",
			},
			Spec: extendPodSpecwithTolerations(validPodSpec(nil), []core.Toleration{{Key: "foo", Operator: "Equal", Value: "bar", Effect: "NoSchedule"}}),
		},
		"populate tolerations exists operator in annotations.": {
			ObjectMeta: metav1.ObjectMeta{
				Name:      "123",
				Namespace: "ns",
			},
			Spec: validPodSpec(nil),
		},
		"empty key with Exists operator is OK for toleration, empty toleration key means match all taint keys.": {
			ObjectMeta: metav1.ObjectMeta{
				Name:      "123",
				Namespace: "ns",
			},
			Spec: extendPodSpecwithTolerations(validPodSpec(nil), []core.Toleration{{Operator: "Exists", Effect: "NoSchedule"}}),
		},
		"empty operator is OK for toleration, defaults to Equal.": {
			ObjectMeta: metav1.ObjectMeta{
				Name:      "123",
				Namespace: "ns",
			},
			Spec: extendPodSpecwithTolerations(validPodSpec(nil), []core.Toleration{{Key: "foo", Value: "bar", Effect: "NoSchedule"}}),
		},
		"empty effect is OK for toleration, empty toleration effect means match all taint effects.": {
			ObjectMeta: metav1.ObjectMeta{
				Name:      "123",
				Namespace: "ns",
			},
			Spec: extendPodSpecwithTolerations(validPodSpec(nil), []core.Toleration{{Key: "foo", Operator: "Equal", Value: "bar"}}),
		},
		"negative tolerationSeconds is OK for toleration.": {
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod-forgiveness-invalid",
				Namespace: "ns",
			},
			Spec: extendPodSpecwithTolerations(validPodSpec(nil), []core.Toleration{{Key: "node.kubernetes.io/not-ready", Operator: "Exists", Effect: "NoExecute", TolerationSeconds: &[]int64{-2}[0]}}),
		},
		"runtime default seccomp profile": {
			ObjectMeta: metav1.ObjectMeta{
				Name:      "123",
				Namespace: "ns",
				Annotations: map[string]string{
					core.SeccompPodAnnotationKey: core.SeccompProfileRuntimeDefault,
				},
			},
			Spec: validPodSpec(nil),
		},
		"docker default seccomp profile": {
			ObjectMeta: metav1.ObjectMeta{
				Name:      "123",
				Namespace: "ns",
				Annotations: map[string]string{
					core.SeccompPodAnnotationKey: core.DeprecatedSeccompProfileDockerDefault,
				},
			},
			Spec: validPodSpec(nil),
		},
		"unconfined seccomp profile": {
			ObjectMeta: metav1.ObjectMeta{
				Name:      "123",
				Namespace: "ns",
				Annotations: map[string]string{
					core.SeccompPodAnnotationKey: "unconfined",
				},
			},
			Spec: validPodSpec(nil),
		},
		"localhost seccomp profile": {
			ObjectMeta: metav1.ObjectMeta{
				Name:      "123",
				Namespace: "ns",
				Annotations: map[string]string{
					core.SeccompPodAnnotationKey: "localhost/foo",
				},
			},
			Spec: validPodSpec(nil),
		},
		"localhost seccomp profile for a container": {
			ObjectMeta: metav1.ObjectMeta{
				Name:      "123",
				Namespace: "ns",
				Annotations: map[string]string{
					core.SeccompContainerAnnotationKeyPrefix + "foo": "localhost/foo",
				},
			},
			Spec: validPodSpec(nil),
		},
		"runtime default seccomp profile for a pod": {
			ObjectMeta: metav1.ObjectMeta{
				Name:      "123",
				Namespace: "ns",
			},
			Spec: core.PodSpec{
				Containers:    []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
				RestartPolicy: core.RestartPolicyAlways,
				DNSPolicy:     core.DNSDefault,
				SecurityContext: &core.PodSecurityContext{
					SeccompProfile: &core.SeccompProfile{
						Type: core.SeccompProfileTypeRuntimeDefault,
					},
				},
			},
		},
		"runtime default seccomp profile for a container": {
			ObjectMeta: metav1.ObjectMeta{
				Name:      "123",
				Namespace: "ns",
			},
			Spec: core.PodSpec{
				Containers: []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File",
					SecurityContext: &core.SecurityContext{
						SeccompProfile: &core.SeccompProfile{
							Type: core.SeccompProfileTypeRuntimeDefault,
						},
					},
				}},
				RestartPolicy: core.RestartPolicyAlways,
				DNSPolicy:     core.DNSDefault,
			},
		},
		"unconfined seccomp profile for a pod": {
			ObjectMeta: metav1.ObjectMeta{
				Name:      "123",
				Namespace: "ns",
			},
			Spec: core.PodSpec{
				Containers:    []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
				RestartPolicy: core.RestartPolicyAlways,
				DNSPolicy:     core.DNSDefault,
				SecurityContext: &core.PodSecurityContext{
					SeccompProfile: &core.SeccompProfile{
						Type: core.SeccompProfileTypeUnconfined,
					},
				},
			},
		},
		"unconfined seccomp profile for a container": {
			ObjectMeta: metav1.ObjectMeta{
				Name:      "123",
				Namespace: "ns",
			},
			Spec: core.PodSpec{
				Containers: []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File",
					SecurityContext: &core.SecurityContext{
						SeccompProfile: &core.SeccompProfile{
							Type: core.SeccompProfileTypeUnconfined,
						},
					},
				}},
				RestartPolicy: core.RestartPolicyAlways,
				DNSPolicy:     core.DNSDefault,
			},
		},
		"localhost seccomp profile for a pod": {
			ObjectMeta: metav1.ObjectMeta{
				Name:      "123",
				Namespace: "ns",
			},
			Spec: core.PodSpec{
				Containers:    []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
				RestartPolicy: core.RestartPolicyAlways,
				DNSPolicy:     core.DNSDefault,
				SecurityContext: &core.PodSecurityContext{
					SeccompProfile: &core.SeccompProfile{
						Type:             core.SeccompProfileTypeLocalhost,
						LocalhostProfile: utilpointer.StringPtr("filename.json"),
					},
				},
			},
		},
		"localhost seccomp profile for a container, II": {
			ObjectMeta: metav1.ObjectMeta{
				Name:      "123",
				Namespace: "ns",
			},
			Spec: core.PodSpec{
				Containers: []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File",
					SecurityContext: &core.SecurityContext{
						SeccompProfile: &core.SeccompProfile{
							Type:             core.SeccompProfileTypeLocalhost,
							LocalhostProfile: utilpointer.StringPtr("filename.json"),
						},
					},
				}},
				RestartPolicy: core.RestartPolicyAlways,
				DNSPolicy:     core.DNSDefault,
			},
		},
		"default AppArmor profile for a container": {
			ObjectMeta: metav1.ObjectMeta{
				Name:      "123",
				Namespace: "ns",
				Annotations: map[string]string{
					v1.AppArmorBetaContainerAnnotationKeyPrefix + "ctr": v1.AppArmorBetaProfileRuntimeDefault,
				},
			},
			Spec: validPodSpec(nil),
		},
		"default AppArmor profile for an init container": {
			ObjectMeta: metav1.ObjectMeta{
				Name:      "123",
				Namespace: "ns",
				Annotations: map[string]string{
					v1.AppArmorBetaContainerAnnotationKeyPrefix + "init-ctr": v1.AppArmorBetaProfileRuntimeDefault,
				},
			},
			Spec: core.PodSpec{
				InitContainers: []core.Container{{Name: "init-ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
				Containers:     []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
				RestartPolicy:  core.RestartPolicyAlways,
				DNSPolicy:      core.DNSClusterFirst,
			},
		},
		"localhost AppArmor profile for a container": {
			ObjectMeta: metav1.ObjectMeta{
				Name:      "123",
				Namespace: "ns",
				Annotations: map[string]string{
					v1.AppArmorBetaContainerAnnotationKeyPrefix + "ctr": v1.AppArmorBetaProfileNamePrefix + "foo",
				},
			},
			Spec: validPodSpec(nil),
		},
		"syntactically valid sysctls": {
			ObjectMeta: metav1.ObjectMeta{
				Name:      "123",
				Namespace: "ns",
			},
			Spec: core.PodSpec{
				Containers:    []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
				RestartPolicy: core.RestartPolicyAlways,
				DNSPolicy:     core.DNSClusterFirst,
				SecurityContext: &core.PodSecurityContext{
					Sysctls: []core.Sysctl{
						{
							Name:  "kernel.shmmni",
							Value: "32768",
						},
						{
							Name:  "kernel.shmmax",
							Value: "1000000000",
						},
						{
							Name:  "knet.ipv4.route.min_pmtu",
							Value: "1000",
						},
					},
				},
			},
		},
		"valid extended resources for init container": {
			ObjectMeta: metav1.ObjectMeta{Name: "valid-extended", Namespace: "ns"},
			Spec: core.PodSpec{
				InitContainers: []core.Container{
					{
						Name:            "valid-extended",
						Image:           "image",
						ImagePullPolicy: "IfNotPresent",
						Resources: core.ResourceRequirements{
							Requests: core.ResourceList{
								core.ResourceName("example.com/a"): resource.MustParse("10"),
							},
							Limits: core.ResourceList{
								core.ResourceName("example.com/a"): resource.MustParse("10"),
							},
						},
						TerminationMessagePolicy: "File",
					},
				},
				Containers:    []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
				RestartPolicy: core.RestartPolicyAlways,
				DNSPolicy:     core.DNSClusterFirst,
			},
		},
		"valid extended resources for regular container": {
			ObjectMeta: metav1.ObjectMeta{Name: "valid-extended", Namespace: "ns"},
			Spec: core.PodSpec{
				InitContainers: []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
				Containers: []core.Container{
					{
						Name:            "valid-extended",
						Image:           "image",
						ImagePullPolicy: "IfNotPresent",
						Resources: core.ResourceRequirements{
							Requests: core.ResourceList{
								core.ResourceName("example.com/a"): resource.MustParse("10"),
							},
							Limits: core.ResourceList{
								core.ResourceName("example.com/a"): resource.MustParse("10"),
							},
						},
						TerminationMessagePolicy: "File",
					},
				},
				RestartPolicy: core.RestartPolicyAlways,
				DNSPolicy:     core.DNSClusterFirst,
			},
		},
		"valid serviceaccount token projected volume with serviceaccount name specified": {
			ObjectMeta: metav1.ObjectMeta{Name: "valid-extended", Namespace: "ns"},
			Spec: core.PodSpec{
				ServiceAccountName: "some-service-account",
				Containers:         []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
				RestartPolicy:      core.RestartPolicyAlways,
				DNSPolicy:          core.DNSClusterFirst,
				Volumes: []core.Volume{
					{
						Name: "projected-volume",
						VolumeSource: core.VolumeSource{
							Projected: &core.ProjectedVolumeSource{
								Sources: []core.VolumeProjection{
									{
										ServiceAccountToken: &core.ServiceAccountTokenProjection{
											Audience:          "foo-audience",
											ExpirationSeconds: 6000,
											Path:              "foo-path",
										},
									},
								},
							},
						},
					},
				},
			},
		},
		"ephemeral volume + PVC, no conflict between them": {
			ObjectMeta: metav1.ObjectMeta{Name: "123", Namespace: "ns"},
			Spec: core.PodSpec{
				Volumes: []core.Volume{
					{Name: "pvc", VolumeSource: core.VolumeSource{PersistentVolumeClaim: &core.PersistentVolumeClaimVolumeSource{ClaimName: "my-pvc"}}},
					{Name: "ephemeral", VolumeSource: core.VolumeSource{Ephemeral: &core.EphemeralVolumeSource{VolumeClaimTemplate: &validPVCTemplate}}},
				},
				Containers:    []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
				RestartPolicy: core.RestartPolicyAlways,
				DNSPolicy:     core.DNSClusterFirst,
			},
		},
		"negative pod-deletion-cost": {
			ObjectMeta: metav1.ObjectMeta{Name: "123", Namespace: "ns", Annotations: map[string]string{core.PodDeletionCost: "-100"}},
			Spec: core.PodSpec{
				Containers:    []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
				RestartPolicy: core.RestartPolicyAlways,
				DNSPolicy:     core.DNSClusterFirst,
			},
		},
		"positive pod-deletion-cost": {
			ObjectMeta: metav1.ObjectMeta{Name: "123", Namespace: "ns", Annotations: map[string]string{core.PodDeletionCost: "100"}},
			Spec: core.PodSpec{
				Containers:    []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
				RestartPolicy: core.RestartPolicyAlways,
				DNSPolicy:     core.DNSClusterFirst,
			},
		},
	}
	for k, v := range successCases {
		t.Run(k, func(t *testing.T) {
			if errs := ValidatePodCreate(&v, PodValidationOptions{}); len(errs) != 0 {
				t.Errorf("expected success: %v", errs)
			}
		})
	}

	errorCases := map[string]struct {
		spec          core.Pod
		expectedError string
	}{
		"bad name": {
			expectedError: "metadata.name",
			spec: core.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "", Namespace: "ns"},
				Spec: core.PodSpec{
					RestartPolicy: core.RestartPolicyAlways,
					DNSPolicy:     core.DNSClusterFirst,
					Containers:    []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
				},
			},
		},
		"image whitespace": {
			expectedError: "spec.containers[0].image",
			spec: core.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: "ns"},
				Spec: core.PodSpec{
					RestartPolicy: core.RestartPolicyAlways,
					DNSPolicy:     core.DNSClusterFirst,
					Containers:    []core.Container{{Name: "ctr", Image: " ", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
				},
			},
		},
		"image leading and trailing whitespace": {
			expectedError: "spec.containers[0].image",
			spec: core.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: "ns"},
				Spec: core.PodSpec{
					RestartPolicy: core.RestartPolicyAlways,
					DNSPolicy:     core.DNSClusterFirst,
					Containers:    []core.Container{{Name: "ctr", Image: " something ", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
				},
			},
		},
		"bad namespace": {
			expectedError: "metadata.namespace",
			spec: core.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: ""},
				Spec: core.PodSpec{
					RestartPolicy: core.RestartPolicyAlways,
					DNSPolicy:     core.DNSClusterFirst,
					Containers:    []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
				},
			},
		},
		"bad spec": {
			expectedError: "spec.containers[0].name",
			spec: core.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: "ns"},
				Spec: core.PodSpec{
					Containers: []core.Container{{}},
				},
			},
		},
		"bad label": {
			expectedError: "NoUppercaseOrSpecialCharsLike=Equals",
			spec: core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "abc",
					Namespace: "ns",
					Labels: map[string]string{
						"NoUppercaseOrSpecialCharsLike=Equals": "bar",
					},
				},
				Spec: core.PodSpec{
					RestartPolicy: core.RestartPolicyAlways,
					DNSPolicy:     core.DNSClusterFirst,
					Containers:    []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
				},
			},
		},
		"invalid node selector requirement in node affinity, operator can't be null": {
			expectedError: "spec.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms[0].matchExpressions[0].operator",
			spec: core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "123",
					Namespace: "ns",
				},
				Spec: validPodSpec(&core.Affinity{
					NodeAffinity: &core.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &core.NodeSelector{
							NodeSelectorTerms: []core.NodeSelectorTerm{
								{
									MatchExpressions: []core.NodeSelectorRequirement{
										{
											Key: "key1",
										},
									},
								},
							},
						},
					},
				}),
			},
		},
		"invalid node selector requirement in node affinity, key is invalid": {
			expectedError: "spec.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms[0].matchExpressions[0].key",
			spec: core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "123",
					Namespace: "ns",
				},
				Spec: validPodSpec(&core.Affinity{
					NodeAffinity: &core.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &core.NodeSelector{
							NodeSelectorTerms: []core.NodeSelectorTerm{
								{
									MatchExpressions: []core.NodeSelectorRequirement{
										{
											Key:      "invalid key ___@#",
											Operator: core.NodeSelectorOpExists,
										},
									},
								},
							},
						},
					},
				}),
			},
		},
		"invalid node field selector requirement in node affinity, more values for field selector": {
			expectedError: "spec.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms[0].matchFields[0].values",
			spec: core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "123",
					Namespace: "ns",
				},
				Spec: validPodSpec(&core.Affinity{
					NodeAffinity: &core.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &core.NodeSelector{
							NodeSelectorTerms: []core.NodeSelectorTerm{
								{
									MatchFields: []core.NodeSelectorRequirement{
										{
											Key:      "metadata.name",
											Operator: core.NodeSelectorOpIn,
											Values:   []string{"host1", "host2"},
										},
									},
								},
							},
						},
					},
				}),
			},
		},
		"invalid node field selector requirement in node affinity, invalid operator": {
			expectedError: "spec.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms[0].matchFields[0].operator",
			spec: core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "123",
					Namespace: "ns",
				},
				Spec: validPodSpec(&core.Affinity{
					NodeAffinity: &core.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &core.NodeSelector{
							NodeSelectorTerms: []core.NodeSelectorTerm{
								{
									MatchFields: []core.NodeSelectorRequirement{
										{
											Key:      "metadata.name",
											Operator: core.NodeSelectorOpExists,
										},
									},
								},
							},
						},
					},
				}),
			},
		},
		"invalid node field selector requirement in node affinity, invalid key": {
			expectedError: "spec.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms[0].matchFields[0].key",
			spec: core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "123",
					Namespace: "ns",
				},
				Spec: validPodSpec(&core.Affinity{
					NodeAffinity: &core.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &core.NodeSelector{
							NodeSelectorTerms: []core.NodeSelectorTerm{
								{
									MatchFields: []core.NodeSelectorRequirement{
										{
											Key:      "metadata.namespace",
											Operator: core.NodeSelectorOpIn,
											Values:   []string{"ns1"},
										},
									},
								},
							},
						},
					},
				}),
			},
		},
		"invalid preferredSchedulingTerm in node affinity, weight should be in range 1-100": {
			expectedError: "must be in the range 1-100",
			spec: core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "123",
					Namespace: "ns",
				},
				Spec: validPodSpec(&core.Affinity{
					NodeAffinity: &core.NodeAffinity{
						PreferredDuringSchedulingIgnoredDuringExecution: []core.PreferredSchedulingTerm{
							{
								Weight: 199,
								Preference: core.NodeSelectorTerm{
									MatchExpressions: []core.NodeSelectorRequirement{
										{
											Key:      "foo",
											Operator: core.NodeSelectorOpIn,
											Values:   []string{"bar"},
										},
									},
								},
							},
						},
					},
				}),
			},
		},
		"invalid requiredDuringSchedulingIgnoredDuringExecution node selector, nodeSelectorTerms must have at least one term": {
			expectedError: "spec.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms",
			spec: core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "123",
					Namespace: "ns",
				},
				Spec: validPodSpec(&core.Affinity{
					NodeAffinity: &core.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &core.NodeSelector{
							NodeSelectorTerms: []core.NodeSelectorTerm{},
						},
					},
				}),
			},
		},
		"invalid weight in preferredDuringSchedulingIgnoredDuringExecution in pod affinity annotations, weight should be in range 1-100": {
			expectedError: "must be in the range 1-100",
			spec: core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "123",
					Namespace: "ns",
				},
				Spec: validPodSpec(&core.Affinity{
					PodAffinity: &core.PodAffinity{
						PreferredDuringSchedulingIgnoredDuringExecution: []core.WeightedPodAffinityTerm{
							{
								Weight: 109,
								PodAffinityTerm: core.PodAffinityTerm{
									LabelSelector: &metav1.LabelSelector{
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Key:      "key2",
												Operator: metav1.LabelSelectorOpNotIn,
												Values:   []string{"value1", "value2"},
											},
										},
									},
									Namespaces:  []string{"ns"},
									TopologyKey: "region",
								},
							},
						},
					},
				}),
			},
		},
		"invalid labelSelector in preferredDuringSchedulingIgnoredDuringExecution in podaffinity annotations, values should be empty if the operator is Exists": {
			expectedError: "spec.affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution[0].podAffinityTerm.labelSelector.matchExpressions[0].values",
			spec: core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "123",
					Namespace: "ns",
				},
				Spec: validPodSpec(&core.Affinity{
					PodAntiAffinity: &core.PodAntiAffinity{
						PreferredDuringSchedulingIgnoredDuringExecution: []core.WeightedPodAffinityTerm{
							{
								Weight: 10,
								PodAffinityTerm: core.PodAffinityTerm{
									LabelSelector: &metav1.LabelSelector{
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Key:      "key2",
												Operator: metav1.LabelSelectorOpExists,
												Values:   []string{"value1", "value2"},
											},
										},
									},
									Namespaces:  []string{"ns"},
									TopologyKey: "region",
								},
							},
						},
					},
				}),
			},
		},
		"invalid namespaceSelector in preferredDuringSchedulingIgnoredDuringExecution in podaffinity, In operator must include Values": {
			expectedError: "spec.affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution[0].podAffinityTerm.namespaceSelector.matchExpressions[0].values",
			spec: core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "123",
					Namespace: "ns",
				},
				Spec: validPodSpec(&core.Affinity{
					PodAntiAffinity: &core.PodAntiAffinity{
						PreferredDuringSchedulingIgnoredDuringExecution: []core.WeightedPodAffinityTerm{
							{
								Weight: 10,
								PodAffinityTerm: core.PodAffinityTerm{
									NamespaceSelector: &metav1.LabelSelector{
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Key:      "key2",
												Operator: metav1.LabelSelectorOpIn,
											},
										},
									},
									Namespaces:  []string{"ns"},
									TopologyKey: "region",
								},
							},
						},
					},
				}),
			},
		},
		"invalid namespaceSelector in preferredDuringSchedulingIgnoredDuringExecution in podaffinity, Exists operator can not have values": {
			expectedError: "spec.affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution[0].podAffinityTerm.namespaceSelector.matchExpressions[0].values",
			spec: core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "123",
					Namespace: "ns",
				},
				Spec: validPodSpec(&core.Affinity{
					PodAntiAffinity: &core.PodAntiAffinity{
						PreferredDuringSchedulingIgnoredDuringExecution: []core.WeightedPodAffinityTerm{
							{
								Weight: 10,
								PodAffinityTerm: core.PodAffinityTerm{
									NamespaceSelector: &metav1.LabelSelector{
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Key:      "key2",
												Operator: metav1.LabelSelectorOpExists,
												Values:   []string{"value1", "value2"},
											},
										},
									},
									Namespaces:  []string{"ns"},
									TopologyKey: "region",
								},
							},
						},
					},
				}),
			},
		},
		"invalid name space in preferredDuringSchedulingIgnoredDuringExecution in podaffinity annotations, namespace should be valid": {
			expectedError: "spec.affinity.podAffinity.preferredDuringSchedulingIgnoredDuringExecution[0].podAffinityTerm.namespace",
			spec: core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "123",
					Namespace: "ns",
				},
				Spec: validPodSpec(&core.Affinity{
					PodAffinity: &core.PodAffinity{
						PreferredDuringSchedulingIgnoredDuringExecution: []core.WeightedPodAffinityTerm{
							{
								Weight: 10,
								PodAffinityTerm: core.PodAffinityTerm{
									LabelSelector: &metav1.LabelSelector{
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Key:      "key2",
												Operator: metav1.LabelSelectorOpExists,
											},
										},
									},
									Namespaces:  []string{"INVALID_NAMESPACE"},
									TopologyKey: "region",
								},
							},
						},
					},
				}),
			},
		},
		"invalid hard pod affinity, empty topologyKey is not allowed for hard pod affinity": {
			expectedError: "can not be empty",
			spec: core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "123",
					Namespace: "ns",
				},
				Spec: validPodSpec(&core.Affinity{
					PodAffinity: &core.PodAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: []core.PodAffinityTerm{
							{
								LabelSelector: &metav1.LabelSelector{
									MatchExpressions: []metav1.LabelSelectorRequirement{
										{
											Key:      "key2",
											Operator: metav1.LabelSelectorOpIn,
											Values:   []string{"value1", "value2"},
										},
									},
								},
								Namespaces: []string{"ns"},
							},
						},
					},
				}),
			},
		},
		"invalid hard pod anti-affinity, empty topologyKey is not allowed for hard pod anti-affinity": {
			expectedError: "can not be empty",
			spec: core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "123",
					Namespace: "ns",
				},
				Spec: validPodSpec(&core.Affinity{
					PodAntiAffinity: &core.PodAntiAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: []core.PodAffinityTerm{
							{
								LabelSelector: &metav1.LabelSelector{
									MatchExpressions: []metav1.LabelSelectorRequirement{
										{
											Key:      "key2",
											Operator: metav1.LabelSelectorOpIn,
											Values:   []string{"value1", "value2"},
										},
									},
								},
								Namespaces: []string{"ns"},
							},
						},
					},
				}),
			},
		},
		"invalid soft pod affinity, empty topologyKey is not allowed for soft pod affinity": {
			expectedError: "can not be empty",
			spec: core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "123",
					Namespace: "ns",
				},
				Spec: validPodSpec(&core.Affinity{
					PodAffinity: &core.PodAffinity{
						PreferredDuringSchedulingIgnoredDuringExecution: []core.WeightedPodAffinityTerm{
							{
								Weight: 10,
								PodAffinityTerm: core.PodAffinityTerm{
									LabelSelector: &metav1.LabelSelector{
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Key:      "key2",
												Operator: metav1.LabelSelectorOpNotIn,
												Values:   []string{"value1", "value2"},
											},
										},
									},
									Namespaces: []string{"ns"},
								},
							},
						},
					},
				}),
			},
		},
		"invalid soft pod anti-affinity, empty topologyKey is not allowed for soft pod anti-affinity": {
			expectedError: "can not be empty",
			spec: core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "123",
					Namespace: "ns",
				},
				Spec: validPodSpec(&core.Affinity{
					PodAntiAffinity: &core.PodAntiAffinity{
						PreferredDuringSchedulingIgnoredDuringExecution: []core.WeightedPodAffinityTerm{
							{
								Weight: 10,
								PodAffinityTerm: core.PodAffinityTerm{
									LabelSelector: &metav1.LabelSelector{
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Key:      "key2",
												Operator: metav1.LabelSelectorOpNotIn,
												Values:   []string{"value1", "value2"},
											},
										},
									},
									Namespaces: []string{"ns"},
								},
							},
						},
					},
				}),
			},
		},
		"invalid toleration key": {
			expectedError: "spec.tolerations[0].key",
			spec: core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "123",
					Namespace: "ns",
				},
				Spec: extendPodSpecwithTolerations(validPodSpec(nil), []core.Toleration{{Key: "nospecialchars^=@", Operator: "Equal", Value: "bar", Effect: "NoSchedule"}}),
			},
		},
		"invalid toleration operator": {
			expectedError: "spec.tolerations[0].operator",
			spec: core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "123",
					Namespace: "ns",
				},
				Spec: extendPodSpecwithTolerations(validPodSpec(nil), []core.Toleration{{Key: "foo", Operator: "In", Value: "bar", Effect: "NoSchedule"}}),
			},
		},
		"value must be empty when `operator` is 'Exists'": {
			expectedError: "spec.tolerations[0].operator",
			spec: core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "123",
					Namespace: "ns",
				},
				Spec: extendPodSpecwithTolerations(validPodSpec(nil), []core.Toleration{{Key: "foo", Operator: "Exists", Value: "bar", Effect: "NoSchedule"}}),
			},
		},

		"operator must be 'Exists' when `key` is empty": {
			expectedError: "spec.tolerations[0].operator",
			spec: core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "123",
					Namespace: "ns",
				},
				Spec: extendPodSpecwithTolerations(validPodSpec(nil), []core.Toleration{{Operator: "Equal", Value: "bar", Effect: "NoSchedule"}}),
			},
		},
		"effect must be 'NoExecute' when `TolerationSeconds` is set": {
			expectedError: "spec.tolerations[0].effect",
			spec: core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod-forgiveness-invalid",
					Namespace: "ns",
				},
				Spec: extendPodSpecwithTolerations(validPodSpec(nil), []core.Toleration{{Key: "node.kubernetes.io/not-ready", Operator: "Exists", Effect: "NoSchedule", TolerationSeconds: &[]int64{20}[0]}}),
			},
		},
		"must be a valid pod seccomp profile": {
			expectedError: "must be a valid seccomp profile",
			spec: core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "123",
					Namespace: "ns",
					Annotations: map[string]string{
						core.SeccompPodAnnotationKey: "foo",
					},
				},
				Spec: validPodSpec(nil),
			},
		},
		"must be a valid container seccomp profile": {
			expectedError: "must be a valid seccomp profile",
			spec: core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "123",
					Namespace: "ns",
					Annotations: map[string]string{
						core.SeccompContainerAnnotationKeyPrefix + "foo": "foo",
					},
				},
				Spec: validPodSpec(nil),
			},
		},
		"must be a non-empty container name in seccomp annotation": {
			expectedError: "name part must be non-empty",
			spec: core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "123",
					Namespace: "ns",
					Annotations: map[string]string{
						core.SeccompContainerAnnotationKeyPrefix: "foo",
					},
				},
				Spec: validPodSpec(nil),
			},
		},
		"must be a non-empty container profile in seccomp annotation": {
			expectedError: "must be a valid seccomp profile",
			spec: core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "123",
					Namespace: "ns",
					Annotations: map[string]string{
						core.SeccompContainerAnnotationKeyPrefix + "foo": "",
					},
				},
				Spec: validPodSpec(nil),
			},
		},
		"must match seccomp profile type and pod annotation": {
			expectedError: "seccomp type in annotation and field must match",
			spec: core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "123",
					Namespace: "ns",
					Annotations: map[string]string{
						core.SeccompPodAnnotationKey: "unconfined",
					},
				},
				Spec: core.PodSpec{
					SecurityContext: &core.PodSecurityContext{
						SeccompProfile: &core.SeccompProfile{
							Type: core.SeccompProfileTypeRuntimeDefault,
						},
					},
					Containers:    []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
					RestartPolicy: core.RestartPolicyAlways,
					DNSPolicy:     core.DNSClusterFirst,
				},
			},
		},
		"must match seccomp profile type and container annotation": {
			expectedError: "seccomp type in annotation and field must match",
			spec: core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "123",
					Namespace: "ns",
					Annotations: map[string]string{
						core.SeccompContainerAnnotationKeyPrefix + "ctr": "unconfined",
					},
				},
				Spec: core.PodSpec{
					Containers: []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File",
						SecurityContext: &core.SecurityContext{
							SeccompProfile: &core.SeccompProfile{
								Type: core.SeccompProfileTypeRuntimeDefault,
							},
						}}},
					RestartPolicy: core.RestartPolicyAlways,
					DNSPolicy:     core.DNSClusterFirst,
				},
			},
		},
		"must be a relative path in a node-local seccomp profile annotation": {
			expectedError: "must be a relative path",
			spec: core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "123",
					Namespace: "ns",
					Annotations: map[string]string{
						core.SeccompPodAnnotationKey: "localhost//foo",
					},
				},
				Spec: validPodSpec(nil),
			},
		},
		"must not start with '../'": {
			expectedError: "must not contain '..'",
			spec: core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "123",
					Namespace: "ns",
					Annotations: map[string]string{
						core.SeccompPodAnnotationKey: "localhost/../foo",
					},
				},
				Spec: validPodSpec(nil),
			},
		},
		"AppArmor profile must apply to a container": {
			expectedError: "metadata.annotations[container.apparmor.security.beta.kubernetes.io/fake-ctr]",
			spec: core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "123",
					Namespace: "ns",
					Annotations: map[string]string{
						v1.AppArmorBetaContainerAnnotationKeyPrefix + "ctr":      v1.AppArmorBetaProfileRuntimeDefault,
						v1.AppArmorBetaContainerAnnotationKeyPrefix + "init-ctr": v1.AppArmorBetaProfileRuntimeDefault,
						v1.AppArmorBetaContainerAnnotationKeyPrefix + "fake-ctr": v1.AppArmorBetaProfileRuntimeDefault,
					},
				},
				Spec: core.PodSpec{
					InitContainers: []core.Container{{Name: "init-ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
					Containers:     []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
					RestartPolicy:  core.RestartPolicyAlways,
					DNSPolicy:      core.DNSClusterFirst,
				},
			},
		},
		"AppArmor profile format must be valid": {
			expectedError: "invalid AppArmor profile name",
			spec: core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "123",
					Namespace: "ns",
					Annotations: map[string]string{
						v1.AppArmorBetaContainerAnnotationKeyPrefix + "ctr": "bad-name",
					},
				},
				Spec: validPodSpec(nil),
			},
		},
		"only default AppArmor profile may start with runtime/": {
			expectedError: "invalid AppArmor profile name",
			spec: core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "123",
					Namespace: "ns",
					Annotations: map[string]string{
						v1.AppArmorBetaContainerAnnotationKeyPrefix + "ctr": "runtime/foo",
					},
				},
				Spec: validPodSpec(nil),
			},
		},
		"invalid extended resource name in container request": {
			expectedError: "must be a standard resource for containers",
			spec: core.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "123", Namespace: "ns"},
				Spec: core.PodSpec{
					Containers: []core.Container{
						{
							Name:            "invalid",
							Image:           "image",
							ImagePullPolicy: "IfNotPresent",
							Resources: core.ResourceRequirements{
								Requests: core.ResourceList{
									core.ResourceName("invalid-name"): resource.MustParse("2"),
								},
								Limits: core.ResourceList{
									core.ResourceName("invalid-name"): resource.MustParse("2"),
								},
							},
						},
					},
					RestartPolicy: core.RestartPolicyAlways,
					DNSPolicy:     core.DNSClusterFirst,
				},
			},
		},
		"invalid extended resource requirement: request must be == limit": {
			expectedError: "must be equal to example.com/a",
			spec: core.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "123", Namespace: "ns"},
				Spec: core.PodSpec{
					Containers: []core.Container{
						{
							Name:            "invalid",
							Image:           "image",
							ImagePullPolicy: "IfNotPresent",
							Resources: core.ResourceRequirements{
								Requests: core.ResourceList{
									core.ResourceName("example.com/a"): resource.MustParse("2"),
								},
								Limits: core.ResourceList{
									core.ResourceName("example.com/a"): resource.MustParse("1"),
								},
							},
						},
					},
					RestartPolicy: core.RestartPolicyAlways,
					DNSPolicy:     core.DNSClusterFirst,
				},
			},
		},
		"invalid extended resource requirement without limit": {
			expectedError: "Limit must be set",
			spec: core.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "123", Namespace: "ns"},
				Spec: core.PodSpec{
					Containers: []core.Container{
						{
							Name:            "invalid",
							Image:           "image",
							ImagePullPolicy: "IfNotPresent",
							Resources: core.ResourceRequirements{
								Requests: core.ResourceList{
									core.ResourceName("example.com/a"): resource.MustParse("2"),
								},
							},
						},
					},
					RestartPolicy: core.RestartPolicyAlways,
					DNSPolicy:     core.DNSClusterFirst,
				},
			},
		},
		"invalid fractional extended resource in container request": {
			expectedError: "must be an integer",
			spec: core.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "123", Namespace: "ns"},
				Spec: core.PodSpec{
					Containers: []core.Container{
						{
							Name:            "invalid",
							Image:           "image",
							ImagePullPolicy: "IfNotPresent",
							Resources: core.ResourceRequirements{
								Requests: core.ResourceList{
									core.ResourceName("example.com/a"): resource.MustParse("500m"),
								},
							},
						},
					},
					RestartPolicy: core.RestartPolicyAlways,
					DNSPolicy:     core.DNSClusterFirst,
				},
			},
		},
		"invalid fractional extended resource in init container request": {
			expectedError: "must be an integer",
			spec: core.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "123", Namespace: "ns"},
				Spec: core.PodSpec{
					InitContainers: []core.Container{
						{
							Name:            "invalid",
							Image:           "image",
							ImagePullPolicy: "IfNotPresent",
							Resources: core.ResourceRequirements{
								Requests: core.ResourceList{
									core.ResourceName("example.com/a"): resource.MustParse("500m"),
								},
							},
						},
					},
					Containers:    []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
					RestartPolicy: core.RestartPolicyAlways,
					DNSPolicy:     core.DNSClusterFirst,
				},
			},
		},
		"invalid fractional extended resource in container limit": {
			expectedError: "must be an integer",
			spec: core.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "123", Namespace: "ns"},
				Spec: core.PodSpec{
					Containers: []core.Container{
						{
							Name:            "invalid",
							Image:           "image",
							ImagePullPolicy: "IfNotPresent",
							Resources: core.ResourceRequirements{
								Requests: core.ResourceList{
									core.ResourceName("example.com/a"): resource.MustParse("5"),
								},
								Limits: core.ResourceList{
									core.ResourceName("example.com/a"): resource.MustParse("2.5"),
								},
							},
						},
					},
					RestartPolicy: core.RestartPolicyAlways,
					DNSPolicy:     core.DNSClusterFirst,
				},
			},
		},
		"invalid fractional extended resource in init container limit": {
			expectedError: "must be an integer",
			spec: core.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "123", Namespace: "ns"},
				Spec: core.PodSpec{
					InitContainers: []core.Container{
						{
							Name:            "invalid",
							Image:           "image",
							ImagePullPolicy: "IfNotPresent",
							Resources: core.ResourceRequirements{
								Requests: core.ResourceList{
									core.ResourceName("example.com/a"): resource.MustParse("2.5"),
								},
								Limits: core.ResourceList{
									core.ResourceName("example.com/a"): resource.MustParse("2.5"),
								},
							},
						},
					},
					Containers:    []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
					RestartPolicy: core.RestartPolicyAlways,
					DNSPolicy:     core.DNSClusterFirst,
				},
			},
		},
		"mirror-pod present without nodeName": {
			expectedError: "mirror",
			spec: core.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "123", Namespace: "ns", Annotations: map[string]string{core.MirrorPodAnnotationKey: ""}},
				Spec: core.PodSpec{
					Containers:    []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
					RestartPolicy: core.RestartPolicyAlways,
					DNSPolicy:     core.DNSClusterFirst,
				},
			},
		},
		"mirror-pod populated without nodeName": {
			expectedError: "mirror",
			spec: core.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "123", Namespace: "ns", Annotations: map[string]string{core.MirrorPodAnnotationKey: "foo"}},
				Spec: core.PodSpec{
					Containers:    []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
					RestartPolicy: core.RestartPolicyAlways,
					DNSPolicy:     core.DNSClusterFirst,
				},
			},
		},
		"serviceaccount token projected volume with no serviceaccount name specified": {
			expectedError: "must not be specified when serviceAccountName is not set",
			spec: core.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "123", Namespace: "ns"},
				Spec: core.PodSpec{
					Containers:    []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
					RestartPolicy: core.RestartPolicyAlways,
					DNSPolicy:     core.DNSClusterFirst,
					Volumes: []core.Volume{
						{
							Name: "projected-volume",
							VolumeSource: core.VolumeSource{
								Projected: &core.ProjectedVolumeSource{
									Sources: []core.VolumeProjection{
										{
											ServiceAccountToken: &core.ServiceAccountTokenProjection{
												Audience:          "foo-audience",
												ExpirationSeconds: 6000,
												Path:              "foo-path",
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
		"final PVC name for ephemeral volume must be valid": {
			expectedError: "spec.volumes[1].name: Invalid value: \"" + longVolName + "\": PVC name \"" + longPodName + "-" + longVolName + "\": must be no more than 253 characters",
			spec: core.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: longPodName, Namespace: "ns"},
				Spec: core.PodSpec{
					Volumes: []core.Volume{
						{Name: "pvc", VolumeSource: core.VolumeSource{PersistentVolumeClaim: &core.PersistentVolumeClaimVolumeSource{ClaimName: "my-pvc"}}},
						{Name: longVolName, VolumeSource: core.VolumeSource{Ephemeral: &core.EphemeralVolumeSource{VolumeClaimTemplate: &validPVCTemplate}}},
					},
					Containers:    []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
					RestartPolicy: core.RestartPolicyAlways,
					DNSPolicy:     core.DNSClusterFirst,
				},
			},
		},
		"PersistentVolumeClaimVolumeSource must not reference a generated PVC": {
			expectedError: "spec.volumes[0].persistentVolumeClaim.claimName: Invalid value: \"123-ephemeral-volume\": must not reference a PVC that gets created for an ephemeral volume",
			spec: core.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "123", Namespace: "ns"},
				Spec: core.PodSpec{
					Volumes: []core.Volume{
						{Name: "pvc-volume", VolumeSource: core.VolumeSource{PersistentVolumeClaim: &core.PersistentVolumeClaimVolumeSource{ClaimName: "123-ephemeral-volume"}}},
						{Name: "ephemeral-volume", VolumeSource: core.VolumeSource{Ephemeral: &core.EphemeralVolumeSource{VolumeClaimTemplate: &validPVCTemplate}}},
					},
					Containers:    []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
					RestartPolicy: core.RestartPolicyAlways,
					DNSPolicy:     core.DNSClusterFirst,
				},
			},
		},
		"invalid pod-deletion-cost": {
			expectedError: "metadata.annotations[controller.kubernetes.io/pod-deletion-cost]: Invalid value: \"text\": must be a 32bit integer",
			spec: core.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "123", Namespace: "ns", Annotations: map[string]string{core.PodDeletionCost: "text"}},
				Spec: core.PodSpec{
					Containers:    []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
					RestartPolicy: core.RestartPolicyAlways,
					DNSPolicy:     core.DNSClusterFirst,
				},
			},
		},
		"invalid leading zeros pod-deletion-cost": {
			expectedError: "metadata.annotations[controller.kubernetes.io/pod-deletion-cost]: Invalid value: \"008\": must be a 32bit integer",
			spec: core.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "123", Namespace: "ns", Annotations: map[string]string{core.PodDeletionCost: "008"}},
				Spec: core.PodSpec{
					Containers:    []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
					RestartPolicy: core.RestartPolicyAlways,
					DNSPolicy:     core.DNSClusterFirst,
				},
			},
		},
		"invalid leading plus sign pod-deletion-cost": {
			expectedError: "metadata.annotations[controller.kubernetes.io/pod-deletion-cost]: Invalid value: \"+10\": must be a 32bit integer",
			spec: core.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "123", Namespace: "ns", Annotations: map[string]string{core.PodDeletionCost: "+10"}},
				Spec: core.PodSpec{
					Containers:    []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
					RestartPolicy: core.RestartPolicyAlways,
					DNSPolicy:     core.DNSClusterFirst,
				},
			},
		},
	}
	for k, v := range errorCases {
		t.Run(k, func(t *testing.T) {
			if errs := ValidatePodCreate(&v.spec, PodValidationOptions{AllowInvalidPodDeletionCost: false}); len(errs) == 0 {
				t.Errorf("expected failure")
			} else if v.expectedError == "" {
				t.Errorf("missing expectedError, got %q", errs.ToAggregate().Error())
			} else if actualError := errs.ToAggregate().Error(); !strings.Contains(actualError, v.expectedError) {
				t.Errorf("expected error to contain %q, got %q", v.expectedError, actualError)
			}
		})
	}
}

func TestValidatePodUpdate(t *testing.T) {
	var (
		activeDeadlineSecondsZero     = int64(0)
		activeDeadlineSecondsNegative = int64(-30)
		activeDeadlineSecondsPositive = int64(30)
		activeDeadlineSecondsLarger   = int64(31)
		validfsGroupChangePolicy      = core.FSGroupChangeOnRootMismatch

		now    = metav1.Now()
		grace  = int64(30)
		grace2 = int64(31)
	)

	tests := []struct {
		new  core.Pod
		old  core.Pod
		err  string
		test string
	}{
		{core.Pod{}, core.Pod{}, "", "nothing"},
		{
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "foo"},
			},
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "bar"},
			},
			"metadata.name",
			"ids",
		},
		{
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
					Labels: map[string]string{
						"foo": "bar",
					},
				},
			},
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
					Labels: map[string]string{
						"bar": "foo",
					},
				},
			},
			"",
			"labels",
		},
		{
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
					Annotations: map[string]string{
						"foo": "bar",
					},
				},
			},
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
					Annotations: map[string]string{
						"bar": "foo",
					},
				},
			},
			"",
			"annotations",
		},
		{
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: core.PodSpec{
					Containers: []core.Container{
						{
							Image: "foo:V1",
						},
					},
				},
			},
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "foo"},
				Spec: core.PodSpec{
					Containers: []core.Container{
						{
							Image: "foo:V2",
						},
						{
							Image: "bar:V2",
						},
					},
				},
			},
			"may not add or remove containers",
			"less containers",
		},
		{
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: core.PodSpec{
					Containers: []core.Container{
						{
							Image: "foo:V1",
						},
						{
							Image: "bar:V2",
						},
					},
				},
			},
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "foo"},
				Spec: core.PodSpec{
					Containers: []core.Container{
						{
							Image: "foo:V2",
						},
					},
				},
			},
			"may not add or remove containers",
			"more containers",
		},
		{
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: core.PodSpec{
					InitContainers: []core.Container{
						{
							Image: "foo:V1",
						},
					},
				},
			},
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "foo"},
				Spec: core.PodSpec{
					InitContainers: []core.Container{
						{
							Image: "foo:V2",
						},
						{
							Image: "bar:V2",
						},
					},
				},
			},
			"may not add or remove containers",
			"more init containers",
		},
		{
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "foo"},
				Spec:       core.PodSpec{Containers: []core.Container{{Image: "foo:V1"}}},
			},
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "foo", DeletionTimestamp: &now},
				Spec:       core.PodSpec{Containers: []core.Container{{Image: "foo:V1"}}},
			},
			"metadata.deletionTimestamp",
			"deletion timestamp removed",
		},
		{
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "foo", DeletionTimestamp: &now},
				Spec:       core.PodSpec{Containers: []core.Container{{Image: "foo:V1"}}},
			},
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "foo"},
				Spec:       core.PodSpec{Containers: []core.Container{{Image: "foo:V1"}}},
			},
			"metadata.deletionTimestamp",
			"deletion timestamp added",
		},
		{
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "foo", DeletionTimestamp: &now, DeletionGracePeriodSeconds: &grace},
				Spec:       core.PodSpec{Containers: []core.Container{{Image: "foo:V1"}}},
			},
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "foo", DeletionTimestamp: &now, DeletionGracePeriodSeconds: &grace2},
				Spec:       core.PodSpec{Containers: []core.Container{{Image: "foo:V1"}}},
			},
			"metadata.deletionGracePeriodSeconds",
			"deletion grace period seconds changed",
		},
		{
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "foo"},
				Spec: core.PodSpec{
					Containers: []core.Container{
						{
							Name:                     "container",
							Image:                    "foo:V1",
							TerminationMessagePolicy: "File",
							ImagePullPolicy:          "Always",
						},
					},
				},
			},
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "foo"},
				Spec: core.PodSpec{
					Containers: []core.Container{
						{
							Name:                     "container",
							Image:                    "foo:V2",
							TerminationMessagePolicy: "File",
							ImagePullPolicy:          "Always",
						},
					},
				},
			},
			"",
			"image change",
		},
		{
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "foo"},
				Spec: core.PodSpec{
					InitContainers: []core.Container{
						{
							Name:                     "container",
							Image:                    "foo:V1",
							TerminationMessagePolicy: "File",
							ImagePullPolicy:          "Always",
						},
					},
				},
			},
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "foo"},
				Spec: core.PodSpec{
					InitContainers: []core.Container{
						{
							Name:                     "container",
							Image:                    "foo:V2",
							TerminationMessagePolicy: "File",
							ImagePullPolicy:          "Always",
						},
					},
				},
			},
			"",
			"init container image change",
		},
		{
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "foo"},
				Spec: core.PodSpec{
					Containers: []core.Container{
						{
							Name:                     "container",
							TerminationMessagePolicy: "File",
							ImagePullPolicy:          "Always",
						},
					},
				},
			},
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "foo"},
				Spec: core.PodSpec{
					Containers: []core.Container{
						{
							Name:                     "container",
							Image:                    "foo:V2",
							TerminationMessagePolicy: "File",
							ImagePullPolicy:          "Always",
						},
					},
				},
			},
			"spec.containers[0].image",
			"image change to empty",
		},
		{
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "foo"},
				Spec: core.PodSpec{
					InitContainers: []core.Container{
						{
							Name:                     "container",
							TerminationMessagePolicy: "File",
							ImagePullPolicy:          "Always",
						},
					},
				},
			},
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "foo"},
				Spec: core.PodSpec{
					InitContainers: []core.Container{
						{
							Name:                     "container",
							Image:                    "foo:V2",
							TerminationMessagePolicy: "File",
							ImagePullPolicy:          "Always",
						},
					},
				},
			},
			"spec.initContainers[0].image",
			"init container image change to empty",
		},
		{
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "foo"},
				Spec: core.PodSpec{
					EphemeralContainers: []core.EphemeralContainer{
						{
							EphemeralContainerCommon: core.EphemeralContainerCommon{
								Name:  "ephemeral",
								Image: "busybox",
							},
						},
					},
				},
			},
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "foo"},
				Spec:       core.PodSpec{},
			},
			"Forbidden: pod updates may not change fields other than",
			"ephemeralContainer changes are not allowed via normal pod update",
		},
		{
			core.Pod{
				Spec: core.PodSpec{},
			},
			core.Pod{
				Spec: core.PodSpec{},
			},
			"",
			"activeDeadlineSeconds no change, nil",
		},
		{
			core.Pod{
				Spec: core.PodSpec{
					ActiveDeadlineSeconds: &activeDeadlineSecondsPositive,
				},
			},
			core.Pod{
				Spec: core.PodSpec{
					ActiveDeadlineSeconds: &activeDeadlineSecondsPositive,
				},
			},
			"",
			"activeDeadlineSeconds no change, set",
		},
		{
			core.Pod{
				Spec: core.PodSpec{
					ActiveDeadlineSeconds: &activeDeadlineSecondsPositive,
				},
			},
			core.Pod{},
			"",
			"activeDeadlineSeconds change to positive from nil",
		},
		{
			core.Pod{
				Spec: core.PodSpec{
					ActiveDeadlineSeconds: &activeDeadlineSecondsPositive,
				},
			},
			core.Pod{
				Spec: core.PodSpec{
					ActiveDeadlineSeconds: &activeDeadlineSecondsLarger,
				},
			},
			"",
			"activeDeadlineSeconds change to smaller positive",
		},
		{
			core.Pod{
				Spec: core.PodSpec{
					ActiveDeadlineSeconds: &activeDeadlineSecondsLarger,
				},
			},
			core.Pod{
				Spec: core.PodSpec{
					ActiveDeadlineSeconds: &activeDeadlineSecondsPositive,
				},
			},
			"spec.activeDeadlineSeconds",
			"activeDeadlineSeconds change to larger positive",
		},

		{
			core.Pod{
				Spec: core.PodSpec{
					ActiveDeadlineSeconds: &activeDeadlineSecondsNegative,
				},
			},
			core.Pod{},
			"spec.activeDeadlineSeconds",
			"activeDeadlineSeconds change to negative from nil",
		},
		{
			core.Pod{
				Spec: core.PodSpec{
					ActiveDeadlineSeconds: &activeDeadlineSecondsNegative,
				},
			},
			core.Pod{
				Spec: core.PodSpec{
					ActiveDeadlineSeconds: &activeDeadlineSecondsPositive,
				},
			},
			"spec.activeDeadlineSeconds",
			"activeDeadlineSeconds change to negative from positive",
		},
		{
			core.Pod{
				Spec: core.PodSpec{
					ActiveDeadlineSeconds: &activeDeadlineSecondsZero,
				},
			},
			core.Pod{
				Spec: core.PodSpec{
					ActiveDeadlineSeconds: &activeDeadlineSecondsPositive,
				},
			},
			"spec.activeDeadlineSeconds",
			"activeDeadlineSeconds change to zero from positive",
		},
		{
			core.Pod{
				Spec: core.PodSpec{
					ActiveDeadlineSeconds: &activeDeadlineSecondsZero,
				},
			},
			core.Pod{},
			"spec.activeDeadlineSeconds",
			"activeDeadlineSeconds change to zero from nil",
		},
		{
			core.Pod{},
			core.Pod{
				Spec: core.PodSpec{
					ActiveDeadlineSeconds: &activeDeadlineSecondsPositive,
				},
			},
			"spec.activeDeadlineSeconds",
			"activeDeadlineSeconds change to nil from positive",
		},
		{
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "foo"},
				Spec: core.PodSpec{
					Containers: []core.Container{
						{
							Image: "foo:V1",
							Resources: core.ResourceRequirements{
								Limits: getResourceLimits("100m", "0"),
							},
						},
					},
				},
			},
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "foo"},
				Spec: core.PodSpec{
					Containers: []core.Container{
						{
							Image: "foo:V2",
							Resources: core.ResourceRequirements{
								Limits: getResourceLimits("1000m", "0"),
							},
						},
					},
				},
			},
			"spec: Forbidden: pod updates may not change fields",
			"cpu change",
		},
		{
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "foo"},
				Spec: core.PodSpec{
					Containers: []core.Container{
						{
							Image: "foo:V1",
						},
					},
					SecurityContext: &core.PodSecurityContext{
						FSGroupChangePolicy: &validfsGroupChangePolicy,
					},
				},
			},
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "foo"},
				Spec: core.PodSpec{
					Containers: []core.Container{
						{
							Image: "foo:V2",
						},
					},
					SecurityContext: &core.PodSecurityContext{
						FSGroupChangePolicy: nil,
					},
				},
			},
			"spec: Forbidden: pod updates may not change fields",
			"fsGroupChangePolicy change",
		},
		{
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "foo"},
				Spec: core.PodSpec{
					Containers: []core.Container{
						{
							Image: "foo:V1",
							Ports: []core.ContainerPort{
								{HostPort: 8080, ContainerPort: 80},
							},
						},
					},
				},
			},
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "foo"},
				Spec: core.PodSpec{
					Containers: []core.Container{
						{
							Image: "foo:V2",
							Ports: []core.ContainerPort{
								{HostPort: 8000, ContainerPort: 80},
							},
						},
					},
				},
			},
			"spec: Forbidden: pod updates may not change fields",
			"port change",
		},
		{
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
					Labels: map[string]string{
						"foo": "bar",
					},
				},
			},
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
					Labels: map[string]string{
						"Bar": "foo",
					},
				},
			},
			"",
			"bad label change",
		},
		{
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: core.PodSpec{
					NodeName:    "node1",
					Tolerations: []core.Toleration{{Key: "key1", Value: "value2"}},
				},
			},
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: core.PodSpec{
					NodeName:    "node1",
					Tolerations: []core.Toleration{{Key: "key1", Value: "value1"}},
				},
			},
			"spec.tolerations: Forbidden",
			"existing toleration value modified in pod spec updates",
		},
		{
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: core.PodSpec{
					NodeName:    "node1",
					Tolerations: []core.Toleration{{Key: "key1", Value: "value2", Operator: "Equal", Effect: "NoExecute", TolerationSeconds: nil}},
				},
			},
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: core.PodSpec{
					NodeName:    "node1",
					Tolerations: []core.Toleration{{Key: "key1", Value: "value1", Operator: "Equal", Effect: "NoExecute", TolerationSeconds: &[]int64{10}[0]}},
				},
			},
			"spec.tolerations: Forbidden",
			"existing toleration value modified in pod spec updates with modified tolerationSeconds",
		},
		{
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: core.PodSpec{
					NodeName:    "node1",
					Tolerations: []core.Toleration{{Key: "key1", Value: "value1", Operator: "Equal", Effect: "NoExecute", TolerationSeconds: &[]int64{10}[0]}},
				},
			},
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: core.PodSpec{
					NodeName:    "node1",
					Tolerations: []core.Toleration{{Key: "key1", Value: "value1", Operator: "Equal", Effect: "NoExecute", TolerationSeconds: &[]int64{20}[0]}},
				}},
			"",
			"modified tolerationSeconds in existing toleration value in pod spec updates",
		},
		{
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: core.PodSpec{
					Tolerations: []core.Toleration{{Key: "key1", Value: "value2"}},
				},
			},
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: core.PodSpec{
					NodeName:    "",
					Tolerations: []core.Toleration{{Key: "key1", Value: "value1"}},
				},
			},
			"spec.tolerations: Forbidden",
			"toleration modified in updates to an unscheduled pod",
		},
		{
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: core.PodSpec{
					NodeName:    "node1",
					Tolerations: []core.Toleration{{Key: "key1", Value: "value1"}},
				},
			},
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: core.PodSpec{
					NodeName:    "node1",
					Tolerations: []core.Toleration{{Key: "key1", Value: "value1"}},
				},
			},
			"",
			"tolerations unmodified in updates to a scheduled pod",
		},
		{
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: core.PodSpec{
					NodeName: "node1",
					Tolerations: []core.Toleration{
						{Key: "key1", Value: "value1", Operator: "Equal", Effect: "NoExecute", TolerationSeconds: &[]int64{20}[0]},
						{Key: "key2", Value: "value2", Operator: "Equal", Effect: "NoExecute", TolerationSeconds: &[]int64{30}[0]},
					},
				}},
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: core.PodSpec{
					NodeName:    "node1",
					Tolerations: []core.Toleration{{Key: "key1", Value: "value1", Operator: "Equal", Effect: "NoExecute", TolerationSeconds: &[]int64{10}[0]}},
				},
			},
			"",
			"added valid new toleration to existing tolerations in pod spec updates",
		},
		{
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "foo"}, Spec: core.PodSpec{
					NodeName: "node1",
					Tolerations: []core.Toleration{
						{Key: "key1", Value: "value1", Operator: "Equal", Effect: "NoExecute", TolerationSeconds: &[]int64{20}[0]},
						{Key: "key2", Value: "value2", Operator: "Equal", Effect: "NoSchedule", TolerationSeconds: &[]int64{30}[0]},
					},
				}},
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: core.PodSpec{
					NodeName: "node1", Tolerations: []core.Toleration{{Key: "key1", Value: "value1", Operator: "Equal", Effect: "NoExecute", TolerationSeconds: &[]int64{10}[0]}},
				}},
			"spec.tolerations[1].effect",
			"added invalid new toleration to existing tolerations in pod spec updates",
		},
		{
			core.Pod{ObjectMeta: metav1.ObjectMeta{Name: "foo"}, Spec: core.PodSpec{NodeName: "foo"}},
			core.Pod{ObjectMeta: metav1.ObjectMeta{Name: "foo"}},
			"spec: Forbidden: pod updates may not change fields",
			"removed nodeName from pod spec",
		},
		{
			core.Pod{ObjectMeta: metav1.ObjectMeta{Name: "foo", Annotations: map[string]string{core.MirrorPodAnnotationKey: ""}}, Spec: core.PodSpec{NodeName: "foo"}},
			core.Pod{ObjectMeta: metav1.ObjectMeta{Name: "foo"}, Spec: core.PodSpec{NodeName: "foo"}},
			"metadata.annotations[kubernetes.io/config.mirror]",
			"added mirror pod annotation",
		},
		{
			core.Pod{ObjectMeta: metav1.ObjectMeta{Name: "foo"}, Spec: core.PodSpec{NodeName: "foo"}},
			core.Pod{ObjectMeta: metav1.ObjectMeta{Name: "foo", Annotations: map[string]string{core.MirrorPodAnnotationKey: ""}}, Spec: core.PodSpec{NodeName: "foo"}},
			"metadata.annotations[kubernetes.io/config.mirror]",
			"removed mirror pod annotation",
		},
		{
			core.Pod{ObjectMeta: metav1.ObjectMeta{Name: "foo", Annotations: map[string]string{core.MirrorPodAnnotationKey: "foo"}}, Spec: core.PodSpec{NodeName: "foo"}},
			core.Pod{ObjectMeta: metav1.ObjectMeta{Name: "foo", Annotations: map[string]string{core.MirrorPodAnnotationKey: "bar"}}, Spec: core.PodSpec{NodeName: "foo"}},
			"metadata.annotations[kubernetes.io/config.mirror]",
			"changed mirror pod annotation",
		},
		{
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: core.PodSpec{
					NodeName:          "node1",
					PriorityClassName: "bar-priority",
				},
			},
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: core.PodSpec{
					NodeName:          "node1",
					PriorityClassName: "foo-priority",
				},
			},
			"spec: Forbidden: pod updates",
			"changed priority class name",
		},
		{
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: core.PodSpec{
					NodeName:          "node1",
					PriorityClassName: "",
				},
			},
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: core.PodSpec{
					NodeName:          "node1",
					PriorityClassName: "foo-priority",
				},
			},
			"spec: Forbidden: pod updates",
			"removed priority class name",
		},
		{
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: core.PodSpec{
					TerminationGracePeriodSeconds: utilpointer.Int64Ptr(1),
				},
			},
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: core.PodSpec{
					TerminationGracePeriodSeconds: utilpointer.Int64Ptr(-1),
				},
			},
			"",
			"update termination grace period seconds",
		},
		{
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: core.PodSpec{
					TerminationGracePeriodSeconds: utilpointer.Int64Ptr(0),
				},
			},
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: core.PodSpec{
					TerminationGracePeriodSeconds: utilpointer.Int64Ptr(-1),
				},
			},
			"spec: Forbidden: pod updates",
			"update termination grace period seconds not 1",
		},
	}

	for _, test := range tests {
		test.new.ObjectMeta.ResourceVersion = "1"
		test.old.ObjectMeta.ResourceVersion = "1"

		// set required fields if old and new match and have no opinion on the value
		if test.new.Name == "" && test.old.Name == "" {
			test.new.Name = "name"
			test.old.Name = "name"
		}
		if test.new.Namespace == "" && test.old.Namespace == "" {
			test.new.Namespace = "namespace"
			test.old.Namespace = "namespace"
		}
		if test.new.Spec.Containers == nil && test.old.Spec.Containers == nil {
			test.new.Spec.Containers = []core.Container{{Name: "autoadded", Image: "image", TerminationMessagePolicy: "File", ImagePullPolicy: "Always"}}
			test.old.Spec.Containers = []core.Container{{Name: "autoadded", Image: "image", TerminationMessagePolicy: "File", ImagePullPolicy: "Always"}}
		}
		if len(test.new.Spec.DNSPolicy) == 0 && len(test.old.Spec.DNSPolicy) == 0 {
			test.new.Spec.DNSPolicy = core.DNSClusterFirst
			test.old.Spec.DNSPolicy = core.DNSClusterFirst
		}
		if len(test.new.Spec.RestartPolicy) == 0 && len(test.old.Spec.RestartPolicy) == 0 {
			test.new.Spec.RestartPolicy = "Always"
			test.old.Spec.RestartPolicy = "Always"
		}

		errs := ValidatePodUpdate(&test.new, &test.old, PodValidationOptions{})
		if test.err == "" {
			if len(errs) != 0 {
				t.Errorf("unexpected invalid: %s (%+v)\nA: %+v\nB: %+v", test.test, errs, test.new, test.old)
			}
		} else {
			if len(errs) == 0 {
				t.Errorf("unexpected valid: %s\nA: %+v\nB: %+v", test.test, test.new, test.old)
			} else if actualErr := errs.ToAggregate().Error(); !strings.Contains(actualErr, test.err) {
				t.Errorf("unexpected error message: %s\nExpected error: %s\nActual error: %s", test.test, test.err, actualErr)
			}
		}
	}
}

func TestValidatePodStatusUpdate(t *testing.T) {
	tests := []struct {
		new  core.Pod
		old  core.Pod
		err  string
		test string
	}{
		{
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: core.PodSpec{
					NodeName: "node1",
				},
				Status: core.PodStatus{
					NominatedNodeName: "node1",
				},
			},
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: core.PodSpec{
					NodeName: "node1",
				},
				Status: core.PodStatus{},
			},
			"",
			"removed nominatedNodeName",
		},
		{
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: core.PodSpec{
					NodeName: "node1",
				},
			},
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: core.PodSpec{
					NodeName: "node1",
				},
				Status: core.PodStatus{
					NominatedNodeName: "node1",
				},
			},
			"",
			"add valid nominatedNodeName",
		},
		{
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: core.PodSpec{
					NodeName: "node1",
				},
				Status: core.PodStatus{
					NominatedNodeName: "Node1",
				},
			},
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: core.PodSpec{
					NodeName: "node1",
				},
			},
			"nominatedNodeName",
			"Add invalid nominatedNodeName",
		},
		{
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: core.PodSpec{
					NodeName: "node1",
				},
				Status: core.PodStatus{
					NominatedNodeName: "node1",
				},
			},
			core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: core.PodSpec{
					NodeName: "node1",
				},
				Status: core.PodStatus{
					NominatedNodeName: "node2",
				},
			},
			"",
			"Update nominatedNodeName",
		},
	}

	for _, test := range tests {
		test.new.ObjectMeta.ResourceVersion = "1"
		test.old.ObjectMeta.ResourceVersion = "1"
		errs := ValidatePodStatusUpdate(&test.new, &test.old, PodValidationOptions{})
		if test.err == "" {
			if len(errs) != 0 {
				t.Errorf("unexpected invalid: %s (%+v)\nA: %+v\nB: %+v", test.test, errs, test.new, test.old)
			}
		} else {
			if len(errs) == 0 {
				t.Errorf("unexpected valid: %s\nA: %+v\nB: %+v", test.test, test.new, test.old)
			} else if actualErr := errs.ToAggregate().Error(); !strings.Contains(actualErr, test.err) {
				t.Errorf("unexpected error message: %s\nExpected error: %s\nActual error: %s", test.test, test.err, actualErr)
			}
		}
	}
}

func TestValidatePodEphemeralContainersUpdate(t *testing.T) {
	tests := []struct {
		new  []core.EphemeralContainer
		old  []core.EphemeralContainer
		err  string
		test string
	}{
		{[]core.EphemeralContainer{}, []core.EphemeralContainer{}, "", "nothing"},
		{
			[]core.EphemeralContainer{{
				EphemeralContainerCommon: core.EphemeralContainerCommon{
					Name:                     "debugger",
					Image:                    "busybox",
					ImagePullPolicy:          "IfNotPresent",
					TerminationMessagePolicy: "File",
				},
			}, {
				EphemeralContainerCommon: core.EphemeralContainerCommon{
					Name:                     "debugger2",
					Image:                    "busybox",
					ImagePullPolicy:          "IfNotPresent",
					TerminationMessagePolicy: "File",
				},
			}},
			[]core.EphemeralContainer{{
				EphemeralContainerCommon: core.EphemeralContainerCommon{
					Name:                     "debugger",
					Image:                    "busybox",
					ImagePullPolicy:          "IfNotPresent",
					TerminationMessagePolicy: "File",
				},
			}, {
				EphemeralContainerCommon: core.EphemeralContainerCommon{
					Name:                     "debugger2",
					Image:                    "busybox",
					ImagePullPolicy:          "IfNotPresent",
					TerminationMessagePolicy: "File",
				},
			}},
			"",
			"No change in Ephemeral Containers",
		},
		{
			[]core.EphemeralContainer{{
				EphemeralContainerCommon: core.EphemeralContainerCommon{
					Name:                     "debugger",
					Image:                    "busybox",
					ImagePullPolicy:          "IfNotPresent",
					TerminationMessagePolicy: "File",
				},
			}, {
				EphemeralContainerCommon: core.EphemeralContainerCommon{
					Name:                     "debugger2",
					Image:                    "busybox",
					ImagePullPolicy:          "IfNotPresent",
					TerminationMessagePolicy: "File",
				},
			}},
			[]core.EphemeralContainer{{
				EphemeralContainerCommon: core.EphemeralContainerCommon{
					Name:                     "debugger2",
					Image:                    "busybox",
					ImagePullPolicy:          "IfNotPresent",
					TerminationMessagePolicy: "File",
				},
			}, {
				EphemeralContainerCommon: core.EphemeralContainerCommon{
					Name:                     "debugger",
					Image:                    "busybox",
					ImagePullPolicy:          "IfNotPresent",
					TerminationMessagePolicy: "File",
				},
			}},
			"",
			"Ephemeral Container list order changes",
		},
		{
			[]core.EphemeralContainer{{
				EphemeralContainerCommon: core.EphemeralContainerCommon{
					Name:                     "debugger",
					Image:                    "busybox",
					ImagePullPolicy:          "IfNotPresent",
					TerminationMessagePolicy: "File",
				},
			}},
			[]core.EphemeralContainer{},
			"",
			"Add an Ephemeral Container",
		},
		{
			[]core.EphemeralContainer{{
				EphemeralContainerCommon: core.EphemeralContainerCommon{
					Name:                     "debugger1",
					Image:                    "busybox",
					ImagePullPolicy:          "IfNotPresent",
					TerminationMessagePolicy: "File",
				},
			}, {
				EphemeralContainerCommon: core.EphemeralContainerCommon{
					Name:                     "debugger2",
					Image:                    "busybox",
					ImagePullPolicy:          "IfNotPresent",
					TerminationMessagePolicy: "File",
				},
			}},
			[]core.EphemeralContainer{},
			"",
			"Add two Ephemeral Containers",
		},
		{
			[]core.EphemeralContainer{{
				EphemeralContainerCommon: core.EphemeralContainerCommon{
					Name:                     "debugger",
					Image:                    "busybox",
					ImagePullPolicy:          "IfNotPresent",
					TerminationMessagePolicy: "File",
				},
			}, {
				EphemeralContainerCommon: core.EphemeralContainerCommon{
					Name:                     "debugger2",
					Image:                    "busybox",
					ImagePullPolicy:          "IfNotPresent",
					TerminationMessagePolicy: "File",
				},
			}},
			[]core.EphemeralContainer{{
				EphemeralContainerCommon: core.EphemeralContainerCommon{
					Name:                     "debugger",
					Image:                    "busybox",
					ImagePullPolicy:          "IfNotPresent",
					TerminationMessagePolicy: "File",
				},
			}},
			"",
			"Add to an existing Ephemeral Containers",
		},
		{
			[]core.EphemeralContainer{{
				EphemeralContainerCommon: core.EphemeralContainerCommon{
					Name:                     "debugger3",
					Image:                    "busybox",
					ImagePullPolicy:          "IfNotPresent",
					TerminationMessagePolicy: "File",
				},
			}, {
				EphemeralContainerCommon: core.EphemeralContainerCommon{
					Name:                     "debugger2",
					Image:                    "busybox",
					ImagePullPolicy:          "IfNotPresent",
					TerminationMessagePolicy: "File",
				},
			}, {
				EphemeralContainerCommon: core.EphemeralContainerCommon{
					Name:                     "debugger",
					Image:                    "busybox",
					ImagePullPolicy:          "IfNotPresent",
					TerminationMessagePolicy: "File",
				},
			}},
			[]core.EphemeralContainer{{
				EphemeralContainerCommon: core.EphemeralContainerCommon{
					Name:                     "debugger",
					Image:                    "busybox",
					ImagePullPolicy:          "IfNotPresent",
					TerminationMessagePolicy: "File",
				},
			}, {
				EphemeralContainerCommon: core.EphemeralContainerCommon{
					Name:                     "debugger2",
					Image:                    "busybox",
					ImagePullPolicy:          "IfNotPresent",
					TerminationMessagePolicy: "File",
				},
			}},
			"",
			"Add to an existing Ephemeral Containers, list order changes",
		},
		{
			[]core.EphemeralContainer{},
			[]core.EphemeralContainer{{
				EphemeralContainerCommon: core.EphemeralContainerCommon{
					Name:                     "debugger",
					Image:                    "busybox",
					ImagePullPolicy:          "IfNotPresent",
					TerminationMessagePolicy: "File",
				},
			}},
			"may not be removed",
			"Remove an Ephemeral Container",
		},
		{
			[]core.EphemeralContainer{{
				EphemeralContainerCommon: core.EphemeralContainerCommon{
					Name:                     "firstone",
					Image:                    "busybox",
					ImagePullPolicy:          "IfNotPresent",
					TerminationMessagePolicy: "File",
				},
			}},
			[]core.EphemeralContainer{{
				EphemeralContainerCommon: core.EphemeralContainerCommon{
					Name:                     "thentheother",
					Image:                    "busybox",
					ImagePullPolicy:          "IfNotPresent",
					TerminationMessagePolicy: "File",
				},
			}},
			"may not be removed",
			"Replace an Ephemeral Container",
		},
		{
			[]core.EphemeralContainer{{
				EphemeralContainerCommon: core.EphemeralContainerCommon{
					Name:                     "debugger1",
					Image:                    "busybox",
					ImagePullPolicy:          "IfNotPresent",
					TerminationMessagePolicy: "File",
				},
			}, {
				EphemeralContainerCommon: core.EphemeralContainerCommon{
					Name:                     "debugger2",
					Image:                    "busybox",
					ImagePullPolicy:          "IfNotPresent",
					TerminationMessagePolicy: "File",
				},
			}},
			[]core.EphemeralContainer{{
				EphemeralContainerCommon: core.EphemeralContainerCommon{
					Name:                     "debugger1",
					Image:                    "debian",
					ImagePullPolicy:          "IfNotPresent",
					TerminationMessagePolicy: "File",
				},
			}, {
				EphemeralContainerCommon: core.EphemeralContainerCommon{
					Name:                     "debugger2",
					Image:                    "busybox",
					ImagePullPolicy:          "IfNotPresent",
					TerminationMessagePolicy: "File",
				},
			}},
			"may not be changed",
			"Change an Ephemeral Containers",
		},
	}

	for _, test := range tests {
		new := core.Pod{Spec: core.PodSpec{EphemeralContainers: test.new}}
		old := core.Pod{Spec: core.PodSpec{EphemeralContainers: test.old}}
		errs := ValidatePodEphemeralContainersUpdate(&new, &old, PodValidationOptions{})
		if test.err == "" {
			if len(errs) != 0 {
				t.Errorf("unexpected invalid: %s (%+v)\nA: %+v\nB: %+v", test.test, errs, test.new, test.old)
			}
		} else {
			if len(errs) == 0 {
				t.Errorf("unexpected valid: %s\nA: %+v\nB: %+v", test.test, test.new, test.old)
			} else if actualErr := errs.ToAggregate().Error(); !strings.Contains(actualErr, test.err) {
				t.Errorf("unexpected error message: %s\nExpected error: %s\nActual error: %s", test.test, test.err, actualErr)
			}
		}
	}
}

func TestValidateReplicationControllerStatus(t *testing.T) {
	tests := []struct {
		name string

		replicas             int32
		fullyLabeledReplicas int32
		readyReplicas        int32
		availableReplicas    int32
		observedGeneration   int64

		expectedErr bool
	}{
		{
			name:                 "valid status",
			replicas:             3,
			fullyLabeledReplicas: 3,
			readyReplicas:        2,
			availableReplicas:    1,
			observedGeneration:   2,
			expectedErr:          false,
		},
		{
			name:                 "invalid replicas",
			replicas:             -1,
			fullyLabeledReplicas: 3,
			readyReplicas:        2,
			availableReplicas:    1,
			observedGeneration:   2,
			expectedErr:          true,
		},
		{
			name:                 "invalid fullyLabeledReplicas",
			replicas:             3,
			fullyLabeledReplicas: -1,
			readyReplicas:        2,
			availableReplicas:    1,
			observedGeneration:   2,
			expectedErr:          true,
		},
		{
			name:                 "invalid readyReplicas",
			replicas:             3,
			fullyLabeledReplicas: 3,
			readyReplicas:        -1,
			availableReplicas:    1,
			observedGeneration:   2,
			expectedErr:          true,
		},
		{
			name:                 "invalid availableReplicas",
			replicas:             3,
			fullyLabeledReplicas: 3,
			readyReplicas:        3,
			availableReplicas:    -1,
			observedGeneration:   2,
			expectedErr:          true,
		},
		{
			name:                 "invalid observedGeneration",
			replicas:             3,
			fullyLabeledReplicas: 3,
			readyReplicas:        3,
			availableReplicas:    3,
			observedGeneration:   -1,
			expectedErr:          true,
		},
		{
			name:                 "fullyLabeledReplicas greater than replicas",
			replicas:             3,
			fullyLabeledReplicas: 4,
			readyReplicas:        3,
			availableReplicas:    3,
			observedGeneration:   1,
			expectedErr:          true,
		},
		{
			name:                 "readyReplicas greater than replicas",
			replicas:             3,
			fullyLabeledReplicas: 3,
			readyReplicas:        4,
			availableReplicas:    3,
			observedGeneration:   1,
			expectedErr:          true,
		},
		{
			name:                 "availableReplicas greater than replicas",
			replicas:             3,
			fullyLabeledReplicas: 3,
			readyReplicas:        3,
			availableReplicas:    4,
			observedGeneration:   1,
			expectedErr:          true,
		},
		{
			name:                 "availableReplicas greater than readyReplicas",
			replicas:             3,
			fullyLabeledReplicas: 3,
			readyReplicas:        2,
			availableReplicas:    3,
			observedGeneration:   1,
			expectedErr:          true,
		},
	}

	for _, test := range tests {
		status := core.ReplicationControllerStatus{
			Replicas:             test.replicas,
			FullyLabeledReplicas: test.fullyLabeledReplicas,
			ReadyReplicas:        test.readyReplicas,
			AvailableReplicas:    test.availableReplicas,
			ObservedGeneration:   test.observedGeneration,
		}

		if hasErr := len(ValidateReplicationControllerStatus(status, field.NewPath("status"))) > 0; hasErr != test.expectedErr {
			t.Errorf("%s: expected error: %t, got error: %t", test.name, test.expectedErr, hasErr)
		}
	}
}

func TestValidateReplicationControllerStatusUpdate(t *testing.T) {
	validSelector := map[string]string{"a": "b"}
	validPodTemplate := core.PodTemplate{
		Template: core.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: validSelector,
			},
			Spec: core.PodSpec{
				RestartPolicy: core.RestartPolicyAlways,
				DNSPolicy:     core.DNSClusterFirst,
				Containers:    []core.Container{{Name: "abc", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
			},
		},
	}
	type rcUpdateTest struct {
		old    core.ReplicationController
		update core.ReplicationController
	}
	successCases := []rcUpdateTest{
		{
			old: core.ReplicationController{
				ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault},
				Spec: core.ReplicationControllerSpec{
					Selector: validSelector,
					Template: &validPodTemplate.Template,
				},
				Status: core.ReplicationControllerStatus{
					Replicas: 2,
				},
			},
			update: core.ReplicationController{
				ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault},
				Spec: core.ReplicationControllerSpec{
					Replicas: 3,
					Selector: validSelector,
					Template: &validPodTemplate.Template,
				},
				Status: core.ReplicationControllerStatus{
					Replicas: 4,
				},
			},
		},
	}
	for _, successCase := range successCases {
		successCase.old.ObjectMeta.ResourceVersion = "1"
		successCase.update.ObjectMeta.ResourceVersion = "1"
		if errs := ValidateReplicationControllerStatusUpdate(&successCase.update, &successCase.old); len(errs) != 0 {
			t.Errorf("expected success: %v", errs)
		}
	}
	errorCases := map[string]rcUpdateTest{
		"negative replicas": {
			old: core.ReplicationController{
				ObjectMeta: metav1.ObjectMeta{Name: "", Namespace: metav1.NamespaceDefault},
				Spec: core.ReplicationControllerSpec{
					Selector: validSelector,
					Template: &validPodTemplate.Template,
				},
				Status: core.ReplicationControllerStatus{
					Replicas: 3,
				},
			},
			update: core.ReplicationController{
				ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault},
				Spec: core.ReplicationControllerSpec{
					Replicas: 2,
					Selector: validSelector,
					Template: &validPodTemplate.Template,
				},
				Status: core.ReplicationControllerStatus{
					Replicas: -3,
				},
			},
		},
	}
	for testName, errorCase := range errorCases {
		if errs := ValidateReplicationControllerStatusUpdate(&errorCase.update, &errorCase.old); len(errs) == 0 {
			t.Errorf("expected failure: %s", testName)
		}
	}

}

func TestValidateReplicationControllerUpdate(t *testing.T) {
	validSelector := map[string]string{"a": "b"}
	validPodTemplate := core.PodTemplate{
		Template: core.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: validSelector,
			},
			Spec: core.PodSpec{
				RestartPolicy: core.RestartPolicyAlways,
				DNSPolicy:     core.DNSClusterFirst,
				Containers:    []core.Container{{Name: "abc", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
			},
		},
	}
	readWriteVolumePodTemplate := core.PodTemplate{
		Template: core.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: validSelector,
			},
			Spec: core.PodSpec{
				RestartPolicy: core.RestartPolicyAlways,
				DNSPolicy:     core.DNSClusterFirst,
				Containers:    []core.Container{{Name: "abc", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
				Volumes:       []core.Volume{{Name: "gcepd", VolumeSource: core.VolumeSource{GCEPersistentDisk: &core.GCEPersistentDiskVolumeSource{PDName: "my-PD", FSType: "ext4", Partition: 1, ReadOnly: false}}}},
			},
		},
	}
	invalidSelector := map[string]string{"NoUppercaseOrSpecialCharsLike=Equals": "b"}
	invalidPodTemplate := core.PodTemplate{
		Template: core.PodTemplateSpec{
			Spec: core.PodSpec{
				RestartPolicy: core.RestartPolicyAlways,
				DNSPolicy:     core.DNSClusterFirst,
			},
			ObjectMeta: metav1.ObjectMeta{
				Labels: invalidSelector,
			},
		},
	}
	type rcUpdateTest struct {
		old    core.ReplicationController
		update core.ReplicationController
	}
	successCases := []rcUpdateTest{
		{
			old: core.ReplicationController{
				ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault},
				Spec: core.ReplicationControllerSpec{
					Selector: validSelector,
					Template: &validPodTemplate.Template,
				},
			},
			update: core.ReplicationController{
				ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault},
				Spec: core.ReplicationControllerSpec{
					Replicas: 3,
					Selector: validSelector,
					Template: &validPodTemplate.Template,
				},
			},
		},
		{
			old: core.ReplicationController{
				ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault},
				Spec: core.ReplicationControllerSpec{
					Selector: validSelector,
					Template: &validPodTemplate.Template,
				},
			},
			update: core.ReplicationController{
				ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault},
				Spec: core.ReplicationControllerSpec{
					Replicas: 1,
					Selector: validSelector,
					Template: &readWriteVolumePodTemplate.Template,
				},
			},
		},
	}
	for _, successCase := range successCases {
		successCase.old.ObjectMeta.ResourceVersion = "1"
		successCase.update.ObjectMeta.ResourceVersion = "1"
		if errs := ValidateReplicationControllerUpdate(&successCase.update, &successCase.old, PodValidationOptions{}); len(errs) != 0 {
			t.Errorf("expected success: %v", errs)
		}
	}
	errorCases := map[string]rcUpdateTest{
		"more than one read/write": {
			old: core.ReplicationController{
				ObjectMeta: metav1.ObjectMeta{Name: "", Namespace: metav1.NamespaceDefault},
				Spec: core.ReplicationControllerSpec{
					Selector: validSelector,
					Template: &validPodTemplate.Template,
				},
			},
			update: core.ReplicationController{
				ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault},
				Spec: core.ReplicationControllerSpec{
					Replicas: 2,
					Selector: validSelector,
					Template: &readWriteVolumePodTemplate.Template,
				},
			},
		},
		"invalid selector": {
			old: core.ReplicationController{
				ObjectMeta: metav1.ObjectMeta{Name: "", Namespace: metav1.NamespaceDefault},
				Spec: core.ReplicationControllerSpec{
					Selector: validSelector,
					Template: &validPodTemplate.Template,
				},
			},
			update: core.ReplicationController{
				ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault},
				Spec: core.ReplicationControllerSpec{
					Replicas: 2,
					Selector: invalidSelector,
					Template: &validPodTemplate.Template,
				},
			},
		},
		"invalid pod": {
			old: core.ReplicationController{
				ObjectMeta: metav1.ObjectMeta{Name: "", Namespace: metav1.NamespaceDefault},
				Spec: core.ReplicationControllerSpec{
					Selector: validSelector,
					Template: &validPodTemplate.Template,
				},
			},
			update: core.ReplicationController{
				ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault},
				Spec: core.ReplicationControllerSpec{
					Replicas: 2,
					Selector: validSelector,
					Template: &invalidPodTemplate.Template,
				},
			},
		},
		"negative replicas": {
			old: core.ReplicationController{
				ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault},
				Spec: core.ReplicationControllerSpec{
					Selector: validSelector,
					Template: &validPodTemplate.Template,
				},
			},
			update: core.ReplicationController{
				ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault},
				Spec: core.ReplicationControllerSpec{
					Replicas: -1,
					Selector: validSelector,
					Template: &validPodTemplate.Template,
				},
			},
		},
	}
	for testName, errorCase := range errorCases {
		if errs := ValidateReplicationControllerUpdate(&errorCase.update, &errorCase.old, PodValidationOptions{}); len(errs) == 0 {
			t.Errorf("expected failure: %s", testName)
		}
	}
}

func TestValidateReplicationController(t *testing.T) {
	validSelector := map[string]string{"a": "b"}
	validPodTemplate := core.PodTemplate{
		Template: core.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: validSelector,
			},
			Spec: core.PodSpec{
				RestartPolicy: core.RestartPolicyAlways,
				DNSPolicy:     core.DNSClusterFirst,
				Containers:    []core.Container{{Name: "abc", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
			},
		},
	}
	readWriteVolumePodTemplate := core.PodTemplate{
		Template: core.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: validSelector,
			},
			Spec: core.PodSpec{
				Volumes:       []core.Volume{{Name: "gcepd", VolumeSource: core.VolumeSource{GCEPersistentDisk: &core.GCEPersistentDiskVolumeSource{PDName: "my-PD", FSType: "ext4", Partition: 1, ReadOnly: false}}}},
				RestartPolicy: core.RestartPolicyAlways,
				DNSPolicy:     core.DNSClusterFirst,
				Containers:    []core.Container{{Name: "abc", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
			},
		},
	}
	invalidSelector := map[string]string{"NoUppercaseOrSpecialCharsLike=Equals": "b"}
	invalidPodTemplate := core.PodTemplate{
		Template: core.PodTemplateSpec{
			Spec: core.PodSpec{
				RestartPolicy: core.RestartPolicyAlways,
				DNSPolicy:     core.DNSClusterFirst,
			},
			ObjectMeta: metav1.ObjectMeta{
				Labels: invalidSelector,
			},
		},
	}
	successCases := []core.ReplicationController{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault},
			Spec: core.ReplicationControllerSpec{
				Selector: validSelector,
				Template: &validPodTemplate.Template,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "abc-123", Namespace: metav1.NamespaceDefault},
			Spec: core.ReplicationControllerSpec{
				Selector: validSelector,
				Template: &validPodTemplate.Template,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "abc-123", Namespace: metav1.NamespaceDefault},
			Spec: core.ReplicationControllerSpec{
				Replicas: 1,
				Selector: validSelector,
				Template: &readWriteVolumePodTemplate.Template,
			},
		},
	}
	for _, successCase := range successCases {
		if errs := ValidateReplicationController(&successCase, PodValidationOptions{}); len(errs) != 0 {
			t.Errorf("expected success: %v", errs)
		}
	}

	errorCases := map[string]core.ReplicationController{
		"zero-length ID": {
			ObjectMeta: metav1.ObjectMeta{Name: "", Namespace: metav1.NamespaceDefault},
			Spec: core.ReplicationControllerSpec{
				Selector: validSelector,
				Template: &validPodTemplate.Template,
			},
		},
		"missing-namespace": {
			ObjectMeta: metav1.ObjectMeta{Name: "abc-123"},
			Spec: core.ReplicationControllerSpec{
				Selector: validSelector,
				Template: &validPodTemplate.Template,
			},
		},
		"empty selector": {
			ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault},
			Spec: core.ReplicationControllerSpec{
				Template: &validPodTemplate.Template,
			},
		},
		"selector_doesnt_match": {
			ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault},
			Spec: core.ReplicationControllerSpec{
				Selector: map[string]string{"foo": "bar"},
				Template: &validPodTemplate.Template,
			},
		},
		"invalid manifest": {
			ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault},
			Spec: core.ReplicationControllerSpec{
				Selector: validSelector,
			},
		},
		"read-write persistent disk with > 1 pod": {
			ObjectMeta: metav1.ObjectMeta{Name: "abc"},
			Spec: core.ReplicationControllerSpec{
				Replicas: 2,
				Selector: validSelector,
				Template: &readWriteVolumePodTemplate.Template,
			},
		},
		"negative_replicas": {
			ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault},
			Spec: core.ReplicationControllerSpec{
				Replicas: -1,
				Selector: validSelector,
			},
		},
		"invalid_label": {
			ObjectMeta: metav1.ObjectMeta{
				Name:      "abc-123",
				Namespace: metav1.NamespaceDefault,
				Labels: map[string]string{
					"NoUppercaseOrSpecialCharsLike=Equals": "bar",
				},
			},
			Spec: core.ReplicationControllerSpec{
				Selector: validSelector,
				Template: &validPodTemplate.Template,
			},
		},
		"invalid_label 2": {
			ObjectMeta: metav1.ObjectMeta{
				Name:      "abc-123",
				Namespace: metav1.NamespaceDefault,
				Labels: map[string]string{
					"NoUppercaseOrSpecialCharsLike=Equals": "bar",
				},
			},
			Spec: core.ReplicationControllerSpec{
				Template: &invalidPodTemplate.Template,
			},
		},
		"invalid_annotation": {
			ObjectMeta: metav1.ObjectMeta{
				Name:      "abc-123",
				Namespace: metav1.NamespaceDefault,
				Annotations: map[string]string{
					"NoUppercaseOrSpecialCharsLike=Equals": "bar",
				},
			},
			Spec: core.ReplicationControllerSpec{
				Selector: validSelector,
				Template: &validPodTemplate.Template,
			},
		},
		"invalid restart policy 1": {
			ObjectMeta: metav1.ObjectMeta{
				Name:      "abc-123",
				Namespace: metav1.NamespaceDefault,
			},
			Spec: core.ReplicationControllerSpec{
				Selector: validSelector,
				Template: &core.PodTemplateSpec{
					Spec: core.PodSpec{
						RestartPolicy: core.RestartPolicyOnFailure,
						DNSPolicy:     core.DNSClusterFirst,
						Containers:    []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
					},
					ObjectMeta: metav1.ObjectMeta{
						Labels: validSelector,
					},
				},
			},
		},
		"invalid restart policy 2": {
			ObjectMeta: metav1.ObjectMeta{
				Name:      "abc-123",
				Namespace: metav1.NamespaceDefault,
			},
			Spec: core.ReplicationControllerSpec{
				Selector: validSelector,
				Template: &core.PodTemplateSpec{
					Spec: core.PodSpec{
						RestartPolicy: core.RestartPolicyNever,
						DNSPolicy:     core.DNSClusterFirst,
						Containers:    []core.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
					},
					ObjectMeta: metav1.ObjectMeta{
						Labels: validSelector,
					},
				},
			},
		},
		"template may not contain ephemeral containers": {
			ObjectMeta: metav1.ObjectMeta{Name: "abc-123", Namespace: metav1.NamespaceDefault},
			Spec: core.ReplicationControllerSpec{
				Replicas: 1,
				Selector: validSelector,
				Template: &core.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: validSelector,
					},
					Spec: core.PodSpec{
						RestartPolicy:       core.RestartPolicyAlways,
						DNSPolicy:           core.DNSClusterFirst,
						Containers:          []core.Container{{Name: "abc", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
						EphemeralContainers: []core.EphemeralContainer{{EphemeralContainerCommon: core.EphemeralContainerCommon{Name: "debug", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}}},
					},
				},
			},
		},
	}
	for k, v := range errorCases {
		errs := ValidateReplicationController(&v, PodValidationOptions{})
		if len(errs) == 0 {
			t.Errorf("expected failure for %s", k)
		}
		for i := range errs {
			field := errs[i].Field
			if !strings.HasPrefix(field, "spec.template.") &&
				field != "metadata.name" &&
				field != "metadata.namespace" &&
				field != "spec.selector" &&
				field != "spec.template" &&
				field != "GCEPersistentDisk.ReadOnly" &&
				field != "spec.replicas" &&
				field != "spec.template.labels" &&
				field != "metadata.annotations" &&
				field != "metadata.labels" &&
				field != "status.replicas" {
				t.Errorf("%s: missing prefix for: %v", k, errs[i])
			}
		}
	}
}

func TestValidateResourceNames(t *testing.T) {
	table := []struct {
		input   string
		success bool
		expect  string
	}{
		{"memory", true, ""},
		{"cpu", true, ""},
		{"storage", true, ""},
		{"requests.cpu", true, ""},
		{"requests.memory", true, ""},
		{"requests.storage", true, ""},
		{"limits.cpu", true, ""},
		{"limits.memory", true, ""},
		{"network", false, ""},
		{"disk", false, ""},
		{"", false, ""},
		{".", false, ""},
		{"..", false, ""},
		{"my.favorite.app.co/12345", true, ""},
		{"my.favorite.app.co/_12345", false, ""},
		{"my.favorite.app.co/12345_", false, ""},
		{"kubernetes.io/..", false, ""},
		{"kubernetes.io/" + strings.Repeat("a", 63), true, ""},
		{"kubernetes.io/" + strings.Repeat("a", 64), false, ""},
		{"kubernetes.io//", false, ""},
		{"kubernetes.io", false, ""},
		{"kubernetes.io/will/not/work/", false, ""},
	}
	for k, item := range table {
		err := validateResourceName(item.input, field.NewPath("field"))
		if len(err) != 0 && item.success {
			t.Errorf("expected no failure for input %q", item.input)
		} else if len(err) == 0 && !item.success {
			t.Errorf("expected failure for input %q", item.input)
			for i := range err {
				detail := err[i].Detail
				if detail != "" && !strings.Contains(detail, item.expect) {
					t.Errorf("%d: expected error detail either empty or %s, got %s", k, item.expect, detail)
				}
			}
		}
	}
}

func getResourceList(cpu, memory string) core.ResourceList {
	res := core.ResourceList{}
	if cpu != "" {
		res[core.ResourceCPU] = resource.MustParse(cpu)
	}
	if memory != "" {
		res[core.ResourceMemory] = resource.MustParse(memory)
	}
	return res
}

func getStorageResourceList(storage string) core.ResourceList {
	res := core.ResourceList{}
	if storage != "" {
		res[core.ResourceStorage] = resource.MustParse(storage)
	}
	return res
}

func getLocalStorageResourceList(ephemeralStorage string) core.ResourceList {
	res := core.ResourceList{}
	if ephemeralStorage != "" {
		res[core.ResourceEphemeralStorage] = resource.MustParse(ephemeralStorage)
	}
	return res
}

func TestValidateLimitRangeForLocalStorage(t *testing.T) {
	testCases := []struct {
		name string
		spec core.LimitRangeSpec
	}{
		{
			name: "all-fields-valid",
			spec: core.LimitRangeSpec{
				Limits: []core.LimitRangeItem{
					{
						Type:                 core.LimitTypePod,
						Max:                  getLocalStorageResourceList("10000Mi"),
						Min:                  getLocalStorageResourceList("100Mi"),
						MaxLimitRequestRatio: getLocalStorageResourceList(""),
					},
					{
						Type:                 core.LimitTypeContainer,
						Max:                  getLocalStorageResourceList("10000Mi"),
						Min:                  getLocalStorageResourceList("100Mi"),
						Default:              getLocalStorageResourceList("500Mi"),
						DefaultRequest:       getLocalStorageResourceList("200Mi"),
						MaxLimitRequestRatio: getLocalStorageResourceList(""),
					},
				},
			},
		},
	}

	for _, testCase := range testCases {
		limitRange := &core.LimitRange{ObjectMeta: metav1.ObjectMeta{Name: testCase.name, Namespace: "foo"}, Spec: testCase.spec}
		if errs := ValidateLimitRange(limitRange); len(errs) != 0 {
			t.Errorf("Case %v, unexpected error: %v", testCase.name, errs)
		}
	}
}

func TestValidateLimitRange(t *testing.T) {
	successCases := []struct {
		name string
		spec core.LimitRangeSpec
	}{
		{
			name: "all-fields-valid",
			spec: core.LimitRangeSpec{
				Limits: []core.LimitRangeItem{
					{
						Type:                 core.LimitTypePod,
						Max:                  getResourceList("100m", "10000Mi"),
						Min:                  getResourceList("5m", "100Mi"),
						MaxLimitRequestRatio: getResourceList("10", ""),
					},
					{
						Type:                 core.LimitTypeContainer,
						Max:                  getResourceList("100m", "10000Mi"),
						Min:                  getResourceList("5m", "100Mi"),
						Default:              getResourceList("50m", "500Mi"),
						DefaultRequest:       getResourceList("10m", "200Mi"),
						MaxLimitRequestRatio: getResourceList("10", ""),
					},
					{
						Type: core.LimitTypePersistentVolumeClaim,
						Max:  getStorageResourceList("10Gi"),
						Min:  getStorageResourceList("5Gi"),
					},
				},
			},
		},
		{
			name: "pvc-min-only",
			spec: core.LimitRangeSpec{
				Limits: []core.LimitRangeItem{
					{
						Type: core.LimitTypePersistentVolumeClaim,
						Min:  getStorageResourceList("5Gi"),
					},
				},
			},
		},
		{
			name: "pvc-max-only",
			spec: core.LimitRangeSpec{
				Limits: []core.LimitRangeItem{
					{
						Type: core.LimitTypePersistentVolumeClaim,
						Max:  getStorageResourceList("10Gi"),
					},
				},
			},
		},
		{
			name: "all-fields-valid-big-numbers",
			spec: core.LimitRangeSpec{
				Limits: []core.LimitRangeItem{
					{
						Type:                 core.LimitTypeContainer,
						Max:                  getResourceList("100m", "10000T"),
						Min:                  getResourceList("5m", "100Mi"),
						Default:              getResourceList("50m", "500Mi"),
						DefaultRequest:       getResourceList("10m", "200Mi"),
						MaxLimitRequestRatio: getResourceList("10", ""),
					},
				},
			},
		},
		{
			name: "thirdparty-fields-all-valid-standard-container-resources",
			spec: core.LimitRangeSpec{
				Limits: []core.LimitRangeItem{
					{
						Type:                 "thirdparty.com/foo",
						Max:                  getResourceList("100m", "10000T"),
						Min:                  getResourceList("5m", "100Mi"),
						Default:              getResourceList("50m", "500Mi"),
						DefaultRequest:       getResourceList("10m", "200Mi"),
						MaxLimitRequestRatio: getResourceList("10", ""),
					},
				},
			},
		},
		{
			name: "thirdparty-fields-all-valid-storage-resources",
			spec: core.LimitRangeSpec{
				Limits: []core.LimitRangeItem{
					{
						Type:                 "thirdparty.com/foo",
						Max:                  getStorageResourceList("10000T"),
						Min:                  getStorageResourceList("100Mi"),
						Default:              getStorageResourceList("500Mi"),
						DefaultRequest:       getStorageResourceList("200Mi"),
						MaxLimitRequestRatio: getStorageResourceList(""),
					},
				},
			},
		},
	}

	for _, successCase := range successCases {
		limitRange := &core.LimitRange{ObjectMeta: metav1.ObjectMeta{Name: successCase.name, Namespace: "foo"}, Spec: successCase.spec}
		if errs := ValidateLimitRange(limitRange); len(errs) != 0 {
			t.Errorf("Case %v, unexpected error: %v", successCase.name, errs)
		}
	}

	errorCases := map[string]struct {
		R core.LimitRange
		D string
	}{
		"zero-length-name": {
			core.LimitRange{ObjectMeta: metav1.ObjectMeta{Name: "", Namespace: "foo"}, Spec: core.LimitRangeSpec{}},
			"name or generateName is required",
		},
		"zero-length-namespace": {
			core.LimitRange{ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: ""}, Spec: core.LimitRangeSpec{}},
			"",
		},
		"invalid-name": {
			core.LimitRange{ObjectMeta: metav1.ObjectMeta{Name: "^Invalid", Namespace: "foo"}, Spec: core.LimitRangeSpec{}},
			dnsSubdomainLabelErrMsg,
		},
		"invalid-namespace": {
			core.LimitRange{ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: "^Invalid"}, Spec: core.LimitRangeSpec{}},
			dnsLabelErrMsg,
		},
		"duplicate-limit-type": {
			core.LimitRange{ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: "foo"}, Spec: core.LimitRangeSpec{
				Limits: []core.LimitRangeItem{
					{
						Type: core.LimitTypePod,
						Max:  getResourceList("100m", "10000m"),
						Min:  getResourceList("0m", "100m"),
					},
					{
						Type: core.LimitTypePod,
						Min:  getResourceList("0m", "100m"),
					},
				},
			}},
			"",
		},
		"default-limit-type-pod": {
			core.LimitRange{ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: "foo"}, Spec: core.LimitRangeSpec{
				Limits: []core.LimitRangeItem{
					{
						Type:    core.LimitTypePod,
						Max:     getResourceList("100m", "10000m"),
						Min:     getResourceList("0m", "100m"),
						Default: getResourceList("10m", "100m"),
					},
				},
			}},
			"may not be specified when `type` is 'Pod'",
		},
		"default-request-limit-type-pod": {
			core.LimitRange{ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: "foo"}, Spec: core.LimitRangeSpec{
				Limits: []core.LimitRangeItem{
					{
						Type:           core.LimitTypePod,
						Max:            getResourceList("100m", "10000m"),
						Min:            getResourceList("0m", "100m"),
						DefaultRequest: getResourceList("10m", "100m"),
					},
				},
			}},
			"may not be specified when `type` is 'Pod'",
		},
		"min value 100m is greater than max value 10m": {
			core.LimitRange{ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: "foo"}, Spec: core.LimitRangeSpec{
				Limits: []core.LimitRangeItem{
					{
						Type: core.LimitTypePod,
						Max:  getResourceList("10m", ""),
						Min:  getResourceList("100m", ""),
					},
				},
			}},
			"min value 100m is greater than max value 10m",
		},
		"invalid spec default outside range": {
			core.LimitRange{ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: "foo"}, Spec: core.LimitRangeSpec{
				Limits: []core.LimitRangeItem{
					{
						Type:    core.LimitTypeContainer,
						Max:     getResourceList("1", ""),
						Min:     getResourceList("100m", ""),
						Default: getResourceList("2000m", ""),
					},
				},
			}},
			"default value 2 is greater than max value 1",
		},
		"invalid spec default request outside range": {
			core.LimitRange{ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: "foo"}, Spec: core.LimitRangeSpec{
				Limits: []core.LimitRangeItem{
					{
						Type:           core.LimitTypeContainer,
						Max:            getResourceList("1", ""),
						Min:            getResourceList("100m", ""),
						DefaultRequest: getResourceList("2000m", ""),
					},
				},
			}},
			"default request value 2 is greater than max value 1",
		},
		"invalid spec default request more than default": {
			core.LimitRange{ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: "foo"}, Spec: core.LimitRangeSpec{
				Limits: []core.LimitRangeItem{
					{
						Type:           core.LimitTypeContainer,
						Max:            getResourceList("2", ""),
						Min:            getResourceList("100m", ""),
						Default:        getResourceList("500m", ""),
						DefaultRequest: getResourceList("800m", ""),
					},
				},
			}},
			"default request value 800m is greater than default limit value 500m",
		},
		"invalid spec maxLimitRequestRatio less than 1": {
			core.LimitRange{ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: "foo"}, Spec: core.LimitRangeSpec{
				Limits: []core.LimitRangeItem{
					{
						Type:                 core.LimitTypePod,
						MaxLimitRequestRatio: getResourceList("800m", ""),
					},
				},
			}},
			"ratio 800m is less than 1",
		},
		"invalid spec maxLimitRequestRatio greater than max/min": {
			core.LimitRange{ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: "foo"}, Spec: core.LimitRangeSpec{
				Limits: []core.LimitRangeItem{
					{
						Type:                 core.LimitTypeContainer,
						Max:                  getResourceList("", "2Gi"),
						Min:                  getResourceList("", "512Mi"),
						MaxLimitRequestRatio: getResourceList("", "10"),
					},
				},
			}},
			"ratio 10 is greater than max/min = 4.000000",
		},
		"invalid non standard limit type": {
			core.LimitRange{ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: "foo"}, Spec: core.LimitRangeSpec{
				Limits: []core.LimitRangeItem{
					{
						Type:                 "foo",
						Max:                  getStorageResourceList("10000T"),
						Min:                  getStorageResourceList("100Mi"),
						Default:              getStorageResourceList("500Mi"),
						DefaultRequest:       getStorageResourceList("200Mi"),
						MaxLimitRequestRatio: getStorageResourceList(""),
					},
				},
			}},
			"must be a standard limit type or fully qualified",
		},
		"min and max values missing, one required": {
			core.LimitRange{ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: "foo"}, Spec: core.LimitRangeSpec{
				Limits: []core.LimitRangeItem{
					{
						Type: core.LimitTypePersistentVolumeClaim,
					},
				},
			}},
			"either minimum or maximum storage value is required, but neither was provided",
		},
		"invalid min greater than max": {
			core.LimitRange{ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: "foo"}, Spec: core.LimitRangeSpec{
				Limits: []core.LimitRangeItem{
					{
						Type: core.LimitTypePersistentVolumeClaim,
						Min:  getStorageResourceList("10Gi"),
						Max:  getStorageResourceList("1Gi"),
					},
				},
			}},
			"min value 10Gi is greater than max value 1Gi",
		},
	}

	for k, v := range errorCases {
		errs := ValidateLimitRange(&v.R)
		if len(errs) == 0 {
			t.Errorf("expected failure for %s", k)
		}
		for i := range errs {
			detail := errs[i].Detail
			if !strings.Contains(detail, v.D) {
				t.Errorf("[%s]: expected error detail either empty or %q, got %q", k, v.D, detail)
			}
		}
	}

}

func TestValidatePersistentVolumeClaimStatusUpdate(t *testing.T) {
	validClaim := testVolumeClaim("foo", "ns", core.PersistentVolumeClaimSpec{
		AccessModes: []core.PersistentVolumeAccessMode{
			core.ReadWriteOnce,
			core.ReadOnlyMany,
		},
		Resources: core.ResourceRequirements{
			Requests: core.ResourceList{
				core.ResourceName(core.ResourceStorage): resource.MustParse("10G"),
			},
		},
	})
	validConditionUpdate := testVolumeClaimWithStatus("foo", "ns", core.PersistentVolumeClaimSpec{
		AccessModes: []core.PersistentVolumeAccessMode{
			core.ReadWriteOnce,
			core.ReadOnlyMany,
		},
		Resources: core.ResourceRequirements{
			Requests: core.ResourceList{
				core.ResourceName(core.ResourceStorage): resource.MustParse("10G"),
			},
		},
	}, core.PersistentVolumeClaimStatus{
		Phase: core.ClaimPending,
		Conditions: []core.PersistentVolumeClaimCondition{
			{Type: core.PersistentVolumeClaimResizing, Status: core.ConditionTrue},
		},
	})
	scenarios := map[string]struct {
		isExpectedFailure bool
		oldClaim          *core.PersistentVolumeClaim
		newClaim          *core.PersistentVolumeClaim
		enableResize      bool
	}{
		"condition-update-with-enabled-feature-gate": {
			isExpectedFailure: false,
			oldClaim:          validClaim,
			newClaim:          validConditionUpdate,
			enableResize:      true,
		},
	}
	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			// ensure we have a resource version specified for updates
			scenario.oldClaim.ResourceVersion = "1"
			scenario.newClaim.ResourceVersion = "1"
			errs := ValidatePersistentVolumeClaimStatusUpdate(scenario.newClaim, scenario.oldClaim)
			if len(errs) == 0 && scenario.isExpectedFailure {
				t.Errorf("Unexpected success for scenario: %s", name)
			}
			if len(errs) > 0 && !scenario.isExpectedFailure {
				t.Errorf("Unexpected failure for scenario: %s - %+v", name, errs)
			}
		})
	}
}

func TestValidateResourceQuota(t *testing.T) {
	spec := core.ResourceQuotaSpec{
		Hard: core.ResourceList{
			core.ResourceCPU:                    resource.MustParse("100"),
			core.ResourceMemory:                 resource.MustParse("10000"),
			core.ResourceRequestsCPU:            resource.MustParse("100"),
			core.ResourceRequestsMemory:         resource.MustParse("10000"),
			core.ResourceLimitsCPU:              resource.MustParse("100"),
			core.ResourceLimitsMemory:           resource.MustParse("10000"),
			core.ResourcePods:                   resource.MustParse("10"),
			core.ResourceServices:               resource.MustParse("0"),
			core.ResourceReplicationControllers: resource.MustParse("10"),
			core.ResourceQuotas:                 resource.MustParse("10"),
			core.ResourceConfigMaps:             resource.MustParse("10"),
			core.ResourceSecrets:                resource.MustParse("10"),
		},
	}

	terminatingSpec := core.ResourceQuotaSpec{
		Hard: core.ResourceList{
			core.ResourceCPU:       resource.MustParse("100"),
			core.ResourceLimitsCPU: resource.MustParse("200"),
		},
		Scopes: []core.ResourceQuotaScope{core.ResourceQuotaScopeTerminating},
	}

	nonTerminatingSpec := core.ResourceQuotaSpec{
		Hard: core.ResourceList{
			core.ResourceCPU: resource.MustParse("100"),
		},
		Scopes: []core.ResourceQuotaScope{core.ResourceQuotaScopeNotTerminating},
	}

	bestEffortSpec := core.ResourceQuotaSpec{
		Hard: core.ResourceList{
			core.ResourcePods: resource.MustParse("100"),
		},
		Scopes: []core.ResourceQuotaScope{core.ResourceQuotaScopeBestEffort},
	}

	nonBestEffortSpec := core.ResourceQuotaSpec{
		Hard: core.ResourceList{
			core.ResourceCPU: resource.MustParse("100"),
		},
		Scopes: []core.ResourceQuotaScope{core.ResourceQuotaScopeNotBestEffort},
	}

	crossNamespaceAffinitySpec := core.ResourceQuotaSpec{
		Hard: core.ResourceList{
			core.ResourceCPU:       resource.MustParse("100"),
			core.ResourceLimitsCPU: resource.MustParse("200"),
		},
		Scopes: []core.ResourceQuotaScope{core.ResourceQuotaScopeCrossNamespacePodAffinity},
	}

	scopeSelectorSpec := core.ResourceQuotaSpec{
		ScopeSelector: &core.ScopeSelector{
			MatchExpressions: []core.ScopedResourceSelectorRequirement{
				{
					ScopeName: core.ResourceQuotaScopePriorityClass,
					Operator:  core.ScopeSelectorOpIn,
					Values:    []string{"cluster-services"},
				},
			},
		},
	}

	// storage is not yet supported as a quota tracked resource
	invalidQuotaResourceSpec := core.ResourceQuotaSpec{
		Hard: core.ResourceList{
			core.ResourceStorage: resource.MustParse("10"),
		},
	}

	negativeSpec := core.ResourceQuotaSpec{
		Hard: core.ResourceList{
			core.ResourceCPU:                    resource.MustParse("-100"),
			core.ResourceMemory:                 resource.MustParse("-10000"),
			core.ResourcePods:                   resource.MustParse("-10"),
			core.ResourceServices:               resource.MustParse("-10"),
			core.ResourceReplicationControllers: resource.MustParse("-10"),
			core.ResourceQuotas:                 resource.MustParse("-10"),
			core.ResourceConfigMaps:             resource.MustParse("-10"),
			core.ResourceSecrets:                resource.MustParse("-10"),
		},
	}

	fractionalComputeSpec := core.ResourceQuotaSpec{
		Hard: core.ResourceList{
			core.ResourceCPU: resource.MustParse("100m"),
		},
	}

	fractionalPodSpec := core.ResourceQuotaSpec{
		Hard: core.ResourceList{
			core.ResourcePods:                   resource.MustParse(".1"),
			core.ResourceServices:               resource.MustParse(".5"),
			core.ResourceReplicationControllers: resource.MustParse("1.25"),
			core.ResourceQuotas:                 resource.MustParse("2.5"),
		},
	}

	invalidTerminatingScopePairsSpec := core.ResourceQuotaSpec{
		Hard: core.ResourceList{
			core.ResourceCPU: resource.MustParse("100"),
		},
		Scopes: []core.ResourceQuotaScope{core.ResourceQuotaScopeTerminating, core.ResourceQuotaScopeNotTerminating},
	}

	invalidBestEffortScopePairsSpec := core.ResourceQuotaSpec{
		Hard: core.ResourceList{
			core.ResourcePods: resource.MustParse("100"),
		},
		Scopes: []core.ResourceQuotaScope{core.ResourceQuotaScopeBestEffort, core.ResourceQuotaScopeNotBestEffort},
	}

	invalidCrossNamespaceAffinitySpec := core.ResourceQuotaSpec{
		ScopeSelector: &core.ScopeSelector{
			MatchExpressions: []core.ScopedResourceSelectorRequirement{
				{
					ScopeName: core.ResourceQuotaScopeCrossNamespacePodAffinity,
					Operator:  core.ScopeSelectorOpIn,
					Values:    []string{"cluster-services"},
				},
			},
		},
	}

	invalidScopeNameSpec := core.ResourceQuotaSpec{
		Hard: core.ResourceList{
			core.ResourceCPU: resource.MustParse("100"),
		},
		Scopes: []core.ResourceQuotaScope{core.ResourceQuotaScope("foo")},
	}

	testCases := map[string]struct {
		rq                       core.ResourceQuota
		errDetail                string
		errField                 string
		disableNamespaceSelector bool
	}{
		"no-scope": {
			rq: core.ResourceQuota{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "abc",
					Namespace: "foo",
				},
				Spec: spec,
			},
		},
		"fractional-compute-spec": {
			rq: core.ResourceQuota{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "abc",
					Namespace: "foo",
				},
				Spec: fractionalComputeSpec,
			},
		},
		"terminating-spec": {
			rq: core.ResourceQuota{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "abc",
					Namespace: "foo",
				},
				Spec: terminatingSpec,
			},
		},
		"non-terminating-spec": {
			rq: core.ResourceQuota{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "abc",
					Namespace: "foo",
				},
				Spec: nonTerminatingSpec,
			},
		},
		"best-effort-spec": {
			rq: core.ResourceQuota{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "abc",
					Namespace: "foo",
				},
				Spec: bestEffortSpec,
			},
		},
		"cross-namespace-affinity-spec": {
			rq: core.ResourceQuota{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "abc",
					Namespace: "foo",
				},
				Spec: crossNamespaceAffinitySpec,
			},
		},
		"scope-selector-spec": {
			rq: core.ResourceQuota{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "abc",
					Namespace: "foo",
				},
				Spec: scopeSelectorSpec,
			},
		},
		"non-best-effort-spec": {
			rq: core.ResourceQuota{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "abc",
					Namespace: "foo",
				},
				Spec: nonBestEffortSpec,
			},
		},
		"zero-length Name": {
			rq:        core.ResourceQuota{ObjectMeta: metav1.ObjectMeta{Name: "", Namespace: "foo"}, Spec: spec},
			errDetail: "name or generateName is required",
		},
		"zero-length Namespace": {
			rq:       core.ResourceQuota{ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: ""}, Spec: spec},
			errField: "metadata.namespace",
		},
		"invalid Name": {
			rq:        core.ResourceQuota{ObjectMeta: metav1.ObjectMeta{Name: "^Invalid", Namespace: "foo"}, Spec: spec},
			errDetail: dnsSubdomainLabelErrMsg,
		},
		"invalid Namespace": {
			rq:        core.ResourceQuota{ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: "^Invalid"}, Spec: spec},
			errDetail: dnsLabelErrMsg,
		},
		"negative-limits": {
			rq:        core.ResourceQuota{ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: "foo"}, Spec: negativeSpec},
			errDetail: isNegativeErrorMsg,
		},
		"fractional-api-resource": {
			rq:        core.ResourceQuota{ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: "foo"}, Spec: fractionalPodSpec},
			errDetail: isNotIntegerErrorMsg,
		},
		"invalid-quota-resource": {
			rq:        core.ResourceQuota{ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: "foo"}, Spec: invalidQuotaResourceSpec},
			errDetail: isInvalidQuotaResource,
		},
		"invalid-quota-terminating-pair": {
			rq:        core.ResourceQuota{ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: "foo"}, Spec: invalidTerminatingScopePairsSpec},
			errDetail: "conflicting scopes",
		},
		"invalid-quota-besteffort-pair": {
			rq:        core.ResourceQuota{ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: "foo"}, Spec: invalidBestEffortScopePairsSpec},
			errDetail: "conflicting scopes",
		},
		"invalid-quota-scope-name": {
			rq:        core.ResourceQuota{ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: "foo"}, Spec: invalidScopeNameSpec},
			errDetail: "unsupported scope",
		},
		"invalid-cross-namespace-affinity": {
			rq:        core.ResourceQuota{ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: "foo"}, Spec: invalidCrossNamespaceAffinitySpec},
			errDetail: "must be 'Exist' when scope is any of ResourceQuotaScopeTerminating, ResourceQuotaScopeNotTerminating, ResourceQuotaScopeBestEffort, ResourceQuotaScopeNotBestEffort or ResourceQuotaScopeCrossNamespacePodAffinity",
		},
		"cross-namespace-affinity-disabled": {
			rq:                       core.ResourceQuota{ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: "foo"}, Spec: crossNamespaceAffinitySpec},
			errDetail:                "unsupported scope",
			disableNamespaceSelector: true,
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			errs := ValidateResourceQuota(&tc.rq, ResourceQuotaValidationOptions{
				AllowPodAffinityNamespaceSelector: !tc.disableNamespaceSelector,
			})
			if len(tc.errDetail) == 0 && len(tc.errField) == 0 && len(errs) != 0 {
				t.Errorf("expected success: %v", errs)
			} else if (len(tc.errDetail) != 0 || len(tc.errField) != 0) && len(errs) == 0 {
				t.Errorf("expected failure")
			} else {
				for i := range errs {
					if !strings.Contains(errs[i].Detail, tc.errDetail) {
						t.Errorf("expected error detail either empty or %s, got %s", tc.errDetail, errs[i].Detail)
					}
				}
			}
		})
	}
}

func TestValidateNamespace(t *testing.T) {
	validLabels := map[string]string{"a": "b"}
	invalidLabels := map[string]string{"NoUppercaseOrSpecialCharsLike=Equals": "b"}
	successCases := []core.Namespace{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "abc", Labels: validLabels},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "abc-123"},
			Spec: core.NamespaceSpec{
				Finalizers: []core.FinalizerName{"example.com/something", "example.com/other"},
			},
		},
	}
	for _, successCase := range successCases {
		if errs := ValidateNamespace(&successCase); len(errs) != 0 {
			t.Errorf("expected success: %v", errs)
		}
	}
	errorCases := map[string]struct {
		R core.Namespace
		D string
	}{
		"zero-length name": {
			core.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ""}},
			"",
		},
		"defined-namespace": {
			core.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "abc-123", Namespace: "makesnosense"}},
			"",
		},
		"invalid-labels": {
			core.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "abc", Labels: invalidLabels}},
			"",
		},
	}
	for k, v := range errorCases {
		errs := ValidateNamespace(&v.R)
		if len(errs) == 0 {
			t.Errorf("expected failure for %s", k)
		}
	}
}

func TestValidateNamespaceFinalizeUpdate(t *testing.T) {
	tests := []struct {
		oldNamespace core.Namespace
		namespace    core.Namespace
		valid        bool
	}{
		{core.Namespace{}, core.Namespace{}, true},
		{core.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "foo"}},
			core.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo"},
				Spec: core.NamespaceSpec{
					Finalizers: []core.FinalizerName{"Foo"},
				},
			}, false},
		{core.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "foo"},
			Spec: core.NamespaceSpec{
				Finalizers: []core.FinalizerName{"foo.com/bar"},
			},
		},
			core.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo"},
				Spec: core.NamespaceSpec{
					Finalizers: []core.FinalizerName{"foo.com/bar", "what.com/bar"},
				},
			}, true},
		{core.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "fooemptyfinalizer"},
			Spec: core.NamespaceSpec{
				Finalizers: []core.FinalizerName{"foo.com/bar"},
			},
		},
			core.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "fooemptyfinalizer"},
				Spec: core.NamespaceSpec{
					Finalizers: []core.FinalizerName{"", "foo.com/bar", "what.com/bar"},
				},
			}, false},
	}
	for i, test := range tests {
		test.namespace.ObjectMeta.ResourceVersion = "1"
		test.oldNamespace.ObjectMeta.ResourceVersion = "1"
		errs := ValidateNamespaceFinalizeUpdate(&test.namespace, &test.oldNamespace)
		if test.valid && len(errs) > 0 {
			t.Errorf("%d: Unexpected error: %v", i, errs)
			t.Logf("%#v vs %#v", test.oldNamespace, test.namespace)
		}
		if !test.valid && len(errs) == 0 {
			t.Errorf("%d: Unexpected non-error", i)
		}
	}
}

func TestValidateNamespaceStatusUpdate(t *testing.T) {
	now := metav1.Now()

	tests := []struct {
		oldNamespace core.Namespace
		namespace    core.Namespace
		valid        bool
	}{
		{core.Namespace{}, core.Namespace{
			Status: core.NamespaceStatus{
				Phase: core.NamespaceActive,
			},
		}, true},
		// Cannot set deletionTimestamp via status update
		{core.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "foo"}},
			core.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "foo",
					DeletionTimestamp: &now},
				Status: core.NamespaceStatus{
					Phase: core.NamespaceTerminating,
				},
			}, false},
		// Can update phase via status update
		{core.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "foo",
				DeletionTimestamp: &now}},
			core.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "foo",
					DeletionTimestamp: &now},
				Status: core.NamespaceStatus{
					Phase: core.NamespaceTerminating,
				},
			}, true},
		{core.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "foo"}},
			core.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo"},
				Status: core.NamespaceStatus{
					Phase: core.NamespaceTerminating,
				},
			}, false},
		{core.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "foo"}},
			core.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "bar"},
				Status: core.NamespaceStatus{
					Phase: core.NamespaceTerminating,
				},
			}, false},
	}
	for i, test := range tests {
		test.namespace.ObjectMeta.ResourceVersion = "1"
		test.oldNamespace.ObjectMeta.ResourceVersion = "1"
		errs := ValidateNamespaceStatusUpdate(&test.namespace, &test.oldNamespace)
		if test.valid && len(errs) > 0 {
			t.Errorf("%d: Unexpected error: %v", i, errs)
			t.Logf("%#v vs %#v", test.oldNamespace.ObjectMeta, test.namespace.ObjectMeta)
		}
		if !test.valid && len(errs) == 0 {
			t.Errorf("%d: Unexpected non-error", i)
		}
	}
}

func TestValidateNamespaceUpdate(t *testing.T) {
	tests := []struct {
		oldNamespace core.Namespace
		namespace    core.Namespace
		valid        bool
	}{
		{core.Namespace{}, core.Namespace{}, true},
		{core.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "foo1"}},
			core.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "bar1"},
			}, false},
		{core.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "foo2",
				Labels: map[string]string{"foo": "bar"},
			},
		}, core.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "foo2",
				Labels: map[string]string{"foo": "baz"},
			},
		}, true},
		{core.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "foo3",
			},
		}, core.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "foo3",
				Labels: map[string]string{"foo": "baz"},
			},
		}, true},
		{core.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "foo4",
				Labels: map[string]string{"bar": "foo"},
			},
		}, core.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "foo4",
				Labels: map[string]string{"foo": "baz"},
			},
		}, true},
		{core.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "foo5",
				Labels: map[string]string{"foo": "baz"},
			},
		}, core.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "foo5",
				Labels: map[string]string{"Foo": "baz"},
			},
		}, true},
		{core.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "foo6",
				Labels: map[string]string{"foo": "baz"},
			},
		}, core.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "foo6",
				Labels: map[string]string{"Foo": "baz"},
			},
			Spec: core.NamespaceSpec{
				Finalizers: []core.FinalizerName{"kubernetes"},
			},
			Status: core.NamespaceStatus{
				Phase: core.NamespaceTerminating,
			},
		}, true},
	}
	for i, test := range tests {
		test.namespace.ObjectMeta.ResourceVersion = "1"
		test.oldNamespace.ObjectMeta.ResourceVersion = "1"
		errs := ValidateNamespaceUpdate(&test.namespace, &test.oldNamespace)
		if test.valid && len(errs) > 0 {
			t.Errorf("%d: Unexpected error: %v", i, errs)
			t.Logf("%#v vs %#v", test.oldNamespace.ObjectMeta, test.namespace.ObjectMeta)
		}
		if !test.valid && len(errs) == 0 {
			t.Errorf("%d: Unexpected non-error", i)
		}
	}
}

func TestValidateSecret(t *testing.T) {
	// Opaque secret validation
	validSecret := func() core.Secret {
		return core.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "bar"},
			Data: map[string][]byte{
				"data-1": []byte("bar"),
			},
		}
	}

	var (
		emptyName     = validSecret()
		invalidName   = validSecret()
		emptyNs       = validSecret()
		invalidNs     = validSecret()
		overMaxSize   = validSecret()
		invalidKey    = validSecret()
		leadingDotKey = validSecret()
		dotKey        = validSecret()
		doubleDotKey  = validSecret()
	)

	emptyName.Name = ""
	invalidName.Name = "NoUppercaseOrSpecialCharsLike=Equals"
	emptyNs.Namespace = ""
	invalidNs.Namespace = "NoUppercaseOrSpecialCharsLike=Equals"
	overMaxSize.Data = map[string][]byte{
		"over": make([]byte, core.MaxSecretSize+1),
	}
	invalidKey.Data["a*b"] = []byte("whoops")
	leadingDotKey.Data[".key"] = []byte("bar")
	dotKey.Data["."] = []byte("bar")
	doubleDotKey.Data[".."] = []byte("bar")

	// kubernetes.io/service-account-token secret validation
	validServiceAccountTokenSecret := func() core.Secret {
		return core.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "bar",
				Annotations: map[string]string{
					core.ServiceAccountNameKey: "foo",
				},
			},
			Type: core.SecretTypeServiceAccountToken,
			Data: map[string][]byte{
				"data-1": []byte("bar"),
			},
		}
	}

	var (
		emptyTokenAnnotation    = validServiceAccountTokenSecret()
		missingTokenAnnotation  = validServiceAccountTokenSecret()
		missingTokenAnnotations = validServiceAccountTokenSecret()
	)
	emptyTokenAnnotation.Annotations[core.ServiceAccountNameKey] = ""
	delete(missingTokenAnnotation.Annotations, core.ServiceAccountNameKey)
	missingTokenAnnotations.Annotations = nil

	tests := map[string]struct {
		secret core.Secret
		valid  bool
	}{
		"valid":                                     {validSecret(), true},
		"empty name":                                {emptyName, false},
		"invalid name":                              {invalidName, false},
		"empty namespace":                           {emptyNs, false},
		"invalid namespace":                         {invalidNs, false},
		"over max size":                             {overMaxSize, false},
		"invalid key":                               {invalidKey, false},
		"valid service-account-token secret":        {validServiceAccountTokenSecret(), true},
		"empty service-account-token annotation":    {emptyTokenAnnotation, false},
		"missing service-account-token annotation":  {missingTokenAnnotation, false},
		"missing service-account-token annotations": {missingTokenAnnotations, false},
		"leading dot key":                           {leadingDotKey, true},
		"dot key":                                   {dotKey, false},
		"double dot key":                            {doubleDotKey, false},
	}

	for name, tc := range tests {
		errs := ValidateSecret(&tc.secret)
		if tc.valid && len(errs) > 0 {
			t.Errorf("%v: Unexpected error: %v", name, errs)
		}
		if !tc.valid && len(errs) == 0 {
			t.Errorf("%v: Unexpected non-error", name)
		}
	}
}

func TestValidateSecretUpdate(t *testing.T) {
	validSecret := func() core.Secret {
		return core.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "foo",
				Namespace:       "bar",
				ResourceVersion: "20",
			},
			Data: map[string][]byte{
				"data-1": []byte("bar"),
			},
		}
	}

	falseVal := false
	trueVal := true

	secret := validSecret()
	immutableSecret := validSecret()
	immutableSecret.Immutable = &trueVal
	mutableSecret := validSecret()
	mutableSecret.Immutable = &falseVal

	secretWithData := validSecret()
	secretWithData.Data["data-2"] = []byte("baz")
	immutableSecretWithData := validSecret()
	immutableSecretWithData.Immutable = &trueVal
	immutableSecretWithData.Data["data-2"] = []byte("baz")

	secretWithChangedData := validSecret()
	secretWithChangedData.Data["data-1"] = []byte("foo")
	immutableSecretWithChangedData := validSecret()
	immutableSecretWithChangedData.Immutable = &trueVal
	immutableSecretWithChangedData.Data["data-1"] = []byte("foo")

	tests := []struct {
		name      string
		oldSecret core.Secret
		newSecret core.Secret
		valid     bool
	}{
		{
			name:      "mark secret immutable",
			oldSecret: secret,
			newSecret: immutableSecret,
			valid:     true,
		},
		{
			name:      "revert immutable secret",
			oldSecret: immutableSecret,
			newSecret: secret,
			valid:     false,
		},
		{
			name:      "makr immutable secret mutable",
			oldSecret: immutableSecret,
			newSecret: mutableSecret,
			valid:     false,
		},
		{
			name:      "add data in secret",
			oldSecret: secret,
			newSecret: secretWithData,
			valid:     true,
		},
		{
			name:      "add data in immutable secret",
			oldSecret: immutableSecret,
			newSecret: immutableSecretWithData,
			valid:     false,
		},
		{
			name:      "change data in secret",
			oldSecret: secret,
			newSecret: secretWithChangedData,
			valid:     true,
		},
		{
			name:      "change data in immutable secret",
			oldSecret: immutableSecret,
			newSecret: immutableSecretWithChangedData,
			valid:     false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := ValidateSecretUpdate(&tc.newSecret, &tc.oldSecret)
			if tc.valid && len(errs) > 0 {
				t.Errorf("Unexpected error: %v", errs)
			}
			if !tc.valid && len(errs) == 0 {
				t.Errorf("Unexpected lack of error")
			}
		})
	}
}

func TestValidateDockerConfigSecret(t *testing.T) {
	validDockerSecret := func() core.Secret {
		return core.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "bar"},
			Type:       core.SecretTypeDockercfg,
			Data: map[string][]byte{
				core.DockerConfigKey: []byte(`{"https://index.docker.io/v1/": {"auth": "Y2x1ZWRyb29sZXIwMDAxOnBhc3N3b3Jk","email": "fake@example.com"}}`),
			},
		}
	}
	validDockerSecret2 := func() core.Secret {
		return core.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "bar"},
			Type:       core.SecretTypeDockerConfigJSON,
			Data: map[string][]byte{
				core.DockerConfigJSONKey: []byte(`{"auths":{"https://index.docker.io/v1/": {"auth": "Y2x1ZWRyb29sZXIwMDAxOnBhc3N3b3Jk","email": "fake@example.com"}}}`),
			},
		}
	}

	var (
		missingDockerConfigKey  = validDockerSecret()
		emptyDockerConfigKey    = validDockerSecret()
		invalidDockerConfigKey  = validDockerSecret()
		missingDockerConfigKey2 = validDockerSecret2()
		emptyDockerConfigKey2   = validDockerSecret2()
		invalidDockerConfigKey2 = validDockerSecret2()
	)

	delete(missingDockerConfigKey.Data, core.DockerConfigKey)
	emptyDockerConfigKey.Data[core.DockerConfigKey] = []byte("")
	invalidDockerConfigKey.Data[core.DockerConfigKey] = []byte("bad")
	delete(missingDockerConfigKey2.Data, core.DockerConfigJSONKey)
	emptyDockerConfigKey2.Data[core.DockerConfigJSONKey] = []byte("")
	invalidDockerConfigKey2.Data[core.DockerConfigJSONKey] = []byte("bad")

	tests := map[string]struct {
		secret core.Secret
		valid  bool
	}{
		"valid dockercfg":     {validDockerSecret(), true},
		"missing dockercfg":   {missingDockerConfigKey, false},
		"empty dockercfg":     {emptyDockerConfigKey, false},
		"invalid dockercfg":   {invalidDockerConfigKey, false},
		"valid config.json":   {validDockerSecret2(), true},
		"missing config.json": {missingDockerConfigKey2, false},
		"empty config.json":   {emptyDockerConfigKey2, false},
		"invalid config.json": {invalidDockerConfigKey2, false},
	}

	for name, tc := range tests {
		errs := ValidateSecret(&tc.secret)
		if tc.valid && len(errs) > 0 {
			t.Errorf("%v: Unexpected error: %v", name, errs)
		}
		if !tc.valid && len(errs) == 0 {
			t.Errorf("%v: Unexpected non-error", name)
		}
	}
}

func TestValidateBasicAuthSecret(t *testing.T) {
	validBasicAuthSecret := func() core.Secret {
		return core.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "bar"},
			Type:       core.SecretTypeBasicAuth,
			Data: map[string][]byte{
				core.BasicAuthUsernameKey: []byte("username"),
				core.BasicAuthPasswordKey: []byte("password"),
			},
		}
	}

	var (
		missingBasicAuthUsernamePasswordKeys = validBasicAuthSecret()
	)

	delete(missingBasicAuthUsernamePasswordKeys.Data, core.BasicAuthUsernameKey)
	delete(missingBasicAuthUsernamePasswordKeys.Data, core.BasicAuthPasswordKey)

	tests := map[string]struct {
		secret core.Secret
		valid  bool
	}{
		"valid":                         {validBasicAuthSecret(), true},
		"missing username and password": {missingBasicAuthUsernamePasswordKeys, false},
	}

	for name, tc := range tests {
		errs := ValidateSecret(&tc.secret)
		if tc.valid && len(errs) > 0 {
			t.Errorf("%v: Unexpected error: %v", name, errs)
		}
		if !tc.valid && len(errs) == 0 {
			t.Errorf("%v: Unexpected non-error", name)
		}
	}
}

func TestValidateSSHAuthSecret(t *testing.T) {
	validSSHAuthSecret := func() core.Secret {
		return core.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "bar"},
			Type:       core.SecretTypeSSHAuth,
			Data: map[string][]byte{
				core.SSHAuthPrivateKey: []byte("foo-bar-baz"),
			},
		}
	}

	missingSSHAuthPrivateKey := validSSHAuthSecret()

	delete(missingSSHAuthPrivateKey.Data, core.SSHAuthPrivateKey)

	tests := map[string]struct {
		secret core.Secret
		valid  bool
	}{
		"valid":               {validSSHAuthSecret(), true},
		"missing private key": {missingSSHAuthPrivateKey, false},
	}

	for name, tc := range tests {
		errs := ValidateSecret(&tc.secret)
		if tc.valid && len(errs) > 0 {
			t.Errorf("%v: Unexpected error: %v", name, errs)
		}
		if !tc.valid && len(errs) == 0 {
			t.Errorf("%v: Unexpected non-error", name)
		}
	}
}

func TestValidateEndpointsCreate(t *testing.T) {
	successCases := map[string]struct {
		endpoints core.Endpoints
	}{
		"simple endpoint": {
			endpoints: core.Endpoints{
				ObjectMeta: metav1.ObjectMeta{Name: "mysvc", Namespace: "namespace"},
				Subsets: []core.EndpointSubset{
					{
						Addresses: []core.EndpointAddress{{IP: "10.10.1.1"}, {IP: "10.10.2.2"}},
						Ports:     []core.EndpointPort{{Name: "a", Port: 8675, Protocol: "TCP"}, {Name: "b", Port: 309, Protocol: "TCP"}},
					},
					{
						Addresses: []core.EndpointAddress{{IP: "10.10.3.3"}},
						Ports:     []core.EndpointPort{{Name: "a", Port: 93, Protocol: "TCP"}, {Name: "b", Port: 76, Protocol: "TCP"}},
					},
				},
			},
		},
		"empty subsets": {
			endpoints: core.Endpoints{
				ObjectMeta: metav1.ObjectMeta{Name: "mysvc", Namespace: "namespace"},
			},
		},
		"no name required for singleton port": {
			endpoints: core.Endpoints{
				ObjectMeta: metav1.ObjectMeta{Name: "mysvc", Namespace: "namespace"},
				Subsets: []core.EndpointSubset{
					{
						Addresses: []core.EndpointAddress{{IP: "10.10.1.1"}},
						Ports:     []core.EndpointPort{{Port: 8675, Protocol: "TCP"}},
					},
				},
			},
		},
		"valid appProtocol": {
			endpoints: core.Endpoints{
				ObjectMeta: metav1.ObjectMeta{Name: "mysvc", Namespace: "namespace"},
				Subsets: []core.EndpointSubset{
					{
						Addresses: []core.EndpointAddress{{IP: "10.10.1.1"}},
						Ports:     []core.EndpointPort{{Port: 8675, Protocol: "TCP", AppProtocol: utilpointer.StringPtr("HTTP")}},
					},
				},
			},
		},
		"empty ports": {
			endpoints: core.Endpoints{
				ObjectMeta: metav1.ObjectMeta{Name: "mysvc", Namespace: "namespace"},
				Subsets: []core.EndpointSubset{
					{
						Addresses: []core.EndpointAddress{{IP: "10.10.3.3"}},
					},
				},
			},
		},
	}

	for name, tc := range successCases {
		t.Run(name, func(t *testing.T) {
			errs := ValidateEndpointsCreate(&tc.endpoints)
			if len(errs) != 0 {
				t.Errorf("Expected no validation errors, got %v", errs)
			}

		})
	}

	errorCases := map[string]struct {
		endpoints   core.Endpoints
		errorType   field.ErrorType
		errorDetail string
	}{
		"missing namespace": {
			endpoints: core.Endpoints{ObjectMeta: metav1.ObjectMeta{Name: "mysvc"}},
			errorType: "FieldValueRequired",
		},
		"missing name": {
			endpoints: core.Endpoints{ObjectMeta: metav1.ObjectMeta{Namespace: "namespace"}},
			errorType: "FieldValueRequired",
		},
		"invalid namespace": {
			endpoints:   core.Endpoints{ObjectMeta: metav1.ObjectMeta{Name: "mysvc", Namespace: "no@#invalid.;chars\"allowed"}},
			errorType:   "FieldValueInvalid",
			errorDetail: dnsLabelErrMsg,
		},
		"invalid name": {
			endpoints:   core.Endpoints{ObjectMeta: metav1.ObjectMeta{Name: "-_Invliad^&Characters", Namespace: "namespace"}},
			errorType:   "FieldValueInvalid",
			errorDetail: dnsSubdomainLabelErrMsg,
		},
		"empty addresses": {
			endpoints: core.Endpoints{
				ObjectMeta: metav1.ObjectMeta{Name: "mysvc", Namespace: "namespace"},
				Subsets: []core.EndpointSubset{
					{
						Ports: []core.EndpointPort{{Name: "a", Port: 93, Protocol: "TCP"}},
					},
				},
			},
			errorType: "FieldValueRequired",
		},
		"invalid IP": {
			endpoints: core.Endpoints{
				ObjectMeta: metav1.ObjectMeta{Name: "mysvc", Namespace: "namespace"},
				Subsets: []core.EndpointSubset{
					{
						Addresses: []core.EndpointAddress{{IP: "[2001:0db8:85a3:0042:1000:8a2e:0370:7334]"}},
						Ports:     []core.EndpointPort{{Name: "a", Port: 93, Protocol: "TCP"}},
					},
				},
			},
			errorType:   "FieldValueInvalid",
			errorDetail: "must be a valid IP address",
		},
		"Multiple ports, one without name": {
			endpoints: core.Endpoints{
				ObjectMeta: metav1.ObjectMeta{Name: "mysvc", Namespace: "namespace"},
				Subsets: []core.EndpointSubset{
					{
						Addresses: []core.EndpointAddress{{IP: "10.10.1.1"}},
						Ports:     []core.EndpointPort{{Port: 8675, Protocol: "TCP"}, {Name: "b", Port: 309, Protocol: "TCP"}},
					},
				},
			},
			errorType: "FieldValueRequired",
		},
		"Invalid port number": {
			endpoints: core.Endpoints{
				ObjectMeta: metav1.ObjectMeta{Name: "mysvc", Namespace: "namespace"},
				Subsets: []core.EndpointSubset{
					{
						Addresses: []core.EndpointAddress{{IP: "10.10.1.1"}},
						Ports:     []core.EndpointPort{{Name: "a", Port: 66000, Protocol: "TCP"}},
					},
				},
			},
			errorType:   "FieldValueInvalid",
			errorDetail: "between",
		},
		"Invalid protocol": {
			endpoints: core.Endpoints{
				ObjectMeta: metav1.ObjectMeta{Name: "mysvc", Namespace: "namespace"},
				Subsets: []core.EndpointSubset{
					{
						Addresses: []core.EndpointAddress{{IP: "10.10.1.1"}},
						Ports:     []core.EndpointPort{{Name: "a", Port: 93, Protocol: "Protocol"}},
					},
				},
			},
			errorType: "FieldValueNotSupported",
		},
		"Address missing IP": {
			endpoints: core.Endpoints{
				ObjectMeta: metav1.ObjectMeta{Name: "mysvc", Namespace: "namespace"},
				Subsets: []core.EndpointSubset{
					{
						Addresses: []core.EndpointAddress{{}},
						Ports:     []core.EndpointPort{{Name: "a", Port: 93, Protocol: "TCP"}},
					},
				},
			},
			errorType:   "FieldValueInvalid",
			errorDetail: "must be a valid IP address",
		},
		"Port missing number": {
			endpoints: core.Endpoints{
				ObjectMeta: metav1.ObjectMeta{Name: "mysvc", Namespace: "namespace"},
				Subsets: []core.EndpointSubset{
					{
						Addresses: []core.EndpointAddress{{IP: "10.10.1.1"}},
						Ports:     []core.EndpointPort{{Name: "a", Protocol: "TCP"}},
					},
				},
			},
			errorType:   "FieldValueInvalid",
			errorDetail: "between",
		},
		"Port missing protocol": {
			endpoints: core.Endpoints{
				ObjectMeta: metav1.ObjectMeta{Name: "mysvc", Namespace: "namespace"},
				Subsets: []core.EndpointSubset{
					{
						Addresses: []core.EndpointAddress{{IP: "10.10.1.1"}},
						Ports:     []core.EndpointPort{{Name: "a", Port: 93}},
					},
				},
			},
			errorType: "FieldValueRequired",
		},
		"Address is loopback": {
			endpoints: core.Endpoints{
				ObjectMeta: metav1.ObjectMeta{Name: "mysvc", Namespace: "namespace"},
				Subsets: []core.EndpointSubset{
					{
						Addresses: []core.EndpointAddress{{IP: "127.0.0.1"}},
						Ports:     []core.EndpointPort{{Name: "p", Port: 93, Protocol: "TCP"}},
					},
				},
			},
			errorType:   "FieldValueInvalid",
			errorDetail: "loopback",
		},
		"Address is link-local": {
			endpoints: core.Endpoints{
				ObjectMeta: metav1.ObjectMeta{Name: "mysvc", Namespace: "namespace"},
				Subsets: []core.EndpointSubset{
					{
						Addresses: []core.EndpointAddress{{IP: "169.254.169.254"}},
						Ports:     []core.EndpointPort{{Name: "p", Port: 93, Protocol: "TCP"}},
					},
				},
			},
			errorType:   "FieldValueInvalid",
			errorDetail: "link-local",
		},
		"Address is link-local multicast": {
			endpoints: core.Endpoints{
				ObjectMeta: metav1.ObjectMeta{Name: "mysvc", Namespace: "namespace"},
				Subsets: []core.EndpointSubset{
					{
						Addresses: []core.EndpointAddress{{IP: "224.0.0.1"}},
						Ports:     []core.EndpointPort{{Name: "p", Port: 93, Protocol: "TCP"}},
					},
				},
			},
			errorType:   "FieldValueInvalid",
			errorDetail: "link-local multicast",
		},
		"Invalid AppProtocol": {
			endpoints: core.Endpoints{
				ObjectMeta: metav1.ObjectMeta{Name: "mysvc", Namespace: "namespace"},
				Subsets: []core.EndpointSubset{
					{
						Addresses: []core.EndpointAddress{{IP: "10.10.1.1"}},
						Ports:     []core.EndpointPort{{Name: "p", Port: 93, Protocol: "TCP", AppProtocol: utilpointer.StringPtr("lots-of[invalid]-{chars}")}},
					},
				},
			},
			errorType:   "FieldValueInvalid",
			errorDetail: "name part must consist of alphanumeric characters, '-', '_' or '.', and must start and end with an alphanumeric character",
		},
	}

	for k, v := range errorCases {
		t.Run(k, func(t *testing.T) {
			if errs := ValidateEndpointsCreate(&v.endpoints); len(errs) == 0 || errs[0].Type != v.errorType || !strings.Contains(errs[0].Detail, v.errorDetail) {
				t.Errorf("Expected error type %s with detail %q, got %v", v.errorType, v.errorDetail, errs)
			}
		})
	}
}

func TestValidateEndpointsUpdate(t *testing.T) {
	baseEndpoints := core.Endpoints{
		ObjectMeta: metav1.ObjectMeta{Name: "mysvc", Namespace: "namespace", ResourceVersion: "1234"},
		Subsets: []core.EndpointSubset{{
			Addresses: []core.EndpointAddress{{IP: "10.1.2.3"}},
		}},
	}

	testCases := map[string]struct {
		tweakOldEndpoints func(ep *core.Endpoints)
		tweakNewEndpoints func(ep *core.Endpoints)
		numExpectedErrors int
	}{
		"update to valid app protocol": {
			tweakOldEndpoints: func(ep *core.Endpoints) {
				ep.Subsets[0].Ports = []core.EndpointPort{{Name: "a", Port: 8675, Protocol: "TCP"}}
			},
			tweakNewEndpoints: func(ep *core.Endpoints) {
				ep.Subsets[0].Ports = []core.EndpointPort{{Name: "a", Port: 8675, Protocol: "TCP", AppProtocol: utilpointer.StringPtr("https")}}
			},
			numExpectedErrors: 0,
		},
		"update to invalid app protocol": {
			tweakOldEndpoints: func(ep *core.Endpoints) {
				ep.Subsets[0].Ports = []core.EndpointPort{{Name: "a", Port: 8675, Protocol: "TCP"}}
			},
			tweakNewEndpoints: func(ep *core.Endpoints) {
				ep.Subsets[0].Ports = []core.EndpointPort{{Name: "a", Port: 8675, Protocol: "TCP", AppProtocol: utilpointer.StringPtr("~https")}}
			},
			numExpectedErrors: 1,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			oldEndpoints := baseEndpoints.DeepCopy()
			tc.tweakOldEndpoints(oldEndpoints)
			newEndpoints := baseEndpoints.DeepCopy()
			tc.tweakNewEndpoints(newEndpoints)

			errs := ValidateEndpointsUpdate(newEndpoints, oldEndpoints)
			if len(errs) != tc.numExpectedErrors {
				t.Errorf("Expected %d validation errors, got %d: %v", tc.numExpectedErrors, len(errs), errs)
			}

		})
	}
}

func TestValidateTLSSecret(t *testing.T) {
	successCases := map[string]core.Secret{
		"empty certificate chain": {
			ObjectMeta: metav1.ObjectMeta{Name: "tls-cert", Namespace: "namespace"},
			Data: map[string][]byte{
				core.TLSCertKey:       []byte("public key"),
				core.TLSPrivateKeyKey: []byte("private key"),
			},
		},
	}
	for k, v := range successCases {
		if errs := ValidateSecret(&v); len(errs) != 0 {
			t.Errorf("Expected success for %s, got %v", k, errs)
		}
	}
	errorCases := map[string]struct {
		secrets     core.Secret
		errorType   field.ErrorType
		errorDetail string
	}{
		"missing public key": {
			secrets: core.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "tls-cert"},
				Data: map[string][]byte{
					core.TLSCertKey: []byte("public key"),
				},
			},
			errorType: "FieldValueRequired",
		},
		"missing private key": {
			secrets: core.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "tls-cert"},
				Data: map[string][]byte{
					core.TLSCertKey: []byte("public key"),
				},
			},
			errorType: "FieldValueRequired",
		},
	}
	for k, v := range errorCases {
		if errs := ValidateSecret(&v.secrets); len(errs) == 0 || errs[0].Type != v.errorType || !strings.Contains(errs[0].Detail, v.errorDetail) {
			t.Errorf("[%s] Expected error type %s with detail %q, got %v", k, v.errorType, v.errorDetail, errs)
		}
	}
}

func TestValidateSecurityContext(t *testing.T) {
	runAsUser := int64(1)
	fullValidSC := func() *core.SecurityContext {
		return &core.SecurityContext{
			Privileged: utilpointer.BoolPtr(false),
			Capabilities: &core.Capabilities{
				Add:  []core.Capability{"foo"},
				Drop: []core.Capability{"bar"},
			},
			SELinuxOptions: &core.SELinuxOptions{
				User:  "user",
				Role:  "role",
				Type:  "type",
				Level: "level",
			},
			RunAsUser: &runAsUser,
		}
	}

	//setup data
	allSettings := fullValidSC()
	noCaps := fullValidSC()
	noCaps.Capabilities = nil

	noSELinux := fullValidSC()
	noSELinux.SELinuxOptions = nil

	noPrivRequest := fullValidSC()
	noPrivRequest.Privileged = nil

	noRunAsUser := fullValidSC()
	noRunAsUser.RunAsUser = nil

	successCases := map[string]struct {
		sc *core.SecurityContext
	}{
		"all settings":    {allSettings},
		"no capabilities": {noCaps},
		"no selinux":      {noSELinux},
		"no priv request": {noPrivRequest},
		"no run as user":  {noRunAsUser},
	}
	for k, v := range successCases {
		if errs := ValidateSecurityContext(v.sc, field.NewPath("field")); len(errs) != 0 {
			t.Errorf("[%s] Expected success, got %v", k, errs)
		}
	}

	privRequestWithGlobalDeny := fullValidSC()
	privRequestWithGlobalDeny.Privileged = utilpointer.BoolPtr(true)

	negativeRunAsUser := fullValidSC()
	negativeUser := int64(-1)
	negativeRunAsUser.RunAsUser = &negativeUser

	privWithoutEscalation := fullValidSC()
	privWithoutEscalation.Privileged = utilpointer.BoolPtr(true)
	privWithoutEscalation.AllowPrivilegeEscalation = utilpointer.BoolPtr(false)

	capSysAdminWithoutEscalation := fullValidSC()
	capSysAdminWithoutEscalation.Capabilities.Add = []core.Capability{"CAP_SYS_ADMIN"}
	capSysAdminWithoutEscalation.AllowPrivilegeEscalation = utilpointer.BoolPtr(false)

	errorCases := map[string]struct {
		sc           *core.SecurityContext
		errorType    field.ErrorType
		errorDetail  string
		capAllowPriv bool
	}{
		"request privileged when capabilities forbids": {
			sc:          privRequestWithGlobalDeny,
			errorType:   "FieldValueForbidden",
			errorDetail: "disallowed by cluster policy",
		},
		"negative RunAsUser": {
			sc:          negativeRunAsUser,
			errorType:   "FieldValueInvalid",
			errorDetail: "must be between",
		},
		"with CAP_SYS_ADMIN and allowPrivilegeEscalation false": {
			sc:          capSysAdminWithoutEscalation,
			errorType:   "FieldValueInvalid",
			errorDetail: "cannot set `allowPrivilegeEscalation` to false and `capabilities.Add` CAP_SYS_ADMIN",
		},
		"with privileged and allowPrivilegeEscalation false": {
			sc:           privWithoutEscalation,
			errorType:    "FieldValueInvalid",
			errorDetail:  "cannot set `allowPrivilegeEscalation` to false and `privileged` to true",
			capAllowPriv: true,
		},
	}
	for k, v := range errorCases {
		capabilities.SetForTests(capabilities.Capabilities{
			AllowPrivileged: v.capAllowPriv,
		})
		if errs := ValidateSecurityContext(v.sc, field.NewPath("field")); len(errs) == 0 || errs[0].Type != v.errorType || !strings.Contains(errs[0].Detail, v.errorDetail) {
			t.Errorf("[%s] Expected error type %q with detail %q, got %v", k, v.errorType, v.errorDetail, errs)
		}
	}
}

func fakeValidSecurityContext(priv bool) *core.SecurityContext {
	return &core.SecurityContext{
		Privileged: &priv,
	}
}

func TestValidPodLogOptions(t *testing.T) {
	now := metav1.Now()
	negative := int64(-1)
	zero := int64(0)
	positive := int64(1)
	tests := []struct {
		opt  core.PodLogOptions
		errs int
	}{
		{core.PodLogOptions{}, 0},
		{core.PodLogOptions{Previous: true}, 0},
		{core.PodLogOptions{Follow: true}, 0},
		{core.PodLogOptions{TailLines: &zero}, 0},
		{core.PodLogOptions{TailLines: &negative}, 1},
		{core.PodLogOptions{TailLines: &positive}, 0},
		{core.PodLogOptions{LimitBytes: &zero}, 1},
		{core.PodLogOptions{LimitBytes: &negative}, 1},
		{core.PodLogOptions{LimitBytes: &positive}, 0},
		{core.PodLogOptions{SinceSeconds: &negative}, 1},
		{core.PodLogOptions{SinceSeconds: &positive}, 0},
		{core.PodLogOptions{SinceSeconds: &zero}, 1},
		{core.PodLogOptions{SinceTime: &now}, 0},
	}
	for i, test := range tests {
		errs := ValidatePodLogOptions(&test.opt)
		if test.errs != len(errs) {
			t.Errorf("%d: Unexpected errors: %v", i, errs)
		}
	}
}

func TestValidateConfigMap(t *testing.T) {
	newConfigMap := func(name, namespace string, data map[string]string, binaryData map[string][]byte) core.ConfigMap {
		return core.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Data:       data,
			BinaryData: binaryData,
		}
	}

	var (
		validConfigMap = newConfigMap("validname", "validns", map[string]string{"key": "value"}, map[string][]byte{"bin": []byte("value")})
		maxKeyLength   = newConfigMap("validname", "validns", map[string]string{strings.Repeat("a", 253): "value"}, nil)

		emptyName               = newConfigMap("", "validns", nil, nil)
		invalidName             = newConfigMap("NoUppercaseOrSpecialCharsLike=Equals", "validns", nil, nil)
		emptyNs                 = newConfigMap("validname", "", nil, nil)
		invalidNs               = newConfigMap("validname", "NoUppercaseOrSpecialCharsLike=Equals", nil, nil)
		invalidKey              = newConfigMap("validname", "validns", map[string]string{"a*b": "value"}, nil)
		leadingDotKey           = newConfigMap("validname", "validns", map[string]string{".ab": "value"}, nil)
		dotKey                  = newConfigMap("validname", "validns", map[string]string{".": "value"}, nil)
		doubleDotKey            = newConfigMap("validname", "validns", map[string]string{"..": "value"}, nil)
		overMaxKeyLength        = newConfigMap("validname", "validns", map[string]string{strings.Repeat("a", 254): "value"}, nil)
		overMaxSize             = newConfigMap("validname", "validns", map[string]string{"key": strings.Repeat("a", v1.MaxSecretSize+1)}, nil)
		duplicatedKey           = newConfigMap("validname", "validns", map[string]string{"key": "value1"}, map[string][]byte{"key": []byte("value2")})
		binDataInvalidKey       = newConfigMap("validname", "validns", nil, map[string][]byte{"a*b": []byte("value")})
		binDataLeadingDotKey    = newConfigMap("validname", "validns", nil, map[string][]byte{".ab": []byte("value")})
		binDataDotKey           = newConfigMap("validname", "validns", nil, map[string][]byte{".": []byte("value")})
		binDataDoubleDotKey     = newConfigMap("validname", "validns", nil, map[string][]byte{"..": []byte("value")})
		binDataOverMaxKeyLength = newConfigMap("validname", "validns", nil, map[string][]byte{strings.Repeat("a", 254): []byte("value")})
		binDataOverMaxSize      = newConfigMap("validname", "validns", nil, map[string][]byte{"bin": bytes.Repeat([]byte("a"), v1.MaxSecretSize+1)})
		binNonUtf8Value         = newConfigMap("validname", "validns", nil, map[string][]byte{"key": {0, 0xFE, 0, 0xFF}})
	)

	tests := map[string]struct {
		cfg     core.ConfigMap
		isValid bool
	}{
		"valid":                           {validConfigMap, true},
		"max key length":                  {maxKeyLength, true},
		"leading dot key":                 {leadingDotKey, true},
		"empty name":                      {emptyName, false},
		"invalid name":                    {invalidName, false},
		"invalid key":                     {invalidKey, false},
		"empty namespace":                 {emptyNs, false},
		"invalid namespace":               {invalidNs, false},
		"dot key":                         {dotKey, false},
		"double dot key":                  {doubleDotKey, false},
		"over max key length":             {overMaxKeyLength, false},
		"over max size":                   {overMaxSize, false},
		"duplicated key":                  {duplicatedKey, false},
		"binary data invalid key":         {binDataInvalidKey, false},
		"binary data leading dot key":     {binDataLeadingDotKey, true},
		"binary data dot key":             {binDataDotKey, false},
		"binary data double dot key":      {binDataDoubleDotKey, false},
		"binary data over max key length": {binDataOverMaxKeyLength, false},
		"binary data max size":            {binDataOverMaxSize, false},
		"binary data non utf-8 bytes":     {binNonUtf8Value, true},
	}

	for name, tc := range tests {
		errs := ValidateConfigMap(&tc.cfg)
		if tc.isValid && len(errs) > 0 {
			t.Errorf("%v: unexpected error: %v", name, errs)
		}
		if !tc.isValid && len(errs) == 0 {
			t.Errorf("%v: unexpected non-error", name)
		}
	}
}

func TestValidateConfigMapUpdate(t *testing.T) {
	newConfigMap := func(version, name, namespace string, data map[string]string) core.ConfigMap {
		return core.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:            name,
				Namespace:       namespace,
				ResourceVersion: version,
			},
			Data: data,
		}
	}
	validConfigMap := func() core.ConfigMap {
		return newConfigMap("1", "validname", "validdns", map[string]string{"key": "value"})
	}

	falseVal := false
	trueVal := true

	configMap := validConfigMap()
	immutableConfigMap := validConfigMap()
	immutableConfigMap.Immutable = &trueVal
	mutableConfigMap := validConfigMap()
	mutableConfigMap.Immutable = &falseVal

	configMapWithData := validConfigMap()
	configMapWithData.Data["key-2"] = "value-2"
	immutableConfigMapWithData := validConfigMap()
	immutableConfigMapWithData.Immutable = &trueVal
	immutableConfigMapWithData.Data["key-2"] = "value-2"

	configMapWithChangedData := validConfigMap()
	configMapWithChangedData.Data["key"] = "foo"
	immutableConfigMapWithChangedData := validConfigMap()
	immutableConfigMapWithChangedData.Immutable = &trueVal
	immutableConfigMapWithChangedData.Data["key"] = "foo"

	noVersion := newConfigMap("", "validname", "validns", map[string]string{"key": "value"})

	cases := []struct {
		name   string
		newCfg core.ConfigMap
		oldCfg core.ConfigMap
		valid  bool
	}{
		{
			name:   "valid",
			newCfg: configMap,
			oldCfg: configMap,
			valid:  true,
		},
		{
			name:   "invalid",
			newCfg: noVersion,
			oldCfg: configMap,
			valid:  false,
		},
		{
			name:   "mark configmap immutable",
			oldCfg: configMap,
			newCfg: immutableConfigMap,
			valid:  true,
		},
		{
			name:   "revert immutable configmap",
			oldCfg: immutableConfigMap,
			newCfg: configMap,
			valid:  false,
		},
		{
			name:   "mark immutable configmap mutable",
			oldCfg: immutableConfigMap,
			newCfg: mutableConfigMap,
			valid:  false,
		},
		{
			name:   "add data in configmap",
			oldCfg: configMap,
			newCfg: configMapWithData,
			valid:  true,
		},
		{
			name:   "add data in immutable configmap",
			oldCfg: immutableConfigMap,
			newCfg: immutableConfigMapWithData,
			valid:  false,
		},
		{
			name:   "change data in configmap",
			oldCfg: configMap,
			newCfg: configMapWithChangedData,
			valid:  true,
		},
		{
			name:   "change data in immutable configmap",
			oldCfg: immutableConfigMap,
			newCfg: immutableConfigMapWithChangedData,
			valid:  false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			errs := ValidateConfigMapUpdate(&tc.newCfg, &tc.oldCfg)
			if tc.valid && len(errs) > 0 {
				t.Errorf("Unexpected error: %v", errs)
			}
			if !tc.valid && len(errs) == 0 {
				t.Errorf("Unexpected lack of error")
			}
		})
	}
}

func TestValidateHasLabel(t *testing.T) {
	successCase := metav1.ObjectMeta{
		Name:      "123",
		Namespace: "ns",
		Labels: map[string]string{
			"other": "blah",
			"foo":   "bar",
		},
	}
	if errs := ValidateHasLabel(successCase, field.NewPath("field"), "foo", "bar"); len(errs) != 0 {
		t.Errorf("expected success: %v", errs)
	}

	missingCase := metav1.ObjectMeta{
		Name:      "123",
		Namespace: "ns",
		Labels: map[string]string{
			"other": "blah",
		},
	}
	if errs := ValidateHasLabel(missingCase, field.NewPath("field"), "foo", "bar"); len(errs) == 0 {
		t.Errorf("expected failure")
	}

	wrongValueCase := metav1.ObjectMeta{
		Name:      "123",
		Namespace: "ns",
		Labels: map[string]string{
			"other": "blah",
			"foo":   "notbar",
		},
	}
	if errs := ValidateHasLabel(wrongValueCase, field.NewPath("field"), "foo", "bar"); len(errs) == 0 {
		t.Errorf("expected failure")
	}
}

func TestIsValidSysctlName(t *testing.T) {
	valid := []string{
		"a.b.c.d",
		"a",
		"a_b",
		"a-b",
		"abc",
		"abc.def",
	}
	invalid := []string{
		"",
		"*",
		"ä",
		"a_",
		"_",
		"__",
		"_a",
		"_a._b",
		"-",
		".",
		"a.",
		".a",
		"a.b.",
		"a*.b",
		"a*b",
		"*a",
		"a.*",
		"*",
		"abc*",
		"a.abc*",
		"a.b.*",
		"Abc",
		func(n int) string {
			x := make([]byte, n)
			for i := range x {
				x[i] = byte('a')
			}
			return string(x)
		}(256),
	}
	for _, s := range valid {
		if !IsValidSysctlName(s) {
			t.Errorf("%q expected to be a valid sysctl name", s)
		}
	}
	for _, s := range invalid {
		if IsValidSysctlName(s) {
			t.Errorf("%q expected to be an invalid sysctl name", s)
		}
	}
}

func TestValidateSysctls(t *testing.T) {
	valid := []string{
		"net.foo.bar",
		"kernel.shmmax",
	}
	invalid := []string{
		"i..nvalid",
		"_invalid",
	}

	duplicates := []string{
		"kernel.shmmax",
		"kernel.shmmax",
	}

	sysctls := make([]core.Sysctl, len(valid))
	for i, sysctl := range valid {
		sysctls[i].Name = sysctl
	}
	errs := validateSysctls(sysctls, field.NewPath("foo"))
	if len(errs) != 0 {
		t.Errorf("unexpected validation errors: %v", errs)
	}

	sysctls = make([]core.Sysctl, len(invalid))
	for i, sysctl := range invalid {
		sysctls[i].Name = sysctl
	}
	errs = validateSysctls(sysctls, field.NewPath("foo"))
	if len(errs) != 2 {
		t.Errorf("expected 2 validation errors. Got: %v", errs)
	} else {
		if got, expected := errs[0].Error(), "foo"; !strings.Contains(got, expected) {
			t.Errorf("unexpected errors: expected=%q, got=%q", expected, got)
		}
		if got, expected := errs[1].Error(), "foo"; !strings.Contains(got, expected) {
			t.Errorf("unexpected errors: expected=%q, got=%q", expected, got)
		}
	}

	sysctls = make([]core.Sysctl, len(duplicates))
	for i, sysctl := range duplicates {
		sysctls[i].Name = sysctl
	}
	errs = validateSysctls(sysctls, field.NewPath("foo"))
	if len(errs) != 1 {
		t.Errorf("unexpected validation errors: %v", errs)
	} else if errs[0].Type != field.ErrorTypeDuplicate {
		t.Errorf("expected error type %v, got %v", field.ErrorTypeDuplicate, errs[0].Type)
	}
}

func newNodeNameEndpoint(nodeName string) *core.Endpoints {
	ep := &core.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "foo",
			Namespace:       metav1.NamespaceDefault,
			ResourceVersion: "1",
		},
		Subsets: []core.EndpointSubset{
			{
				NotReadyAddresses: []core.EndpointAddress{},
				Ports:             []core.EndpointPort{{Name: "https", Port: 443, Protocol: "TCP"}},
				Addresses: []core.EndpointAddress{
					{
						IP:       "8.8.8.8",
						Hostname: "zookeeper1",
						NodeName: &nodeName}}}}}
	return ep
}

func TestEndpointAddressNodeNameUpdateRestrictions(t *testing.T) {
	oldEndpoint := newNodeNameEndpoint("kubernetes-node-setup-by-backend")
	updatedEndpoint := newNodeNameEndpoint("kubernetes-changed-nodename")
	// Check that NodeName can be changed during update, this is to accommodate the case where nodeIP or PodCIDR is reused.
	// The same ip will now have a different nodeName.
	errList := ValidateEndpoints(updatedEndpoint)
	errList = append(errList, ValidateEndpointsUpdate(updatedEndpoint, oldEndpoint)...)
	if len(errList) != 0 {
		t.Error("Endpoint should allow changing of Subset.Addresses.NodeName on update")
	}
}

func TestEndpointAddressNodeNameInvalidDNSSubdomain(t *testing.T) {
	// Check NodeName DNS validation
	endpoint := newNodeNameEndpoint("illegal*.nodename")
	errList := ValidateEndpoints(endpoint)
	if len(errList) == 0 {
		t.Error("Endpoint should reject invalid NodeName")
	}
}

func TestEndpointAddressNodeNameCanBeAnIPAddress(t *testing.T) {
	endpoint := newNodeNameEndpoint("10.10.1.1")
	errList := ValidateEndpoints(endpoint)
	if len(errList) != 0 {
		t.Error("Endpoint should accept a NodeName that is an IP address")
	}
}

func TestValidateFlexVolumeSource(t *testing.T) {
	testcases := map[string]struct {
		source       *core.FlexVolumeSource
		expectedErrs map[string]string
	}{
		"valid": {
			source:       &core.FlexVolumeSource{Driver: "foo"},
			expectedErrs: map[string]string{},
		},
		"valid with options": {
			source:       &core.FlexVolumeSource{Driver: "foo", Options: map[string]string{"foo": "bar"}},
			expectedErrs: map[string]string{},
		},
		"no driver": {
			source:       &core.FlexVolumeSource{Driver: ""},
			expectedErrs: map[string]string{"driver": "Required value"},
		},
		"reserved option keys": {
			source: &core.FlexVolumeSource{
				Driver: "foo",
				Options: map[string]string{
					// valid options
					"myns.io":               "A",
					"myns.io/bar":           "A",
					"myns.io/kubernetes.io": "A",

					// invalid options
					"KUBERNETES.IO":     "A",
					"kubernetes.io":     "A",
					"kubernetes.io/":    "A",
					"kubernetes.io/foo": "A",

					"alpha.kubernetes.io":     "A",
					"alpha.kubernetes.io/":    "A",
					"alpha.kubernetes.io/foo": "A",

					"k8s.io":     "A",
					"k8s.io/":    "A",
					"k8s.io/foo": "A",

					"alpha.k8s.io":     "A",
					"alpha.k8s.io/":    "A",
					"alpha.k8s.io/foo": "A",
				},
			},
			expectedErrs: map[string]string{
				"options[KUBERNETES.IO]":           "reserved",
				"options[kubernetes.io]":           "reserved",
				"options[kubernetes.io/]":          "reserved",
				"options[kubernetes.io/foo]":       "reserved",
				"options[alpha.kubernetes.io]":     "reserved",
				"options[alpha.kubernetes.io/]":    "reserved",
				"options[alpha.kubernetes.io/foo]": "reserved",
				"options[k8s.io]":                  "reserved",
				"options[k8s.io/]":                 "reserved",
				"options[k8s.io/foo]":              "reserved",
				"options[alpha.k8s.io]":            "reserved",
				"options[alpha.k8s.io/]":           "reserved",
				"options[alpha.k8s.io/foo]":        "reserved",
			},
		},
	}

	for k, tc := range testcases {
		errs := validateFlexVolumeSource(tc.source, nil)
		for _, err := range errs {
			expectedErr, ok := tc.expectedErrs[err.Field]
			if !ok {
				t.Errorf("%s: unexpected err on field %s: %v", k, err.Field, err)
				continue
			}
			if !strings.Contains(err.Error(), expectedErr) {
				t.Errorf("%s: expected err on field %s to contain '%s', was %v", k, err.Field, expectedErr, err.Error())
				continue
			}
		}
		if len(errs) != len(tc.expectedErrs) {
			t.Errorf("%s: expected errs %#v, got %#v", k, tc.expectedErrs, errs)
			continue
		}
	}
}

func TestValidateOrSetClientIPAffinityConfig(t *testing.T) {
	successCases := map[string]*core.SessionAffinityConfig{
		"non-empty config, valid timeout: 1": {
			ClientIP: &core.ClientIPConfig{
				TimeoutSeconds: utilpointer.Int32Ptr(1),
			},
		},
		"non-empty config, valid timeout: core.MaxClientIPServiceAffinitySeconds-1": {
			ClientIP: &core.ClientIPConfig{
				TimeoutSeconds: utilpointer.Int32Ptr(core.MaxClientIPServiceAffinitySeconds - 1),
			},
		},
		"non-empty config, valid timeout: core.MaxClientIPServiceAffinitySeconds": {
			ClientIP: &core.ClientIPConfig{
				TimeoutSeconds: utilpointer.Int32Ptr(core.MaxClientIPServiceAffinitySeconds),
			},
		},
	}

	for name, test := range successCases {
		if errs := validateClientIPAffinityConfig(test, field.NewPath("field")); len(errs) != 0 {
			t.Errorf("case: %s, expected success: %v", name, errs)
		}
	}

	errorCases := map[string]*core.SessionAffinityConfig{
		"empty session affinity config": nil,
		"empty client IP config": {
			ClientIP: nil,
		},
		"empty timeoutSeconds": {
			ClientIP: &core.ClientIPConfig{
				TimeoutSeconds: nil,
			},
		},
		"non-empty config, invalid timeout: core.MaxClientIPServiceAffinitySeconds+1": {
			ClientIP: &core.ClientIPConfig{
				TimeoutSeconds: utilpointer.Int32Ptr(core.MaxClientIPServiceAffinitySeconds + 1),
			},
		},
		"non-empty config, invalid timeout: -1": {
			ClientIP: &core.ClientIPConfig{
				TimeoutSeconds: utilpointer.Int32Ptr(-1),
			},
		},
		"non-empty config, invalid timeout: 0": {
			ClientIP: &core.ClientIPConfig{
				TimeoutSeconds: utilpointer.Int32Ptr(0),
			},
		},
	}

	for name, test := range errorCases {
		if errs := validateClientIPAffinityConfig(test, field.NewPath("field")); len(errs) == 0 {
			t.Errorf("case: %v, expected failures: %v", name, errs)
		}
	}
}

func TestValidateWindowsSecurityContextOptions(t *testing.T) {
	toPtr := func(s string) *string {
		return &s
	}

	testCases := []struct {
		testName string

		windowsOptions         *core.WindowsSecurityContextOptions
		expectedErrorSubstring string
	}{
		{
			testName: "a nil pointer",
		},
		{
			testName:       "an empty struct",
			windowsOptions: &core.WindowsSecurityContextOptions{},
		},
		{
			testName: "a valid input",
			windowsOptions: &core.WindowsSecurityContextOptions{
				GMSACredentialSpecName: toPtr("dummy-gmsa-crep-spec-name"),
				GMSACredentialSpec:     toPtr("dummy-gmsa-crep-spec-contents"),
			},
		},
		{
			testName: "a GMSA cred spec name that is not a valid resource name",
			windowsOptions: &core.WindowsSecurityContextOptions{
				// invalid because of the underscore
				GMSACredentialSpecName: toPtr("not_a-valid-gmsa-crep-spec-name"),
			},
			expectedErrorSubstring: dnsSubdomainLabelErrMsg,
		},
		{
			testName: "empty GMSA cred spec contents",
			windowsOptions: &core.WindowsSecurityContextOptions{
				GMSACredentialSpec: toPtr(""),
			},
			expectedErrorSubstring: "gmsaCredentialSpec cannot be an empty string",
		},
		{
			testName: "GMSA cred spec contents that are too long",
			windowsOptions: &core.WindowsSecurityContextOptions{
				GMSACredentialSpec: toPtr(strings.Repeat("a", maxGMSACredentialSpecLength+1)),
			},
			expectedErrorSubstring: "gmsaCredentialSpec size must be under",
		},
		{
			testName: "RunAsUserName is nil",
			windowsOptions: &core.WindowsSecurityContextOptions{
				RunAsUserName: nil,
			},
		},
		{
			testName: "a valid RunAsUserName",
			windowsOptions: &core.WindowsSecurityContextOptions{
				RunAsUserName: toPtr("Container. User"),
			},
		},
		{
			testName: "a valid RunAsUserName with NetBios Domain",
			windowsOptions: &core.WindowsSecurityContextOptions{
				RunAsUserName: toPtr("Network Service\\Container. User"),
			},
		},
		{
			testName: "a valid RunAsUserName with DNS Domain",
			windowsOptions: &core.WindowsSecurityContextOptions{
				RunAsUserName: toPtr(strings.Repeat("fOo", 20) + ".liSH\\Container. User"),
			},
		},
		{
			testName: "a valid RunAsUserName with DNS Domain with a single character segment",
			windowsOptions: &core.WindowsSecurityContextOptions{
				RunAsUserName: toPtr(strings.Repeat("fOo", 20) + ".l\\Container. User"),
			},
		},
		{
			testName: "a valid RunAsUserName with a long single segment DNS Domain",
			windowsOptions: &core.WindowsSecurityContextOptions{
				RunAsUserName: toPtr(strings.Repeat("a", 42) + "\\Container. User"),
			},
		},
		{
			testName: "an empty RunAsUserName",
			windowsOptions: &core.WindowsSecurityContextOptions{
				RunAsUserName: toPtr(""),
			},
			expectedErrorSubstring: "runAsUserName cannot be an empty string",
		},
		{
			testName: "RunAsUserName containing a control character",
			windowsOptions: &core.WindowsSecurityContextOptions{
				RunAsUserName: toPtr("Container\tUser"),
			},
			expectedErrorSubstring: "runAsUserName cannot contain control characters",
		},
		{
			testName: "RunAsUserName containing too many backslashes",
			windowsOptions: &core.WindowsSecurityContextOptions{
				RunAsUserName: toPtr("Container\\Foo\\Lish"),
			},
			expectedErrorSubstring: "runAsUserName cannot contain more than one backslash",
		},
		{
			testName: "RunAsUserName containing backslash but empty Domain",
			windowsOptions: &core.WindowsSecurityContextOptions{
				RunAsUserName: toPtr("\\User"),
			},
			expectedErrorSubstring: "runAsUserName's Domain doesn't match the NetBios nor the DNS format",
		},
		{
			testName: "RunAsUserName containing backslash but empty User",
			windowsOptions: &core.WindowsSecurityContextOptions{
				RunAsUserName: toPtr("Container\\"),
			},
			expectedErrorSubstring: "runAsUserName's User cannot be empty",
		},
		{
			testName: "RunAsUserName's NetBios Domain is too long",
			windowsOptions: &core.WindowsSecurityContextOptions{
				RunAsUserName: toPtr("NetBios " + strings.Repeat("a", 8) + "\\user"),
			},
			expectedErrorSubstring: "runAsUserName's Domain doesn't match the NetBios",
		},
		{
			testName: "RunAsUserName's DNS Domain is too long",
			windowsOptions: &core.WindowsSecurityContextOptions{
				// even if this tests the max Domain length, the Domain should still be "valid".
				RunAsUserName: toPtr(strings.Repeat(strings.Repeat("a", 63)+".", 4)[:253] + ".com\\user"),
			},
			expectedErrorSubstring: "runAsUserName's Domain length must be under",
		},
		{
			testName: "RunAsUserName's User is too long",
			windowsOptions: &core.WindowsSecurityContextOptions{
				RunAsUserName: toPtr(strings.Repeat("a", maxRunAsUserNameUserLength+1)),
			},
			expectedErrorSubstring: "runAsUserName's User length must not be longer than",
		},
		{
			testName: "RunAsUserName's User cannot contain only spaces or periods",
			windowsOptions: &core.WindowsSecurityContextOptions{
				RunAsUserName: toPtr("... ..."),
			},
			expectedErrorSubstring: "runAsUserName's User cannot contain only periods or spaces",
		},
		{
			testName: "RunAsUserName's NetBios Domain cannot start with a dot",
			windowsOptions: &core.WindowsSecurityContextOptions{
				RunAsUserName: toPtr(".FooLish\\User"),
			},
			expectedErrorSubstring: "runAsUserName's Domain doesn't match the NetBios",
		},
		{
			testName: "RunAsUserName's NetBios Domain cannot contain invalid characters",
			windowsOptions: &core.WindowsSecurityContextOptions{
				RunAsUserName: toPtr("Foo? Lish?\\User"),
			},
			expectedErrorSubstring: "runAsUserName's Domain doesn't match the NetBios",
		},
		{
			testName: "RunAsUserName's DNS Domain cannot contain invalid characters",
			windowsOptions: &core.WindowsSecurityContextOptions{
				RunAsUserName: toPtr(strings.Repeat("a", 32) + ".com-\\user"),
			},
			expectedErrorSubstring: "runAsUserName's Domain doesn't match the NetBios nor the DNS format",
		},
		{
			testName: "RunAsUserName's User cannot contain invalid characters",
			windowsOptions: &core.WindowsSecurityContextOptions{
				RunAsUserName: toPtr("Container/User"),
			},
			expectedErrorSubstring: "runAsUserName's User cannot contain the following characters",
		},
	}

	for _, testCase := range testCases {
		t.Run("validateWindowsSecurityContextOptions with"+testCase.testName, func(t *testing.T) {
			errs := validateWindowsSecurityContextOptions(testCase.windowsOptions, field.NewPath("field"))

			switch len(errs) {
			case 0:
				if testCase.expectedErrorSubstring != "" {
					t.Errorf("expected a failure containing the substring: %q", testCase.expectedErrorSubstring)
				}
			case 1:
				if testCase.expectedErrorSubstring == "" {
					t.Errorf("didn't expect a failure, got: %q", errs[0].Error())
				} else if !strings.Contains(errs[0].Error(), testCase.expectedErrorSubstring) {
					t.Errorf("expected a failure with the substring %q, got %q instead", testCase.expectedErrorSubstring, errs[0].Error())
				}
			default:
				t.Errorf("got %d failures", len(errs))
				for i, err := range errs {
					t.Errorf("error %d: %q", i, err.Error())
				}
			}
		})
	}
}

func testDataSourceInSpec(name, kind, apiGroup string) *core.PersistentVolumeClaimSpec {
	scName := "csi-plugin"
	dataSourceInSpec := core.PersistentVolumeClaimSpec{
		AccessModes: []core.PersistentVolumeAccessMode{
			core.ReadOnlyMany,
		},
		Resources: core.ResourceRequirements{
			Requests: core.ResourceList{
				core.ResourceName(core.ResourceStorage): resource.MustParse("10G"),
			},
		},
		StorageClassName: &scName,
		DataSource: &core.TypedLocalObjectReference{
			APIGroup: &apiGroup,
			Kind:     kind,
			Name:     name,
		},
	}

	return &dataSourceInSpec
}

func TestAlphaVolumePVCDataSource(t *testing.T) {
	testCases := []struct {
		testName     string
		claimSpec    core.PersistentVolumeClaimSpec
		expectedFail bool
	}{
		{
			testName:  "test create from valid snapshot source",
			claimSpec: *testDataSourceInSpec("test_snapshot", "VolumeSnapshot", "snapshot.storage.k8s.io"),
		},
		{
			testName:  "test create from valid pvc source",
			claimSpec: *testDataSourceInSpec("test_pvc", "PersistentVolumeClaim", ""),
		},
		{
			testName:     "test missing name in snapshot datasource should fail",
			claimSpec:    *testDataSourceInSpec("", "VolumeSnapshot", "snapshot.storage.k8s.io"),
			expectedFail: true,
		},
		{
			testName:     "test missing kind in snapshot datasource should fail",
			claimSpec:    *testDataSourceInSpec("test_snapshot", "", "snapshot.storage.k8s.io"),
			expectedFail: true,
		},
		{
			testName:  "test create from valid generic custom resource source",
			claimSpec: *testDataSourceInSpec("test_generic", "Generic", "generic.storage.k8s.io"),
		},
		{
			testName:     "test invalid datasource should fail",
			claimSpec:    *testDataSourceInSpec("test_pod", "Pod", ""),
			expectedFail: true,
		},
	}

	for _, tc := range testCases {
		opts := PersistentVolumeClaimSpecValidationOptions{}
		if tc.expectedFail {
			if errs := ValidatePersistentVolumeClaimSpec(&tc.claimSpec, field.NewPath("spec"), opts); len(errs) == 0 {
				t.Errorf("expected failure: %v", errs)
			}

		} else {
			if errs := ValidatePersistentVolumeClaimSpec(&tc.claimSpec, field.NewPath("spec"), opts); len(errs) != 0 {
				t.Errorf("expected success: %v", errs)
			}
		}
	}
}

func testAnyDataSource(t *testing.T, ds, dsRef bool) {
	testCases := []struct {
		testName     string
		claimSpec    core.PersistentVolumeClaimSpec
		expectedFail bool
	}{
		{
			testName:  "test create from valid snapshot source",
			claimSpec: *testDataSourceInSpec("test_snapshot", "VolumeSnapshot", "snapshot.storage.k8s.io"),
		},
		{
			testName:  "test create from valid pvc source",
			claimSpec: *testDataSourceInSpec("test_pvc", "PersistentVolumeClaim", ""),
		},
		{
			testName:     "test missing name in snapshot datasource should fail",
			claimSpec:    *testDataSourceInSpec("", "VolumeSnapshot", "snapshot.storage.k8s.io"),
			expectedFail: true,
		},
		{
			testName:     "test missing kind in snapshot datasource should fail",
			claimSpec:    *testDataSourceInSpec("test_snapshot", "", "snapshot.storage.k8s.io"),
			expectedFail: true,
		},
		{
			testName:  "test create from valid generic custom resource source",
			claimSpec: *testDataSourceInSpec("test_generic", "Generic", "generic.storage.k8s.io"),
		},
		{
			testName:     "test invalid datasource should fail",
			claimSpec:    *testDataSourceInSpec("test_pod", "Pod", ""),
			expectedFail: true,
		},
	}

	for _, tc := range testCases {
		if dsRef {
			tc.claimSpec.DataSourceRef = tc.claimSpec.DataSource.DeepCopy()
		}
		if !ds {
			tc.claimSpec.DataSource = nil
		}
		opts := PersistentVolumeClaimSpecValidationOptions{}
		if tc.expectedFail {
			if errs := ValidatePersistentVolumeClaimSpec(&tc.claimSpec, field.NewPath("spec"), opts); len(errs) == 0 {
				t.Errorf("expected failure: %v", errs)
			}
		} else {
			if errs := ValidatePersistentVolumeClaimSpec(&tc.claimSpec, field.NewPath("spec"), opts); len(errs) != 0 {
				t.Errorf("expected success: %v", errs)
			}
		}
	}
}

func TestAnyDataSource(t *testing.T) {
	testAnyDataSource(t, true, false)
	testAnyDataSource(t, false, true)
	testAnyDataSource(t, true, false)
}

func TestValidateTopologySpreadConstraints(t *testing.T) {
	testCases := []struct {
		name        string
		constraints []core.TopologySpreadConstraint
		errtype     field.ErrorType
		errfield    string
	}{
		{
			name: "all required fields ok",
			constraints: []core.TopologySpreadConstraint{
				{MaxSkew: 1, TopologyKey: "k8s.io/zone", WhenUnsatisfiable: core.DoNotSchedule},
			},
		},
		{
			name: "missing MaxSkew",
			constraints: []core.TopologySpreadConstraint{
				{TopologyKey: "k8s.io/zone", WhenUnsatisfiable: core.DoNotSchedule},
			},
			errtype:  field.ErrorTypeInvalid,
			errfield: "maxSkew",
		},
		{
			name: "invalid MaxSkew",
			constraints: []core.TopologySpreadConstraint{
				{MaxSkew: 0, TopologyKey: "k8s.io/zone", WhenUnsatisfiable: core.DoNotSchedule},
			},
			errtype:  field.ErrorTypeInvalid,
			errfield: "maxSkew",
		},
		{
			name: "missing TopologyKey",
			constraints: []core.TopologySpreadConstraint{
				{MaxSkew: 1, WhenUnsatisfiable: core.DoNotSchedule},
			},
			errtype:  field.ErrorTypeRequired,
			errfield: "topologyKey",
		},
		{
			name: "missing scheduling mode",
			constraints: []core.TopologySpreadConstraint{
				{MaxSkew: 1, TopologyKey: "k8s.io/zone"},
			},
			errtype:  field.ErrorTypeNotSupported,
			errfield: "whenUnsatisfiable",
		},
		{
			name: "unsupported scheduling mode",
			constraints: []core.TopologySpreadConstraint{
				{MaxSkew: 1, TopologyKey: "k8s.io/zone", WhenUnsatisfiable: core.UnsatisfiableConstraintAction("N/A")},
			},
			errtype:  field.ErrorTypeNotSupported,
			errfield: "whenUnsatisfiable",
		},
		{
			name: "multiple constraints ok with all required fields",
			constraints: []core.TopologySpreadConstraint{
				{MaxSkew: 1, TopologyKey: "k8s.io/zone", WhenUnsatisfiable: core.DoNotSchedule},
				{MaxSkew: 2, TopologyKey: "k8s.io/node", WhenUnsatisfiable: core.ScheduleAnyway},
			},
		},
		{
			name: "multiple constraints missing TopologyKey on partial ones",
			constraints: []core.TopologySpreadConstraint{
				{MaxSkew: 1, TopologyKey: "k8s.io/zone", WhenUnsatisfiable: core.DoNotSchedule},
				{MaxSkew: 2, WhenUnsatisfiable: core.ScheduleAnyway},
			},
			errtype:  field.ErrorTypeRequired,
			errfield: "topologyKey",
		},
		{
			name: "duplicate constraints",
			constraints: []core.TopologySpreadConstraint{
				{MaxSkew: 1, TopologyKey: "k8s.io/zone", WhenUnsatisfiable: core.DoNotSchedule},
				{MaxSkew: 2, TopologyKey: "k8s.io/zone", WhenUnsatisfiable: core.DoNotSchedule},
			},
			errtype:  field.ErrorTypeDuplicate,
			errfield: "{topologyKey, whenUnsatisfiable}",
		},
	}

	for i, tc := range testCases {
		errs := validateTopologySpreadConstraints(tc.constraints, field.NewPath("field"))

		if len(errs) > 0 && tc.errtype == "" {
			t.Errorf("[%d: %q] unexpected error(s): %v", i, tc.name, errs)
		} else if len(errs) == 0 && tc.errtype != "" {
			t.Errorf("[%d: %q] expected error type %v", i, tc.name, tc.errtype)
		} else if len(errs) >= 1 {
			if errs[0].Type != tc.errtype {
				t.Errorf("[%d: %q] expected error type %v, got %v", i, tc.name, tc.errtype, errs[0].Type)
			} else if !strings.HasSuffix(errs[0].Field, "."+tc.errfield) {
				t.Errorf("[%d: %q] expected error on field %q, got %q", i, tc.name, tc.errfield, errs[0].Field)
			}
		}
	}
}

func TestValidateOverhead(t *testing.T) {
	successCase := []struct {
		Name     string
		overhead core.ResourceList
	}{
		{
			Name: "Valid Overhead for CPU + Memory",
			overhead: core.ResourceList{
				core.ResourceName(core.ResourceCPU):    resource.MustParse("10"),
				core.ResourceName(core.ResourceMemory): resource.MustParse("10G"),
			},
		},
	}
	for _, tc := range successCase {
		if errs := validateOverhead(tc.overhead, field.NewPath("overheads"), PodValidationOptions{}); len(errs) != 0 {
			t.Errorf("%q unexpected error: %v", tc.Name, errs)
		}
	}

	errorCase := []struct {
		Name     string
		overhead core.ResourceList
	}{
		{
			Name: "Invalid Overhead Resources",
			overhead: core.ResourceList{
				core.ResourceName("my.org"): resource.MustParse("10m"),
			},
		},
	}
	for _, tc := range errorCase {
		if errs := validateOverhead(tc.overhead, field.NewPath("resources"), PodValidationOptions{}); len(errs) == 0 {
			t.Errorf("%q expected error", tc.Name)
		}
	}
}

// helper creates a pod with name, namespace and IPs
func makePod(podName string, podNamespace string, podIPs []core.PodIP) core.Pod {
	return core.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: podName, Namespace: podNamespace},
		Spec: core.PodSpec{
			Containers: []core.Container{
				{
					Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File",
				},
			},
			RestartPolicy: core.RestartPolicyAlways,
			DNSPolicy:     core.DNSClusterFirst,
		},
		Status: core.PodStatus{
			PodIPs: podIPs,
		},
	}
}
func TestPodIPsValidation(t *testing.T) {
	testCases := []struct {
		pod         core.Pod
		expectError bool
	}{
		{
			expectError: false,
			pod:         makePod("nil-ips", "ns", nil),
		},
		{
			expectError: false,
			pod:         makePod("empty-podips-list", "ns", []core.PodIP{}),
		},
		{
			expectError: false,
			pod:         makePod("single-ip-family-6", "ns", []core.PodIP{{IP: "::1"}}),
		},
		{
			expectError: false,
			pod:         makePod("single-ip-family-4", "ns", []core.PodIP{{IP: "1.1.1.1"}}),
		},
		{
			expectError: false,
			pod:         makePod("dual-stack-4-6", "ns", []core.PodIP{{IP: "1.1.1.1"}, {IP: "::1"}}),
		},
		{
			expectError: false,
			pod:         makePod("dual-stack-6-4", "ns", []core.PodIP{{IP: "::1"}, {IP: "1.1.1.1"}}),
		},
		/* failure cases start here */
		{
			expectError: true,
			pod:         makePod("invalid-pod-ip", "ns", []core.PodIP{{IP: "this-is-not-an-ip"}}),
		},
		{
			expectError: true,
			pod:         makePod("dualstack-same-ip-family-6", "ns", []core.PodIP{{IP: "::1"}, {IP: "::2"}}),
		},
		{
			expectError: true,
			pod:         makePod("dualstack-same-ip-family-4", "ns", []core.PodIP{{IP: "1.1.1.1"}, {IP: "2.2.2.2"}}),
		},
		{
			expectError: true,
			pod:         makePod("dualstack-repeated-ip-family-6", "ns", []core.PodIP{{IP: "1.1.1.1"}, {IP: "::1"}, {IP: "::2"}}),
		},
		{
			expectError: true,
			pod:         makePod("dualstack-repeated-ip-family-4", "ns", []core.PodIP{{IP: "1.1.1.1"}, {IP: "::1"}, {IP: "2.2.2.2"}}),
		},

		{
			expectError: true,
			pod:         makePod("dualstack-duplicate-ip-family-4", "ns", []core.PodIP{{IP: "1.1.1.1"}, {IP: "1.1.1.1"}, {IP: "::1"}}),
		},
		{
			expectError: true,
			pod:         makePod("dualstack-duplicate-ip-family-6", "ns", []core.PodIP{{IP: "1.1.1.1"}, {IP: "::1"}, {IP: "::1"}}),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.pod.Name, func(t *testing.T) {
			for _, oldTestCase := range testCases {
				newPod := testCase.pod.DeepCopy()
				newPod.ResourceVersion = "1"

				oldPod := oldTestCase.pod.DeepCopy()
				oldPod.ResourceVersion = "1"
				oldPod.Name = newPod.Name

				errs := ValidatePodStatusUpdate(newPod, oldPod, PodValidationOptions{})

				if len(errs) == 0 && testCase.expectError {
					t.Fatalf("expected failure for %s, but there were none", testCase.pod.Name)
				}
				if len(errs) != 0 && !testCase.expectError {
					t.Fatalf("expected success for %s, but there were errors: %v", testCase.pod.Name, errs)
				}
			}
		})
	}
}

func TestValidateSeccompAnnotationAndField(t *testing.T) {
	const containerName = "container"
	testProfile := "test"

	for _, test := range []struct {
		description string
		pod         *core.Pod
		validation  func(*testing.T, string, field.ErrorList, *v1.Pod)
	}{
		{
			description: "Field type unconfined and annotation does not match",
			pod: &core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						v1.SeccompPodAnnotationKey: "not-matching",
					},
				},
				Spec: core.PodSpec{
					SecurityContext: &core.PodSecurityContext{
						SeccompProfile: &core.SeccompProfile{
							Type: core.SeccompProfileTypeUnconfined,
						},
					},
				},
			},
			validation: func(t *testing.T, desc string, allErrs field.ErrorList, pod *v1.Pod) {
				require.NotNil(t, allErrs, desc)
			},
		},
		{
			description: "Field type default and annotation does not match",
			pod: &core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						v1.SeccompPodAnnotationKey: "not-matching",
					},
				},
				Spec: core.PodSpec{
					SecurityContext: &core.PodSecurityContext{
						SeccompProfile: &core.SeccompProfile{
							Type: core.SeccompProfileTypeRuntimeDefault,
						},
					},
				},
			},
			validation: func(t *testing.T, desc string, allErrs field.ErrorList, pod *v1.Pod) {
				require.NotNil(t, allErrs, desc)
			},
		},
		{
			description: "Field type localhost and annotation does not match",
			pod: &core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						v1.SeccompPodAnnotationKey: "not-matching",
					},
				},
				Spec: core.PodSpec{
					SecurityContext: &core.PodSecurityContext{
						SeccompProfile: &core.SeccompProfile{
							Type:             core.SeccompProfileTypeLocalhost,
							LocalhostProfile: &testProfile,
						},
					},
				},
			},
			validation: func(t *testing.T, desc string, allErrs field.ErrorList, pod *v1.Pod) {
				require.NotNil(t, allErrs, desc)
			},
		},
		{
			description: "Field type localhost and localhost/ prefixed annotation does not match",
			pod: &core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						v1.SeccompPodAnnotationKey: "localhost/not-matching",
					},
				},
				Spec: core.PodSpec{
					SecurityContext: &core.PodSecurityContext{
						SeccompProfile: &core.SeccompProfile{
							Type:             core.SeccompProfileTypeLocalhost,
							LocalhostProfile: &testProfile,
						},
					},
				},
			},
			validation: func(t *testing.T, desc string, allErrs field.ErrorList, pod *v1.Pod) {
				require.NotNil(t, allErrs, desc)
			},
		},
		{
			description: "Field type unconfined and annotation does not match (container)",
			pod: &core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						v1.SeccompContainerAnnotationKeyPrefix + containerName: "not-matching",
					},
				},
				Spec: core.PodSpec{
					Containers: []core.Container{{
						Name: containerName,
						SecurityContext: &core.SecurityContext{
							SeccompProfile: &core.SeccompProfile{
								Type: core.SeccompProfileTypeUnconfined,
							},
						},
					}},
				},
			},
			validation: func(t *testing.T, desc string, allErrs field.ErrorList, pod *v1.Pod) {
				require.NotNil(t, allErrs, desc)
			},
		},
		{
			description: "Field type default and annotation does not match (container)",
			pod: &core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						v1.SeccompContainerAnnotationKeyPrefix + containerName: "not-matching",
					},
				},
				Spec: core.PodSpec{
					Containers: []core.Container{{
						Name: containerName,
						SecurityContext: &core.SecurityContext{
							SeccompProfile: &core.SeccompProfile{
								Type: core.SeccompProfileTypeRuntimeDefault,
							},
						},
					}},
				},
			},
			validation: func(t *testing.T, desc string, allErrs field.ErrorList, pod *v1.Pod) {
				require.NotNil(t, allErrs, desc)
			},
		},
		{
			description: "Field type localhost and annotation does not match (container)",
			pod: &core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						v1.SeccompContainerAnnotationKeyPrefix + containerName: "not-matching",
					},
				},
				Spec: core.PodSpec{
					Containers: []core.Container{{
						Name: containerName,
						SecurityContext: &core.SecurityContext{
							SeccompProfile: &core.SeccompProfile{
								Type:             core.SeccompProfileTypeLocalhost,
								LocalhostProfile: &testProfile,
							},
						},
					}},
				},
			},
			validation: func(t *testing.T, desc string, allErrs field.ErrorList, pod *v1.Pod) {
				require.NotNil(t, allErrs, desc)
			},
		},
		{
			description: "Field type localhost and localhost/ prefixed annotation does not match (container)",
			pod: &core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						v1.SeccompContainerAnnotationKeyPrefix + containerName: "localhost/not-matching",
					},
				},
				Spec: core.PodSpec{
					Containers: []core.Container{{
						Name: containerName,
						SecurityContext: &core.SecurityContext{
							SeccompProfile: &core.SeccompProfile{
								Type:             core.SeccompProfileTypeLocalhost,
								LocalhostProfile: &testProfile,
							},
						},
					}},
				},
			},
			validation: func(t *testing.T, desc string, allErrs field.ErrorList, pod *v1.Pod) {
				require.NotNil(t, allErrs, desc)
			},
		},
		{
			description: "Nil errors must not be appended (pod)",
			pod: &core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						v1.SeccompPodAnnotationKey: "localhost/anyprofile",
					},
				},
				Spec: core.PodSpec{
					SecurityContext: &core.PodSecurityContext{
						SeccompProfile: &core.SeccompProfile{
							Type: "Abc",
						},
					},
					Containers: []core.Container{{
						Name: containerName,
					}},
				},
			},
			validation: func(t *testing.T, desc string, allErrs field.ErrorList, pod *v1.Pod) {
				require.Empty(t, allErrs, desc)
			},
		},
		{
			description: "Nil errors must not be appended (container)",
			pod: &core.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						v1.SeccompContainerAnnotationKeyPrefix + containerName: "localhost/not-matching",
					},
				},
				Spec: core.PodSpec{
					Containers: []core.Container{{
						SecurityContext: &core.SecurityContext{
							SeccompProfile: &core.SeccompProfile{
								Type: "Abc",
							},
						},
						Name: containerName,
					}},
				},
			},
			validation: func(t *testing.T, desc string, allErrs field.ErrorList, pod *v1.Pod) {
				require.Empty(t, allErrs, desc)
			},
		},
	} {
		output := &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{}},
		}
		for i, ctr := range test.pod.Spec.Containers {
			output.Spec.Containers = append(output.Spec.Containers, v1.Container{})
			if ctr.SecurityContext != nil && ctr.SecurityContext.SeccompProfile != nil {
				output.Spec.Containers[i].SecurityContext = &v1.SecurityContext{
					SeccompProfile: &v1.SeccompProfile{
						Type:             v1.SeccompProfileType(ctr.SecurityContext.SeccompProfile.Type),
						LocalhostProfile: ctr.SecurityContext.SeccompProfile.LocalhostProfile,
					},
				}
			}
		}
		errList := validateSeccompAnnotationsAndFields(test.pod.ObjectMeta, &test.pod.Spec, field.NewPath(""))
		test.validation(t, test.description, errList, output)
	}
}

func TestValidateSeccompAnnotationsAndFieldsMatch(t *testing.T) {
	rootFld := field.NewPath("")
	tests := []struct {
		description     string
		annotationValue string
		seccompField    *core.SeccompProfile
		fldPath         *field.Path
		expectedErr     *field.Error
	}{
		{
			description: "seccompField nil should return empty",
			expectedErr: nil,
		},
		{
			description:     "unconfined annotation and SeccompProfileTypeUnconfined should return empty",
			annotationValue: "unconfined",
			seccompField:    &core.SeccompProfile{Type: core.SeccompProfileTypeUnconfined},
			expectedErr:     nil,
		},
		{
			description:     "runtime/default annotation and SeccompProfileTypeRuntimeDefault should return empty",
			annotationValue: "runtime/default",
			seccompField:    &core.SeccompProfile{Type: core.SeccompProfileTypeRuntimeDefault},
			expectedErr:     nil,
		},
		{
			description:     "docker/default annotation and SeccompProfileTypeRuntimeDefault should return empty",
			annotationValue: "docker/default",
			seccompField:    &core.SeccompProfile{Type: core.SeccompProfileTypeRuntimeDefault},
			expectedErr:     nil,
		},
		{
			description:     "localhost/test.json annotation and SeccompProfileTypeLocalhost with correct profile should return empty",
			annotationValue: "localhost/test.json",
			seccompField:    &core.SeccompProfile{Type: core.SeccompProfileTypeLocalhost, LocalhostProfile: utilpointer.StringPtr("test.json")},
			expectedErr:     nil,
		},
		{
			description:     "localhost/test.json annotation and SeccompProfileTypeLocalhost without profile should error",
			annotationValue: "localhost/test.json",
			seccompField:    &core.SeccompProfile{Type: core.SeccompProfileTypeLocalhost},
			fldPath:         rootFld,
			expectedErr:     field.Forbidden(rootFld.Child("localhostProfile"), "seccomp profile in annotation and field must match"),
		},
		{
			description:     "localhost/test.json annotation and SeccompProfileTypeLocalhost with different profile should error",
			annotationValue: "localhost/test.json",
			seccompField:    &core.SeccompProfile{Type: core.SeccompProfileTypeLocalhost, LocalhostProfile: utilpointer.StringPtr("different.json")},
			fldPath:         rootFld,
			expectedErr:     field.Forbidden(rootFld.Child("localhostProfile"), "seccomp profile in annotation and field must match"),
		},
		{
			description:     "localhost/test.json annotation and SeccompProfileTypeUnconfined with different profile should error",
			annotationValue: "localhost/test.json",
			seccompField:    &core.SeccompProfile{Type: core.SeccompProfileTypeUnconfined},
			fldPath:         rootFld,
			expectedErr:     field.Forbidden(rootFld.Child("type"), "seccomp type in annotation and field must match"),
		},
		{
			description:     "localhost/test.json annotation and SeccompProfileTypeRuntimeDefault with different profile should error",
			annotationValue: "localhost/test.json",
			seccompField:    &core.SeccompProfile{Type: core.SeccompProfileTypeRuntimeDefault},
			fldPath:         rootFld,
			expectedErr:     field.Forbidden(rootFld.Child("type"), "seccomp type in annotation and field must match"),
		},
	}

	for i, test := range tests {
		err := validateSeccompAnnotationsAndFieldsMatch(test.annotationValue, test.seccompField, test.fldPath)
		asserttestify.Equal(t, test.expectedErr, err, "TestCase[%d]: %s", i, test.description)
	}
}

func TestValidatePodTemplateSpecSeccomp(t *testing.T) {
	rootFld := field.NewPath("template")
	tests := []struct {
		description string
		spec        *core.PodTemplateSpec
		fldPath     *field.Path
		expectedErr field.ErrorList
	}{
		{
			description: "seccomp field and container annotation must match",
			fldPath:     rootFld,
			expectedErr: field.ErrorList{
				field.Forbidden(
					rootFld.Child("spec").Child("containers").Index(1).Child("securityContext").Child("seccompProfile").Child("type"),
					"seccomp type in annotation and field must match"),
			},
			spec: &core.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"container.seccomp.security.alpha.kubernetes.io/test2": "unconfined",
					},
				},
				Spec: core.PodSpec{
					Containers: []core.Container{
						{
							Name:                     "test1",
							Image:                    "alpine",
							ImagePullPolicy:          core.PullAlways,
							TerminationMessagePolicy: core.TerminationMessageFallbackToLogsOnError,
						},
						{
							SecurityContext: &core.SecurityContext{
								SeccompProfile: &core.SeccompProfile{
									Type: core.SeccompProfileTypeRuntimeDefault,
								},
							},
							Name:                     "test2",
							Image:                    "alpine",
							ImagePullPolicy:          core.PullAlways,
							TerminationMessagePolicy: core.TerminationMessageFallbackToLogsOnError,
						},
					},
					RestartPolicy: core.RestartPolicyAlways,
					DNSPolicy:     core.DNSDefault,
				},
			},
		},
		{
			description: "seccomp field and pod annotation must match",
			fldPath:     rootFld,
			expectedErr: field.ErrorList{
				field.Forbidden(
					rootFld.Child("spec").Child("securityContext").Child("seccompProfile").Child("type"),
					"seccomp type in annotation and field must match"),
			},
			spec: &core.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"seccomp.security.alpha.kubernetes.io/pod": "runtime/default",
					},
				},
				Spec: core.PodSpec{
					SecurityContext: &core.PodSecurityContext{
						SeccompProfile: &core.SeccompProfile{
							Type: core.SeccompProfileTypeUnconfined,
						},
					},
					Containers: []core.Container{
						{
							Name:                     "test",
							Image:                    "alpine",
							ImagePullPolicy:          core.PullAlways,
							TerminationMessagePolicy: core.TerminationMessageFallbackToLogsOnError,
						},
					},
					RestartPolicy: core.RestartPolicyAlways,
					DNSPolicy:     core.DNSDefault,
				},
			},
		},
		{
			description: "init seccomp field and container annotation must match",
			fldPath:     rootFld,
			expectedErr: field.ErrorList{
				field.Forbidden(
					rootFld.Child("spec").Child("initContainers").Index(0).Child("securityContext").Child("seccompProfile").Child("type"),
					"seccomp type in annotation and field must match"),
			},
			spec: &core.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"container.seccomp.security.alpha.kubernetes.io/init-test": "unconfined",
					},
				},
				Spec: core.PodSpec{
					Containers: []core.Container{
						{
							Name:                     "test",
							Image:                    "alpine",
							ImagePullPolicy:          core.PullAlways,
							TerminationMessagePolicy: core.TerminationMessageFallbackToLogsOnError,
						},
					},
					InitContainers: []core.Container{
						{
							Name: "init-test",
							SecurityContext: &core.SecurityContext{
								SeccompProfile: &core.SeccompProfile{
									Type: core.SeccompProfileTypeRuntimeDefault,
								},
							},
							Image:                    "alpine",
							ImagePullPolicy:          core.PullAlways,
							TerminationMessagePolicy: core.TerminationMessageFallbackToLogsOnError,
						},
					},
					RestartPolicy: core.RestartPolicyAlways,
					DNSPolicy:     core.DNSDefault,
				},
			},
		},
	}

	for i, test := range tests {
		err := ValidatePodTemplateSpec(test.spec, rootFld, PodValidationOptions{})
		asserttestify.Equal(t, test.expectedErr, err, "TestCase[%d]: %s", i, test.description)
	}
}

func TestValidateResourceRequirements(t *testing.T) {
	path := field.NewPath("resources")
	tests := []struct {
		name         string
		requirements core.ResourceRequirements
		opts         PodValidationOptions
	}{
		{
			name: "limits and requests of hugepage resource are equal",
			requirements: core.ResourceRequirements{
				Limits: core.ResourceList{
					core.ResourceCPU: resource.MustParse("10"),
					core.ResourceName(core.ResourceHugePagesPrefix + "2Mi"): resource.MustParse("2Mi"),
				},
				Requests: core.ResourceList{
					core.ResourceCPU: resource.MustParse("10"),
					core.ResourceName(core.ResourceHugePagesPrefix + "2Mi"): resource.MustParse("2Mi"),
				},
			},
			opts: PodValidationOptions{},
		},
		{
			name: "limits and requests of memory resource are equal",
			requirements: core.ResourceRequirements{
				Limits: core.ResourceList{
					core.ResourceMemory: resource.MustParse("2Mi"),
				},
				Requests: core.ResourceList{
					core.ResourceMemory: resource.MustParse("2Mi"),
				},
			},
			opts: PodValidationOptions{},
		},
		{
			name: "limits and requests of cpu resource are equal",
			requirements: core.ResourceRequirements{
				Limits: core.ResourceList{
					core.ResourceCPU: resource.MustParse("10"),
				},
				Requests: core.ResourceList{
					core.ResourceCPU: resource.MustParse("10"),
				},
			},
			opts: PodValidationOptions{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if errs := ValidateResourceRequirements(&tc.requirements, path, tc.opts); len(errs) != 0 {
				t.Errorf("unexpected errors: %v", errs)
			}
		})
	}

	errTests := []struct {
		name         string
		requirements core.ResourceRequirements
		opts         PodValidationOptions
	}{
		{
			name: "hugepage resource without cpu or memory",
			requirements: core.ResourceRequirements{
				Limits: core.ResourceList{
					core.ResourceName(core.ResourceHugePagesPrefix + "2Mi"): resource.MustParse("2Mi"),
				},
				Requests: core.ResourceList{
					core.ResourceName(core.ResourceHugePagesPrefix + "2Mi"): resource.MustParse("2Mi"),
				},
			},
			opts: PodValidationOptions{},
		},
	}

	for _, tc := range errTests {
		t.Run(tc.name, func(t *testing.T) {
			if errs := ValidateResourceRequirements(&tc.requirements, path, tc.opts); len(errs) == 0 {
				t.Error("expected errors")
			}
		})
	}
}

func TestValidateNonSpecialIP(t *testing.T) {
	fp := field.NewPath("ip")

	// Valid values.
	for _, tc := range []struct {
		desc string
		ip   string
	}{
		{"ipv4", "10.1.2.3"},
		{"ipv6", "2000::1"},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			errs := ValidateNonSpecialIP(tc.ip, fp)
			if len(errs) != 0 {
				t.Errorf("ValidateNonSpecialIP(%q, ...) = %v; want nil", tc.ip, errs)
			}
		})
	}
	// Invalid cases
	for _, tc := range []struct {
		desc string
		ip   string
	}{
		{"ipv4 unspecified", "0.0.0.0"},
		{"ipv6 unspecified", "::0"},
		{"ipv4 localhost", "127.0.0.0"},
		{"ipv4 localhost", "127.255.255.255"},
		{"ipv6 localhost", "::1"},
		{"ipv6 link local", "fe80::"},
		{"ipv6 local multicast", "ff02::"},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			errs := ValidateNonSpecialIP(tc.ip, fp)
			if len(errs) == 0 {
				t.Errorf("ValidateNonSpecialIP(%q, ...) = nil; want non-nil (errors)", tc.ip)
			}
		})
	}
}

func TestValidateWindowsHostProcessPod(t *testing.T) {
	const containerName = "container"
	falseVar := false
	trueVar := true

	testCases := []struct {
		name            string
		expectError     bool
		featureEnabled  bool
		allowPrivileged bool
		podSpec         *core.PodSpec
	}{
		{
			name:            "Spec with feature disabled and pod-wide HostProcess=false and should not validate",
			expectError:     true,
			featureEnabled:  false,
			allowPrivileged: true,
			podSpec: &core.PodSpec{
				SecurityContext: &core.PodSecurityContext{
					WindowsOptions: &core.WindowsSecurityContextOptions{
						HostProcess: &falseVar,
					},
				},
				Containers: []core.Container{{
					Name: containerName,
				}},
			},
		},
		{
			name:            "Spec with feature disabled and pod-wide HostProcess=nil set should valildate",
			expectError:     false,
			featureEnabled:  false,
			allowPrivileged: true,
			podSpec: &core.PodSpec{
				SecurityContext: &core.PodSecurityContext{
					WindowsOptions: &core.WindowsSecurityContextOptions{
						HostProcess: nil,
					},
				},
				Containers: []core.Container{{
					Name: containerName,
				}},
			},
		},
		{
			name:            "Spec with feature disabled and container setting HostProcess=true should not valildate",
			expectError:     true,
			featureEnabled:  false,
			allowPrivileged: true,
			podSpec: &core.PodSpec{
				Containers: []core.Container{{
					Name: containerName,
					SecurityContext: &core.SecurityContext{
						WindowsOptions: &core.WindowsSecurityContextOptions{
							HostProcess: &trueVar,
						},
					},
				}},
			},
		},
		{
			name:            "Spec with feature disabled and init container setting HostProcess=true should not valildate",
			expectError:     true,
			featureEnabled:  false,
			allowPrivileged: true,
			podSpec: &core.PodSpec{
				InitContainers: []core.Container{{
					Name: containerName,
					SecurityContext: &core.SecurityContext{
						WindowsOptions: &core.WindowsSecurityContextOptions{
							HostProcess: &trueVar,
						},
					},
				}},
			},
		},
		{
			name:            "Spec with feature enabled, pod-wide HostProcess=true, and HostNetwork unset should not validate",
			expectError:     true,
			featureEnabled:  true,
			allowPrivileged: true,
			podSpec: &core.PodSpec{
				SecurityContext: &core.PodSecurityContext{
					WindowsOptions: &core.WindowsSecurityContextOptions{
						HostProcess: &trueVar,
					},
				},
				Containers: []core.Container{{
					Name: containerName,
				}},
			},
		},
		{
			name:            "Spec with feature enabled, pod-wide HostProcess=ture, and HostNetwork set should validate",
			expectError:     false,
			featureEnabled:  true,
			allowPrivileged: true,
			podSpec: &core.PodSpec{
				SecurityContext: &core.PodSecurityContext{
					HostNetwork: true,
					WindowsOptions: &core.WindowsSecurityContextOptions{
						HostProcess: &trueVar,
					},
				},
				Containers: []core.Container{{
					Name: containerName,
				}},
			},
		},
		{
			name:            "Spec with feature enabled, pod-wide HostProcess=ture, HostNetwork set, and containers setting HostProcess=true should validate",
			expectError:     false,
			featureEnabled:  true,
			allowPrivileged: true,
			podSpec: &core.PodSpec{
				SecurityContext: &core.PodSecurityContext{
					HostNetwork: true,
					WindowsOptions: &core.WindowsSecurityContextOptions{
						HostProcess: &trueVar,
					},
				},
				Containers: []core.Container{{
					Name: containerName,
					SecurityContext: &core.SecurityContext{
						WindowsOptions: &core.WindowsSecurityContextOptions{
							HostProcess: &trueVar,
						},
					},
				}},
				InitContainers: []core.Container{{
					Name: containerName,
					SecurityContext: &core.SecurityContext{
						WindowsOptions: &core.WindowsSecurityContextOptions{
							HostProcess: &trueVar,
						},
					},
				}},
			},
		},
		{
			name:            "Spec with feature enabled, pod-wide HostProcess=nil, HostNetwork set, and all containers setting HostProcess=true should validate",
			expectError:     false,
			featureEnabled:  true,
			allowPrivileged: true,
			podSpec: &core.PodSpec{
				SecurityContext: &core.PodSecurityContext{
					HostNetwork: true,
				},
				Containers: []core.Container{{
					Name: containerName,
					SecurityContext: &core.SecurityContext{
						WindowsOptions: &core.WindowsSecurityContextOptions{
							HostProcess: &trueVar,
						},
					},
				}},
				InitContainers: []core.Container{{
					Name: containerName,
					SecurityContext: &core.SecurityContext{
						WindowsOptions: &core.WindowsSecurityContextOptions{
							HostProcess: &trueVar,
						},
					},
				}},
			},
		},
		{
			name:            "Pods with feature enabled, some containers setting HostProcess=true, and others setting HostProcess=false should not validate",
			expectError:     true,
			featureEnabled:  true,
			allowPrivileged: true,
			podSpec: &core.PodSpec{
				SecurityContext: &core.PodSecurityContext{
					HostNetwork: true,
				},
				Containers: []core.Container{{
					Name: containerName,
					SecurityContext: &core.SecurityContext{
						WindowsOptions: &core.WindowsSecurityContextOptions{
							HostProcess: &trueVar,
						},
					},
				}},
				InitContainers: []core.Container{{
					Name: containerName,
					SecurityContext: &core.SecurityContext{
						WindowsOptions: &core.WindowsSecurityContextOptions{
							HostProcess: &falseVar,
						},
					},
				}},
			},
		},
		{
			name:            "Spec with feature enabled, some containers setting HostProcess=true, and other leaving HostProcess unset should not validate",
			expectError:     true,
			featureEnabled:  true,
			allowPrivileged: true,
			podSpec: &core.PodSpec{
				SecurityContext: &core.PodSecurityContext{
					HostNetwork: true,
				},
				Containers: []core.Container{{
					Name: containerName,
					SecurityContext: &core.SecurityContext{
						WindowsOptions: &core.WindowsSecurityContextOptions{
							HostProcess: &trueVar,
						},
					},
				}},
				InitContainers: []core.Container{{
					Name: containerName,
				}},
			},
		},
		{
			name:            "Spec with feature enabled, pod-wide HostProcess=true, some containers setting HostProcess=true, and init containers setting HostProcess=false should not validate",
			expectError:     true,
			featureEnabled:  true,
			allowPrivileged: true,
			podSpec: &core.PodSpec{
				SecurityContext: &core.PodSecurityContext{
					HostNetwork: true,
					WindowsOptions: &core.WindowsSecurityContextOptions{
						HostProcess: &trueVar,
					},
				},
				Containers: []core.Container{{
					Name: containerName,
					SecurityContext: &core.SecurityContext{
						WindowsOptions: &core.WindowsSecurityContextOptions{
							HostProcess: &trueVar,
						},
					},
				}},
				InitContainers: []core.Container{{
					Name: containerName,
					SecurityContext: &core.SecurityContext{
						WindowsOptions: &core.WindowsSecurityContextOptions{
							HostProcess: &falseVar,
						},
					},
				}},
			},
		},
		{
			name:            "Spec with feature enabled, pod-wide HostProcess=true, some containers setting HostProcess=true, and others setting HostProcess=false should not validate",
			expectError:     true,
			featureEnabled:  true,
			allowPrivileged: true,
			podSpec: &core.PodSpec{
				SecurityContext: &core.PodSecurityContext{
					HostNetwork: true,
					WindowsOptions: &core.WindowsSecurityContextOptions{
						HostProcess: &trueVar,
					},
				},
				Containers: []core.Container{
					{
						Name: containerName,
						SecurityContext: &core.SecurityContext{
							WindowsOptions: &core.WindowsSecurityContextOptions{
								HostProcess: &trueVar,
							},
						},
					}, {
						Name: containerName,
						SecurityContext: &core.SecurityContext{
							WindowsOptions: &core.WindowsSecurityContextOptions{
								HostProcess: &falseVar,
							},
						},
					},
				},
			},
		},
		{
			name:            "Spec with feature enabled, pod-wide HostProcess=true, some containers setting HostProcess=true, and others leaving HostProcess=nil should validate",
			expectError:     false,
			featureEnabled:  true,
			allowPrivileged: true,
			podSpec: &core.PodSpec{
				SecurityContext: &core.PodSecurityContext{
					HostNetwork: true,
					WindowsOptions: &core.WindowsSecurityContextOptions{
						HostProcess: &trueVar,
					},
				},
				Containers: []core.Container{{
					Name: containerName,
					SecurityContext: &core.SecurityContext{
						WindowsOptions: &core.WindowsSecurityContextOptions{
							HostProcess: &trueVar,
						},
					},
				}},
				InitContainers: []core.Container{{
					Name: containerName,
				}},
			},
		},
		{
			name:            "Spec with feature enabled, pod-wide HostProcess=false, some contaienrs setting HostProccess=true should not validate",
			expectError:     true,
			featureEnabled:  true,
			allowPrivileged: true,
			podSpec: &core.PodSpec{
				SecurityContext: &core.PodSecurityContext{
					HostNetwork: true,
					WindowsOptions: &core.WindowsSecurityContextOptions{
						HostProcess: &falseVar,
					},
				},
				Containers: []core.Container{{
					Name: containerName,
					SecurityContext: &core.SecurityContext{
						WindowsOptions: &core.WindowsSecurityContextOptions{
							HostProcess: &trueVar,
						},
					},
				}},
				InitContainers: []core.Container{{
					Name: containerName,
				}},
			},
		},
		{
			name:            "Pod's HostProcess set to true but all containers override to false should not validate",
			expectError:     true,
			featureEnabled:  true,
			allowPrivileged: true,
			podSpec: &core.PodSpec{
				SecurityContext: &core.PodSecurityContext{
					HostNetwork: true,
					WindowsOptions: &core.WindowsSecurityContextOptions{
						HostProcess: &trueVar,
					},
				},
				Containers: []core.Container{{
					Name: containerName,
					SecurityContext: &core.SecurityContext{
						WindowsOptions: &core.WindowsSecurityContextOptions{
							HostProcess: &falseVar,
						},
					},
				}},
			},
		},
		{
			name:            "Valid HostProcess pod should spec should not validate if allowPrivileged is not set",
			expectError:     true,
			featureEnabled:  true,
			allowPrivileged: false,
			podSpec: &core.PodSpec{
				SecurityContext: &core.PodSecurityContext{
					HostNetwork: true,
				},
				Containers: []core.Container{{
					Name: containerName,
					SecurityContext: &core.SecurityContext{
						WindowsOptions: &core.WindowsSecurityContextOptions{
							HostProcess: &trueVar,
						},
					},
				}},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.WindowsHostProcessContainers, testCase.featureEnabled)()

			opts := PodValidationOptions{AllowWindowsHostProcessField: testCase.featureEnabled}

			capabilities.SetForTests(capabilities.Capabilities{
				AllowPrivileged: testCase.allowPrivileged,
			})

			errs := validateWindowsHostProcessPod(testCase.podSpec, field.NewPath("spec"), opts)
			if testCase.expectError && len(errs) == 0 {
				t.Errorf("Unexpected success")
			}
			if !testCase.expectError && len(errs) != 0 {
				t.Errorf("Unexpected error(s): %v", errs)
			}
		})
	}
}
