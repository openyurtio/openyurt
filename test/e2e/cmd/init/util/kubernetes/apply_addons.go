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
	"fmt"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/resource"
	kubectlutil "k8s.io/kubectl/pkg/cmd/util"
)

type Builder struct {
	kubectlutil.Factory
}

func NewBuilder(kubeconfig string) *Builder {
	kubeConfigFlags := genericclioptions.NewConfigFlags(false).WithDeprecatedPasswordFlag()
	kubeConfigFlags.KubeConfig = &kubeconfig
	f := kubectlutil.NewFactory(kubeConfigFlags)
	return &Builder{f}
}

func (b Builder) InstallComponents(path string, recursive bool) error {
	opts := resource.FilenameOptions{
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
		FilenameParam(enforceNs, &opts).
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
