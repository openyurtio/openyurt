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

package platformadmin

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"

	iotv1alpha2 "github.com/openyurtio/openyurt/pkg/apis/iot/v1alpha2"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/platformadmin/config"
	utils "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/platformadmin/utils"
)

// newYurtIoTDockComponent initialize the configuration of yurt-iot-dock component
func newYurtIoTDockComponent(platformAdmin *iotv1alpha2.PlatformAdmin, platformAdminFramework *PlatformAdminFramework) (*config.Component, error) {
	var yurtIotDockComponent config.Component

	// If the configuration of the yurt-iot-dock component that customized in the platformAdminFramework
	for _, cp := range platformAdminFramework.Components {
		if cp.Name != utils.IotDockName {
			continue
		}
		return cp, nil
	}

	// Otherwise, the default configuration is used to start
	ver, ns, err := utils.DefaultVersion(platformAdmin)
	if err != nil {
		return nil, err
	}

	yurtIotDockComponent.Name = utils.IotDockName
	yurtIotDockComponent.Deployment = &appsv1.DeploymentSpec{
		Selector: &metav1.LabelSelector{
			MatchLabels: map[string]string{
				"app":           utils.IotDockName,
				"control-plane": utils.IotDockControlPlane,
			},
		},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					"app":           utils.IotDockName,
					"control-plane": utils.IotDockControlPlane,
				},
				Namespace: ns,
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:            utils.IotDockName,
						Image:           fmt.Sprintf("%s:%s", utils.IotDockImage, ver),
						ImagePullPolicy: corev1.PullAlways,
						Args: []string{
							"--health-probe-bind-address=:8081",
							"--metrics-bind-address=127.0.0.1:8080",
							"--leader-elect=false",
							fmt.Sprintf("--namespace=%s", ns),
							fmt.Sprintf("--version=%s", platformAdmin.Spec.Version),
						},
						LivenessProbe: &corev1.Probe{
							InitialDelaySeconds: 15,
							PeriodSeconds:       20,
							Handler: corev1.Handler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/healthz",
									Port: intstr.FromInt(8081),
								},
							},
						},
						ReadinessProbe: &corev1.Probe{
							InitialDelaySeconds: 5,
							PeriodSeconds:       10,
							Handler: corev1.Handler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/readyz",
									Port: intstr.FromInt(8081),
								},
							},
						},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("512m"),
								corev1.ResourceMemory: resource.MustParse("256Mi"),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("1024m"),
								corev1.ResourceMemory: resource.MustParse("512Mi"),
							},
						},
						SecurityContext: &corev1.SecurityContext{
							AllowPrivilegeEscalation: pointer.Bool(false),
						},
					},
				},
				TerminationGracePeriodSeconds: pointer.Int64(10),
				SecurityContext: &corev1.PodSecurityContext{
					RunAsUser: pointer.Int64(65532),
				},
			},
		},
	}
	// YurtIoTDock doesn't need a service yet
	yurtIotDockComponent.Service = nil

	return &yurtIotDockComponent, nil
}
