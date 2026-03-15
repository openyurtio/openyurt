/*
Copyright 2026 The OpenYurt Authors.

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

package convert

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/pflag"

	"github.com/openyurtio/openyurt/pkg/node-servant/components"
	"github.com/openyurtio/openyurt/pkg/yurtadm/constants"
	enutil "github.com/openyurtio/openyurt/pkg/yurtadm/util/edgenode"
)

var getNodeNameFunc = enutil.GetNodeName

// Options has the information required by the convert operation.
type Options struct {
	kubeadmConfPaths string
	namespace        string
	nodeName         string
	nodePoolName     string
	openyurtDir      string
	workingMode      string
	yurthubBinaryURL string
	yurthubVersion   string
	Version          bool
}

// NewConvertOptions creates a new Options.
func NewConvertOptions() *Options {
	return &Options{
		kubeadmConfPaths: strings.Join(components.GetDefaultKubeadmConfPath(), ","),
		namespace:        constants.YurthubNamespace,
		openyurtDir:      constants.OpenyurtDir,
		workingMode:      constants.EdgeNode,
		yurthubVersion:   constants.YurthubVersion,
	}
}

// Complete completes all the required options.
func (o *Options) Complete(flags *pflag.FlagSet) error {
	kubeadmConfPaths, err := flags.GetString("kubeadm-conf-path")
	if err != nil {
		return err
	}
	if kubeadmConfPaths != "" {
		o.kubeadmConfPaths = kubeadmConfPaths
	}

	if o.nodeName == "" {
		if nodeName := os.Getenv(enutil.NODE_NAME); nodeName != "" {
			o.nodeName = nodeName
		} else {
			for _, kubeadmConfPath := range strings.Split(o.kubeadmConfPaths, ",") {
				kubeadmConfPath = strings.TrimSpace(kubeadmConfPath)
				if kubeadmConfPath == "" {
					continue
				}
				nodeName, err := getNodeNameFunc(kubeadmConfPath)
				if err == nil && nodeName != "" {
					o.nodeName = nodeName
					break
				}
			}
		}
	}

	if openyurtDir := os.Getenv("OPENYURT_DIR"); openyurtDir != "" {
		o.openyurtDir = openyurtDir
	}

	return nil
}

// Validate validates Options.
func (o *Options) Validate() error {
	if len(o.nodeName) == 0 {
		return fmt.Errorf("node name is empty")
	}

	if len(o.nodePoolName) == 0 {
		return fmt.Errorf("nodepool name is empty")
	}

	if len(o.namespace) == 0 {
		return fmt.Errorf("namespace is empty")
	}

	if len(o.workingMode) == 0 {
		return fmt.Errorf("working mode is empty")
	}

	if len(o.yurthubBinaryURL) == 0 && len(o.yurthubVersion) == 0 {
		return fmt.Errorf("yurthub version and binary URL are both empty")
	}

	return nil
}

// AddFlags sets flags.
func (o *Options) AddFlags(fs *pflag.FlagSet) {
	fs.StringVarP(&o.kubeadmConfPaths, "kubeadm-conf-path", "k", o.kubeadmConfPaths, "The path to kubelet service conf that is used by kubelet component to join the cluster on the work node. Support multiple values, will search in order until get the file.(e.g -k kbcfg1,kbcfg2)")
	fs.StringVar(&o.namespace, constants.Namespace, o.namespace, "The namespace where yurthub runs.")
	fs.StringVar(&o.nodeName, constants.NodeName, o.nodeName, "The node name where convert is executed.")
	fs.StringVar(&o.nodePoolName, constants.NodePoolName, o.nodePoolName, "The nodepool name which the node will be added.")
	fs.StringVar(&o.workingMode, "working-mode", o.workingMode, "The working mode of yurthub(edge, cloud, local).")
	fs.StringVar(&o.yurthubBinaryURL, constants.YurtHubBinaryUrl, o.yurthubBinaryURL, "The yurthub binary download URL.")
	fs.StringVar(&o.yurthubVersion, "yurthub-version", o.yurthubVersion, "The yurthub binary version to install when binary URL is not set.")
	fs.BoolVar(&o.Version, "version", o.Version, "print the version information.")
}
