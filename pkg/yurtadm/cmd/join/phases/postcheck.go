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
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurtadm/cmd/join/joindata"
	"github.com/openyurtio/openyurt/pkg/yurtadm/util/kubernetes"
	"github.com/openyurtio/openyurt/pkg/yurtadm/util/yurthub"
)

// RunPostCheck executes the node health check and clean process.
func RunPostCheck(data joindata.YurtJoinData) error {
	klog.V(1).Infof("check kubelet status.")
	if err := kubernetes.CheckKubeletStatus(); err != nil {
		return err
	}
	klog.V(1).Infof("kubelet service is active")

	klog.V(1).Infof("waiting hub agent ready.")
	if err := yurthub.CheckYurthubHealthz(data.YurtHubServer()); err != nil {
		return err
	}
	klog.V(1).Infof("hub agent is ready")

	if err := yurthub.CleanHubBootstrapConfig(); err != nil {
		return err
	}
	klog.V(1).Infof("clean yurthub bootstrap config file success")

	return nil
}
