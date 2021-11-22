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
	"time"

	"github.com/spf13/pflag"
	"k8s.io/klog"
	"k8s.io/kubernetes/cmd/kubeadm/app/cmd/phases/workflow"

	nodeconverter "github.com/openyurtio/openyurt/pkg/node-servant/convert"
	"github.com/openyurtio/openyurt/pkg/yurtctl/constants"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

const (
	defaultYurthubHealthCheckTimeout = 2 * time.Minute
)

// NewConvertPhase creates a yurtctl workflow phase that convert native k8s node to openyurt node.
func NewConvertPhase() workflow.Phase {
	return workflow.Phase{
		Name:  "Convert node to OpenYurt node. ",
		Short: "Convert node",
		Run:   runConvertNode,
	}
}

// if comes to this phase, means node is up to running as a k8s node
// then we convert it into a edge-node or cloud-node
func runConvertNode(c workflow.RunData) error {
	data, ok := c.(YurtJoinData)
	if !ok {
		return fmt.Errorf("Join edge-node phase invoked with an invalid data struct. ")
	}

	// convert node
	o := nodeconverter.NewConvertOptions()
	f := constructFlagSet(data)
	if err := o.Complete(f); err != nil {
		return fmt.Errorf("fail to convert the kubernetes node to a yurt node: %s", err)
	}

	klog.Infof("convert with options:%v ", o)
	converter := nodeconverter.NewConverterWithOptions(o)
	if err := converter.Do(); err != nil {
		return fmt.Errorf("fail to convert the kubernetes node to a yurt node: %s", err)
	}

	return nil
}

func constructFlagSet(data YurtJoinData) *pflag.FlagSet {
	f := pflag.FlagSet{}

	if data.NodeType() == constants.CloudNode {
		f.String("working-mode", string(util.WorkingModeCloud), "")
	} else if data.NodeType() == constants.EdgeNode {
		f.String("working-mode", string(util.WorkingModeEdge), "")
	}

	yurtHubImg := data.YurtHubImage()
	if len(yurtHubImg) == 0 {
		yurtHubImg = fmt.Sprintf("%s/%s:%s", constants.DefaultOpenYurtImageRegistry, constants.Yurthub, constants.DefaultOpenYurtVersion)
	}
	f.String("yurthub-image", yurtHubImg, "")

	token := data.Cfg().Discovery.TLSBootstrapToken
	f.String("join-token", token, "")

	f.Duration("yurthub-healthcheck-timeout", defaultYurthubHealthCheckTimeout, "")
	f.String("kubeadm-conf-path", "", "")

	return &f
}
