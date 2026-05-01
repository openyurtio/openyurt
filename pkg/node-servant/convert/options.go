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
	nodeName         string
	nodePoolName     string
	openyurtDir      string
}

// NewConvertOptions creates a new Options.
func NewConvertOptions() *Options {
	return &Options{
		kubeadmConfPaths: strings.Join(components.GetDefaultKubeadmConfPath(), ","),
		openyurtDir:      constants.OpenyurtDir,
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
	if openyurtDir := os.Getenv("OPENYURT_DIR"); openyurtDir != "" {
		o.openyurtDir = openyurtDir
	}

	if o.nodeName != "" {
		return nil
	}
	if nodeName := os.Getenv(enutil.NodeName); nodeName != "" {
		o.nodeName = nodeName
		return nil
	}
	for _, kubeadmConfPath := range strings.Split(o.kubeadmConfPaths, ",") {
		kubeadmConfPath = strings.TrimSpace(kubeadmConfPath)
		if kubeadmConfPath == "" {
			continue
		}
		nodeName, err := getNodeNameFunc(kubeadmConfPath)
		if err == nil && nodeName != "" {
			o.nodeName = nodeName
			return nil
		}
	}
	return fmt.Errorf("node name is empty")
}

// Validate validates Options.
func (o *Options) Validate() error {
	if len(o.nodeName) == 0 {
		return fmt.Errorf("node name is empty")
	}

	if len(o.nodePoolName) == 0 {
		return fmt.Errorf("nodepool name is empty")
	}

	return nil
}

// AddFlags sets flags.
func (o *Options) AddFlags(fs *pflag.FlagSet) {
	fs.StringVarP(&o.kubeadmConfPaths, "kubeadm-conf-path", "k", o.kubeadmConfPaths, "The path to kubelet service conf that is used by kubelet component to join the cluster on the work node. Support multiple values, will search in order until get the file.(e.g -k kbcfg1,kbcfg2)")
	fs.StringVar(&o.nodeName, constants.NodeName, o.nodeName, "The node name where convert is executed.")
	fs.StringVar(&o.nodePoolName, constants.NodePoolName, o.nodePoolName, "The nodepool name which the node will be added.")
}
