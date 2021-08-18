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

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/resource"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util"
	"k8s.io/kubernetes/cmd/kubeadm/app/cmd/phases/workflow"
	kubeContants "k8s.io/kubernetes/cmd/kubeadm/app/constants"

	"github.com/openyurtio/openyurt/pkg/yurtctl/constants"
)

func NewInstallCNIPhase() workflow.Phase {
	return workflow.Phase{
		Name:  "Install flannel.",
		Short: "Install flannel.",
		Run:   runInstallCNI,
	}
}

func runInstallCNI(c workflow.RunData) error {
	data, ok := c.(YurtInitData)
	if !ok {
		return fmt.Errorf("Install cni phase invoked with an invalid data struct. ")
	}
	cniInstallFile := constants.FlannelIntallFile
	if data.CNIFileName() != "" {
		cniInstallFile = data.CNIFileName()
	}
	fileNameOption := &resource.FilenameOptions{
		Filenames: []string{cniInstallFile},
	}
	adminKubeConfig := kubeContants.GetAdminKubeConfigPath()
	configFlag := &genericclioptions.ConfigFlags{KubeConfig: &adminKubeConfig}
	builder := resource.NewBuilder(configFlag)
	r := builder.Unstructured().
		Schema(nil).
		ContinueOnError().
		NamespaceParam("").DefaultNamespace().
		FilenameParam(false, fileNameOption).
		LabelSelectorParam("").
		Flatten().
		Do()
	infos, err := r.Infos()
	if err != nil {
		return err
	}
	for _, info := range infos {
		if err := applyOneObject(info); err != nil {
			return err
		}
	}
	return nil
}

func applyOneObject(info *resource.Info) error {
	if err := util.CreateApplyAnnotation(info.Object, unstructured.UnstructuredJSONScheme); err != nil {
		return cmdutil.AddSourceToErr("creating", info.Source, err)
	}

	helper := resource.NewHelper(info.Client, info.Mapping)
	obj, err := helper.Create(info.Namespace, true, info.Object)
	if err != nil {
		return cmdutil.AddSourceToErr("creating", info.Source, err)
	}
	info.Refresh(obj, true)
	return nil
}
