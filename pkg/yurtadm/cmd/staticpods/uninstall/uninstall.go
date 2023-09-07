/*
Copyright 2023 The OpenYurt Authors.

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

package uninstall

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	flag "github.com/spf13/pflag"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurtadm/constants"
	"github.com/openyurtio/openyurt/pkg/yurtadm/util/edgenode"
	yurtadmutil "github.com/openyurtio/openyurt/pkg/yurtadm/util/kubernetes"
	"github.com/openyurtio/openyurt/pkg/yurtadm/util/yurthub"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/yurtstaticset/util"
)

type uninstallOptions struct {
	staticPods            string
	staticPodManifestList []string
}

// NewCmdUnInstall returns "yurtadm staticpods uninstall" command.
func NewCmdUnInstall() *cobra.Command {
	o := &uninstallOptions{}

	uninstallCmd := &cobra.Command{
		Use:   "uninstall",
		Short: "UnInstall static pods for user specified.",
		RunE: func(uninstallCmd *cobra.Command, args []string) error {
			if err := o.validate(); err != nil {
				klog.Fatalf("validate options: %v", err)
			}

			klog.Infof("UnInstall static pods %+v", o.staticPods)

			if err := edgenode.RemoveStaticYaml(o.staticPodManifestList, constants.StaticPodPath); err != nil {
				return err
			}
			return nil
		},
	}

	addUnInstallConfigFlags(uninstallCmd.Flags(), o)
	return uninstallCmd
}

func (options *uninstallOptions) validate() error {
	if len(options.staticPods) == 0 {
		return fmt.Errorf("static-pods is empty")
	}

	yssList := strings.Split(options.staticPods, ",")
	if len(yssList) < 1 {
		return errors.Errorf("static-pods (%s) format is invalid, expect yss1.ns/yss1.name,yss2.ns/yss2.name", options.staticPods)
	}

	clientSet, err := yurtadmutil.GetDefaultClientSet()
	if err != nil {
		return err
	}

	manifestList := make([]string, len(yssList))
	for i, yss := range yssList {
		info := strings.Split(yss, "/")
		if len(info) != 2 {
			return errors.Errorf("static-pods (%s) format is invalid, expect yss1.ns/yss1.name,yss2.ns/yss2.name", options.staticPods)
		}

		// yurthub is system static pod, can not operate
		if yurthub.CheckYurtHubItself(info[0], info[1]) {
			return errors.Errorf("static-pods (%s) value is invalid, can not operate yurt-hub static pod", options.staticPods)
		}

		// get static pod template
		manifest, _, err := yurtadmutil.GetStaticPodTemplateFromConfigMap(clientSet, info[0], util.WithConfigMapPrefix(info[1]))
		if err != nil {
			return errors.Errorf("when --static-podsis specified, the specified yurtstaticset and configmap should be exist.")
		}
		manifestList[i] = manifest
	}
	options.staticPodManifestList = manifestList

	return nil
}

// addUnInstallConfigFlags adds uninstall flags
func addUnInstallConfigFlags(flagSet *flag.FlagSet, uninstallOptions *uninstallOptions) {
	flagSet.StringVar(
		&uninstallOptions.staticPods, constants.StaticPods, uninstallOptions.staticPods,
		"Set the specified static pods on this node want to uninstall.",
	)
}
