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

package revert

import (
	"os"

	"github.com/spf13/pflag"

	"github.com/openyurtio/openyurt/pkg/yurtadm/constants"
	enutil "github.com/openyurtio/openyurt/pkg/yurtadm/util/edgenode"
)

// Options has the information that required by revert operation
type Options struct {
	kubeadmConfPath string
	openyurtDir     string
	nodeName        string
}

// NewRevertOptions creates a new Options
func NewRevertOptions() *Options {
	return &Options{}
}

// Complete completes all the required options.
func (o *Options) Complete(flags *pflag.FlagSet) error {

	kubeadmConfPath, err := flags.GetString("kubeadm-conf-path")
	if err != nil {
		return err
	}
	if kubeadmConfPath == "" {
		kubeadmConfPath = os.Getenv("KUBELET_SVC")
	}
	if kubeadmConfPath == "" {
		kubeadmConfPath = constants.KubeletSvcPath
	}
	o.kubeadmConfPath = kubeadmConfPath

	nodeName, err := enutil.GetNodeName(kubeadmConfPath)
	if err != nil {
		return err
	}
	o.nodeName = nodeName

	openyurtDir := os.Getenv("OPENYURT_DIR")
	if openyurtDir == "" {
		openyurtDir = constants.OpenyurtDir
	}
	o.openyurtDir = openyurtDir

	return nil
}
