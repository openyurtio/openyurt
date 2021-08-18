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

package kubernetes

import (
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/yurtctl/constants"
)

func DeployYurtControllerManager(client *kubernetes.Clientset, yurtControllerManagerImage string) error {
	if err := CreateServiceAccountFromYaml(client,
		"kube-system", constants.YurtControllerManagerServiceAccount); err != nil {
		return err
	}
	// create the clusterrole
	if err := CreateClusterRoleFromYaml(client,
		constants.YurtControllerManagerClusterRole); err != nil {
		return err
	}
	// bind the clusterrole
	if err := CreateClusterRoleBindingFromYaml(client,
		constants.YurtControllerManagerClusterRoleBinding); err != nil {
		return err
	}
	// create the yurt-controller-manager deployment
	if err := CreateDeployFromYaml(client,
		"kube-system",
		constants.YurtControllerManagerDeployment,
		map[string]string{
			"image":         yurtControllerManagerImage,
			"edgeNodeLabel": projectinfo.GetEdgeWorkerLabelKey()}); err != nil {
		return err
	}
	return nil
}

func DeployYurtAppManager(
	client *kubernetes.Clientset,
	yurtappmanagerImage string,
	yurtAppManagerClient dynamic.Interface,
	systemArchitecture string) error {

	// 1.create the YurtAppManagerCustomResourceDefinition
	// 1.1 nodepool
	if err := CreateCRDFromYaml(client, yurtAppManagerClient, "", []byte(constants.YurtAppManagerNodePool)); err != nil {
		return err
	}

	// 1.2 uniteddeployment
	if err := CreateCRDFromYaml(client, yurtAppManagerClient, "", []byte(constants.YurtAppManagerUnitedDeployment)); err != nil {
		return err
	}

	// 2. create the YurtAppManagerRole
	if err := CreateRoleFromYaml(client, "kube-system",
		constants.YurtAppManagerRole); err != nil {
		return err
	}

	// 3. create the ClusterRole
	if err := CreateClusterRoleFromYaml(client,
		constants.YurtAppManagerClusterRole); err != nil {
		return err
	}

	// 4. create the RoleBinding
	if err := CreateRoleBindingFromYaml(client, "kube-system",
		constants.YurtAppManagerRolebinding); err != nil {
		return err
	}

	// 5. create the ClusterRoleBinding
	if err := CreateClusterRoleBindingFromYaml(client,
		constants.YurtAppManagerClusterRolebinding); err != nil {
		return err
	}

	// 6. create the Secret
	if err := CreateSecretFromYaml(client, "kube-system",
		constants.YurtAppManagerSecret); err != nil {
		return err
	}

	// 7. create the Service
	if err := CreateServiceFromYaml(client,
		constants.YurtAppManagerService); err != nil {
		return err
	}

	// 8. create the Deployment
	if err := CreateDeployFromYaml(client,
		"kube-system",
		constants.YurtAppManagerDeployment,
		map[string]string{
			"image":           yurtappmanagerImage,
			"arch":            systemArchitecture,
			"edgeWorkerLabel": projectinfo.GetEdgeWorkerLabelKey()}); err != nil {
		return err
	}

	// 9. create the YurtAppManagerMutatingWebhookConfiguration
	if err := CreateMutatingWebhookConfigurationFromYaml(client,
		constants.YurtAppManagerMutatingWebhookConfiguration); err != nil {
		return err
	}

	// 10. create the YurtAppManagerValidatingWebhookConfiguration
	if err := CreateValidatingWebhookConfigurationFromYaml(client,
		constants.YurtAppManagerValidatingWebhookConfiguration); err != nil {
		return err
	}

	return nil
}

func DeployYurttunnelServer(
	client *kubernetes.Clientset,
	cloudNodes []string,
	yurttunnelServerImage string,
	systemArchitecture string) error {
	// 1. create the ClusterRole
	if err := CreateClusterRoleFromYaml(client,
		constants.YurttunnelServerClusterRole); err != nil {
		return err
	}

	// 2. create the ServiceAccount
	if err := CreateServiceAccountFromYaml(client, "kube-system",
		constants.YurttunnelServerServiceAccount); err != nil {
		return err
	}

	// 3. create the ClusterRoleBinding
	if err := CreateClusterRoleBindingFromYaml(client,
		constants.YurttunnelServerClusterRolebinding); err != nil {
		return err
	}

	// 4. create the Service
	if err := CreateServiceFromYaml(client,
		constants.YurttunnelServerService); err != nil {
		return err
	}

	// 5. create the internal Service(type=ClusterIP)
	if err := CreateServiceFromYaml(client,
		constants.YurttunnelServerInternalService); err != nil {
		return err
	}

	// 6. create the Configmap
	if err := CreateConfigMapFromYaml(client,
		"kube-system",
		constants.YurttunnelServerConfigMap); err != nil {
		return err
	}

	// 7. create the Deployment
	if err := CreateDeployFromYaml(client,
		"kube-system",
		constants.YurttunnelServerDeployment,
		map[string]string{
			"image":           yurttunnelServerImage,
			"arch":            systemArchitecture,
			"edgeWorkerLabel": projectinfo.GetEdgeWorkerLabelKey()}); err != nil {
		return err
	}

	return nil
}

func DeployYurttunnelAgent(
	client *kubernetes.Clientset,
	tunnelAgentNodes []string,
	yurttunnelAgentImage string) error {
	// 1. Deploy the yurt-tunnel-agent DaemonSet
	if err := CreateDaemonSetFromYaml(client,
		constants.YurttunnelAgentDaemonSet,
		map[string]string{
			"image":           yurttunnelAgentImage,
			"edgeWorkerLabel": projectinfo.GetEdgeWorkerLabelKey()}); err != nil {
		return err
	}
	return nil
}
