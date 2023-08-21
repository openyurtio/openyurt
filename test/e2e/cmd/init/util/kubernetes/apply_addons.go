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
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/resource"
	kubeclientset "k8s.io/client-go/kubernetes"
	kubectlutil "k8s.io/kubectl/pkg/cmd/util"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/test/e2e/cmd/init/constants"
)

// DeployYurthubSetting deploy clusterrole, clusterrolebinding for yurthub static pod.
func DeployYurthubSetting(client kubeclientset.Interface) error {
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
func DeleteYurthubSetting(client kubeclientset.Interface) error {

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

func CreateYurtManager(client kubeclientset.Interface, yurtManagerImage string) error {
	if err := CreateSecretFromYaml(client, SystemNamespace, constants.YurtManagerCertsSecret); err != nil {
		return err
	}

	if err := CreateServiceAccountFromYaml(client,
		SystemNamespace, constants.YurtManagerServiceAccount); err != nil {
		return err
	}

	// bind the clusterrole
	if err := CreateClusterRoleBindingFromYaml(client,
		constants.YurtManagerClusterRoleBinding); err != nil {
		return err
	}

	// bind the role
	if err := CreateRoleBindingFromYaml(client,
		constants.YurtManagerRoleBinding); err != nil {
		return err
	}

	// create the Service
	if err := CreateServiceFromYaml(client,
		SystemNamespace,
		constants.YurtManagerService); err != nil {
		return err
	}

	// create the yurt-manager deployment
	if err := CreateDeployFromYaml(client,
		SystemNamespace,
		constants.YurtManagerDeployment,
		map[string]string{
			"image":         yurtManagerImage,
			"edgeNodeLabel": projectinfo.GetEdgeWorkerLabelKey()}); err != nil {
		return err
	}
	return nil
}

type Builder struct {
	kubectlutil.Factory
}

func NewBuilder(kubeconfig string) *Builder {
	kubeConfigFlags := genericclioptions.NewConfigFlags(true).WithDeprecatedPasswordFlag()
	kubeConfigFlags.KubeConfig = &kubeconfig
	f := kubectlutil.NewFactory(kubeConfigFlags)
	return &Builder{f}
}

func (b Builder) InstallComponents(path string, recursive bool) error {
	fo := resource.FilenameOptions{
		Filenames: []string{path},
		Recursive: recursive,
	}
	cmdNs, enforceNs, err := b.ToRawKubeConfigLoader().Namespace()
	if err != nil {
		return err
	}

	r := b.NewBuilder().
		Unstructured().
		ContinueOnError().
		NamespaceParam(cmdNs).DefaultNamespace().
		FilenameParam(enforceNs, &fo).
		Flatten().
		Do()
	err = r.Err()
	if err != nil {
		return err
	}

	cnt := 0
	err = r.Visit(func(info *resource.Info, err error) error {
		if err != nil {
			return err
		}

		obj, err := resource.NewHelper(info.Client, info.Mapping).
			Create(info.Namespace, true, info.Object)
		if err != nil {
			return kubectlutil.AddSourceToErr("creating", info.Source, err)
		}
		info.Refresh(obj, true)
		cnt++
		return nil
	})

	if err != nil {
		return err
	}

	if cnt == 0 {
		return fmt.Errorf("no objects passed to create")
	}

	return nil
}
