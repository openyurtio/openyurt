/*
Copyright 2021 The OpenYurt Authors.

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

package phases

import (
	"fmt"
	"strings"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeadmapi "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm"
	"k8s.io/kubernetes/cmd/kubeadm/app/cmd/phases/workflow"
	kubeadmconstants "k8s.io/kubernetes/cmd/kubeadm/app/constants"
	"k8s.io/kubernetes/cmd/kubeadm/app/phases/controlplane"
	"k8s.io/kubernetes/cmd/kubeadm/app/phases/etcd"
	etcdutil "k8s.io/kubernetes/cmd/kubeadm/app/util/etcd"
	"k8s.io/kubernetes/cmd/kubeadm/app/util/staticpod"
)

func NewAllInAoneControlPlanePhase() workflow.Phase {
	return workflow.Phase{
		Name:  "Generate all-in-one controlplane static pod yaml.",
		Short: "Generate all-in-one controlplane static pod yaml.",
		Run:   runAllInAoneControlPlane,
	}
}

func runAllInAoneControlPlane(c workflow.RunData) error {
	data, ok := c.(YurtInitData)
	if !ok {
		return fmt.Errorf("Install controlplane phase invoked with an invalid data struct. ")
	}
	cfg := data.Cfg()
	return CreateStaticPodFile(data.ManifestDir(), cfg.NodeRegistration.Name, &cfg.ClusterConfiguration, &cfg.LocalAPIEndpoint, data.IsConvertYurtCluster())
}

func CreateStaticPodFile(manifestDir, nodeName string,
	cfg *kubeadmapi.ClusterConfiguration,
	endpoint *kubeadmapi.APIEndpoint,
	convertYurtCluster bool) error {
	masterSpecs := controlplane.GetStaticPodSpecs(cfg, endpoint)
	if convertYurtCluster {
		if err := closeNodeLifeCycleController(masterSpecs); err != nil {
			return err
		}
	}
	var containers []v1.Container
	var volumes []v1.Volume
	for _, spec := range masterSpecs {
		componentName := spec.Name
		for _, v := range spec.Spec.Volumes {
			v.Name = componentName + "-" + v.Name
			volumes = append(volumes, v)
		}
		for _, c := range spec.Spec.Containers {
			for i := 0; i < len(c.VolumeMounts); i++ {
				c.VolumeMounts[i].Name = componentName + "-" + c.VolumeMounts[i].Name
			}
			containers = append(containers, c)
		}
	}

	etcdSpec := etcd.GetEtcdPodSpec(cfg, endpoint, nodeName, []etcdutil.Member{})
	componentName := etcdSpec.Name
	for _, v := range etcdSpec.Spec.Volumes {
		v.Name = componentName + "-" + v.Name
		volumes = append(volumes, v)
	}
	for _, c := range etcdSpec.Spec.Containers {
		for i := 0; i < len(c.VolumeMounts); i++ {
			c.VolumeMounts[i].Name = componentName + "-" + c.VolumeMounts[i].Name
		}
		containers = append(containers, c)
	}
	pod := componentPod("controlplane", containers, volumes, map[string]string{kubeadmconstants.KubeAPIServerAdvertiseAddressEndpointAnnotationKey: endpoint.String()})
	return staticpod.WriteStaticPodToDisk("controlplane", manifestDir, pod)
}

func componentPod(name string, containers []v1.Container, volumes []v1.Volume, annotations map[string]string) v1.Pod {
	return v1.Pod{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Pod",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   metav1.NamespaceSystem,
			Labels:      map[string]string{"component": name, "tier": kubeadmconstants.ControlPlaneTier},
			Annotations: annotations,
		},
		Spec: v1.PodSpec{
			Containers:        containers,
			PriorityClassName: "system-cluster-critical",
			HostNetwork:       true,
			Volumes:           volumes,
		},
	}
}

func closeNodeLifeCycleController(masterSpecs map[string]v1.Pod) error {
	cmPod, ok := masterSpecs[kubeadmconstants.KubeControllerManager]
	if !ok {
		return fmt.Errorf("Spec %s is not exists.", kubeadmconstants.KubeControllerManager)
	}
	cmd := cmPod.Spec.Containers[0].Command
	var newCmd []string
	for _, p := range cmd {
		if strings.Contains(p, "--controllers=") {
			p = p + ",-nodelifecycle"
		}
		newCmd = append(newCmd, p)
	}
	cmPod.Spec.Containers[0].Command = newCmd
	masterSpecs[kubeadmconstants.KubeControllerManager] = cmPod
	return nil
}
