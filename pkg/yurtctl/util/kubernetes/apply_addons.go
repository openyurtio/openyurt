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
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/yurtctl/constants"
)

func DeployYurtControllerManager(client *kubernetes.Clientset, yurtControllerManagerImage string) error {
	if err := CreateServiceAccountFromYaml(client,
		SystemNamespace, constants.YurtControllerManagerServiceAccount); err != nil {
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
		SystemNamespace,
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
	if err := CreateRoleFromYaml(client, SystemNamespace,
		constants.YurtAppManagerRole); err != nil {
		return err
	}

	// 3. create the ClusterRole
	if err := CreateClusterRoleFromYaml(client,
		constants.YurtAppManagerClusterRole); err != nil {
		return err
	}

	// 4. create the RoleBinding
	if err := CreateRoleBindingFromYaml(client, SystemNamespace,
		constants.YurtAppManagerRolebinding); err != nil {
		return err
	}

	// 5. create the ClusterRoleBinding
	if err := CreateClusterRoleBindingFromYaml(client,
		constants.YurtAppManagerClusterRolebinding); err != nil {
		return err
	}

	// 6. create the Secret
	if err := CreateSecretFromYaml(client, SystemNamespace,
		constants.YurtAppManagerSecret); err != nil {
		return err
	}

	// 7. create the Service
	if err := CreateServiceFromYaml(client,
		SystemNamespace,
		constants.YurtAppManagerService); err != nil {
		return err
	}

	// 8. create the Deployment
	if err := CreateDeployFromYaml(client,
		SystemNamespace,
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
	certIP string,
	yurttunnelServerImage string,
	systemArchitecture string) error {
	// 1. create the ClusterRole
	if err := CreateClusterRoleFromYaml(client,
		constants.YurttunnelServerClusterRole); err != nil {
		return err
	}
	if err := CreateClusterRoleFromYaml(client,
		constants.YurttunnelProxyClientClusterRole); err != nil {
		return err
	}

	// 2. create the ServiceAccount
	if err := CreateServiceAccountFromYaml(client, SystemNamespace,
		constants.YurttunnelServerServiceAccount); err != nil {
		return err
	}

	// 3. create the ClusterRoleBinding
	if err := CreateClusterRoleBindingFromYaml(client,
		constants.YurttunnelServerClusterRolebinding); err != nil {
		return err
	}

	if err := CreateClusterRoleBindingFromYaml(client,
		constants.YurttunnelProxyClientClusterRolebinding); err != nil {
		return err
	}

	// 4. create the Service
	if err := CreateServiceFromYaml(client,
		SystemNamespace,
		constants.YurttunnelServerService); err != nil {
		return err
	}

	// 5. create the internal Service(type=ClusterIP)
	if err := CreateServiceFromYaml(client,
		SystemNamespace,
		constants.YurttunnelServerInternalService); err != nil {
		return err
	}

	// 6. create the Configmap
	if err := CreateConfigMapFromYaml(client,
		SystemNamespace,
		constants.YurttunnelServerConfigMap); err != nil {
		return err
	}

	// 7. create the Deployment
	if err := CreateDeployFromYaml(client,
		SystemNamespace,
		constants.YurttunnelServerDeployment,
		map[string]string{
			"image":           yurttunnelServerImage,
			"arch":            systemArchitecture,
			"certIP":          certIP,
			"edgeWorkerLabel": projectinfo.GetEdgeWorkerLabelKey()}); err != nil {
		return err
	}

	return nil
}

func DeployYurttunnelAgent(
	client *kubernetes.Clientset,
	tunnelServerAddress string,
	yurttunnelAgentImage string) error {
	// 1. Deploy the yurt-tunnel-agent DaemonSet
	if err := CreateDaemonSetFromYaml(client,
		SystemNamespace,
		constants.YurttunnelAgentDaemonSet,
		map[string]string{
			"image":               yurttunnelAgentImage,
			"edgeWorkerLabel":     projectinfo.GetEdgeWorkerLabelKey(),
			"tunnelServerAddress": tunnelServerAddress}); err != nil {
		return err
	}
	return nil
}

// DeployYurthubSetting deploy clusterrole, clusterrolebinding for yurthub static pod.
func DeployYurthubSetting(client *kubernetes.Clientset) error {
	// 1. create the ClusterRole
	if err := CreateClusterRoleFromYaml(client, constants.YurthubClusterRole); err != nil {
		return err
	}

	// 2. create the ClusterRoleBinding
	if err := CreateClusterRoleBindingFromYaml(client, constants.YurthubClusterRoleBinding); err != nil {
		return err
	}

	// 3. create the Configmap
	if err := CreateConfigMapFromYaml(client,
		SystemNamespace,
		constants.YurthubConfigMap); err != nil {
		return err
	}

	return nil
}

// DeleteYurthubSetting rm settings for yurthub pod
func DeleteYurthubSetting(client *kubernetes.Clientset) error {

	// 1. delete the ClusterRoleBinding
	if err := client.RbacV1().ClusterRoleBindings().
		Delete(context.Background(), constants.YurthubComponentName,
			metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("fail to delete the clusterrolebinding/%s: %w",
			constants.YurthubComponentName, err)
	}

	// 2. delete the ClusterRole
	if err := client.RbacV1().ClusterRoles().
		Delete(context.Background(), constants.YurthubComponentName,
			metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("fail to delete the clusterrole/%s: %w",
			constants.YurthubComponentName, err)
	}

	// 3. remove the ConfigMap
	if err := client.CoreV1().ConfigMaps(constants.YurthubNamespace).
		Delete(context.Background(), constants.YurthubCmName,
			metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("fail to delete the configmap/%s: %w",
			constants.YurthubCmName, err)
	}

	return nil
}
