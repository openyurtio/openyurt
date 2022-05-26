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

package components

import (
	"os"
	"path/filepath"

	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
	enutil "github.com/openyurtio/openyurt/pkg/yurtadm/util/edgenode"
)

func UnInstallYurtTunnelAgent() error {
	yurttunnelAgentWorkdir := filepath.Join("/var/lib/", projectinfo.GetAgentName())

	// remove yurttunnel-agent directory and certificates in it
	if _, err := enutil.FileExists(yurttunnelAgentWorkdir); os.IsNotExist(err) {
		klog.Infof("UnInstallYurtTunnelAgent: dir %s is not exists, skip delete", yurttunnelAgentWorkdir)
		return nil
	}
	err := os.RemoveAll(yurttunnelAgentWorkdir)
	if err != nil {
		return err
	}
	klog.Infof("UnInstallYurtTunnelAgent: config dir %s  has been removed", yurttunnelAgentWorkdir)

	return nil
}

func UnInstallYurtTunnelServer() error {
	yurttunnelServerWorkdir := filepath.Join("/var/lib/", projectinfo.GetServerName())

	// remove yurttunnel-server directory and certificates in it
	if _, err := enutil.FileExists(yurttunnelServerWorkdir); os.IsNotExist(err) {
		klog.Infof("UnInstallYurtTunnelServer: dir %s is not exists, skip delete", yurttunnelServerWorkdir)
		return nil
	}
	err := os.RemoveAll(yurttunnelServerWorkdir)
	if err != nil {
		return err
	}
	klog.Infof("UnInstallYurtTunnelServer: config dir %s  has been removed", yurttunnelServerWorkdir)

	return nil
}
